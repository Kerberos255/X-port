#!/usr/bin/env bash
set -Eeuo pipefail

# Exercise the actual authenticated POST /api/xport/update on the real v0.2.8
# handler, using a loopback-only fake release service and an isolated systemd
# unit. Only the endpoint string in the *test-built* old binary is overlaid.
# No production code, release tags, real networks or real firewall rules change.
ROOT="$(pwd)"
TMP="$(mktemp -d -t xport-http-upgrade-ci.XXXXXXXX)"
UNIT="xport"
MOCK_PID=""
cleanup() {
  sudo systemctl stop "$UNIT.service" >/dev/null 2>&1 || true
  sudo systemctl reset-failed "$UNIT.service" >/dev/null 2>&1 || true
  if [[ -n "$MOCK_PID" ]]; then kill "$MOCK_PID" >/dev/null 2>&1 || true; fi
  git worktree remove --force "$TMP/old-src" >/dev/null 2>&1 || true
  rm -rf "$TMP"
}
trap cleanup EXIT
if [[ "$(sudo systemctl show -p LoadState --value "$UNIT.service" 2>/dev/null)" != "not-found" ]]; then
  echo "Refusing to reuse an existing xport.service" >&2
  exit 1
fi
git worktree add --quiet --detach "$TMP/old-src" v0.2.8
mkdir -p "$TMP/live" "$TMP/data"
MOCK_PORT="$(python3 - <<'PY'
import socket
with socket.socket() as s:
  s.bind(("127.0.0.1",0))
  print(s.getsockname()[1])
PY
)"
WEB_PORT="$(python3 - <<'PY'
import socket
with socket.socket() as s:
  s.bind(("127.0.0.1",0))
  print(s.getsockname()[1])
PY
)"
export XPORT_CI_MOCK_ASSET="$TMP/candidate" XPORT_CI_MOCK_PORT="$MOCK_PORT"
python3 - "$TMP/old-src/internal/server/extra.go" "$TMP/old-src/extra-ci.go" "$TMP/old-overlay.json" "$MOCK_PORT" <<'PY'
import json,os,sys
src,dst,overlay,port=sys.argv[1:]
with open(src,encoding="utf-8") as f: data=f.read()
needle='Repo: "Kerberos255/X-port"'
assert data.count(needle)==1, "unexpected v0.2.8 updater configuration"
data=data.replace(needle,f'ReleasesURL: "http://127.0.0.1:{port}/releases", '+needle)
with open(dst,"w",encoding="utf-8") as f:f.write(data)
with open(overlay,"w",encoding="utf-8") as f:json.dump({"Replace":{os.path.abspath(src):os.path.abspath(dst)}},f)
PY
CGO_ENABLED=0 go -C "$TMP/old-src" build -overlay "$TMP/old-overlay.json" -trimpath -o "$TMP/live/xport" ./cmd/xport
[[ "$("$TMP/live/xport" version)" == 0.2.8 ]]
printf 'package buildinfo\nconst Current = "0.2.9"\n' > "$TMP/test-version.go"
python3 - "$ROOT/internal/buildinfo/version.go" "$TMP/test-version.go" "$TMP/new-overlay.json" <<'PY'
import json,os,sys
with open(sys.argv[3],"w",encoding="utf-8") as f:json.dump({"Replace":{os.path.abspath(sys.argv[1]):os.path.abspath(sys.argv[2])}},f)
PY
CGO_ENABLED=0 go build -overlay "$TMP/new-overlay.json" -trimpath -o "$TMP/candidate" ./cmd/xport
[[ "$("$TMP/candidate" version)" == 0.2.9 ]]
cat > "$TMP/mock-release.py" <<'PY'
import hashlib,http.server,json,os
with open(os.environ["XPORT_CI_MOCK_ASSET"],"rb") as f: asset=f.read()
digest=hashlib.sha256(asset).hexdigest()
port=int(os.environ["XPORT_CI_MOCK_PORT"])
class API(http.server.BaseHTTPRequestHandler):
 def do_GET(self):
  if self.path=="/releases":
   body=json.dumps([{"tag_name":"v0.2.9","draft":False,"prerelease":False,
     "assets":[{"name":"xport-linux-amd64","size":len(asset),
       "url":f"http://127.0.0.1:{port}/asset","digest":"sha256:"+digest}]}]).encode()
  elif self.path=="/asset":body=asset
  else:self.send_error(404);return
  self.send_response(200);self.send_header("Content-Type","application/json" if self.path=="/releases" else "application/octet-stream")
  self.send_header("Content-Length",str(len(body)));self.end_headers();self.wfile.write(body)
 def log_message(self,*args):pass
http.server.ThreadingHTTPServer(("127.0.0.1",port),API).serve_forever()
PY
python3 "$TMP/mock-release.py" >/dev/null 2>&1 &
MOCK_PID="$!"
for _ in $(seq 1 50); do
 if curl -fsS "http://127.0.0.1:$MOCK_PORT/releases" > /dev/null 2>&1; then break; fi
 sleep .1
done
curl -fsS "http://127.0.0.1:$MOCK_PORT/releases" >/dev/null
export XPORT_CI_PASSWORD
XPORT_CI_PASSWORD="$(openssl rand -hex 24)"
printf '%s\n' "$XPORT_CI_PASSWORD" | sudo "$TMP/live/xport" init \
  --data "$TMP/data" --admin-user ci-admin --listen "127.0.0.1:$WEB_PORT"
sudo systemd-run --quiet --unit="$UNIT" --property=Type=exec \
  --property=NoNewPrivileges=yes --property=ProtectSystem=strict \
  --property=PrivateTmp=yes \
  --property="ReadWritePaths=$TMP/data $TMP/live" \
  --property="RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6" \
  "$TMP/live/xport" serve --data "$TMP/data" --listen "127.0.0.1:$WEB_PORT"
BASE="http://127.0.0.1:$WEB_PORT"
for _ in $(seq 1 80); do
 if curl -fsS "$BASE/" >/dev/null 2>&1; then break; fi
 sleep .25
done
curl -fsS "$BASE/" >/dev/null
payload="$(python3 - <<'PY'
import json,os
print(json.dumps({"username":"ci-admin","password":os.environ["XPORT_CI_PASSWORD"]}))
PY
)"
curl -fsS -c "$TMP/cookies" -H 'Content-Type: application/json' \
  --data "$payload" "$BASE/api/login" > /dev/null
echo "[http-upgrade] Authenticated GET /api/xport/update (mock Release)"
curl -fsS -b "$TMP/cookies" "$BASE/api/xport/update" > "$TMP/check.json"
python3 - "$TMP/check.json" <<'PY'
import json,sys
with open(sys.argv[1]) as f:d=json.load(f)
assert d["available"] and d["latest"]=="0.2.9",d
print("Web update check detects version 0.2.9")
PY
echo "[http-upgrade] Authenticated POST /api/xport/update"
code="$(curl -sS -o "$TMP/response.json" -w '%{http_code}' -b "$TMP/cookies" \
  -H "Origin: $BASE" -H 'Content-Type: application/json' -X POST --data '{}' "$BASE/api/xport/update")"
if [[ "$code" != 202 ]]; then cat "$TMP/response.json" >&2; echo "HTTP status $code" >&2; exit 1; fi
python3 - "$TMP/response.json" <<'PY'
import json,sys
with open(sys.argv[1]) as f:d=json.load(f)
assert d["scheduled"] and d["update"]["current"]=="0.2.9",d
print("Web upgrade POST returned scheduled=true")
PY
ok=0
for _ in $(seq 1 90); do
 if [[ "$("$TMP/live/xport" version)" == 0.2.9 ]] && \
  curl -fsS "$BASE/" >/dev/null 2>&1 && sudo systemctl is-active --quiet "$UNIT.service"; then
  ok=1;break
 fi
 sleep .5
done
[[ "$ok" == 1 ]] || { echo "Web upgrade did not become healthy" >&2; sudo journalctl -u "$UNIT.service" -n 50 --no-pager >&2 || true; exit 1; }
sleep 12
[[ "$("$TMP/live/xport" version)" == 0.2.9 ]]
[[ "$(sudo systemctl show "$UNIT.service" -p RestrictAddressFamilies --value)" != *AF_NETLINK* ]]
curl -fsS -c "$TMP/cookies2" -H 'Content-Type: application/json' --data "$payload" "$BASE/api/login" >/dev/null
curl -fsS -b "$TMP/cookies2" "$BASE/api/firewall" > "$TMP/firewall.json"
python3 - "$TMP/firewall.json" <<'PY'
import json,sys
with open(sys.argv[1]) as f:d=json.load(f)
assert isinstance(d["rules"],list) and "writable" in d and "backend" in d,d
print("Authenticated Web upgrade v0.2.8 -> v0.2.9 and firewall API passed under old sandbox")
PY
