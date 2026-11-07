#!/usr/bin/env bash
set -Eeuo pipefail

# Optional real coinstallation check, executed only within the disposable
# SELinux-enabled CentOS guest (never on the GitHub runner host).
test "$(getenforce)" = Enforcing
echo "[panel-coexist] CentOS SELinux Enforcing confirmed"
BASE=http://127.0.0.1:28612
ZONE="$(firewall-cmd --get-default-zone | tr -d '\r\n')"
echo "[panel-coexist] Installing a separate server panel without Docker changes"
curl -fLsS --connect-timeout 20 --max-time 100 \
  https://resource.fit2cloud.com/1panel/package/v2/quick_start.sh \
  -o /run/ci-panel-installer.sh
export PANEL_NON_INTERACTIVE=true PANEL_LANG=en PANEL_INSTALL_DIR=/opt \
 PANEL_PORT=19090 PANEL_ENTRANCE=ciPanelEntry PANEL_USERNAME=ciPanelAdmin \
 PANEL_INSTALL_DOCKER=n PANEL_CONFIGURE_ACCELERATOR=n PANEL_REPLACE_DAEMON_JSON=n
PANEL_PASSWORD="$(python3 -c 'import secrets;print(secrets.token_hex(14))')"
export PANEL_PASSWORD
echo "[panel-coexist] Installing official 1Panel V2 in disposable VM; Docker installation disabled"
if ! timeout 360 bash /run/ci-panel-installer.sh >/run/ci-panel-install.log 2>&1; then
 echo "[panel-coexist] Installer failed inside isolated guest; actual coinstallation not established"
 tail -n 30 /run/ci-panel-install.log || true
 exit 1
fi
# Account for slow first-boot database migration in a shared CI runner.
# All three services and the actual management port must become healthy.
for i in $(seq 1 180); do
 if systemctl is-active --quiet 1panel-core &&
    systemctl is-active --quiet 1panel-agent &&
    ss -H -ltn 'sport = :19090' | grep -q LISTEN; then break; fi
 sleep .5
done
for unit in 1panel-core 1panel-agent xport.service; do
 if ! systemctl is-active --quiet "$unit"; then
  echo "[panel-coexist] Service did not become active: $unit"
  systemctl show "$unit" -p ActiveState -p SubState -p Result --no-pager || true
  exit 1
 fi
done
test "$(getenforce)" = Enforcing
# A healthy service does not guarantee that its configured Web port listens.
if ! ss -H -ltn 'sport = :19090' | grep -q LISTEN; then
 echo "[panel-coexist] Official installer completed but management port is not listening"
 ss -H -ltn '( sport = :19090 )' || true
 exit 1
fi
curl -fsS "$BASE/" >/dev/null
echo "[panel-coexist] Both panels running in CentOS SELinux Enforcing guest"

# A separate manager owns its own port; X-port may display but must not remove.
firewall-cmd --zone="$ZONE" --add-port=19090/tcp
firewall-cmd --permanent --zone="$ZONE" --add-port=19090/tcp
curl -fsS -b /run/xport-ci.cookie "$BASE/api/firewall" >/run/panel-coexist-firewall.json
python3 - <<'PY'
import json
with open("/run/panel-coexist-firewall.json") as f:d=json.load(f)
assert d["backend"] in ("firewalld","mixed"),d
print("[panel-coexist] X-port reads firewall with external service present; writable=",d["writable"])
PY
code="$(curl -sS -b /run/xport-ci.cookie -H "Origin: $BASE" \
  -H 'Content-Type: application/json' -o /run/panel-coexist-remove.json -w '%{http_code}' \
  -d "{\"action\":\"remove\",\"port\":19090,\"protocol\":\"tcp\",\"zone\":\"$ZONE\"}" "$BASE/api/firewall/rules")"
if [[ "$code" != 403 && "$code" != 409 ]]; then
 echo "[panel-coexist] External service port removal not rejected: HTTP $code"
 cat /run/panel-coexist-remove.json
 exit 1
fi
firewall-cmd --zone="$ZONE" --query-port=19090/tcp | grep -Fxq yes
firewall-cmd --permanent --zone="$ZONE" --query-port=19090/tcp | grep -Fxq yes
# Simulate a separate management panel editing a non-management rule
# through the shared firewalld backend. This does NOT claim to drive the
# other panel's UI; it tests interoperability at the source of truth.
firewall-cmd --zone="$ZONE" --add-port=19177/tcp
firewall-cmd --permanent --zone="$ZONE" --add-port=19177/tcp
curl -fsS -b /run/xport-ci.cookie "$BASE/api/firewall" >/run/panel-before-external-edit.json
python3 - <<'PY'
import json
d=json.load(open("/run/panel-before-external-edit.json"))
r=[x for x in d["rules"] if x["port"]==19177 and x["protocol"]=="tcp"]
assert len(r)==1 and r[0]["managed"] is False, r
PY
firewall-cmd --zone="$ZONE" --remove-port=19177/tcp
firewall-cmd --permanent --zone="$ZONE" --remove-port=19177/tcp
firewall-cmd --zone="$ZONE" --add-port=19178/tcp
firewall-cmd --permanent --zone="$ZONE" --add-port=19178/tcp
firewall-cmd --reload
curl -fsS -b /run/xport-ci.cookie "$BASE/api/firewall" >/run/panel-after-external-edit.json
python3 - <<'PY'
import json
d=json.load(open("/run/panel-after-external-edit.json"))
assert all(x["port"]!=19177 for x in d["rules"]), d
r=[x for x in d["rules"] if x["port"]==19178 and x["protocol"]=="tcp"]
assert len(r)==1 and r[0]["managed"] is False,r
print("[panel-coexist] External edits and firewalld reload reflected without ownership takeover")
PY
code="$(curl -sS -b /run/xport-ci.cookie -H "Origin: $BASE" \
  -H 'Content-Type: application/json' -o /run/panel-edited-remove.json -w '%{http_code}' \
  -d "{\"action\":\"remove\",\"port\":19178,\"protocol\":\"tcp\",\"zone\":\"$ZONE\"}" "$BASE/api/firewall/rules")"
test "$code" = 403
firewall-cmd --zone="$ZONE" --query-port=19178/tcp | grep -Fxq yes
firewall-cmd --permanent --zone="$ZONE" --query-port=19178/tcp | grep -Fxq yes
firewall-cmd --zone="$ZONE" --query-port=19090/tcp | grep -Fxq yes
# X-port may edit a dormant external SIMPLE allowance with explicit consent.
# The old source is protected by a 120s native rollback and ownership remains
# external. Actual 1Panel Web UI is not being driven in this fixture.
python3 - "$ZONE" <<'PY'
import json,sys
d=json.load(open("/run/panel-after-external-edit.json"))
r=next(x for x in d["rules"] if x["port"]==19178 and x["protocol"]=="tcp")
assert len(r["revision"])==24,r
json.dump({"id":r["id"],"revision":r["revision"],
 "newPort":19179,"newProtocol":"tcp","externalAcknowledged":True},
 open("/run/panel-external-edit-request.json","w"))
PY
code="$(curl -sS -b /run/xport-ci.cookie -H "Origin: $BASE" \
 -H 'Content-Type: application/json' -o /run/panel-edit-accepted.json -w '%{http_code}' \
 -d @/run/panel-external-edit-request.json "$BASE/api/firewall/edit")"
if [[ "$code" != 202 ]]; then
 echo "[panel-coexist] Controlled external edit failed: HTTP $code"
 cat /run/panel-edit-accepted.json; exit 1
fi
test "$(firewall-cmd --zone="$ZONE" --query-port=19178/tcp || true)" = no
firewall-cmd --zone="$ZONE" --query-port=19179/tcp | grep -Fxq yes
python3 - <<'PY'
import json
p=json.load(open("/run/panel-edit-accepted.json"))
assert p["pending"] and len(p["token"])==32 and p["kind"]=="edit-external",p
json.dump({"id":p["id"],"token":p["token"]},open("/run/panel-edit-confirm.json","w"))
PY
code="$(curl -sS -b /run/xport-ci.cookie -H "Origin: $BASE" \
 -H 'Content-Type: application/json' -o /run/panel-edit-confirmed.json -w '%{http_code}' \
 -d @/run/panel-edit-confirm.json "$BASE/api/firewall/rules/confirm")"
if [[ "$code" != 200 ]]; then cat /run/panel-edit-confirmed.json; exit 1; fi
firewall-cmd --reload
curl -fsS -b /run/xport-ci.cookie "$BASE/api/firewall" >/run/panel-edit-final.json
python3 - <<'PY'
import json
d=json.load(open("/run/panel-edit-final.json"))
assert all(r["port"]!=19178 for r in d["rules"])
r=[x for x in d["rules"] if x["port"]==19179 and x["protocol"]=="tcp"]
assert len(r)==1 and r[0]["managed"] is False and r[0]["runtime"] and r[0]["permanent"],r
print("[panel-coexist] Confirmed X-port external edit remains visible through firewalld and externally owned")
PY
# Stale source IDs and old revision tokens cannot be replayed.
code="$(curl -sS -b /run/xport-ci.cookie -H "Origin: $BASE" \
 -H 'Content-Type: application/json' -o /run/panel-edit-stale.json -w '%{http_code}' \
 -d @/run/panel-external-edit-request.json "$BASE/api/firewall/edit")"
test "$code" = 409
# The real external management port still cannot be modified, even with
# the correct revision and explicit cross-panel acknowledgment.
curl -fsS -b /run/xport-ci.cookie "$BASE/api/firewall" >/run/panel-edit-protected.json
python3 - <<'PY'
import json
d=json.load(open("/run/panel-edit-protected.json"))
r=next(x for x in d["rules"] if x["port"]==19090 and x["protocol"]=="tcp")
json.dump({"id":r["id"],"revision":r["revision"],"newPort":19191,
 "newProtocol":"tcp","externalAcknowledged":True},
 open("/run/panel-edit-protected-request.json","w"))
PY
code="$(curl -sS -b /run/xport-ci.cookie -H "Origin: $BASE" \
 -H 'Content-Type: application/json' -o /run/panel-edit-protected-response.json -w '%{http_code}' \
 -d @/run/panel-edit-protected-request.json "$BASE/api/firewall/edit")"
test "$code" = 409
firewall-cmd --zone="$ZONE" --query-port=19090/tcp | grep -Fxq yes
echo "[panel-coexist] Revision checks and management listener protection passed"

# Unconfirmed external edits must restore the old port and remove the new
# rule without depending on X-port being available 120 seconds later.
STAGE=external-edit-watchdog
firewall-cmd --zone="$ZONE" --add-port=19181/tcp
firewall-cmd --permanent --zone="$ZONE" --add-port=19181/tcp
curl -fsS -b /run/xport-ci.cookie "$BASE/api/firewall" >/run/external-watchdog-before.json
python3 - <<'PY'
import json
d=json.load(open("/run/external-watchdog-before.json"))
r=next(x for x in d["rules"] if x["port"]==19181 and x["protocol"]=="tcp")
assert r["managed"] is False
json.dump({"id":r["id"],"revision":r["revision"],"newPort":19182,
 "newProtocol":"tcp","externalAcknowledged":True},
 open("/run/external-watchdog-request.json","w"))
PY
code="$(curl -sS -b /run/xport-ci.cookie -H "Origin: $BASE" \
 -H 'Content-Type: application/json' -o /run/external-watchdog-start.json -w '%{http_code}' \
 -d @/run/external-watchdog-request.json "$BASE/api/firewall/edit")"
if [[ "$code" != 202 ]]; then cat /run/external-watchdog-start.json; exit 1; fi
test "$(firewall-cmd --zone="$ZONE" --query-port=19181/tcp || true)" = no
firewall-cmd --zone="$ZONE" --query-port=19182/tcp | grep -Fxq yes
# Keep the installed panel service running; the watchdog's native worker
# still must not depend on the X-port executable.
mv /usr/local/bin/xport /usr/local/bin/xport.external-watchdog
restored=0
# Allow systemd and firewalld to complete the actual 120s watchdog under
# loaded KVM scheduling; both old and new runtime+permanent states are required.
for i in $(seq 1 185); do
 old_rt="$(firewall-cmd --zone="$ZONE" --query-port=19181/tcp || true)"
 old_pm="$(firewall-cmd --permanent --zone="$ZONE" --query-port=19181/tcp || true)"
 new_rt="$(firewall-cmd --zone="$ZONE" --query-port=19182/tcp || true)"
 new_pm="$(firewall-cmd --permanent --zone="$ZONE" --query-port=19182/tcp || true)"
 if [[ "$old_rt" = yes && "$old_pm" = yes && "$new_rt" = no && "$new_pm" = no ]]; then
  restored=1; break
 fi
 sleep 1
done
mv /usr/local/bin/xport.external-watchdog /usr/local/bin/xport
if [[ "$restored" != 1 ]]; then
 echo "[panel-coexist] Unconfirmed external edit watchdog did not restore old rule"
 exit 1
fi
echo "[panel-coexist] Real 120-second external edit recovered old rule and removed target while binary missing"

systemctl is-active --quiet 1panel-core
systemctl is-active --quiet xport.service
test "$(getenforce)" = Enforcing
echo "[panel-coexist] SELinux remained Enforcing; both services healthy; external rule protected"
echo "XPORT_PANEL_COEXIST_OK"
