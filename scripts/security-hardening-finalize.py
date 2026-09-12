#!/usr/bin/env python3
from pathlib import Path


def read(path):
    return Path(path).read_text(encoding="utf-8")


def write(path, text):
    Path(path).write_text(text, encoding="utf-8")


def replace(path, old, new, count=1):
    text = read(path)
    if text.count(old) < count:
        raise SystemExit(f"{path}: expected patch marker not found")
    write(path, text.replace(old, new, count))


def replace_between(path, start, end, replacement):
    text = read(path)
    a = text.find(start)
    b = text.find(end, a + 1)
    if a < 0 or b < 0:
        raise SystemExit(f"{path}: block markers not found")
    write(path, text[:a] + replacement.rstrip() + "\n\n" + text[b:])


# Eliminate dynamic SQL construction entirely. The importer now uses one fixed
# SELECT and maps the returned SQLite columns in Go, so schema inspection can
# never become a SQL text injection primitive.
read_xui = r'''func ReadXUI(path string) (Result, error) {
	abs, err := filepath.Abs(path)
	if err != nil { return Result{}, err }
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(abs)+"?mode=ro")
	if err != nil { return Result{}, err }
	defer db.Close()

	cols, err := inboundColumns(db)
	if err != nil { return Result{}, err }
	for _, required := range []string{"port", "protocol", "settings"} {
		if !cols[required] { return Result{}, fmt.Errorf("unsupported x-ui database: inbounds.%s is missing", required) }
	}

	rows, err := db.Query(`SELECT rowid AS __xport_rowid, * FROM inbounds ORDER BY port`)
	if err != nil { return Result{}, fmt.Errorf("read x-ui inbounds: %w", err) }
	defer rows.Close()
	columnNames, err := rows.Columns()
	if err != nil { return Result{}, err }
	columnIndex := make(map[string]int, len(columnNames))
	for i, name := range columnNames { columnIndex[strings.ToLower(name)] = i }

	value := func(values []any, name string) any {
		if i, ok := columnIndex[name]; ok && i >= 0 && i < len(values) { return values[i] }
		return nil
	}
	res := Result{}
	ports := map[int]bool{}
	for rows.Next() {
		values := make([]any, len(columnNames))
		dest := make([]any, len(values))
		for i := range values { dest[i] = &values[i] }
		if err := rows.Scan(dest...); err != nil { return Result{}, err }

		id := sqliteInt64(value(values, "id"), sqliteInt64(value(values, "__xport_rowid"), 0))
		in := sourceInbound{
			ID: id,
			Up: sqliteInt64(value(values, "up"), 0),
			Down: sqliteInt64(value(values, "down"), 0),
			Total: sqliteInt64(value(values, "total"), 0),
			AllTime: sqliteInt64(value(values, "all_time"), 0),
			Remark: sqliteString(value(values, "remark"), ""),
			Enable: sqliteBool(value(values, "enable"), true),
			ExpiryTime: sqliteInt64(value(values, "expiry_time"), 0),
			Listen: sqliteString(value(values, "listen"), ""),
			Port: int(sqliteInt64(value(values, "port"), 0)),
			Protocol: sqliteString(value(values, "protocol"), ""),
			Settings: sqliteString(value(values, "settings"), "{}"),
			StreamSettings: sqliteString(value(values, "stream_settings"), "{}"),
			Tag: sqliteString(value(values, "tag"), ""),
			Sniffing: sqliteString(value(values, "sniffing"), "{}"),
		}
		if in.Port < 1 || in.Port > 65535 {
			res.Skipped = append(res.Skipped, fmt.Sprintf("inbound %d: invalid port %d", in.ID, in.Port))
			continue
		}
		if ports[in.Port] { return Result{}, fmt.Errorf("duplicate source port %d", in.Port) }
		ports[in.Port] = true
		hints, count, err := extractCredentialHints(in.Protocol, in.Settings)
		if err != nil {
			res.Skipped = append(res.Skipped, fmt.Sprintf("port %d: invalid settings: %v", in.Port, err))
			continue
		}
		if reason := credentialCountProblem(in.Protocol, count); reason != "" {
			res.Skipped = append(res.Skipped, fmt.Sprintf("port %d: %s", in.Port, reason))
			continue
		}
		name := strings.TrimSpace(in.Remark)
		quota, expiry := in.Total, in.ExpiryTime
		if len(hints) == 1 {
			if name == "" { name = strings.TrimSpace(hints[0].Email); if name == "" { name = strings.TrimSpace(hints[0].User) } }
			if hints[0].TotalGB > 0 { quota = hints[0].TotalGB }
			if hints[0].ExpiryTime > 0 { expiry = hints[0].ExpiryTime }
		}
		if name == "" { name = fmt.Sprintf("account-%d", in.Port) }
		tag := strings.TrimSpace(in.Tag)
		if tag == "" { tag = fmt.Sprintf("xport-%d", in.Port) }
		reason := ""
		if !in.Enable { reason = "manual" }
		res.Accounts = append(res.Accounts, model.Account{
			Name: name, Enabled: in.Enable, DisabledReason: reason, Listen: in.Listen,
			Port: in.Port, Protocol: strings.ToLower(in.Protocol), SettingsJSON: normalizeJSON(in.Settings, "{}"),
			StreamSettingsJSON: normalizeJSON(in.StreamSettings, "{}"), SniffingJSON: normalizeJSON(in.Sniffing, "{}"),
			Tag: tag, UpBytes: in.Up, DownBytes: in.Down, QuotaBytes: quota, AllTimeBytes: in.AllTime, ExpiryTime: expiry,
		})
	}
	if err := rows.Err(); err != nil { return Result{}, err }
	admins, warnings, err := readAdmins(db); if err != nil { return Result{}, err }; res.Admins = admins; res.Warnings = append(res.Warnings, warnings...)
	panelListen, warnings, err := readPanelListen(db); if err != nil { return Result{}, err }; res.PanelListen = panelListen; res.Warnings = append(res.Warnings, warnings...)
	panel, warnings, err := readPanelSettings(db); if err != nil { return Result{}, err }; res.PanelBasePath = panel.BasePath; res.PanelCertFile = panel.CertFile; res.PanelKeyFile = panel.KeyFile; res.PanelDomain = panel.Domain; res.Warnings = append(res.Warnings, warnings...)
	if len(res.Accounts) == 0 { res.Warnings = append(res.Warnings, "no compatible one-account inbounds found") }
	return res, nil
}

func sqliteString(v any, fallback string) string {
	switch x := v.(type) {
	case string: return x
	case []byte: return string(x)
	case nil: return fallback
	default: return fmt.Sprint(x)
	}
}

func sqliteInt64(v any, fallback int64) int64 {
	switch x := v.(type) {
	case int64: return x
	case int: return int64(x)
	case float64: return int64(x)
	case string: if n, err := strconv.ParseInt(strings.TrimSpace(x), 10, 64); err == nil { return n }
	case []byte: if n, err := strconv.ParseInt(strings.TrimSpace(string(x)), 10, 64); err == nil { return n }
	}
	return fallback
}

func sqliteBool(v any, fallback bool) bool {
	switch x := v.(type) {
	case bool: return x
	case int64: return x != 0
	case int: return x != 0
	case float64: return x != 0
	case string:
		x = strings.TrimSpace(strings.ToLower(x)); if x == "1" || x == "true" { return true }; if x == "0" || x == "false" { return false }
	case []byte: return sqliteBool(string(x), fallback)
	}
	return fallback
}'''
replace_between("internal/migrate/xui.go", "func ReadXUI", "func extractCredentialHints", read_xui)

# Tighten the root service's filesystem boundary while retaining only the
# writable locations needed for database/config updates and signed/hashed
# X-port/Xray replacement. The CLI repair command runs outside this sandbox.
path = "scripts/install.sh"
replace(path, 'XPORT_BIN="${XPORT_BIN:-/usr/local/bin/xport}"\n', 'XPORT_BIN="${XPORT_BIN:-/usr/local/bin/xport}"\nXPORT_BIN_DIR="$(dirname "$XPORT_BIN")"\n')
replace(path, 'SystemCallArchitectures=native\n[Install]', 'SystemCallArchitectures=native\nProtectSystem=full\nRestrictAddressFamilies=AF_UNIX AF_INET AF_INET6\n[Install]', 1)
replace(path, 'SystemCallArchitectures=native\n[Install]', 'SystemCallArchitectures=native\nProtectSystem=strict\nReadWritePaths=$DATA_DIR $PREFIX $XPORT_BIN_DIR\nRestrictAddressFamilies=AF_UNIX AF_INET AF_INET6\n[Install]', 1)

path = "cmd/xport/manager.go"
replace(path, 'SystemCallArchitectures=native\n[Install]\nWantedBy=multi-user.target\n`\n\txr :=', 'SystemCallArchitectures=native\nProtectSystem=strict\nReadWritePaths=/etc/x-port /usr/local/x-port /usr/local/bin\nRestrictAddressFamilies=AF_UNIX AF_INET AF_INET6\n[Install]\nWantedBy=multi-user.target\n`\n\txr :=')
replace(path, 'SystemCallArchitectures=native\n[Install]\nWantedBy=multi-user.target\n`\n\tif err := os.WriteFile("/etc/systemd/system/xport.service"', 'SystemCallArchitectures=native\nProtectSystem=full\nRestrictAddressFamilies=AF_UNIX AF_INET AF_INET6\n[Install]\nWantedBy=multi-user.target\n`\n\tif err := os.WriteFile("/etc/systemd/system/xport.service"')

print("final security hardening deltas applied")
