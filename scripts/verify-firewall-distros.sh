#!/usr/bin/env bash
set -Eeuo pipefail

# GitHub-hosted runner only. All nftables changes remain inside Docker netns.
BIN="$1"
DISTRO="$2"
[[ -x "$BIN" ]] || exit 1
case "$DISTRO" in
  debian) IMAGE="debian:trixie-slim";BACKEND="ufw";PORT=53111;;
  rocky) IMAGE="rockylinux:9";BACKEND="firewalld";PORT=53112;;
  *) echo "unknown distro" >&2; exit 1;;
esac
NAME="xport-ci-cross-$DISTRO-$(date +%s)-$$"
cleanup(){ docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT
docker run -d --rm --privileged --network bridge --name "$NAME" \
  --mount "type=bind,src=$BIN,dst=/usr/local/bin/xport,readonly" \
  "$IMAGE" sleep infinity >/dev/null

if [[ "$DISTRO" == debian ]]; then
  docker exec "$NAME" env DEBIAN_FRONTEND=noninteractive sh -ec \
    'apt-get update -qq && apt-get install -y -qq --no-install-recommends ufw nftables iptables >/dev/null'
  docker exec "$NAME" ufw --force enable
  ZONE=""
else
  docker exec "$NAME" sh -ec \
    'dnf -y -q install firewalld dbus-daemon nftables >/tmp/install.log 2>&1 || { tail -n 40 /tmp/install.log; exit 1; }'
  docker exec "$NAME" sh -ec \
    'mkdir -p /run/dbus; dbus-daemon --system --fork; firewalld --nofork --nopid >/tmp/firewalld.log 2>&1 &'
  ready=0
  for _ in $(seq 1 90); do
    if docker exec "$NAME" firewall-cmd --state 2>/dev/null | grep -Fq running; then ready=1; break; fi
    sleep .5
  done
  if [[ "$ready" != 1 ]]; then
    docker exec "$NAME" cat /tmp/firewalld.log >&2
    exit 1
  fi
  ZONE="$(docker exec "$NAME" firewall-cmd --get-default-zone | tr -d '\r\n')"
fi

check(){
  local expected="$1" present="$2"
  docker exec "$NAME" /usr/local/bin/xport firewall-inspect > /tmp/xport-ci-cross-snapshot.json
  python3 - "$BACKEND" "$PORT" "$expected" "$present" <<'PY'
import json,sys
with open("/tmp/xport-ci-cross-snapshot.json",encoding="utf-8") as f: d=json.load(f)
assert d["backend"]==sys.argv[1],d
assert d["writable"] is (sys.argv[3]=="true"),d
found=any(r["port"]==int(sys.argv[2]) and r["protocol"]=="tcp" for r in d["rules"])
assert found==(sys.argv[4]=="yes"),d
print("Cross-distro verified",sys.argv[1],"writable",d["writable"],"port",sys.argv[2],"present",found)
PY
}
check true no
docker exec "$NAME" /usr/local/bin/xport firewall-apply add "$PORT" tcp "$ZONE"
check true yes
docker exec "$NAME" /usr/local/bin/xport firewall-apply remove "$PORT" tcp "$ZONE"
check true no

# An independent nftables input chain must disable Web mutation, regardless
# of the selected manager or Linux distribution.
docker exec "$NAME" nft add table inet xport_ci_custom
docker exec "$NAME" nft 'add chain inet xport_ci_custom input { type filter hook input priority 5; policy accept; }'
check false no
echo "$DISTRO real firewall operations and independent-chain conflict verified"
