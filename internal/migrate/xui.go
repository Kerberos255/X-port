package migrate

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"

	"github.com/Kerberos255/X-port/internal/model"
)

type Result struct {
	Accounts    []model.Account `json:"accounts"`
	Admins      []model.Admin   `json:"-"`
	PanelListen string          `json:"panelListen,omitempty"`
	Warnings    []string        `json:"warnings"`
	Skipped     []string        `json:"skipped"`
}
type sourceInbound struct {
	ID                                                int64
	Up, Down, Total, AllTime                          int64
	Remark                                            string
	Enable                                            bool
	ExpiryTime                                        int64
	Listen                                            string
	Port                                              int
	Protocol, Settings, StreamSettings, Tag, Sniffing string
}
type clientHint struct {
	ID         string `json:"id"`
	Email      string `json:"email"`
	Password   string `json:"password"`
	TotalGB    int64  `json:"totalGB"`
	ExpiryTime int64  `json:"expiryTime"`
	Enable     *bool  `json:"enable"`
}

func ReadXUI(path string) (Result, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Result{}, err
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(abs)+"?mode=ro")
	if err != nil {
		return Result{}, err
	}
	defer db.Close()
	cols, err := inboundColumns(db)
	if err != nil {
		return Result{}, err
	}
	for _, r := range []string{"port", "protocol", "settings"} {
		if !cols[r] {
			return Result{}, fmt.Errorf("unsupported x-ui database: inbounds.%s is missing", r)
		}
	}
	column := func(name, fallback string) string {
		if cols[name] {
			return name
		}
		return fallback
	}
	query := fmt.Sprintf(`SELECT %s,%s,%s,%s,%s,%s,%s,%s,%s,port,protocol,settings,%s,%s,%s FROM inbounds ORDER BY port`, column("id", "rowid"), column("up", "0"), column("down", "0"), column("total", "0"), column("all_time", "0"), column("remark", "''"), column("enable", "1"), column("expiry_time", "0"), column("listen", "''"), column("stream_settings", "'{}'"), column("tag", "''"), column("sniffing", "'{}'"))
	rows, err := db.Query(query)
	if err != nil {
		return Result{}, fmt.Errorf("read x-ui inbounds: %w", err)
	}
	res := Result{}
	ports := map[int]bool{}
	for rows.Next() {
		var in sourceInbound
		if err := rows.Scan(&in.ID, &in.Up, &in.Down, &in.Total, &in.AllTime, &in.Remark, &in.Enable, &in.ExpiryTime, &in.Listen, &in.Port, &in.Protocol, &in.Settings, &in.StreamSettings, &in.Tag, &in.Sniffing); err != nil {
			rows.Close()
			return Result{}, err
		}
		if in.Port < 1 || in.Port > 65535 {
			res.Skipped = append(res.Skipped, fmt.Sprintf("inbound %d: invalid port %d", in.ID, in.Port))
			continue
		}
		if ports[in.Port] {
			rows.Close()
			return Result{}, fmt.Errorf("duplicate source port %d", in.Port)
		}
		ports[in.Port] = true
		clients, err := extractClients(in.Settings)
		if err != nil {
			res.Skipped = append(res.Skipped, fmt.Sprintf("port %d: invalid settings: %v", in.Port, err))
			continue
		}
		if protocolNeedsClient(in.Protocol) && len(clients) != 1 {
			res.Skipped = append(res.Skipped, fmt.Sprintf("port %d: %s inbound has %d clients; X-port requires exactly one", in.Port, in.Protocol, len(clients)))
			continue
		}
		name := strings.TrimSpace(in.Remark)
		quota, expiry := in.Total, in.ExpiryTime
		if len(clients) == 1 {
			if name == "" {
				name = strings.TrimSpace(clients[0].Email)
			}
			if clients[0].TotalGB > 0 {
				quota = clients[0].TotalGB
			}
			if clients[0].ExpiryTime > 0 {
				expiry = clients[0].ExpiryTime
			}
		}
		if name == "" {
			name = fmt.Sprintf("account-%d", in.Port)
		}
		tag := strings.TrimSpace(in.Tag)
		if tag == "" {
			tag = fmt.Sprintf("xport-%d", in.Port)
		}
		res.Accounts = append(res.Accounts, model.Account{Name: name, Enabled: in.Enable, Listen: in.Listen, Port: in.Port, Protocol: in.Protocol, SettingsJSON: normalizeJSON(in.Settings, "{}"), StreamSettingsJSON: normalizeJSON(in.StreamSettings, "{}"), SniffingJSON: normalizeJSON(in.Sniffing, "{}"), Tag: tag, UpBytes: in.Up, DownBytes: in.Down, QuotaBytes: quota, AllTimeBytes: in.AllTime, ExpiryTime: expiry})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Result{}, err
	}
	_ = rows.Close()

	admins, warnings, err := readAdmins(db)
	if err != nil {
		return Result{}, err
	}
	res.Admins = admins
	res.Warnings = append(res.Warnings, warnings...)
	panelListen, warnings, err := readPanelListen(db)
	if err != nil {
		return Result{}, err
	}
	res.PanelListen = panelListen
	res.Warnings = append(res.Warnings, warnings...)
	if len(res.Accounts) == 0 {
		res.Warnings = append(res.Warnings, "no compatible one-client inbounds found")
	}
	return res, nil
}

func readAdmins(db *sql.DB) ([]model.Admin, []string, error) {
	exists, err := tableExists(db, "users")
	if err != nil {
		return nil, nil, err
	}
	if !exists {
		return nil, []string{"users table not found; X-port bootstrap login will be kept"}, nil
	}
	rows, err := db.Query(`SELECT username,password FROM users ORDER BY id`)
	if err != nil {
		return nil, nil, fmt.Errorf("read x-ui users: %w", err)
	}
	defer rows.Close()
	admins := make([]model.Admin, 0)
	warnings := make([]string, 0)
	for rows.Next() {
		var username, hash string
		if err := rows.Scan(&username, &hash); err != nil {
			return nil, nil, err
		}
		username = strings.TrimSpace(username)
		hash = strings.TrimSpace(hash)
		if username == "" || hash == "" {
			warnings = append(warnings, "ignored an empty X-Panel admin record")
			continue
		}
		if _, err := bcrypt.Cost([]byte(hash)); err != nil {
			warnings = append(warnings, fmt.Sprintf("admin %q uses an unsupported password format; bootstrap X-port login will be kept unless another valid admin exists", username))
			continue
		}
		admins = append(admins, model.Admin{Username: username, PasswordHash: hash})
	}
	return admins, warnings, rows.Err()
}

func readPanelListen(db *sql.DB) (string, []string, error) {
	exists, err := tableExists(db, "settings")
	if err != nil {
		return "", nil, err
	}
	if !exists {
		return "", []string{"settings table not found; X-port bootstrap panel port will be kept"}, nil
	}
	rows, err := db.Query(`SELECT key,value FROM settings WHERE key IN ('webListen','webPort')`)
	if err != nil {
		return "", nil, fmt.Errorf("read x-ui panel settings: %w", err)
	}
	defer rows.Close()
	values := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return "", nil, err
		}
		values[key] = strings.TrimSpace(value)
	}
	if err := rows.Err(); err != nil {
		return "", nil, err
	}
	portText := values["webPort"]
	if portText == "" {
		return "", []string{"webPort not found; X-port bootstrap panel port will be kept"}, nil
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return "", []string{fmt.Sprintf("invalid X-Panel webPort %q; X-port bootstrap panel port will be kept", portText)}, nil
	}
	host := strings.Trim(values["webListen"], "[]")
	return net.JoinHostPort(host, strconv.Itoa(port)), nil, nil
}

func tableExists(db *sql.DB, name string) (bool, error) {
	var found string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&found)
	if err == nil {
		return true, nil
	}
	if err == sql.ErrNoRows {
		return false, nil
	}
	return false, err
}

func inboundColumns(db *sql.DB) (map[string]bool, error) {
	rows, err := db.Query(`PRAGMA table_info(inbounds)`)
	if err != nil {
		return nil, fmt.Errorf("inspect x-ui schema: %w", err)
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt any
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols[strings.ToLower(name)] = true
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("unsupported x-ui database: inbounds table not found")
	}
	return cols, rows.Err()
}
func extractClients(raw string) ([]clientHint, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var obj struct {
		Clients []clientHint `json:"clients"`
	}
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return nil, err
	}
	return obj.Clients, nil
}
func protocolNeedsClient(p string) bool {
	switch strings.ToLower(p) {
	case "vless", "vmess", "trojan", "shadowsocks", "socks", "http":
		return true
	default:
		return false
	}
}
func normalizeJSON(raw, fallback string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	var v any
	if json.Unmarshal([]byte(raw), &v) != nil {
		return fallback
	}
	b, _ := json.Marshal(v)
	return string(b)
}
