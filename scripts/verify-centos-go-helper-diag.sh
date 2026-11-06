#!/usr/bin/env bash
set -u
echo "[go-helper-matrix] Running Go process under equivalent legacy X-port sandbox"
install -m 0755 /run/xport-ci-seed/go-helper-diag /usr/local/bin/xport-ci-go-diag
restorecon -v /usr/local/bin/xport-ci-go-diag || true
systemd-run --quiet --wait --collect --pipe --unit=xport-go-diag-outer \
  --property=Type=exec --property=ProtectSystem=strict --property=PrivateTmp=yes \
  --property=NoNewPrivileges=yes --property=PrivateDevices=yes \
  --property="ReadWritePaths=/var/lib/xport-selinux-ci" \
  --property="RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6" \
  /usr/local/bin/xport-ci-go-diag
echo "[go-helper-matrix] finished"
