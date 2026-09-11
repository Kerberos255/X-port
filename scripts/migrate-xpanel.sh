#!/usr/bin/env bash
set -Eeuo pipefail
[[ ${EUID:-$(id -u)} -eq 0 ]] || { echo "Run as root." >&2; exit 1; }
XPORT_BIN="${XPORT_BIN:-/usr/local/bin/xport}"
DATA_DIR="${XPORT_DATA_DIR:-/etc/x-port}"
SOURCE_DB="${1:-/etc/x-ui/x-ui.db}"
SOURCE_CONFIG="${XPORT_SOURCE_CONFIG:-/usr/local/x-ui/bin/config.json}"
XRAY_BIN="${XPORT_XRAY_BIN:-/usr/local/x-port/bin/xray}"
DEST_CONFIG="$DATA_DIR/xray/config.json"
OLD_SERVICE="${XPORT_OLD_SERVICE:-x-ui.service}"
STAMP="$(date +%Y%m%d-%H%M%S)"
BACKUP="$DATA_DIR/backups/xpanel-$STAMP"
STOPPED_OLD=0; CUTOVER_OK=0; WAS_ENABLED=0
[[ -x "$XPORT_BIN" ]] || { echo "Missing $XPORT_BIN. Run install.sh first." >&2; exit 1; }
[[ -x "$XRAY_BIN" ]] || { echo "Missing $XRAY_BIN. Run install.sh first." >&2; exit 1; }
[[ -f "$SOURCE_DB" ]] || { echo "Source database not found: $SOURCE_DB" >&2; exit 1; }
rollback_on_error(){ rc=$?; if [[ $rc -ne 0 && $CUTOVER_OK -eq 0 && $STOPPED_OLD -eq 1 ]]; then echo "Migration failed; restoring old panel..." >&2; systemctl start "$OLD_SERVICE" >/dev/null 2>&1 || true; fi; exit $rc; }
trap rollback_on_error ERR

echo "=== X-port migration dry-run ==="
"$XPORT_BIN" migrate --data "$DATA_DIR" --from "$SOURCE_DB"
echo
read -r -p "Continue with backup and cutover? [y/N] " answer
[[ "$answer" =~ ^[Yy]$ ]] || { echo "Cancelled. Nothing changed."; exit 0; }
install -d -m 0750 "$BACKUP" "$DATA_DIR/xray"
systemctl is-enabled "$OLD_SERVICE" >/dev/null 2>&1 && WAS_ENABLED=1 || true
if systemctl is-active "$OLD_SERVICE" >/dev/null 2>&1; then systemctl stop "$OLD_SERVICE"; STOPPED_OLD=1; fi
cp -a "$SOURCE_DB" "$BACKUP/"
[[ -f "$SOURCE_DB-wal" ]] && cp -a "$SOURCE_DB-wal" "$BACKUP/"
[[ -f "$SOURCE_DB-shm" ]] && cp -a "$SOURCE_DB-shm" "$BACKUP/"
[[ -f "$SOURCE_CONFIG" ]] && cp -a "$SOURCE_CONFIG" "$BACKUP/xray-config.json"
[[ -f "$DATA_DIR/xport.db" ]] && cp -a "$DATA_DIR/xport.db" "$BACKUP/xport-before.db"
[[ -f "$DEST_CONFIG" ]] && cp -a "$DEST_CONFIG" "$BACKUP/xport-config-before.json"
"$XPORT_BIN" migrate --data "$DATA_DIR" --from "$SOURCE_DB" --apply
"$XPORT_BIN" render --data "$DATA_DIR" --output "$DEST_CONFIG"
"$XRAY_BIN" run -test -config "$DEST_CONFIG"
systemctl restart xport-xray.service
sleep 1
systemctl is-active --quiet xport-xray.service
systemctl restart xport.service
systemctl is-active --quiet xport.service
if [[ $STOPPED_OLD -eq 1 ]]; then systemctl disable "$OLD_SERVICE" >/dev/null 2>&1 || true; fi
CUTOVER_OK=1; trap - ERR
cat > "$BACKUP/rollback.sh" <<ROLLBACK
#!/usr/bin/env bash
set -e
systemctl stop xport.service xport-xray.service || true
$(if [[ $WAS_ENABLED -eq 1 ]]; then echo "systemctl enable $OLD_SERVICE >/dev/null 2>&1 || true"; else echo ":"; fi)
systemctl start $OLD_SERVICE
ROLLBACK
chmod 0700 "$BACKUP/rollback.sh"
echo "Migration complete. Backup: $BACKUP"
echo "Old panel was not deleted. Rollback: $BACKUP/rollback.sh"
