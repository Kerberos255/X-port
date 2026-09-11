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
UNIT_STATE_FILE="$BACKUP/old-unit-states.tsv"
XPORT_DB="$DATA_DIR/xport.db"
MIGRATE_GLOBAL_ARGS=()
[[ -f "$SOURCE_CONFIG" ]] && MIGRATE_GLOBAL_ARGS+=(--source-config "$SOURCE_CONFIG")

CUTOVER_OK=0
ROLLBACK_STARTED=0
XPORT_DB_EXISTED=0
XPORT_CONFIG_EXISTED=0
XPORT_PANEL_WAS_ACTIVE=0
XPORT_XRAY_WAS_ACTIVE=0
OLD_WAS_ACTIVE=0

[[ -x "$XPORT_BIN" ]] || { echo "Missing $XPORT_BIN. Run install.sh first." >&2; exit 1; }
[[ -x "$XRAY_BIN" ]] || { echo "Missing $XRAY_BIN. Run install.sh first." >&2; exit 1; }
[[ -f "$SOURCE_DB" ]] || { echo "Source database not found: $SOURCE_DB" >&2; exit 1; }
command -v systemctl >/dev/null || { echo "systemd is required for guarded migration." >&2; exit 1; }

unit_exists() { systemctl cat "$1" >/dev/null 2>&1; }
unit_active() { systemctl is-active --quiet "$1" >/dev/null 2>&1; }

collect_old_units() {
  {
    systemctl list-unit-files --type=service --all --no-legend --no-pager 2>/dev/null || true
    systemctl list-unit-files --type=timer --all --no-legend --no-pager 2>/dev/null || true
  } | awk '{print $1}' \
    | grep -Evi '^xport(-xray)?\.(service|timer)$' \
    | grep -Ei '(^|[-_.])(x-ui|xui|x-panel|xpanel)([-_.]|$)' \
    | sort -u || true
}

record_unit_states() {
  : > "$UNIT_STATE_FILE"
  local units unit enabled active
  units="$(collect_old_units)"
  if ! grep -Fxq "$OLD_SERVICE" <<<"$units" && unit_exists "$OLD_SERVICE"; then
    units="${units}${units:+$'\n'}$OLD_SERVICE"
  fi
  while IFS= read -r unit; do
    [[ -n "$unit" ]] || continue
    enabled="$(systemctl is-enabled "$unit" 2>/dev/null || true)"
    active="$(systemctl is-active "$unit" 2>/dev/null || true)"
    printf '%s\t%s\t%s\n' "$unit" "${enabled:-unknown}" "${active:-unknown}" >> "$UNIT_STATE_FILE"
  done <<<"$units"
}

quiesce_old_stack() {
  local unit
  echo "Suppressing old X-Panel services/timers and watchdogs..."
  while IFS=$'\t' read -r unit _ _; do
    [[ -n "$unit" ]] || continue
    systemctl mask --now "$unit" >/dev/null 2>&1 || {
      echo "Unable to stop/mask old related unit: $unit" >&2
      return 1
    }
  done < "$UNIT_STATE_FILE"
}

assert_old_quiet() {
  local i
  for i in 1 2 3 4 5; do
    if unit_active "$OLD_SERVICE"; then
      echo "Old service restarted while migration guard is active: $OLD_SERVICE" >&2
      return 1
    fi
    if pgrep -af '/usr/local/x-ui/(x-ui|bin/xray)' >/dev/null 2>&1; then
      echo "Detected a live X-Panel/Xray process after shutdown. A non-systemd watchdog may be restarting it:" >&2
      pgrep -af '/usr/local/x-ui/(x-ui|bin/xray)' >&2 || true
      return 1
    fi
    sleep 1
  done
}

restore_unit_states() {
  local unit enabled active
  [[ -f "$UNIT_STATE_FILE" ]] || return 0
  while IFS=$'\t' read -r unit enabled active; do
    [[ -n "$unit" ]] || continue
    case "$enabled" in
      masked|masked-runtime)
        systemctl mask "$unit" >/dev/null 2>&1 || true
        ;;
      *)
        systemctl unmask "$unit" >/dev/null 2>&1 || true
        case "$enabled" in
          enabled|enabled-runtime|linked|linked-runtime|alias)
            systemctl enable "$unit" >/dev/null 2>&1 || true
            ;;
          disabled)
            systemctl disable "$unit" >/dev/null 2>&1 || true
            ;;
        esac
        ;;
    esac
  done < "$UNIT_STATE_FILE"
  systemctl daemon-reload >/dev/null 2>&1 || true
  while IFS=$'\t' read -r unit enabled active; do
    [[ -n "$unit" ]] || continue
    if [[ "$active" == "active" ]]; then
      systemctl start "$unit" >/dev/null 2>&1 || true
    fi
  done < "$UNIT_STATE_FILE"
}

restore_xport_snapshot() {
  systemctl stop xport.service xport-xray.service >/dev/null 2>&1 || true
  rm -f "$XPORT_DB" "$XPORT_DB-wal" "$XPORT_DB-shm"
  if [[ $XPORT_DB_EXISTED -eq 1 ]]; then
    cp -a "$BACKUP/xport-before.db" "$XPORT_DB"
    [[ -f "$BACKUP/xport-before.db-wal" ]] && cp -a "$BACKUP/xport-before.db-wal" "$XPORT_DB-wal"
    [[ -f "$BACKUP/xport-before.db-shm" ]] && cp -a "$BACKUP/xport-before.db-shm" "$XPORT_DB-shm"
  fi
  rm -f "$DEST_CONFIG"
  if [[ $XPORT_CONFIG_EXISTED -eq 1 ]]; then
    cp -a "$BACKUP/xport-config-before.json" "$DEST_CONFIG"
  fi
}

verify_rollback() {
  if [[ $OLD_WAS_ACTIVE -eq 1 ]]; then
    for _ in 1 2 3 4 5; do
      unit_active "$OLD_SERVICE" && return 0
      sleep 1
    done
    echo "Rollback completed but $OLD_SERVICE did not become active." >&2
    return 1
  fi
  return 0
}

rollback() {
  local rc="${1:-1}"
  [[ $ROLLBACK_STARTED -eq 0 ]] || exit "$rc"
  ROLLBACK_STARTED=1
  trap - ERR INT TERM
  echo "Migration failed; restoring pre-migration state..." >&2
  restore_xport_snapshot
  restore_unit_states
  if [[ $XPORT_PANEL_WAS_ACTIVE -eq 1 ]]; then systemctl start xport.service >/dev/null 2>&1 || true; fi
  if [[ $XPORT_XRAY_WAS_ACTIVE -eq 1 ]]; then systemctl start xport-xray.service >/dev/null 2>&1 || true; fi
  if verify_rollback; then echo "Rollback verified: old panel state restored." >&2; else echo "WARNING: automatic rollback needs manual attention." >&2; fi
  exit "$rc"
}

rollback_on_error() { local rc=$?; [[ $CUTOVER_OK -eq 1 ]] || rollback "$rc"; exit "$rc"; }
trap rollback_on_error ERR
trap 'rollback 130' INT TERM

echo "=== X-port migration dry-run ==="
"$XPORT_BIN" migrate --data "$DATA_DIR" --from "$SOURCE_DB" "${MIGRATE_GLOBAL_ARGS[@]}"
echo
read -r -p "Continue with backup and guarded cutover? [y/N] " answer
[[ "$answer" =~ ^[Yy]$ ]] || { echo "Cancelled. Nothing changed."; exit 0; }

install -d -m 0750 "$BACKUP" "$DATA_DIR/xray"
unit_active "$OLD_SERVICE" && OLD_WAS_ACTIVE=1 || true
unit_active xport.service && XPORT_PANEL_WAS_ACTIVE=1 || true
unit_active xport-xray.service && XPORT_XRAY_WAS_ACTIVE=1 || true
[[ -f "$XPORT_DB" ]] && XPORT_DB_EXISTED=1 || true
[[ -f "$DEST_CONFIG" ]] && XPORT_CONFIG_EXISTED=1 || true
record_unit_states

systemctl stop xport.service xport-xray.service >/dev/null 2>&1 || true
[[ -f "$XPORT_DB" ]] && cp -a "$XPORT_DB" "$BACKUP/xport-before.db"
[[ -f "$XPORT_DB-wal" ]] && cp -a "$XPORT_DB-wal" "$BACKUP/xport-before.db-wal"
[[ -f "$XPORT_DB-shm" ]] && cp -a "$XPORT_DB-shm" "$BACKUP/xport-before.db-shm"
[[ -f "$DEST_CONFIG" ]] && cp -a "$DEST_CONFIG" "$BACKUP/xport-config-before.json"

quiesce_old_stack
assert_old_quiet

cp -a "$SOURCE_DB" "$BACKUP/"
[[ -f "$SOURCE_DB-wal" ]] && cp -a "$SOURCE_DB-wal" "$BACKUP/"
[[ -f "$SOURCE_DB-shm" ]] && cp -a "$SOURCE_DB-shm" "$BACKUP/"
[[ -f "$SOURCE_CONFIG" ]] && cp -a "$SOURCE_CONFIG" "$BACKUP/xray-config.json"

"$XPORT_BIN" migrate --data "$DATA_DIR" --from "$SOURCE_DB" "${MIGRATE_GLOBAL_ARGS[@]}" --apply
"$XPORT_BIN" render --data "$DATA_DIR" --output "$DEST_CONFIG"
"$XRAY_BIN" run -test -config "$DEST_CONFIG"
assert_old_quiet

systemctl restart xport-xray.service
sleep 1
systemctl is-active --quiet xport-xray.service
systemctl restart xport.service
systemctl is-active --quiet xport.service

CUTOVER_OK=1
trap - ERR INT TERM

cat > "$BACKUP/rollback.sh" <<ROLLBACK
#!/usr/bin/env bash
set -Eeuo pipefail
BACKUP='$BACKUP'
DEST_CONFIG='$DEST_CONFIG'
XPORT_DB='$XPORT_DB'
UNIT_STATE_FILE='$UNIT_STATE_FILE'
OLD_SERVICE='$OLD_SERVICE'
XPORT_DB_EXISTED='$XPORT_DB_EXISTED'
XPORT_CONFIG_EXISTED='$XPORT_CONFIG_EXISTED'
XPORT_PANEL_WAS_ACTIVE='$XPORT_PANEL_WAS_ACTIVE'
XPORT_XRAY_WAS_ACTIVE='$XPORT_XRAY_WAS_ACTIVE'
OLD_WAS_ACTIVE='$OLD_WAS_ACTIVE'

systemctl stop xport.service xport-xray.service >/dev/null 2>&1 || true
rm -f "\$XPORT_DB" "\$XPORT_DB-wal" "\$XPORT_DB-shm"
if [[ \$XPORT_DB_EXISTED -eq 1 ]]; then
  cp -a "\$BACKUP/xport-before.db" "\$XPORT_DB"
  [[ -f "\$BACKUP/xport-before.db-wal" ]] && cp -a "\$BACKUP/xport-before.db-wal" "\$XPORT_DB-wal"
  [[ -f "\$BACKUP/xport-before.db-shm" ]] && cp -a "\$BACKUP/xport-before.db-shm" "\$XPORT_DB-shm"
fi
rm -f "\$DEST_CONFIG"
[[ \$XPORT_CONFIG_EXISTED -eq 1 ]] && cp -a "\$BACKUP/xport-config-before.json" "\$DEST_CONFIG"

while IFS=\$'\t' read -r unit enabled active; do
  [[ -n "\$unit" ]] || continue
  case "\$enabled" in
    masked|masked-runtime) systemctl mask "\$unit" >/dev/null 2>&1 || true ;;
    *)
      systemctl unmask "\$unit" >/dev/null 2>&1 || true
      case "\$enabled" in
        enabled|enabled-runtime|linked|linked-runtime|alias) systemctl enable "\$unit" >/dev/null 2>&1 || true ;;
        disabled) systemctl disable "\$unit" >/dev/null 2>&1 || true ;;
      esac
      ;;
  esac
done < "\$UNIT_STATE_FILE"
systemctl daemon-reload >/dev/null 2>&1 || true
while IFS=\$'\t' read -r unit enabled active; do
  [[ "\$active" == active ]] && systemctl start "\$unit" >/dev/null 2>&1 || true
done < "\$UNIT_STATE_FILE"

[[ \$XPORT_PANEL_WAS_ACTIVE -eq 1 ]] && systemctl start xport.service >/dev/null 2>&1 || true
[[ \$XPORT_XRAY_WAS_ACTIVE -eq 1 ]] && systemctl start xport-xray.service >/dev/null 2>&1 || true

if [[ \$OLD_WAS_ACTIVE -eq 1 ]]; then
  for _ in 1 2 3 4 5; do
    systemctl is-active --quiet "\$OLD_SERVICE" && { echo "Rollback verified: old panel is active."; exit 0; }
    sleep 1
  done
  echo "Rollback restored files/units, but old panel is not active." >&2
  exit 1
fi

echo "Rollback complete."
ROLLBACK
chmod 0700 "$BACKUP/rollback.sh"

echo "Migration complete. Backup: $BACKUP"
echo "Old X-Panel files were not deleted. Related old systemd units remain masked to prevent watchdog restart."
echo "Snapshot rollback: $BACKUP/rollback.sh"
