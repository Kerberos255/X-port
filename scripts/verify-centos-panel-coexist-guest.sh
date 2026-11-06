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
systemctl is-active --quiet 1panel-core
systemctl is-active --quiet xport.service
test "$(getenforce)" = Enforcing
echo "[panel-coexist] SELinux remained Enforcing; both services healthy; external rule protected"
echo "XPORT_PANEL_COEXIST_OK"
