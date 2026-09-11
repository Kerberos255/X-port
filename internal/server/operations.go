package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
	http.ServeContent(w, r, info.Name, timeFromMillis(info.CreatedAt), f)
}

func (s *Server) restoreBackup(w http.ResponseWriter, r *http.Request) {
	dir := s.backupDir()
	if dir == "" {
		writeError(w, 503, "backup directory is not configured")
		return
	}
	// Always take a fresh recovery point before a restore operation.
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
	writeJSON(w, 200, map[string]any{"ok": true, "loginRequired": true})
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
	XrayAPIPort        int    `json:"xrayApiPort"`
	PortMin            int    `json:"portMin"`
	PortMax            int    `json:"portMax"`
	DefaultRealitySNI  string `json:"defaultRealitySni"`
	DefaultRealityDest string `json:"defaultRealityDest"`
	AdminUsername      string `json:"adminUsername"`
}

type settingsInput struct {
	PanelListen        string `json:"panelListen"`
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
	writeJSON(w, 200, settingsView{
		PanelListen: settingString(s.store, "panel_listen", "127.0.0.1:8080"),
		XrayAPIPort: settingIntServer(s.store, "xray_api_port", 10085),
		PortMin: settingIntServer(s.store, "port_min", 20000),
		PortMax: settingIntServer(s.store, "port_max", 60000),
		DefaultRealitySNI: settingString(s.store, "default_reality_sni", ""),
		DefaultRealityDest: settingString(s.store, "default_reality_dest", ""),
		AdminUsername: username,
	})
}

func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	var in settingsInput
	if decodeJSON(w, r, &in) != nil { return }
	currentUser, ok := s.sessionUsername(r)
	if !ok { writeError(w, 401, "unauthorized"); return }
	in.AdminUsername = strings.TrimSpace(in.AdminUsername)
	in.PanelListen = strings.TrimSpace(in.PanelListen)
	if in.AdminUsername == "" { writeError(w, 400, "admin username is required"); return }
	if _, _, err := net.SplitHostPort(in.PanelListen); err != nil { writeError(w, 400, "panel listen must be host:port"); return }
	if in.XrayAPIPort < 1 || in.XrayAPIPort > 65535 { writeError(w, 400, "invalid Xray API port"); return }
	if in.PortMin < 1 || in.PortMax > 65535 || in.PortMax < in.PortMin { writeError(w, 400, "invalid account port range"); return }
	accounts, err := s.store.Accounts(); if err != nil { writeError(w,500,err.Error()); return }
	for _, a := range accounts { if a.Port == in.XrayAPIPort { writeError(w,400,fmt.Sprintf("Xray API port conflicts with account %q",a.Name)); return } }
	admin, err := s.store.Admin(currentUser); if err != nil { writeError(w,500,"admin lookup failed"); return }
	newHash := admin.PasswordHash
	adminChanging := in.AdminUsername != currentUser || strings.TrimSpace(in.NewPassword) != ""
	if adminChanging {
		if bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(in.CurrentPassword)) != nil { writeError(w,403,"current password is incorrect"); return }
		if in.NewPassword != "" {
			if len(in.NewPassword) < 12 { writeError(w,400,"new password must be at least 12 characters"); return }
			h, err := bcrypt.GenerateFromPassword([]byte(in.NewPassword), bcrypt.DefaultCost); if err != nil { writeError(w,500,"password hashing failed"); return }; newHash=string(h)
		}
	}
	values := map[string]string{
		"panel_listen": in.PanelListen,
		"xray_api_port": strconv.Itoa(in.XrayAPIPort),
		"port_min": strconv.Itoa(in.PortMin),
		"port_max": strconv.Itoa(in.PortMax),
		"default_reality_sni": strings.TrimSpace(in.DefaultRealitySNI),
		"default_reality_dest": strings.TrimSpace(in.DefaultRealityDest),
	}
	if err := s.store.ApplySettings(values, currentUser, in.AdminUsername, newHash); err != nil { writeError(w,400,err.Error()); return }
	if in.AdminUsername != currentUser { s.sessions.RenameUser(currentUser,in.AdminUsername) }
	writeJSON(w,200,map[string]any{"ok":true,"restartRequired":true})
}

func (s *Server) sessionUsername(r *http.Request) (string, bool) {
	c, err := r.Cookie("xport_session")
	if err != nil { return "", false }
	return s.sessions.Username(c.Value)
}

func settingString(st interface{ Setting(string)(string,bool,error) }, key, fallback string) string {
	v, ok, err := st.Setting(key); if err != nil || !ok { return fallback }; return v
}
func settingIntServer(st interface{ Setting(string)(string,bool,error) }, key string, fallback int) int {
	v := settingString(st,key,""); if v=="" { return fallback }; n,err:=strconv.Atoi(v);if err!=nil{return fallback};return n
}

func timeFromMillis(ms int64) time.Time { return time.UnixMilli(ms) }

var _ = json.Valid
