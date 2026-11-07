#!/usr/bin/env bash
set -Eeuo pipefail
if [[ "${XPORT_CI_SSH:-}" != 1 ]]; then exec >/dev/ttyS0 2>&1; fi
STAGE=preflight
fail(){
 local rc="$1"
 echo "XPORT_SELINUX_E2E_FAILED stage=$STAGE rc=$rc"
 journalctl -u xport.service -n 20 --no-pager || true
 journalctl -u firewalld.service -n 12 --no-pager || true
 sync
 systemctl poweroff --no-block || true
}
trap 'rc=$?; fail "$rc"' ERR

echo "[selinux-vm] CentOS Stream 9 guest ready"
. /etc/os-release
test "$ID" = centos
test "$VERSION_ID" = 9
if [[ "$(getenforce)" == "Permissive" ]]; then setenforce 1; fi
test "$(getenforce)" = "Enforcing"
test "$(cat /sys/fs/selinux/enforce)" = 1
echo "[selinux-vm] SELinux Enforcing active, context $(id -Z)"
STAGE=os-packages
echo "[selinux-vm] Checking required firewall tools"
missing=0
for name in firewall-cmd nft ss curl python3; do
 if ! command -v "$name" >/dev/null 2>&1; then echo "[selinux-vm] missing $name"; missing=1; fi
done
if [[ "$missing" == 1 ]]; then
 echo "[selinux-vm] Installing missing tools (bounded)"
 if ! timeout 150 dnf -y -q install firewalld nftables iproute curl python3 >/tmp/xport-dnf.log 2>&1; then
  cat /tmp/xport-dnf.log
  echo "XPORT_SELINUX_E2E_FAILED stage=os-packages"
  systemctl poweroff --no-block || true
  exit 1
 fi
fi
echo "[selinux-vm] Required OS packages ready"
STAGE=firewalld-start
systemctl enable --now firewalld
echo "[selinux-vm] firewalld systemd service started"
firewall-cmd --state
firewall-cmd --state | grep -Fxq running
ZONE="$(firewall-cmd --get-default-zone | tr -d '\r\n')"
echo "[selinux-vm] Active firewall zone: $ZONE"
# Mirror hosts that have standalone IPv4/IPv6 NAT PREROUTING chains.
# These must not be mistaken for competing packet input filters.
STAGE=nat-coexistence
nft add table ip xport_ci_nat
nft 'add chain ip xport_ci_nat PREROUTING { type nat hook prerouting priority -100; policy accept; }'
nft add table ip6 xport_ci_nat
nft 'add chain ip6 xport_ci_nat PREROUTING { type nat hook prerouting priority -100; policy accept; }'
nft -j list ruleset | python3 -c 'import json,sys; cs=[x["chain"] for x in json.load(sys.stdin)["nftables"] if "chain" in x]; assert sum(c.get("table")=="xport_ci_nat" and c.get("type")=="nat" and c.get("hook")=="prerouting" for c in cs)==2'
echo "[selinux-vm] Independent IPv4/IPv6 NAT chains installed; input filtering remains firewalld-owned"
STAGE=binary-install
echo "[selinux-vm] Installing test X-port binary"
install -D -m 0755 /run/xport-ci-seed/xport /usr/local/bin/xport
restorecon -v /usr/local/bin/xport || true
echo "[selinux-vm] Binary SELinux label $(ls -Z /usr/local/bin/xport)"
STAGE=panel-init
DATA=/var/lib/xport-selinux-ci
PORT=28612
mkdir -p "$DATA"
printf 'selinux-ci-pass-12345\n' | /usr/local/bin/xport init --data "$DATA" --admin-user ci-admin --listen "127.0.0.1:$PORT"
STAGE=systemd-panel
echo "[selinux-vm] Starting systemd panel under legacy sandbox"
systemd-run --unit=xport --property=Type=exec \
 --property=ProtectSystem=strict --property=PrivateTmp=yes \
 --property=NoNewPrivileges=yes --property=PrivateDevices=yes \
 --property="ReadWritePaths=$DATA /usr/local/bin" \
 --property="RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6" \
 /usr/local/bin/xport serve --data "$DATA" --listen "127.0.0.1:$PORT"
BASE="http://127.0.0.1:$PORT"
for i in $(seq 1 90); do
 if curl -fsS "$BASE/" >/dev/null 2>&1; then break; fi
 sleep .5
done
curl -fsS "$BASE/" >/dev/null
echo "[selinux-vm] X-port systemd service healthy"
curl -fsS -c /run/xport-ci.cookie -H 'Content-Type: application/json' \
 -d '{"username":"ci-admin","password":"selinux-ci-pass-12345"}' "$BASE/api/login" >/dev/null
STAGE=web-firewall-read
echo "[selinux-vm] Inspecting Web firewall read with full failure diagnostics"
read_code="$(curl -sS -b /run/xport-ci.cookie -o /run/xport-firewall.json -w '%{http_code}' "$BASE/api/firewall")"
if [[ "$read_code" != 200 ]]; then
 echo "[selinux-vm] Web firewall returned HTTP $read_code"
 cat /run/xport-firewall.json || true
 echo "[selinux-vm] Direct firewall-inspect under root outside panel sandbox:"
 /usr/local/bin/xport firewall-inspect | head -c 450 || true
 echo
 echo "[selinux-vm] Systemd helper launched under root:"
 systemd-run --wait --collect --pipe --unit=xport-ci-read-direct --property=Type=exec --property=NoNewPrivileges=yes /usr/local/bin/xport firewall-inspect || true
 echo "[selinux-vm] Reproducing nested helper behavior in equivalent sandbox:"
 bash /run/xport-ci-seed/helper-matrix.sh || true
 echo "[selinux-vm] Comparing Go os/exec standard stream configurations:"
 bash /run/xport-ci-seed/go-helper-matrix.sh || true
 echo "[selinux-vm] Web process systemd and SELinux context:"
 systemctl show xport.service -p NoNewPrivileges -p RestrictAddressFamilies -p PrivateDevices -p PrivateTmp || true
 ps -eZ | grep -E 'xport|systemd-run' || true
 echo "[selinux-vm] Web service recent journal:"
 journalctl -u xport.service --no-pager -n 28 --output=cat || true
 echo "[selinux-vm] Recent systemd helper service logs:"
 journalctl -u 'xport-firewall-*' -n 65 --no-pager --output=cat || true
 echo "[selinux-vm] SELinux AVC audit log (if audit available):"
 if command -v ausearch >/dev/null; then ausearch -m AVC,USER_AVC -ts recent 2>/dev/null | tail -n 35 || true; fi
 echo "XPORT_SELINUX_E2E_FAILED stage=web-firewall-read http=$read_code"
 exit 1
fi
python3 - <<'PY'
import json
with open("/run/xport-firewall.json") as f:d=json.load(f)
assert d["backend"]=="firewalld" and d["writable"] is True,d
print("[selinux-vm] Web firewall read passed in Enforcing")
PY
STAGE=listener-protocol-snapshot
/usr/local/bin/xport firewall-sockets >/run/xport-ci-sockets.json
python3 - <<'PY'
import json
data=json.load(open("/run/xport-ci-sockets.json"))
assert data.get("tcp/28612") is True, data
assert not data.get("udp/28612",False), data
print("[selinux-vm] TCP panel listener is recognized independently from UDP")
PY
STAGE=foreign-input-fail-closed
echo "[selinux-vm] Verifying separate INPUT filter blocks Web writes without changing rules"
nft add table inet xport_ci_external
nft 'add chain inet xport_ci_external INPUT { type filter hook input priority -200; policy accept; }'
curl -fsS -b /run/xport-ci.cookie "$BASE/api/firewall" >/run/foreign-input.json
python3 - <<'PY'
import json
d=json.load(open("/run/foreign-input.json"))
assert d["backend"]=="firewalld" and d["writable"] is False,d
assert "xport_ci_external" in d["unsafeReason"],d
PY
reject_code="$(curl -sS -b /run/xport-ci.cookie -H "Origin: $BASE" \
 -H 'Content-Type: application/json' -o /run/foreign-input-reject.json -w '%{http_code}' \
 -d "{\"action\":\"add\",\"port\":53335,\"protocol\":\"tcp\",\"zone\":\"$ZONE\"}" "$BASE/api/firewall/rules")"
if [[ "$reject_code" != 409 ]]; then
 echo "[selinux-vm] Foreign INPUT must reject writes: HTTP $reject_code"
 cat /run/foreign-input-reject.json; exit 1
fi
[[ "$(firewall-cmd --zone="$ZONE" --query-port=53335/tcp || true)" == no ]]
[[ "$(firewall-cmd --permanent --zone="$ZONE" --query-port=53335/tcp || true)" == no ]]
nft delete table inet xport_ci_external
curl -fsS -b /run/xport-ci.cookie "$BASE/api/firewall" >/run/after-foreign-input.json
python3 - <<'PY'
import json
d=json.load(open("/run/after-foreign-input.json"))
assert d["backend"]=="firewalld" and d["writable"] is True,d
PY
echo "[selinux-vm] Foreign INPUT blocked writes; NAT-only coexistence restored write access"
STAGE=web-firewall-add
echo "[selinux-vm] Testing authenticated Web rule add"
code="$(curl -sS -b /run/xport-ci.cookie -H "Origin: $BASE" \
 -H 'Content-Type: application/json' -o /run/add.json -w '%{http_code}' \
 -d "{\"action\":\"add\",\"port\":53333,\"protocol\":\"tcp\",\"zone\":\"$ZONE\"}" "$BASE/api/firewall/rules")"
if [[ "$code" != 200 ]]; then cat /run/add.json; exit 1; fi
firewall-cmd --zone="$ZONE" --query-port=53333/tcp | grep -Fxq yes
firewall-cmd --permanent --zone="$ZONE" --query-port=53333/tcp | grep -Fxq yes
echo "[selinux-vm] Web rule added, both runtime/permanent"

STAGE=web-firewall-remove
echo "[selinux-vm] Testing authenticated Web remove and confirmation"
code="$(curl -sS -b /run/xport-ci.cookie -H "Origin: $BASE" \
 -H 'Content-Type: application/json' -o /run/remove.json -w '%{http_code}' \
 -d "{\"action\":\"remove\",\"port\":53333,\"protocol\":\"tcp\",\"zone\":\"$ZONE\"}" "$BASE/api/firewall/rules")"
if [[ "$code" != 202 ]]; then cat /run/remove.json; exit 1; fi
python3 - <<'PY'
import json
with open("/run/remove.json") as f:p=json.load(f)
assert p["pending"] and len(p["token"])==32,p
with open("/run/confirm.json","w") as f:json.dump({"id":p["id"],"token":p["token"]},f)
print("[selinux-vm] 120-second external restore timer armed")
PY
code="$(curl -sS -b /run/xport-ci.cookie -H "Origin: $BASE" \
 -H 'Content-Type: application/json' -o /run/confirm-response.json -w '%{http_code}' \
 -d @/run/confirm.json "$BASE/api/firewall/rules/confirm")"
if [[ "$code" != 200 ]]; then cat /run/confirm-response.json; exit 1; fi
runtime_query="$(firewall-cmd --zone="$ZONE" --query-port=53333/tcp || true)"
permanent_query="$(firewall-cmd --permanent --zone="$ZONE" --query-port=53333/tcp || true)"
if [[ "$runtime_query" != no || "$permanent_query" != no ]]; then
 echo "[selinux-vm] Deleted rule still exists: runtime=$runtime_query permanent=$permanent_query" >&2
 exit 1
fi
echo "[selinux-vm] Web rule removal confirmed"

# Exercise the independent authenticated rule control endpoint while SELinux
# remains Enforcing. None of these ports are used for remote administration.
STAGE=web-policy-control
control_request(){
 local payload="$1" output="$2" expected="$3" code
 code="$(curl -sS -b /run/xport-ci.cookie -H "Origin: $BASE" \
  -H 'Content-Type: application/json' -o "$output" -w '%{http_code}' \
  -d "$payload" "$BASE/api/firewall/control")"
 if [[ "$code" != "$expected" ]]; then
  echo "[selinux-vm] control HTTP $code, expected $expected"
  cat "$output"; exit 1
 fi
}
confirm_control(){
 local pending="$1"
 python3 - "$pending" <<'PY'
import json,sys
p=json.load(open(sys.argv[1]))
assert p["pending"] and len(p["token"])==32,p
json.dump({"id":p["id"],"token":p["token"]},open("/run/control-confirm.json","w"))
PY
 local code
 code="$(curl -sS -b /run/xport-ci.cookie -H "Origin: $BASE" \
  -H 'Content-Type: application/json' -o /run/control-confirm-result.json -w '%{http_code}' \
  -d @/run/control-confirm.json "$BASE/api/firewall/rules/confirm")"
 if [[ "$code" != 200 ]]; then cat /run/control-confirm-result.json; exit 1; fi
}
drop_id="firewalld:$ZONE:rich:53342/tcp:deny:both"
allow_id="firewalld:$ZONE:53343/tcp"
control_request "$(printf '{"action":"add-drop","id":"%s","port":53342,"protocol":"tcp","zone":"%s"}' "$drop_id" "$ZONE")" /run/control-drop-add.json 202
firewall-cmd --zone="$ZONE" --list-rich-rules | grep -F 'port="53342"'
firewall-cmd --permanent --zone="$ZONE" --list-rich-rules | grep -F 'port="53342"'
confirm_control /run/control-drop-add.json
curl -fsS -b /run/xport-ci.cookie "$BASE/api/firewall" >/run/control-drop-list.json
echo "[selinux-vm] Pre-assert DROP snapshot comparison"
python3 - <<'PY'
import json
d=json.load(open("/run/control-drop-list.json"))
print("WEB_SNAPSHOT:",json.dumps({"backend":d.get("backend"),"writable":d.get("writable"),
 "unsafeReason":d.get("unsafeReason"),"opaqueRichZones":d.get("opaqueRichZones"),
 "rules":d.get("rules")},ensure_ascii=False)[:5000],flush=True)
PY
/usr/local/bin/xport firewall-inspect >/run/control-drop-root-snapshot.json || true
python3 - <<'PY'
import json
try:
 d=json.load(open("/run/control-drop-root-snapshot.json"))
 print("ROOT_SNAPSHOT:",json.dumps({"backend":d.get("backend"),"writable":d.get("writable"),
  "unsafeReason":d.get("unsafeReason"),"opaqueRichZones":d.get("opaqueRichZones"),
  "rules":d.get("rules")},ensure_ascii=False)[:5000],flush=True)
except Exception as exc:
 print("ROOT_INSPECT_ERROR:",str(exc),flush=True)
PY
if ! python3 - <<'PY'
import json
d=json.load(open("/run/control-drop-list.json"))
r=[r for r in d["rules"] if r["port"]==53342 and r["protocol"]=="tcp"]
assert len(r)==1 and r[0]["action"]=="deny" and r[0]["managed"],r
PY
then
 echo "[selinux-vm] DROP read mismatch: comparing authenticated API vs root helper and firewalld"
 python3 - <<'PY'
import json
d=json.load(open("/run/control-drop-list.json"))
print("WEB:",json.dumps({"backend":d.get("backend"),"writable":d.get("writable"),"unsafeReason":d.get("unsafeReason"),
 "opaqueRichZones":d.get("opaqueRichZones"),"rules":d.get("rules")},ensure_ascii=False)[:5000])
PY
 /usr/local/bin/xport firewall-inspect > /run/control-drop-root-snapshot.json || true
 python3 - <<'PY'
import json
try:
 d=json.load(open("/run/control-drop-root-snapshot.json"))
 print("ROOT:",json.dumps({"backend":d.get("backend"),"writable":d.get("writable"),"unsafeReason":d.get("unsafeReason"),
  "opaqueRichZones":d.get("opaqueRichZones"),"rules":d.get("rules")},ensure_ascii=False)[:5000])
except Exception as exc: print("ROOT ERROR:",str(exc))
PY
 firewall-cmd --zone="$ZONE" --list-rich-rules || true
 firewall-cmd --permanent --zone="$ZONE" --list-rich-rules || true
 exit 1
fi
control_request "$(printf '{"action":"pause-drop","id":"%s","port":53342,"protocol":"tcp","zone":"%s"}' "$drop_id" "$ZONE")" /run/control-drop-pause.json 202
test -z "$(firewall-cmd --zone="$ZONE" --list-rich-rules | grep -F 'port="53342"' || true)"
confirm_control /run/control-drop-pause.json
curl -fsS -b /run/xport-ci.cookie "$BASE/api/firewall" >/run/control-drop-paused.json
python3 - <<'PY'
import json
d=json.load(open("/run/control-drop-paused.json"))
r=[r for r in d["rules"] if r["port"]==53342 and r["protocol"]=="tcp"]
assert len(r)==1 and r[0]["paused"] and r[0]["managed"] and r[0]["action"]=="deny",r
PY
control_request "$(printf '{"action":"resume-drop","id":"%s","port":53342,"protocol":"tcp","zone":"%s"}' "$drop_id" "$ZONE")" /run/control-drop-resume.json 200
firewall-cmd --zone="$ZONE" --list-rich-rules | grep -F 'port="53342"'
control_request "$(printf '{"action":"remove-drop","id":"%s","port":53342,"protocol":"tcp","zone":"%s"}' "$drop_id" "$ZONE")" /run/control-drop-remove.json 202
confirm_control /run/control-drop-remove.json
test -z "$(firewall-cmd --zone="$ZONE" --list-rich-rules | grep -F 'port="53342"' || true)"

payload="$(printf '{"action":"add","port":53343,"protocol":"tcp","zone":"%s"}' "$ZONE")"
code="$(curl -sS -b /run/xport-ci.cookie -H "Origin: $BASE" -H 'Content-Type: application/json' \
 -o /run/control-allow-add.json -w '%{http_code}' -d "$payload" "$BASE/api/firewall/rules")"
test "$code" = 200
control_request "$(printf '{"action":"pause-allow","id":"%s","port":53343,"protocol":"tcp","zone":"%s"}' "$allow_id" "$ZONE")" /run/control-allow-pause.json 202
test "$(firewall-cmd --zone="$ZONE" --query-port=53343/tcp || true)" = no
confirm_control /run/control-allow-pause.json
curl -fsS -b /run/xport-ci.cookie "$BASE/api/firewall" >/run/control-allow-paused.json
python3 - <<'PY'
import json
d=json.load(open("/run/control-allow-paused.json"))
r=[r for r in d["rules"] if r["port"]==53343 and r["protocol"]=="tcp"]
assert len(r)==1 and r[0]["paused"] and r[0]["action"]=="allow",r
PY
control_request "$(printf '{"action":"resume-allow","id":"%s","port":53343,"protocol":"tcp","zone":"%s"}' "$allow_id" "$ZONE")" /run/control-allow-resume.json 200
test "$(firewall-cmd --zone="$ZONE" --query-port=53343/tcp || true)" = yes
test "$(firewall-cmd --permanent --zone="$ZONE" --query-port=53343/tcp || true)" = yes
control_request "$(printf '{"action":"pause-allow","id":"%s","port":22,"protocol":"tcp","zone":"%s"}' "$allow_id" "$ZONE")" /run/control-protected.json 409
echo "[selinux-vm] DROP creation, pause/resume, removal and allow pause/resume passed"

# Confirm the DROP-add watchdog really runs without any X-port binary.
# Only the standalone SELinux VM performs this additional full timer.
if [[ ! -f /run/xport-ci-seed/panel-test.sh ]]; then
 STAGE=drop-recovery-without-panel-binary
 timeout_id="firewalld:$ZONE:rich:53344/tcp:deny:both"
 control_request "$(printf '{"action":"add-drop","id":"%s","port":53344,"protocol":"tcp","zone":"%s"}' "$timeout_id" "$ZONE")" /run/control-drop-timeout.json 202
 test -n "$(firewall-cmd --zone="$ZONE" --list-rich-rules | grep -F 'port="53344"')"
 mv /usr/local/bin/xport /usr/local/bin/xport.drop-timer
 restored=0
 # systemd targets 120s. Under KVM/CI load the native firewalld D-Bus
 # command can finish later; observe real runtime+permanent state with a
 # bounded grace period rather than declaring failure while service runs.
 for i in $(seq 1 185); do
  if [[ -z "$(firewall-cmd --zone="$ZONE" --list-rich-rules | grep -F 'port="53344"' || true)" ]] &&
     [[ -z "$(firewall-cmd --permanent --zone="$ZONE" --list-rich-rules | grep -F 'port="53344"' || true)" ]]; then
   restored=1; break
  fi
  sleep 1
 done
 mv /usr/local/bin/xport.drop-timer /usr/local/bin/xport
 if [[ "$restored" != 1 ]]; then
  echo "[selinux-vm] Native DROP watchdog failed to remove unconfirmed rule"
  exit 1
 fi
 echo "[selinux-vm] Real 120-second DROP watchdog removed rule while panel binary was absent"
fi

# Validate an actual 120-second Web recovery while the panel binary
# is temporarily absent. Run once in the standalone SELinux guest, not the
# second VM which also installs 1Panel.
if [[ ! -f /run/xport-ci-seed/panel-test.sh ]]; then
 STAGE=restore-without-panel-binary
 echo "[selinux-vm] Testing unattended timer after panel binary replacement"
 restore_port=53334
 payload="$(printf '{"action":"add","port":%s,"protocol":"tcp","zone":"%s"}' "$restore_port" "$ZONE")"
 code="$(curl -sS -b /run/xport-ci.cookie -H "Origin: $BASE" \
  -H 'Content-Type: application/json' -o /run/timer-add.json -w '%{http_code}' \
  -d "$payload" "$BASE/api/firewall/rules")"
 [[ "$code" == 200 ]] || { cat /run/timer-add.json; exit 1; }
 payload="$(printf '{"action":"remove","port":%s,"protocol":"tcp","zone":"%s"}' "$restore_port" "$ZONE")"
 code="$(curl -sS -b /run/xport-ci.cookie -H "Origin: $BASE" \
  -H 'Content-Type: application/json' -o /run/timer-remove.json -w '%{http_code}' \
  -d "$payload" "$BASE/api/firewall/rules")"
 [[ "$code" == 202 ]] || { cat /run/timer-remove.json; exit 1; }
 test "$(firewall-cmd --zone="$ZONE" --query-port="$restore_port"/tcp || true)" = no
 test "$(firewall-cmd --permanent --zone="$ZONE" --query-port="$restore_port"/tcp || true)" = no
 # Verify the native systemd timer itself is precise; systemd defaults to
 # AccuracySec=1min and can defer a promised 120-second restore.
 curl -fsS -b /run/xport-ci.cookie "$BASE/api/firewall" >/run/timer-pending.json
 timer_unit="$(python3 - "$ZONE" <<'PY'
import json,sys
pending=json.load(open('/run/timer-pending.json')).get('pending') or []
matches=[p for p in pending if p.get('id')==f'firewalld:{sys.argv[1]}:53334/tcp']
assert len(matches)==1 and matches[0].get('unit','').startswith('xport-fw-restore-'),matches
print(matches[0]['unit'])
PY
 )"
 timer_accuracy="$(systemctl show "$timer_unit.timer" --property=AccuracyUSec --value)"
 if [[ "$timer_accuracy" != "1s" ]]; then
  echo "[selinux-vm] Recovery timer precision unexpected: $timer_accuracy"
  exit 1
 fi
 echo "[selinux-vm] Recovery timer accuracy is 1 second"
 echo "[selinux-vm] Recovery timers immediately after scheduling:"
 systemctl list-timers --all --no-pager 'xport-fw-restore-*' || true
 mv /usr/local/bin/xport /usr/local/bin/xport.timer-test
 restored=0
 # Use the same bounded completion grace for the legacy allow timer.
 for i in $(seq 1 185); do
  runtime="$(firewall-cmd --zone="$ZONE" --query-port="$restore_port"/tcp || true)"
  if [[ "$runtime" == yes ]]; then
   permanent="$(firewall-cmd --permanent --zone="$ZONE" --query-port="$restore_port"/tcp || true)"
   if [[ "$permanent" == yes ]]; then restored=1; break; fi
  fi
  sleep 1
 done
 mv /usr/local/bin/xport.timer-test /usr/local/bin/xport
 if [[ "$restored" != 1 ]]; then
  echo "[selinux-vm] Diagnostic: recovery timer and service status"
  systemctl list-timers --all --no-pager 'xport-fw-restore-*' || true
  systemctl list-units --all --no-pager 'xport-fw-restore-*' || true
  systemctl --failed --no-pager || true
  echo "[selinux-vm] Diagnostic: firewalld port state"
  firewall-cmd --zone="$ZONE" --query-port="$restore_port"/tcp || true
  firewall-cmd --permanent --zone="$ZONE" --query-port="$restore_port"/tcp || true
  echo "[selinux-vm] Diagnostic: recovery service journals"
  journalctl -b --no-pager --output=short-precise --since '-5 min' | grep -E 'xport-fw-restore|firewalld|53334' | tail -n 110 || true
  echo "[selinux-vm] Timed recovery failed when the X-port binary was unavailable"
  journalctl -u 'xport-fw-restore-*' --no-pager -n 30 || true
  exit 1
 fi
 echo "[selinux-vm] Real 120-second recovery succeeded with X-port binary absent"
fi
echo "XPORT_SELINUX_E2E_OK"
if [[ -f /run/xport-ci-seed/panel-test.sh ]]; then
 STAGE=panel-coexist
 echo "[selinux-vm] Starting separate panel coinstallation under Enforcing"
 if ! timeout 450 bash /run/xport-ci-seed/panel-test.sh; then
  echo "XPORT_SELINUX_E2E_FAILED stage=panel-coexist"
  sync
  systemctl poweroff --no-block || true
  exit 1
 fi
fi
sync
systemctl poweroff --no-block
