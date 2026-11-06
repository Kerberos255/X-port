#!/usr/bin/env bash
set -Eeuo pipefail
# Official CentOS Stream 9 qcow2, real KVM guest and SELinux Enforcing.
BIN="$1"
MODE="${2:-selinux}"
SSH_PORT="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')"
if [[ "$MODE" != "selinux" && "$MODE" != "panel" ]]; then echo "Unknown VM test mode" >&2; exit 1; fi
SUCCESS_MARKER=XPORT_SELINUX_E2E_OK
if [[ "$MODE" == panel ]]; then SUCCESS_MARKER=XPORT_PANEL_COEXIST_OK; fi
test -f "$BIN"
test -c /dev/kvm || { echo "Real KVM unavailable; Enforcing cannot be validated by a container" >&2; exit 1; }
TMP="$(mktemp -d -t xport-centos-vm.XXXXXXXX)"
cleanup(){
 if [[ -n "${QPID:-}" ]]; then kill "$QPID" >/dev/null 2>&1 || true; wait "$QPID" >/dev/null 2>&1 || true; fi
 rm -rf "$TMP"
}
trap cleanup EXIT
export DEBIAN_FRONTEND=noninteractive
apt-get -qq update
apt-get -qq -y install --no-install-recommends qemu-system-x86 qemu-utils genisoimage curl ca-certificates openssh-client >/dev/null

URL="https://cloud.centos.org/centos/9-stream/x86_64/images/CentOS-Stream-GenericCloud-9-20261103.0.x86_64.qcow2"
echo "[selinux-vm] Downloading official CentOS Stream 9 QCOW2 and checking checksum"
curl -fLsS --retry 3 --connect-timeout 25 --max-time 540 "$URL" -o "$TMP/centos.qcow2"
curl -fLsS --retry 3 --connect-timeout 25 "$URL.SHA256SUM" -o "$TMP/checksum"
SUM="$(grep -Eo '[A-Fa-f0-9]{64}' "$TMP/checksum" | head -n1)"
test -n "$SUM" || { echo "Official CentOS SHA256 checksum missing" >&2; exit 1; }
echo "$SUM  $TMP/centos.qcow2" | sha256sum -c -
qemu-img info "$TMP/centos.qcow2"
cp "$BIN" "$TMP/xport"
cp scripts/verify-centos-selinux-guest.sh "$TMP/guest-test.sh"
if [[ "$MODE" == panel ]]; then cp scripts/verify-centos-panel-coexist-guest.sh "$TMP/panel-test.sh"; fi
ssh-keygen -q -t ed25519 -N "" -f "$TMP/ci-key"
PUB="$(cat "$TMP/ci-key.pub")"
cat > "$TMP/user-data" <<YAML
#cloud-config
users:
  - default
  - name: xportci
    groups: wheel
    shell: /bin/bash
    lock_passwd: true
    sudo: ALL=(ALL) NOPASSWD:ALL
    ssh_authorized_keys:
      - $PUB
ssh_pwauth: false
disable_root: true
YAML
cat > "$TMP/meta-data" <<'YAML'
instance-id: xport-centos-selinux-ci
local-hostname: xport-centos-ci
YAML
cp scripts/verify-centos-helper-matrix.sh "$TMP/helper-matrix.sh"
ISO_FILES=( "user-data=$TMP/user-data" "meta-data=$TMP/meta-data" "xport=$TMP/xport" "guest-test.sh=$TMP/guest-test.sh" "helper-matrix.sh=$TMP/helper-matrix.sh" )
if [[ "$MODE" == panel ]]; then ISO_FILES+=( "panel-test.sh=$TMP/panel-test.sh" ); fi
genisoimage -quiet -output "$TMP/seed.iso" -volid cidata -joliet -rock -graft-points "${ISO_FILES[@]}"
echo "[selinux-vm] Booting official CentOS guest with KVM"
set +e
timeout --signal=TERM 740 qemu-system-x86_64 -enable-kvm -cpu host -m 3072 -smp 2 \
 -display none -monitor none -serial file:"$TMP/console.log" \
 -drive file="$TMP/centos.qcow2",format=qcow2,if=virtio,cache=none \
 -drive file="$TMP/seed.iso",media=cdrom,readonly=on \
 -netdev "user,id=net0,hostfwd=tcp:127.0.0.1:$SSH_PORT-:22" -device virtio-net-pci,netdev=net0 -no-reboot \
 >/dev/null 2>"$TMP/qemu.stderr" &
QPID="$!"
set -e
SSH_OPTS=(-i "$TMP/ci-key" -p "$SSH_PORT" -o IdentitiesOnly=yes -o BatchMode=yes -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=3)
ready=0
for i in $(seq 1 100); do
 if ssh "${SSH_OPTS[@]}" xportci@127.0.0.1 'echo XPORT_VM_SSH_READY' 2>/dev/null | grep -Fxq XPORT_VM_SSH_READY; then ready=1; break; fi
 if ! kill -0 "$QPID" 2>/dev/null; then break; fi
 if (( i % 10 == 0 )); then
  echo "[selinux-vm] Waiting for cloud-init SSH user"
  tail -n 2 "$TMP/console.log" || true
 fi
 sleep 2
done
if [[ "$ready" != 1 ]]; then
 echo "[selinux-vm] VM SSH readiness failed" >&2
 tail -n 80 "$TMP/console.log" >&2 || true
 exit 1
fi
echo "[selinux-vm] SSH connected to genuine CentOS Stream guest; running Enforcing checks"
set +e
timeout --signal=TERM 650 ssh "${SSH_OPTS[@]}" xportci@127.0.0.1 \
 'sudo mkdir -p /run/xport-ci-seed && sudo mount -o ro /dev/sr0 /run/xport-ci-seed && sudo XPORT_CI_SSH=1 bash /run/xport-ci-seed/guest-test.sh' \
 2>&1 | tee "$TMP/guest.log"
GUEST_RC=${PIPESTATUS[0]}
set -e
if ! grep -Fq "$SUCCESS_MARKER" "$TMP/guest.log"; then
 echo "[selinux-vm] Guest test failed or timed out, ssh=$GUEST_RC" >&2
 tail -n 90 "$TMP/guest.log" >&2 || true
 tail -n 50 "$TMP/console.log" >&2 || true
 exit 1
fi
echo "[selinux-vm] $SUCCESS_MARKER: real CentOS Enforcing firewall Web E2E passed"
