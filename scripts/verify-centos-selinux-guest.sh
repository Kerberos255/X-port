#!/usr/bin/env bash
set -Eeuo pipefail
exec >/dev/ttyS0 2>&1
fail(){
 echo "XPORT_SELINUX_E2E_FAILED line $1"
 journalctl -u xport.service -n 35 --no-pager || true
 journalctl -u firewalld.service -n 20 --no-pager || true
 systemctl poweroff --no-block || true
}
trap 'fail $LINENO' ERR

echo "[selinux-vm] CentOS Stream 9 guest ready"
. /etc/os-release
test "$ID" = centos
test "$VERSION_ID" = 9
if [[ "$(getenforce)" == "Permissive" ]]; then setenforce 1; fi
test "$(getenforce)" = "Enforcing"
test "$(cat /sys/fs/selinux/enforce)" = 1
echo "[selinux-vm] SELinux Enforcing active, context $(id -Z)"
dnf -y -q install firewalld nftables iproute curl >/tmp/xport-dnf.log 2>&1 || {
 cat /tmp/xport-dnf.log
 exit 1
}
systemctl enable --now firewalld
firewall-cmd --state | grep -Fxq running
ZONE="$(firewall-cmd --get-default-zone | tr -d '\r\n')"
install -D -m 0755 /run/xport-ci-seed/xport /usr/local/bin/xport
restorecon -v /usr/local/bin/xport || true
echo "[selinux-vm] Binary SELinux label $(ls -Z /usr/local/bin/xport)"
DATA=/var/lib/xport-selinux-ci
PORT=28612
mkdir -p "$DATA"
printf 'selinux-ci-pass-12345\n' | xport init --data "$DATA" --admin-user ci-admin --listen "127.0.0.1:$PORT"
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
curl -fsS -b /run/xport-ci.cookie "$BASE/api/firewall" > /run/xport-firewall.json
python3 - <<'PY'
import json
with open("/run/xport-firewall.json") as f:d=json.load(f)
assert d["backend"]=="firewalld" and d["writable"] is True,d
print("[selinux-vm] Web firewall read passed in Enforcing")
PY
echo "[selinux-vm] Testing authenticated Web rule add"
code="$(curl -sS -b /run/xport-ci.cookie -H "Origin: $BASE" \
 -H 'Content-Type: application/json' -o /run/add.json -w '%{http_code}' \
 -d "{\"action\":\"add\",\"port\":53333,\"protocol\":\"tcp\",\"zone\":\"$ZONE\"}" "$BASE/api/firewall/rules")"
if [[ "$code" != 200 ]]; then cat /run/add.json; exit 1; fi
firewall-cmd --zone="$ZONE" --query-port=53333/tcp | grep -Fxq yes
firewall-cmd --permanent --zone="$ZONE" --query-port=53333/tcp | grep -Fxq yes
echo "[selinux-vm] Web rule added, both runtime/permanent"

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
firewall-cmd --zone="$ZONE" --query-port=53333/tcp | grep -Fxq no
firewall-cmd --permanent --zone="$ZONE" --query-port=53333/tcp | grep -Fxq no
echo "[selinux-vm] Web rule removal confirmed"
echo "XPORT_SELINUX_E2E_OK"
sync
systemctl poweroff --no-block
