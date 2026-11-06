#!/usr/bin/env bash
set -Eeuo pipefail
# Github runner only: no host firewall mutations.
BIN="$1"
[[ -x "$BIN" ]] || exit 1
[[ "$(ps -p 1 -o comm= | tr -d '[:space:]')" == "systemd" ]] || { echo "systemd required" >&2; exit 1; }
TMP="$(mktemp -d -t xport-fw-ci.XXXXXXXX)"
UNIT="xport-ci-fw-$(date +%s)-$$"
cleanup() {
  sudo systemctl stop "$UNIT.service" >/dev/null 2>&1 || true
  sudo systemctl reset-failed "$UNIT.service" >/dev/null 2>&1 || true
  rm -rf "$TMP"
}
trap cleanup EXIT
PORT="$(python3 - <<'PY'
import socket
with socket.socket() as s:
    s.bind(("127.0.0.1",0))
    print(s.getsockname()[1])
PY
)"
export XPORT_CI_PASSWORD
XPORT_CI_PASSWORD="$(openssl rand -hex 20)"
printf '%s\n' "$XPORT_CI_PASSWORD" | sudo "$BIN" init --data "$TMP" --admin-user ci-admin --listen "127.0.0.1:$PORT"
sudo systemd-run --quiet --unit="$UNIT" --property=Type=exec \
  --property=NoNewPrivileges=yes --property=ProtectSystem=strict \
  --property="ReadWritePaths=$TMP" \
  --property="RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6" \
  "$BIN" serve --data "$TMP" --listen "127.0.0.1:$PORT"
for _ in $(seq 1 80); do
  if curl -fsS "http://127.0.0.1:$PORT/" >/dev/null 2>&1; then break; fi
  sleep 0.25
done
curl -fsS "http://127.0.0.1:$PORT/" >/dev/null
family="$(sudo systemctl show "$UNIT.service" -p RestrictAddressFamilies --value)"
[[ "$family" != *AF_NETLINK* ]] || { echo "Old unit simulation failed" >&2; exit 1; }
payload="$(python3 - <<'PY'
import json,os
print(json.dumps({"username":"ci-admin","password":os.environ["XPORT_CI_PASSWORD"]}))
PY
)"
curl -fsS -c "$TMP/cookie" -H 'Content-Type: application/json' \
  --data "$payload" "http://127.0.0.1:$PORT/api/login" >/dev/null
curl -fsS -b "$TMP/cookie" "http://127.0.0.1:$PORT/api/firewall" > "$TMP/firewall.json"
python3 - "$TMP/firewall.json" <<'PY'
import json,sys
with open(sys.argv[1],encoding="utf-8") as f: d=json.load(f)
assert d["backend"] in ("firewalld","ufw","nftables","iptables","mixed","none"),d
assert isinstance(d["rules"],list),d
assert "writable" in d,d
print("WebUI/firewall helper systemd deployment smoke passed")
PY
