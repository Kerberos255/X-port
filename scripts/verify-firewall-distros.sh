#!/usr/bin/env bash
set -Eeuo pipefail

# GitHub-hosted runner only. All nftables changes remain inside Docker netns.
BIN="$1"
DISTRO="$2"
CANCEL_HARNESS="${3:?missing timer cancellation helper}"
[[ -x "$CANCEL_HARNESS" ]] || exit 1
[[ -x "$BIN" ]] || exit 1
case "$DISTRO" in
  debian) IMAGE="debian:trixie-slim";BACKEND="ufw";PORT=53111;;
  rocky) IMAGE="rockylinux:9";BACKEND="firewalld";PORT=53112;;
  centos9) IMAGE="quay.io/centos/centos:stream9";BACKEND="firewalld";PORT=53113;;
  *) echo "unknown distro" >&2; exit 1;;
esac
NAME="xport-ci-cross-$DISTRO-$(date +%s)"
RESTORE="xport-ci-c9-restore-$(date +%s)"
CANCEL="xport-fw-restore-$(openssl rand -hex 12)"
cleanup(){
  sudo systemctl stop "$RESTORE.timer" "$CANCEL.timer" >/dev/null 2>&1 || true
  sudo systemctl reset-failed "$RESTORE.service" >/dev/null 2>&1 || true
  docker rm -f "$NAME" >/dev/null 2>&1 || true
}
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
  if [[ "$DISTRO" == centos9 ]]; then
    docker exec "$NAME" sh -c '. /etc/os-release; test "$ID" = centos; test "${VERSION_ID%%.*}" = 9; printf "CentOS Stream test image: %s %s\\n" "$PRETTY_NAME" "$VERSION_ID"'
  fi
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

if [[ "$DISTRO" == centos9 ]]; then
  echo "[centos9] Runtime-only firewalld drift must block deletion"
  docker exec "$NAME" firewall-cmd --zone="$ZONE" --add-port="$PORT/tcp"
  docker exec "$NAME" /usr/local/bin/xport firewall-inspect > /tmp/xport-ci-c9-drift.json
  python3 - "$PORT" <<'PY'
import json,sys
with open("/tmp/xport-ci-c9-drift.json",encoding="utf-8") as f: d=json.load(f)
assert d["backend"]=="firewalld" and d["writable"],d
rules=[r for r in d["rules"] if r["port"]==int(sys.argv[1]) and r["protocol"]=="tcp"]
assert len(rules)==1 and rules[0]["runtime"] and not rules[0]["permanent"], rules
print("CentOS 9 firewalld runtime/permanent drift recognized")
PY
  if docker exec "$NAME" /usr/local/bin/xport firewall-apply remove "$PORT" tcp "$ZONE"; then
    echo "Unsafe deletion of runtime-only rule was unexpectedly allowed" >&2
    exit 1
  fi
  docker exec "$NAME" firewall-cmd --zone="$ZONE" --query-port="$PORT/tcp" | grep -Fxq yes
  docker exec "$NAME" firewall-cmd --zone="$ZONE" --remove-port="$PORT/tcp"
  check true no

  echo "[centos9] Real systemd timer restoring a CentOS firewall rule"
  sudo systemd-run --quiet --unit="$RESTORE" --on-active=5s \
    /usr/bin/docker exec "$NAME" /usr/local/bin/xport firewall-restore firewalld "$PORT" tcp "$ZONE"
  sudo systemctl is-active --quiet "$RESTORE.timer"
  restored=0
  for _ in $(seq 1 30); do
    if docker exec "$NAME" firewall-cmd --zone="$ZONE" --query-port="$PORT/tcp" 2>/dev/null | grep -Fxq yes; then
      restored=1; break
    fi
    sleep 1
  done
  [[ "$restored" == 1 ]] || { echo "CentOS firewall rule was not restored" >&2; exit 1; }
  check true yes
  docker exec "$NAME" /usr/local/bin/xport firewall-apply remove "$PORT" tcp "$ZONE"
  check true no
  echo "[centos9] Runtime drift protection and real timer restore passed"

  echo "[centos9] Verify actual X-port timer cancellation API"
  sudo systemd-run --quiet --unit="$CANCEL" --on-active=6s \
    /usr/bin/docker exec "$NAME" /usr/local/bin/xport firewall-restore firewalld "$PORT" tcp "$ZONE"
  sudo systemctl is-active --quiet "$CANCEL.timer"
  sudo "$CANCEL_HARNESS" "$CANCEL"
  sleep 8
  check true no
  echo "[centos9] X-port timer cancellation prevented recovery"
fi

# An independent nftables input chain must disable Web mutation, regardless
# of the selected manager or Linux distribution.
docker exec "$NAME" nft add table inet xport_ci_custom
docker exec "$NAME" nft 'add chain inet xport_ci_custom input { type filter hook input priority 5; policy accept; }'
check false no
echo "$DISTRO real firewall operations and independent-chain conflict verified"
