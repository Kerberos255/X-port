#!/usr/bin/env bash
set -Eeuo pipefail
[[ ${EUID:-$(id -u)} -eq 0 ]] || { echo "Run as root." >&2; exit 1; }
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DATA_DIR="${XPORT_DATA_DIR:-/etc/x-port}"
PREFIX="${XPORT_PREFIX:-/usr/local/x-port}"
XPORT_BIN="${XPORT_BIN:-/usr/local/bin/xport}"
LISTEN="${XPORT_LISTEN:-127.0.0.1:8080}"
ADMIN_USER="${XPORT_ADMIN_USER:-admin}"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) LOCAL_BIN="$ROOT/dist/xport-linux-amd64" ;;
  aarch64|arm64) LOCAL_BIN="$ROOT/dist/xport-linux-arm64" ;;
  *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac
[[ -n "${XPORT_BINARY:-}" ]] && LOCAL_BIN="$XPORT_BINARY"
[[ -f "$LOCAL_BIN" ]] || { echo "X-port binary not found: $LOCAL_BIN" >&2; echo "Run scripts/build-linux.sh first or set XPORT_BINARY." >&2; exit 1; }
command -v systemctl >/dev/null || { echo "systemd is required" >&2; exit 1; }
install -d -m 0750 "$DATA_DIR" "$DATA_DIR/xray" "$DATA_DIR/backups" "$PREFIX/bin"
install -m 0755 "$LOCAL_BIN" "$XPORT_BIN"
if [[ -n "${XPORT_ADMIN_PASSWORD:-}" ]]; then ADMIN_PASSWORD="$XPORT_ADMIN_PASSWORD"; else
  read -r -s -p "Admin password (minimum 12 characters): " ADMIN_PASSWORD; echo
  read -r -s -p "Repeat admin password: " ADMIN_PASSWORD_CONFIRM; echo
  [[ "$ADMIN_PASSWORD" == "$ADMIN_PASSWORD_CONFIRM" ]] || { echo "Passwords do not match." >&2; exit 1; }
fi
[[ ${#ADMIN_PASSWORD} -ge 12 ]] || { echo "Password must be at least 12 characters." >&2; exit 1; }
printf '%s\n' "$ADMIN_PASSWORD" | "$XPORT_BIN" init --data "$DATA_DIR" --admin-user "$ADMIN_USER" --listen "$LISTEN"
unset ADMIN_PASSWORD ADMIN_PASSWORD_CONFIRM XPORT_ADMIN_PASSWORD

# Install the latest published Xray release through X-port itself. The updater
# verifies GitHub's SHA-256 asset digest before installing the binary.
"$XPORT_BIN" xray-update --binary "$PREFIX/bin/xray" --config "$DATA_DIR/xray/config.json"
"$XPORT_BIN" render --data "$DATA_DIR" --output "$DATA_DIR/xray/config.json"
"$PREFIX/bin/xray" run -test -config "$DATA_DIR/xray/config.json" >/dev/null

cat > /etc/systemd/system/xport-xray.service <<UNIT
[Unit]
Description=X-port Xray Core
After=network-online.target
Wants=network-online.target
[Service]
Type=simple
Environment=XRAY_LOCATION_ASSET=$PREFIX/bin
ExecStart=$PREFIX/bin/xray run -config $DATA_DIR/xray/config.json
Restart=on-failure
RestartSec=3
LimitNOFILE=1048576
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=read-only
[Install]
WantedBy=multi-user.target
UNIT
cat > /etc/systemd/system/xport.service <<UNIT
[Unit]
Description=X-port Control Panel
After=network-online.target
Wants=network-online.target
[Service]
Type=simple
ExecStart=$XPORT_BIN serve --data $DATA_DIR --xray-binary $PREFIX/bin/xray --xray-config $DATA_DIR/xray/config.json --xray-service xport-xray.service
Restart=on-failure
RestartSec=3
UMask=0027
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=read-only
[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload
systemctl enable xport.service xport-xray.service >/dev/null
systemctl restart xport.service
if systemctl is-active --quiet x-ui.service 2>/dev/null || [[ -f /etc/x-ui/x-ui.db ]]; then
  echo "Existing x-ui/X-Panel detected. X-port Xray was installed but not started to avoid port conflicts."
  echo "Next: sudo $ROOT/scripts/migrate-xpanel.sh"
else
  systemctl restart xport-xray.service
fi
echo "X-port installed. Panel listen: $LISTEN"
echo "Admin user: $ADMIN_USER"
echo "Password is stored only as a bcrypt hash."
if [[ "$LISTEN" == 127.0.0.1:* ]]; then echo "Panel is local-only by default. Put it behind HTTPS reverse proxy for remote access."; fi