#!/usr/bin/env bash
set -Eeuo pipefail
# Official CentOS Stream 9 qcow2, real KVM guest and SELinux Enforcing.
BIN="$1"
test -f "$BIN"
test -c /dev/kvm || { echo "Real KVM unavailable; Enforcing cannot be validated by a container" >&2; exit 1; }
TMP="$(mktemp -d -t xport-centos-vm.XXXXXXXX)"
cleanup(){ rm -rf "$TMP"; }
trap cleanup EXIT
export DEBIAN_FRONTEND=noninteractive
apt-get -qq update
apt-get -qq -y install --no-install-recommends qemu-system-x86 qemu-utils genisoimage curl ca-certificates >/dev/null

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
cat > "$TMP/user-data" <<'YAML'
#cloud-config
runcmd:
  - [ bash, -lc, "mkdir -p /run/xport-ci-seed; mount -o ro /dev/sr0 /run/xport-ci-seed; bash /run/xport-ci-seed/guest-test.sh" ]
YAML
cat > "$TMP/meta-data" <<'YAML'
instance-id: xport-centos-selinux-ci
local-hostname: xport-centos-ci
YAML
genisoimage -quiet -output "$TMP/seed.iso" -volid cidata -joliet -rock \
 -graft-points user-data="$TMP/user-data" meta-data="$TMP/meta-data" \
  xport="$TMP/xport" guest-test.sh="$TMP/guest-test.sh"
echo "[selinux-vm] Booting official CentOS guest with KVM"
set +e
timeout --signal=TERM 740 qemu-system-x86_64 -enable-kvm -cpu host -m 3072 -smp 2 \
 -display none -monitor none -serial file:"$TMP/console.log" \
 -drive file="$TMP/centos.qcow2",format=qcow2,if=virtio,cache=none \
 -drive file="$TMP/seed.iso",media=cdrom,readonly=on \
 -netdev user,id=net0 -device virtio-net-pci,netdev=net0 -no-reboot \
 >/dev/null 2>"$TMP/qemu.stderr"
RC=$?
set -e
grep -E '\[selinux-vm\]|XPORT_SELINUX_E2E_(OK|FAILED)' "$TMP/console.log" | tail -n 80 || true
if ! grep -Fq XPORT_SELINUX_E2E_OK "$TMP/console.log"; then
 echo "SELinux Enforcing CentOS VM test did not complete successfully (QEMU=$RC)" >&2
 tail -n 140 "$TMP/console.log" >&2 || true
 tail -n 25 "$TMP/qemu.stderr" >&2 || true
 exit 1
fi
echo "CentOS Stream 9 SELinux Enforcing authenticated firewall Web E2E passed"
