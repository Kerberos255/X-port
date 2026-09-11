package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"rsc.io/qr"

	"github.com/Kerberos255/X-port/internal/accountcfg"
	"github.com/Kerberos255/X-port/internal/auth"
	"github.com/Kerberos255/X-port/internal/service"
	"github.com/Kerberos255/X-port/internal/store"
	sysinfo "github.com/Kerberos255/X-port/internal/system"
	"github.com/Kerberos255/X-port/internal/xray"
)

type Server struct {
	store        *store.Store
	accounts     *service.Accounts
	sessions     *auth.Sessions
	loginLimiter *auth.LoginLimiter
	updater      *xray.Updater
	static       fs.FS
	dataDir      string
	xrayService  string
	panelService string
	manager      *xray.Manager
}

func New(st *store.Store, accounts *service.Accounts, updater *xray.Updater, static fs.FS) *Server {
	return &Server{
		store: st, accounts: accounts, updater: updater, static: static,
		sessions: auth.NewSessions(24 * time.Hour), loginLimiter: auth.NewLoginLimiter(8, 10*time.Minute),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/logout", s.logout)
	mux.Handle("GET /api/health", s.require(http.HandlerFunc(s.health)))
	mux.Handle("GET /api/overview", s.require(http.HandlerFunc(s.overview)))
	mux.Handle("GET /api/accounts", s.require(http.HandlerFunc(s.listAccounts)))
	mux.Handle("POST /api/accounts", s.require(http.HandlerFunc(s.createAccount)))
	mux.Handle("GET /api/accounts/{id}", s.require(http.HandlerFunc(s.getAccount)))
	mux.Handle("PUT /api/accounts/{id}", s.require(http.HandlerFunc(s.updateAccount)))
	mux.Handle("DELETE /api/accounts/{id}", s.require(http.HandlerFunc(s.deleteAccount)))
	mux.Handle("POST /api/accounts/{id}/clone", s.require(http.HandlerFunc(s.cloneAccount)))
	mux.Handle("POST /api/accounts/{id}/traffic/reset", s.require(http.HandlerFunc(s.resetTraffic)))
	mux.Handle("GET /api/accounts/{id}/share", s.require(http.HandlerFunc(s.shareAccount)))
	mux.Handle("GET /api/accounts/{id}/qr", s.require(http.HandlerFunc(s.qrAccount)))
	mux.Handle("GET /api/xray/update", s.require(http.HandlerFunc(s.checkXrayUpdate)))
	mux.Handle("POST /api/xray/update", s.require(http.HandlerFunc(s.updateXray)))
	mux.Handle("POST /api/xray/restart", s.require(http.HandlerFunc(s.restartXray)))
	mux.Handle("GET /api/logs", s.require(http.HandlerFunc(s.logs)))
	mux.Handle("GET /api/backups", s.require(http.HandlerFunc(s.listBackups)))
	mux.Handle("POST /api/backups", s.require(http.HandlerFunc(s.createBackup)))
	mux.Handle("GET /api/backups/{name}/download", s.require(http.HandlerFunc(s.downloadBackup)))
	mux.Handle("POST /api/backups/{name}/restore", s.require(http.HandlerFunc(s.restoreBackup)))
	mux.Handle("DELETE /api/backups/{name}", s.require(http.HandlerFunc(s.deleteBackup)))
	mux.Handle("GET /api/settings", s.require(http.HandlerFunc(s.getSettings)))
	mux.Handle("PUT /api/settings", s.require(http.HandlerFunc(s.updateSettings)))
	if s.static != nil {
		files := http.FileServerFS(s.static)
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.NotFound(w, r)
				return
			}
			if r.URL.Path != "/" {
				if f, err := s.static.Open(strings.TrimPrefix(r.URL.Path, "/")); err == nil {
					f.Close()
					files.ServeHTTP(w, r)
					return
				}
			}
			r.URL.Path = "/"
			files.ServeHTTP(w, r)
		})
	}
	return securityHeaders(mux)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if decodeJSON(w, r, &req) != nil { return }
	key := remoteIP(r) + "|" + strings.ToLower(strings.TrimSpace(req.Username))
	if !s.loginLimiter.Allowed(key) {
		writeError(w, http.StatusTooManyRequests, "too many login attempts; try again later")
		return
	}
	admin, err := s.store.Admin(req.Username)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.Password)) != nil {
		subtle.ConstantTimeCompare([]byte("xport"), []byte("xxxxx"))
		s.loginLimiter.Fail(key)
		time.Sleep(120 * time.Millisecond)
		writeError(w, 401, "invalid credentials")
		return
	}
	s.loginLimiter.Success(key)
	token, err := s.sessions.Create(admin.Username)
	if err != nil { writeError(w, 500, "session failure"); return }
	http.SetCookie(w, &http.Cookie{Name: "xport_session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"), MaxAge: 86400})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, e := r.Cookie("xport_session"); e == nil { s.sessions.Delete(c.Value) }
	http.SetCookie(w, &http.Cookie{Name: "xport_session", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, e := r.Cookie("xport_session")
		if e != nil || !s.sessions.Valid(c.Value) { writeError(w, 401, "unauthorized"); return }
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !sameOrigin(r) { writeError(w, 403, "cross-origin request rejected"); return }
		next.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	err := s.store.Ping()
	writeJSON(w, 200, map[string]any{"ok": err == nil, "database": err == nil, "time": time.Now().UnixMilli()})
}
func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	views, err := s.accounts.List(); if err != nil { writeError(w, 500, "database error"); return }
	enabled := 0; for _, v := range views { if v.Enabled { enabled++ } }
	writeJSON(w, 200, map[string]any{"system": sysinfo.Snapshot(), "accounts": map[string]int{"total": len(views), "enabled": enabled}, "xportVersion": "0.1.0-alpha"})
}
func (s *Server) listAccounts(w http.ResponseWriter, r *http.Request) {
	v, e := s.accounts.List(); if e != nil { writeError(w, 500, "database error"); return }; writeJSON(w, 200, map[string]any{"accounts": v})
}
func (s *Server) getAccount(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r); if e != nil { writeError(w, 400, "invalid account id"); return }
	v, e := s.accounts.Get(id); if e != nil { writeError(w, 404, e.Error()); return }; writeJSON(w, 200, v)
}
func (s *Server) createAccount(w http.ResponseWriter, r *http.Request) {
	var in accountcfg.Input; if decodeJSON(w, r, &in) != nil { return }
	v, e := s.accounts.Create(in); if e != nil { writeError(w, 400, e.Error()); return }; writeJSON(w, 201, v)
}
func (s *Server) updateAccount(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r); if e != nil { writeError(w, 400, "invalid account id"); return }
	var in accountcfg.Input; if decodeJSON(w, r, &in) != nil { return }
	v, e := s.accounts.Update(id, in); if e != nil { writeError(w, 400, e.Error()); return }; writeJSON(w, 200, v)
}
func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r); if e != nil { writeError(w, 400, "invalid account id"); return }
	if e = s.accounts.Delete(id); e != nil { writeError(w, 400, e.Error()); return }; writeJSON(w, 200, map[string]any{"ok": true})
}
func (s *Server) cloneAccount(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r); if e != nil { writeError(w, 400, "invalid account id"); return }
	var req struct { Name string `json:"name"`; Port int `json:"port"` }
	if decodeJSON(w, r, &req) != nil { return }
	v, e := s.accounts.Clone(id, req.Name, req.Port); if e != nil { writeError(w, 400, e.Error()); return }; writeJSON(w, 201, v)
}
func (s *Server) shareAccount(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r); if e != nil { writeError(w, 400, "invalid account id"); return }
	a, e := s.accounts.Raw(id); if e != nil { writeError(w, 404, e.Error()); return }
	uri, e := accountcfg.ShareURI(a, shareHost(r)); if e != nil { writeError(w, 400, e.Error()); return }; writeJSON(w, 200, map[string]any{"uri": uri})
}
func (s *Server) qrAccount(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r); if e != nil { writeError(w, 400, "invalid account id"); return }
	a, e := s.accounts.Raw(id); if e != nil { writeError(w, 404, e.Error()); return }
	uri, e := accountcfg.ShareURI(a, shareHost(r)); if e != nil { writeError(w, 400, e.Error()); return }
	code, e := qr.Encode(uri, qr.M); if e != nil { writeError(w, 400, "share link is too long for QR"); return }
	code.Scale = 6; w.Header().Set("Content-Type", "image/png"); w.Header().Set("Cache-Control", "no-store"); _, _ = w.Write(code.PNG())
}
func (s *Server) checkXrayUpdate(w http.ResponseWriter, r *http.Request) {
	if s.updater == nil { writeError(w, 503, "updater not configured"); return }
	ctx, c := contextWithTimeout(r, 12*time.Second); defer c(); info, _, e := s.updater.Check(ctx); if e != nil { writeError(w, 502, e.Error()); return }; writeJSON(w, 200, info)
}
func (s *Server) updateXray(w http.ResponseWriter, r *http.Request) {
	if s.updater == nil { writeError(w, 503, "updater not configured"); return }
	ctx, c := contextWithTimeout(r, 2*time.Minute); defer c(); info, e := s.updater.Update(ctx); if e != nil { writeError(w, 502, e.Error()); return }; writeJSON(w, 200, info)
}

func pathID(r *http.Request) (int64, error) { return strconv.ParseInt(r.PathValue("id"), 10, 64) }
func shareHost(r *http.Request) string {
	if h := strings.TrimSpace(r.URL.Query().Get("host")); h != "" { return h }
	host := r.Host; if h, _, e := net.SplitHostPort(host); e == nil { return h }; return host
}
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)); d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil { writeError(w, 400, "invalid request"); return e }; return nil
}
func writeJSON(w http.ResponseWriter, status int, v any) { w.Header().Set("Content-Type", "application/json; charset=utf-8"); w.WriteHeader(status); _ = json.NewEncoder(w).Encode(v) }
func writeError(w http.ResponseWriter, status int, msg string) { writeJSON(w, status, map[string]any{"error": msg}) }
func contextWithTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) { return context.WithTimeout(r.Context(), d) }
func remoteIP(r *http.Request) string { if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil { return h }; return r.RemoteAddr }
func sameOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin")); if origin == "" { return true }
	u, err := url.Parse(origin); return err == nil && strings.EqualFold(u.Host, r.Host)
}
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff"); w.Header().Set("X-Frame-Options", "DENY"); w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; script-src 'self'")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()"); next.ServeHTTP(w, r)
	})
}
