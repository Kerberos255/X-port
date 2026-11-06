#!/usr/bin/env bash
set -Eeuo pipefail

# Ephemeral GitHub runner test: v0.2.8 tag -> test-built v0.2.9, followed by
# an invalid candidate and the real systemd external restart/rollback supervisor.
# The local httptest server replaces the GitHub release API only inside this test.
ROOT="$(pwd)"
TMP="$(mktemp -d -t xport-web-upgrade-ci.XXXXXXXX)"
UNIT="xport-ci-web-upgrade-$(date +%s)-$$"
cleanup() {
  sudo systemctl stop "$UNIT.service" >/dev/null 2>&1 || true
  sudo systemctl reset-failed "$UNIT.service" >/dev/null 2>&1 || true
  git worktree remove --force "$TMP/old-src" >/dev/null 2>&1 || true
  rm -rf "$TMP"
}
trap cleanup EXIT

echo "[upgrade] Build actual X-port v0.2.8 tag in isolated worktree"
git worktree add --quiet --detach "$TMP/old-src" v0.2.8
mkdir -p "$TMP/live" "$TMP/data"
CGO_ENABLED=0 go -C "$TMP/old-src" build -trimpath -o "$TMP/live/xport" ./cmd/xport
[[ "$("$TMP/live/xport" version)" == 0.2.8 ]] || { echo "wrong old binary" >&2; exit 1; }
echo "[upgrade] Build candidate with version overlay (working tree unchanged)"
printf 'package buildinfo\nconst Current = "0.2.9"\n' > "$TMP/test-version.go"
python3 - "$ROOT/internal/buildinfo/version.go" "$TMP/test-version.go" "$TMP/overlay.json" <<'PY'
import json,sys,os
with open(sys.argv[3],"w",encoding="utf-8") as f:
    json.dump({"Replace":{os.path.abspath(sys.argv[1]):os.path.abspath(sys.argv[2])}},f)
PY
CGO_ENABLED=0 go build -overlay "$TMP/overlay.json" -trimpath -o "$TMP/candidate" ./cmd/xport
[[ "$("$TMP/candidate" version)" == 0.2.9 ]] || { echo "test overlay failed" >&2; exit 1; }
CGO_ENABLED=0 go build -trimpath -o "$TMP/upgrade-harness" ./scripts/testdata/upgrade_harness.go

PORT="$(python3 - <<'PY'
import socket
with socket.socket() as s:
    s.bind(("127.0.0.1",0))
    print(s.getsockname()[1])
PY
)"
export XPORT_UPGRADE_TEST_PASSWORD
XPORT_UPGRADE_TEST_PASSWORD="$(openssl rand -hex 20)"
printf '%s\n' "$XPORT_UPGRADE_TEST_PASSWORD" | sudo "$TMP/live/xport" init \
  --data "$TMP/data" --admin-user ci-admin --listen "127.0.0.1:$PORT"

echo "[upgrade] Start the old binary with old AF_NETLINK-restricted systemd sandbox"
sudo systemd-run --quiet --unit="$UNIT" --property=Type=exec \
  --property=NoNewPrivileges=yes --property=ProtectSystem=strict \
  --property="ReadWritePaths=$TMP/data" \
  --property="RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6" \
  "$TMP/live/xport" serve --data "$TMP/data" --listen "127.0.0.1:$PORT"
health(){
  curl -fsS --max-time 3 "http://127.0.0.1:$PORT/api/health" >/dev/null 2>&1
}
for _ in $(seq 1 80); do
  if health; then break; fi
  sleep .25
done
health || { echo "old Web service failed startup" >&2; exit 1; }
[[ "$(sudo systemctl show "$UNIT.service" -p RestrictAddressFamilies --value)" != *AF_NETLINK* ]] || exit 1

echo "[upgrade] Stage verified candidate and schedule real external systemd restart"
"$TMP/upgrade-harness" 0.2.8 0.2.9 "$TMP/live/xport" "$TMP/candidate" "$UNIT.service"
updated=0
for _ in $(seq 1 80); do
  if [[ "$("$TMP/live/xport" version)" == 0.2.9 ]] && health && sudo systemctl is-active --quiet "$UNIT.service"; then
    updated=1
    break
  fi
  sleep .5
done
[[ "$updated" == 1 ]] || { echo "updated X-port did not become healthy" >&2; exit 1; }
# The supervisor checks service liveness for ten seconds.
sleep 12
health
[[ "$("$TMP/live/xport" version)" == 0.2.9 ]]
[[ "$(sudo systemctl show "$UNIT.service" -p RestrictAddressFamilies --value)" != *AF_NETLINK* ]] || exit 1
payload="$(python3 - <<'PY'
import json,os
print(json.dumps({"username":"ci-admin","password":os.environ["XPORT_UPGRADE_TEST_PASSWORD"]}))
PY
)"
curl -fsS -c "$TMP/cookie" -H 'Content-Type: application/json' \
  --data "$payload" "http://127.0.0.1:$PORT/api/login" >/dev/null
curl -fsS -b "$TMP/cookie" \
  "http://127.0.0.1:$PORT/api/firewall" > "$TMP/after-upgrade.json"
python3 - "$TMP/after-upgrade.json" <<'PY'
import json,sys
with open(sys.argv[1],encoding="utf-8") as f: d=json.load(f)
assert "rules" in d and "writable" in d,d
print("Real old-unit binary upgrade: new firewall Web API operational")
PY

echo "[upgrade] Stage deliberately bad candidate to check external rollback"
cat > "$TMP/invalid-candidate" <<'SH'
#!/bin/sh
if [ "$1" = version ]; then echo 0.2.10; exit 0; fi
exit 13
SH
chmod 755 "$TMP/invalid-candidate"
"$TMP/upgrade-harness" 0.2.9 0.2.10 "$TMP/live/xport" "$TMP/invalid-candidate" "$UNIT.service"
rolled_back=0
for _ in $(seq 1 90); do
  if [[ "$("$TMP/live/xport" version)" == 0.2.9 ]] && health && sudo systemctl is-active --quiet "$UNIT.service"; then
    rolled_back=1
    break
  fi
  sleep .5
done
[[ "$rolled_back" == 1 ]] || { echo "bad candidate did not roll back to healthy binary" >&2; exit 1; }
echo "[upgrade] Bad release binary rolled back successfully; Web service and database remain usable"
