#!/usr/bin/env bash
set -u
# Test systemd-run from within an old-unit-equivalent sandbox without
# changing X-port's original service or the guest firewall policy.
# This script is only invoked after the SELinux VM Web read fails.
echo "[helper-matrix] Checking systemd-run from equivalent sandbox"
for case_name in baseline no-pipe netlink no-private-devices no-no-new-privs; do
  outer=(--property=Type=exec --property=ProtectSystem=strict
    --property=PrivateTmp=yes --property=NoNewPrivileges=yes
    --property=PrivateDevices=yes
    --property="RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6"
    --property="ReadWritePaths=/var/lib/xport-selinux-ci")
  inner=(--quiet --wait --collect --pipe --property=Type=exec
    --property=NoNewPrivileges=yes)
  case "$case_name" in
    no-pipe)
      inner=(--quiet --wait --collect --property=Type=exec --property=NoNewPrivileges=yes);;
    netlink)
      outer+=(--property="RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6 AF_NETLINK");;
    no-private-devices)
      outer+=(--property=PrivateDevices=no);;
    no-no-new-privs)
      outer+=(--property=NoNewPrivileges=no);;
  esac
  line="systemd-run $(printf '%q ' "${inner[@]}") --unit=xport-diag-nested-$case_name /usr/local/bin/xport firewall-inspect"
  echo "[helper-matrix] case=$case_name"
  systemd-run --quiet --wait --collect --pipe --unit="xport-diag-outer-$case_name" "${outer[@]}" \
    /bin/bash -c "$line" 2>&1 | head -c 750
  echo
done
echo "[helper-matrix] finished"
