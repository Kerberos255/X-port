#!/usr/bin/env bash
set -Eeuo pipefail

# CI only: firewall modifications are isolated to privileged containers with
# independent Docker network namespaces. Never use --network host.
BIN="$1"
[[ -x "$BIN" ]] || { echo "binary not found: $BIN" >&2; exit 1; }
command -v docker >/dev/null
command -v systemd-run >/dev/null
[[ "$(ps -p 1 -o comm= | tr -d '[:space:]')" == "systemd" ]] || {
  echo "systemd runner required" >&2; exit 1;
}
TMP="$(mktemp -d -t xport-fw-e2e.XXXXXXXX)"
IDENT="$(date +%s)-$$"
UF="xport-ufw-ci-$IDENT"
FW="xport-firewalld-ci-$IDENT"
RESTORE="xport-ci-restore-$IDENT"
CANCEL="xport-ci-cancel-$IDENT"
cleanup() {
  sudo systemctl stop "$RESTORE.timer" "$CANCEL.timer" >/dev/null 2>&1 || true
  docker rm -f "$UF" "$FW" >/dev/null 2>&1 || true
  rm -rf "$TMP"
}
trap cleanup EXIT
run_container() {
  local name="$1"
  docker run -d --rm --privileged --network bridge --name "$name" \
    --mount "type=bind,src=$BIN,dst=/usr/local/bin/xport,readonly" \
    ubuntu:24.04 sleep infinity >/dev/null
}
check_snapshot() {
  local target="$1" expected="$2" writable="$3" port="$4" should_exist="$5"
  docker exec "$target" /usr/local/bin/xport firewall-inspect > "$TMP/snapshot.json"
  python3 - "$TMP/snapshot.json" "$expected" "$writable" "$port" "$should_exist" <<'PY'
import json,sys
with open(sys.argv[1],encoding="utf-8") as f:
    d=json.load(f)
assert d["backend"]==sys.argv[2], d
assert d["writable"] is (sys.argv[3]=="true"), d
found=any(r["port"]==int(sys.argv[4]) and r["protocol"]=="tcp" for r in d["rules"])
assert found is (sys.argv[5]=="yes"), d
print("Verified",d["backend"],"writable",d["writable"],"port",sys.argv[4],"exists",found)
PY
}

echo "[UFW] Set up isolated Ubuntu container"
run_container "$UF"
docker exec "$UF" env DEBIAN_FRONTEND=noninteractive sh -ec \
  'apt-get update -qq && apt-get install -y -qq --no-install-recommends ufw nftables iptables iproute2 python3 >/dev/null'
docker exec "$UF" ufw --force enable
docker exec "$UF" ufw status verbose
echo "[UFW] Kernel rules in isolated container (diagnostics)"
docker exec "$UF" nft -a list ruleset > "$TMP/ufw-nft.txt"
sed -n "1,110p" "$TMP/ufw-nft.txt"
check_snapshot "$UF" ufw true 54321 no
docker exec "$UF" /usr/local/bin/xport firewall-apply add 54321 tcp ""
check_snapshot "$UF" ufw true 54321 yes
docker exec "$UF" /usr/local/bin/xport firewall-apply remove 54321 tcp ""
check_snapshot "$UF" ufw true 54321 no
docker exec "$UF" /usr/local/bin/xport firewall-restore ufw 54321 tcp ""
check_snapshot "$UF" ufw true 54321 yes
echo "[UFW] Real rule add/remove/restore passed in isolated netns"

echo "[firewalld] Set up isolated Ubuntu container"
run_container "$FW"
docker exec "$FW" env DEBIAN_FRONTEND=noninteractive sh -ec \
  'apt-get update -qq && apt-get install -y -qq --no-install-recommends firewalld dbus nftables iproute2 python3 >/dev/null'
docker exec "$FW" sh -ec \
  'mkdir -p /run/dbus; dbus-daemon --system --fork; firewalld --nofork --nopid >/tmp/firewalld.log 2>&1 &'
active=0
for _ in $(seq 1 80); do
  if docker exec "$FW" firewall-cmd --state 2>/dev/null | grep -Fq running; then active=1; break; fi
  sleep .5
done
if [[ "$active" != 1 ]]; then
  docker exec "$FW" cat /tmp/firewalld.log >&2
  exit 1
fi
check_snapshot "$FW" firewalld true 54322 no
docker exec "$FW" /usr/local/bin/xport firewall-apply add 54322 tcp public
check_snapshot "$FW" firewalld true 54322 yes
docker exec "$FW" firewall-cmd --permanent --zone=public --query-port=54322/tcp | grep -Fq yes
docker exec "$FW" /usr/local/bin/xport firewall-apply remove 54322 tcp public
check_snapshot "$FW" firewalld true 54322 no
echo "[firewalld] Real runtime/permanent add/remove passed in isolated netns"

# A real host systemd timer restores only the container-local firewall.
echo "[timer] Arm a real 120s external systemd recovery timer"
sudo systemd-run --quiet --unit="$RESTORE" --on-active=120s \
  /usr/bin/docker exec "$FW" /usr/local/bin/xport firewall-restore firewalld 54322 tcp public
sudo systemctl is-active --quiet "$RESTORE.timer"
sleep 124
for _ in $(seq 1 30); do
  if docker exec "$FW" firewall-cmd --zone=public --query-port=54322/tcp | grep -Fq yes; then break; fi
  sleep 1
done
check_snapshot "$FW" firewalld true 54322 yes
echo "[timer] 120s actual timer fired; real isolated rule restored"

echo "[timer] Verify cancellation prevents execution"
sudo systemd-run --quiet --unit="$CANCEL" --on-active=6s \
  /usr/bin/touch "$TMP/cancelled-run"
sudo systemctl is-active --quiet "$CANCEL.timer"
sudo systemctl stop "$CANCEL.timer"
sleep 8
[[ ! -e "$TMP/cancelled-run" ]] || { echo "Cancelled timer unexpectedly ran" >&2; exit 1; }
echo "[timer] cancellation verified; host firewall never modified"
