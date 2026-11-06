#!/usr/bin/env bash
set -Eeuo pipefail
# Run only in a disposable CentOS Stream 9 KVM guest.
test "$(getenforce)" = Enforcing
systemctl show xport-xray.service -p LoadState --value | grep -Fxq not-found

DATA=/var/lib/xport-restart-ci
XRAY_UNIT=xport-xray.service
BASE=http://127.0.0.1:28613
COOKIE=/run/xport-restart-ci.cookie
BODY=/run/xport-restart-ci-response.json
FAIL_FLAG=/run/xport-restart-ci-force-failure
mkdir -p "$DATA"
cat > /usr/local/bin/xport-ci-fake-xray <<'SH'
#!/usr/bin/env bash
set -e
if [[ "$1" == run && "$2" == -test && "$3" == -config ]]; then
 exec python3 -c 'import json,sys;json.load(open(sys.argv[1]))' "$4"
fi
if [[ "$1" == version ]]; then echo 'Xray CI mock 1.0'; exit 0; fi
exit 1
SH
chmod 0755 /usr/local/bin/xport-ci-fake-xray
restorecon /usr/local/bin/xport-ci-fake-xray || true
cat > /etc/systemd/system/xport-xray.service <<'UNIT'
[Unit]
Description=X-port disposable CI restart target
StartLimitIntervalSec=10
StartLimitBurst=5
[Service]
Type=simple
ExecStart=/usr/bin/bash -c 'test ! -e /run/xport-restart-ci-force-failure && exec /usr/bin/sleep 3600'
Restart=on-failure
RestartSec=3
[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload
systemctl start "$XRAY_UNIT"
systemctl is-active --quiet "$XRAY_UNIT"

PASS="$(python3 -c 'import secrets;print(secrets.token_urlsafe(24))')"
printf '%s\n' "$PASS" | /usr/local/bin/xport init --data "$DATA" --admin-user ci-account --listen 127.0.0.1:28613
systemd-run --unit=xport-account-restart-ci --property=Type=exec \
  /usr/local/bin/xport serve --data "$DATA" --listen 127.0.0.1:28613 \
  --xray-binary /usr/local/bin/xport-ci-fake-xray \
  --xray-config "$DATA/config.json" --xray-service "$XRAY_UNIT"
for _ in $(seq 1 50); do
 if curl -fsS "$BASE/" >/dev/null 2>&1; then break; fi
 sleep .3
done
curl -fsS "$BASE/" >/dev/null
LOGIN="$(CI_PASS="$PASS" python3 - <<'PY'
import json,os
print(json.dumps({'username':'ci-account','password':os.environ['CI_PASS']}))
PY
)"
curl -fsS -c "$COOKIE" -H 'Content-Type: application/json' -d "$LOGIN" "$BASE/api/login" >/dev/null
unset PASS LOGIN

json_call() {
 local method="$1" route="$2" body="$3"
 if [[ -n "$body" ]]; then
  curl -fsS --max-time 32 -b "$COOKIE" -H "Origin: $BASE" \
   -H 'Content-Type: application/json' -X "$method" --data "$body" "$BASE$route" -o "$BODY"
 else
  curl -fsS --max-time 32 -b "$COOKIE" -H "Origin: $BASE" \
   -H 'Content-Type: application/json' -X "$method" "$BASE$route" -o "$BODY"
 fi
}
account_name() {
 curl -fsS -b "$COOKIE" "$BASE/api/accounts" | python3 -c 'import json,sys; x=json.load(sys.stdin)["accounts"]; print(x[0]["name"] if x else "")'
}
main_pid() { systemctl show "$XRAY_UNIT" -p MainPID --value; }
assert_active() { systemctl is-active --quiet "$XRAY_UNIT" && test "$(main_pid)" != 0; }

echo '[restart-e2e] Create, idempotent save, update through authenticated account API'
json_call POST /api/accounts '{"name":"restart-ci","enabled":true,"port":52873,"protocol":"vmess","security":"none","network":"tcp"}'
ID="$(python3 -c 'import json; print(json.load(open("/run/xport-restart-ci-response.json"))["id"])')"
test "$ID" -gt 0
assert_active
BEFORE="$(main_pid)"
json_call PUT "/api/accounts/$ID" '{"name":"restart-ci","enabled":true,"port":52873,"protocol":"vmess","security":"none","network":"tcp"}'
test "$(main_pid)" = "$BEFORE" || { echo 'Idempotent save restarted Xray' >&2; exit 1; }
json_call PUT "/api/accounts/$ID" '{"name":"restart-ci-updated","enabled":true,"port":52874,"protocol":"vmess","security":"none","network":"tcp"}'
assert_active
test "$(account_name)" = restart-ci-updated
echo '[restart-e2e] Unchanged save skipped restart; changed save applied'

echo '[restart-e2e] Deliberately reproduce StartLimitBurst=5 / 10 seconds'
sleep 11
LIMIT_HIT=0
for _ in $(seq 1 8); do
 if ! systemctl restart "$XRAY_UNIT" >/dev/null 2>&1; then LIMIT_HIT=1; break; fi
done
test "$LIMIT_HIT" = 1 || { echo 'Could not reproduce systemd rate limit' >&2; exit 1; }
systemctl show "$XRAY_UNIT" -p Result --value | grep -Fxq start-limit-hit
CHECKSUM="$(sha256sum "$DATA/config.json" | awk '{print $1}')"
code="$(curl -sS --max-time 32 -b "$COOKIE" -H "Origin: $BASE" -H 'Content-Type: application/json' \
 -X PUT -d '{"name":"do-not-persist","enabled":true,"port":52875,"protocol":"vmess","security":"none","network":"tcp"}' \
 -o "$BODY" -w '%{http_code}' "$BASE/api/accounts/$ID")"
test "$code" = 400 || { echo "Expected rollback HTTP 400, got $code"; cat "$BODY"; exit 1; }
python3 - "$BODY" <<'PY'
import json,sys
msg=json.load(open(sys.argv[1]))['error']
assert 'previous configuration and service restored' in msg, msg
PY
assert_active
test "$(account_name)" = restart-ci-updated
test "$(sha256sum "$DATA/config.json" | awk '{print $1}')" = "$CHECKSUM"
echo '[restart-e2e] Real start-limit-hit recovered without persisting bad edit'

echo '[restart-e2e] Rapid distinct updates changing ports are paced'
for n in 1 2 3; do
 next_port=$((52875+n))
 json_call PUT "/api/accounts/$ID" "{\"name\":\"restart-ci-$n\",\"enabled\":true,\"port\":$next_port,\"protocol\":\"vmess\",\"security\":\"none\",\"network\":\"tcp\"}"
 assert_active
done
test "$(account_name)" = restart-ci-3
echo '[restart-e2e] Rapid config updates remain operable'


echo '[restart-e2e] A truly failing service is not falsely reported recovered'
CHECKSUM="$(sha256sum "$DATA/config.json" | awk '{print $1}')"
touch "$FAIL_FLAG"
code="$(curl -sS --max-time 35 -b "$COOKIE" -H "Origin: $BASE" -H 'Content-Type: application/json' \
 -X PUT -d '{"name":"must-rollback","enabled":true,"port":52879,"protocol":"vmess","security":"none","network":"tcp"}' \
 -o "$BODY" -w '%{http_code}' "$BASE/api/accounts/$ID")"
test "$code" = 400 || { echo "Expected failure HTTP 400, got $code"; cat "$BODY"; exit 1; }
python3 - "$BODY" <<'PY'
import json,sys
msg=json.load(open(sys.argv[1]))['error']
assert 'previous configuration restored but Xray could not restart' in msg or 'systemd recovery failed' in msg, msg
PY
test "$(account_name)" = restart-ci-3
test "$(sha256sum "$DATA/config.json" | awk '{print $1}')" = "$CHECKSUM"
rm "$FAIL_FLAG"
systemctl stop "$XRAY_UNIT" >/dev/null 2>&1 || true
# After a fully failed transient test start, systemd may garbage-collect
# the unit object. A new start will reload the on-disk unit; do not fail
# cleanup solely because reset-failed sees a not-loaded unit.
systemctl reset-failed "$XRAY_UNIT" >/dev/null 2>&1 || true
systemctl start "$XRAY_UNIT"
assert_active
echo '[restart-e2e] Rollback preserved config and DB when core could not recover'

echo '[restart-e2e] Delete account and confirm panel+Xray health'
json_call DELETE "/api/accounts/$ID" ""
test "$(account_name)" = ""
assert_active
curl -fsS -b "$COOKIE" "$BASE/api/health" | python3 -c 'import json,sys;assert json.load(sys.stdin)["ok"]'
echo 'XPORT_ACCOUNT_SYSTEMD_E2E_OK'
systemctl stop xport-account-restart-ci.service "$XRAY_UNIT" >/dev/null 2>&1 || true
