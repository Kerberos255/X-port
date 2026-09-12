package server

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/Kerberos255/X-port/internal/backup"
	"github.com/Kerberos255/X-port/internal/ops"
	"github.com/Kerberos255/X-port/internal/xray"
)

type RuntimeOptions struct {
	DataDir      string
	XrayService  string
	PanelService string
	Manager      *xray.Manager
}

func (s *Server) WithRuntime(opts RuntimeOptions) *Server {
	s.dataDir = opts.DataDir
	s.xrayService = opts.XrayService
	s.panelService = opts.PanelService
	s.manager = opts.Manager
	return s
}

func (s *Server) resetTraffic(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, 400, "invalid account id")
		return
	}
	if err := s.accounts.ResetTraffic(id); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
	service := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("service")))
	unit := ""
	switch service {
	case "xray":
		unit = s.xrayService
	case "xport":
		unit = s.panelService
	default:
		writeError(w, 400, "service must be xray or xport")
		return
	}
	if unit == "" {
		writeError(w, 503, "service logging is not configured")
		return
	}
	lines, _ := strconv.Atoi(r.URL.Query().Get("lines"))
	text, err := ops.Journal(unit, lines)
	if err != nil {
		writeError(w, 502, err.Error())
		return
	}
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="xport-`+service+`-log.txt"`)
		_, _ = w.Write([]byte(text))
		return
	}
	writeJSON(w, 200, map[string]any{"service": service, "logs": text})
}

func (s *Server) restartXray(w http.ResponseWriter, r *http.Request) {
	if s.xrayService == "" {
		writeError(w, 503, "Xray service is not configured")
		return
	}
	if err := ops.RestartService(s.xrayService); err != nil {
		writeError(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
func (s *Server) backupDir() string {
	if strings.TrimSpace(s.dataDir) == "" {
		return ""
	}
	return filepath.Join(s.dataDir, "backups", "manual")
}
func (s *Server) listBackups(w http.ResponseWriter, r *http.Request) {
	dir := s.backupDir()
	if dir == "" {
		writeError(w, 503, "backup directory is not configured")
		return
	}
	items, err := backup.List(dir)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"backups": items})
}
func (s *Server) createBackup(w http.ResponseWriter, r *http.Request) {
	dir := s.backupDir()
	if dir == "" {
		writeError(w, 503, "backup directory is not configured")
		return
	}
	snapshot, err := s.store.Snapshot()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	info, err := backup.Create(dir, snapshot)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, info)
}
func (s *Server) downloadBackup(w http.ResponseWriter, r *http.Request) {
	dir := s.backupDir()
	if dir == "" {
		writeError(w, 503, "backup directory is not configured")
		return
	}
	name := r.PathValue("name")
	f, info, err := backup.Open(dir, name)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+info.Name+`"`)
	http.ServeContent(w, r, info.Name, time.UnixMilli(info.CreatedAt), f)
}
func (s *Server) restoreBackup(w http.ResponseWriter, r *http.Request) {
	dir := s.backupDir()
	if dir == "" {
		writeError(w, 503, "backup directory is not configured")
		return
	}
	current, err := s.store.Snapshot()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	if _, err := backup.Create(dir, current); err != nil {
		writeError(w, 500, "pre-restore backup failed: "+err.Error())
		return
	}
	snapshot, err := backup.Load(dir, r.PathValue("name"))
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if err := s.accounts.RestoreSnapshot(snapshot); err != nil {
		writeError(w, 502, err.Error())
		return
	}
	s.sessions.Clear()
	writeJSON(w, 200, map[string]any{"ok": true, "loginRequired": true, "restartRecommended": true})
}
func (s *Server) deleteBackup(w http.ResponseWriter, r *http.Request) {
	dir := s.backupDir()
	if dir == "" {
		writeError(w, 503, "backup directory is not configured")
		return
	}
	if err := backup.Delete(dir, r.PathValue("name")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, 404, "backup not found")
		} else {
			writeError(w, 400, err.Error())
		}
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

type settingsView struct {
	PanelListen        string `json:"panelListen"`
	PanelBasePath      string `json:"panelBasePath"`
	PanelCertFile      string `json:"panelCertFile"`
	PanelKeyFile       string `json:"panelKeyFile"`
	PanelDomain        string `json:"panelDomain"`
	PanelTLS           bool   `json:"panelTLS"`
	XrayAPIPort        int    `json:"xrayApiPort"`
	PortMin            int    `json:"portMin"`
	PortMax            int    `json:"portMax"`
	DefaultRealitySNI  string `json:"defaultRealitySni"`
	DefaultRealityDest string `json:"defaultRealityDest"`
	AdminUsername      string `json:"adminUsername"`
}
type settingsInput struct {
	PanelListen        string `json:"panelListen"`
	PanelBasePath      string `json:"panelBasePath"`
	PanelCertFile      string `json:"panelCertFile"`
	PanelKeyFile       string `json:"panelKeyFile"`
	PanelDomain        string `json:"panelDomain"`
	XrayAPIPort        int    `json:"xrayApiPort"`
	PortMin            int    `json:"portMin"`
	PortMax            int    `json:"portMax"`
	DefaultRealitySNI  string `json:"defaultRealitySni"`
	DefaultRealityDest string `json:"defaultRealityDest"`
	AdminUsername      string `json:"adminUsername"`
	CurrentPassword    string `json:"currentPassword"`
	NewPassword        string `json:"newPassword"`
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	username, ok := s.sessionUsername(r)
	if !ok {
		writeError(w, 401, "unauthorized")
		return
	}
	cert := settingString(s.store, "panel_cert_file", "")
	key := settingString(s.store, "panel_key_file", "")
	writeJSON(w, 200, settingsView{PanelListen: settingString(s.store, "panel_listen", "127.0.0.1:8080"), PanelBasePath: normalizePanelBasePathSetting(settingString(s.store, "panel_base_path", "/")), PanelCertFile: cert, PanelKeyFile: key, PanelDomain: settingString(s.store, "panel_domain", ""), PanelTLS: cert != "" && key != "", XrayAPIPort: settingIntServer(s.store, "xray_api_port", 10085), PortMin: settingIntServer(s.store, "port_min", 20000), PortMax: settingIntServer(s.store, "port_max", 60000), DefaultRealitySNI: settingString(s.store, "default_reality_sni", ""), DefaultRealityDest: settingString(s.store, "default_reality_dest", ""), AdminUsername: username})
}

func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	var in settingsInput
	if decodeJSON(w, r, &in) != nil {
		return
	}
	currentUser, ok := s.sessionUsername(r)
	if !ok {
		writeError(w, 401, "unauthorized")
		return
	}
	in.AdminUsername = strings.TrimSpace(in.AdminUsername)
	in.PanelListen = strings.TrimSpace(in.PanelListen)
	in.PanelBasePath = normalizePanelBasePathSetting(in.PanelBasePath)
	in.PanelCertFile = strings.TrimSpace(in.PanelCertFile)
	in.PanelKeyFile = strings.TrimSpace(in.PanelKeyFile)
	in.PanelDomain = strings.TrimSpace(in.PanelDomain)
	if err := validateSettingsInput(in); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if in.AdminUsername == "" {
		writeError(w, 400, "admin username is required")
		return
	}
	if _, _, err := net.SplitHostPort(in.PanelListen); err != nil {
		writeError(w, 400, "panel listen must be host:port")
		return
	}
	if !validPanelBasePath(in.PanelBasePath) {
		writeError(w, 400, "invalid panel base path")
		return
	}
	if (in.PanelCertFile == "") != (in.PanelKeyFile == "") {
		writeError(w, 400, "panel certificate and private key must be configured together")
		return
	}
	if in.PanelCertFile != "" {
		if !filepath.IsAbs(in.PanelCertFile) || !filepath.IsAbs(in.PanelKeyFile) {
			writeError(w, 400, "panel certificate paths must be absolute")
			return
		}
		if _, err := tls.LoadX509KeyPair(in.PanelCertFile, in.PanelKeyFile); err != nil {
			writeError(w, 400, "panel TLS certificate/key cannot be loaded: "+err.Error())
			return
		}
	}
	if in.XrayAPIPort < 1 || in.XrayAPIPort > 65535 {
		writeError(w, 400, "invalid Xray API port")
		return
	}
	if in.PortMin < 1 || in.PortMax > 65535 || in.PortMax < in.PortMin {
		writeError(w, 400, "invalid account port range")
		return
	}
	accounts, err := s.store.Accounts()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	for _, a := range accounts {
		if a.Port == in.XrayAPIPort {
			writeError(w, 400, fmt.Sprintf("Xray API port conflicts with account %q", a.Name))
			return
		}
	}
	admin, err := s.store.Admin(currentUser)
	if err != nil {
		writeError(w, 500, "admin lookup failed")
		return
	}
	newHash := admin.PasswordHash
	adminChanging := in.AdminUsername != currentUser || strings.TrimSpace(in.NewPassword) != ""
	if adminChanging {
		if bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(in.CurrentPassword)) != nil {
			writeError(w, 403, "current password is incorrect")
			return
		}
		if in.NewPassword != "" {
			if len(in.NewPassword) < 12 {
				writeError(w, 400, "new password must be at least 12 characters")
				return
			}
			h, err := bcrypt.GenerateFromPassword([]byte(in.NewPassword), bcrypt.DefaultCost)
			if err != nil {
				writeError(w, 500, "password hashing failed")
				return
			}
			newHash = string(h)
		}
	}
	oldPanel := settingString(s.store, "panel_listen", "127.0.0.1:8080")
	oldBase := normalizePanelBasePathSetting(settingString(s.store, "panel_base_path", "/"))
	oldCert := settingString(s.store, "panel_cert_file", "")
	oldKey := settingString(s.store, "panel_key_file", "")
	oldAPI := settingIntServer(s.store, "xray_api_port", 10085)
	apiChanged := oldAPI != in.XrayAPIPort
	if apiChanged {
		if s.manager == nil {
			writeError(w, 503, "Xray manager is not configured")
			return
		}
		s.manager.APIPort = in.XrayAPIPort
		if err := s.manager.Apply(accounts); err != nil {
			s.manager.APIPort = oldAPI
			writeError(w, 502, "Xray API port change failed: "+err.Error())
			return
		}
	}
	values := map[string]string{"panel_listen": in.PanelListen, "panel_base_path": in.PanelBasePath, "panel_cert_file": in.PanelCertFile, "panel_key_file": in.PanelKeyFile, "panel_domain": in.PanelDomain, "xray_api_port": strconv.Itoa(in.XrayAPIPort), "port_min": strconv.Itoa(in.PortMin), "port_max": strconv.Itoa(in.PortMax), "default_reality_sni": strings.TrimSpace(in.DefaultRealitySNI), "default_reality_dest": strings.TrimSpace(in.DefaultRealityDest)}
	if err := s.store.ApplySettings(values, currentUser, in.AdminUsername, newHash); err != nil {
		if apiChanged {
			s.manager.APIPort = oldAPI
			_ = s.manager.Apply(accounts)
		}
		writeError(w, 400, err.Error())
		return
	}
	if adminChanging {
		s.sessions.Clear()
		token, err := s.sessions.Create(in.AdminUsername)
		if err != nil {
			writeError(w, 500, "settings saved but session rotation failed; sign in again")
			return
		}
		s.setSessionCookie(w, r, token)
	}
	restartRequired := oldPanel != in.PanelListen || oldBase != in.PanelBasePath || oldCert != in.PanelCertFile || oldKey != in.PanelKeyFile
	writeJSON(w, 200, map[string]any{"ok": true, "restartRequired": restartRequired, "xrayApiApplied": apiChanged, "sessionRotated": adminChanging})
}

func (s *Server) sessionUsername(r *http.Request) (string, bool) {
	c, err := r.Cookie("xport_session")
	if err != nil {
		return "", false
	}
	return s.sessions.Username(c.Value)
}
func settingString(st interface {
	Setting(string) (string, bool, error)
}, key, fallback string) string {
	v, ok, err := st.Setting(key)
	if err != nil || !ok {
		return fallback
	}
	return v
}
func settingIntServer(st interface {
	Setting(string) (string, bool, error)
}, key string, fallback int) int {
	v := settingString(st, key, "")
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
func normalizePanelBasePathSetting(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || v == "/" {
		return "/"
	}
	return "/" + strings.Trim(v, "/") + "/"
}
func validPanelBasePath(v string) bool {
	if v == "/" {
		return true
	}
	if !strings.HasPrefix(v, "/") || !strings.HasSuffix(v, "/") {
		return false
	}
	if strings.ContainsAny(v, "?#\\\t\r\n ") {
		return false
	}
	for _, part := range strings.Split(strings.Trim(v, "/"), "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func validateSettingsInput(in settingsInput) error {
	checks := []struct {
		name, value string
		max         int
	}{
		{"admin username", in.AdminUsername, 128}, {"panel listen", in.PanelListen, 256},
		{"panel base path", in.PanelBasePath, 256}, {"panel certificate path", in.PanelCertFile, 4096},
		{"panel private key path", in.PanelKeyFile, 4096}, {"panel domain", in.PanelDomain, 253},
		{"default REALITY SNI", in.DefaultRealitySNI, 512}, {"default REALITY target", in.DefaultRealityDest, 1024},
		{"current password", in.CurrentPassword, 4096}, {"new password", in.NewPassword, 4096},
	}
	for _, c := range checks {
		if len(c.value) > c.max {
			return fmt.Errorf("%s is too long", c.name)
		}
		if strings.ContainsRune(c.value, '\x00') || strings.ContainsAny(c.value, "\r\n") {
			return fmt.Errorf("%s contains invalid control characters", c.name)
		}
	}
	if in.PanelDomain != "" && strings.ContainsAny(in.PanelDomain, "/\\?#@ \t") {
		return errors.New("invalid panel domain")
	}
	return nil
}
