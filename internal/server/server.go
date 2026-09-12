package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"rsc.io/qr"

	"github.com/Kerberos255/X-port/internal/accountcfg"
	"github.com/Kerberos255/X-port/internal/auth"
	"github.com/Kerberos255/X-port/internal/buildinfo"
	"github.com/Kerberos255/X-port/internal/service"
	"github.com/Kerberos255/X-port/internal/store"
	sysinfo "github.com/Kerberos255/X-port/internal/system"
	"github.com/Kerberos255/X-port/internal/xray"
)

type Server struct {
	store          *store.Store
	accounts       *service.Accounts
	sessions       *auth.Sessions
	loginLimiter   *auth.LoginLimiter
	ipLimiter      *auth.LoginLimiter
	updater        *xray.Updater
	static         fs.FS
	dataDir        string
	xrayService    string
	panelService   string
	manager        *xray.Manager
	trustedProxies []netip.Prefix
}

func New(st *store.Store, accounts *service.Accounts, updater *xray.Updater, static fs.FS) *Server {
	return &Server{
		store: st, accounts: accounts, updater: updater, static: static,
		sessions:       auth.NewSessions(24 * time.Hour),
		loginLimiter:   auth.NewLoginLimiter(8, 10*time.Minute),
		ipLimiter:      auth.NewLoginLimiter(24, 30*time.Minute),
		trustedProxies: trustedProxyPrefixes(),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/login", s.login)
	mux.Handle("POST /api/logout", s.require(http.HandlerFunc(s.logout)))
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
	s.registerExtraRoutes(mux)
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
	handler := http.Handler(mux)
	if basePath, ok, err := s.store.Setting("panel_base_path"); err == nil && ok {
		handler = MountBasePath(handler, basePath)
	}
	return s.securityHeaders(handler)
}

var dummyLoginHash = func() []byte {
	hash, err := bcrypt.GenerateFromPassword([]byte("xport-login-timing-sentinel"), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	return hash
}()

const (
	maxLoginUsernameBytes = 128
	maxLoginPasswordBytes = 1024
	maxLoginBodyBytes     = 16 << 10
)

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if decodeJSONLimit(w, r, &req, maxLoginBodyBytes) != nil {
		return
	}
	username := strings.TrimSpace(req.Username)
	ip := s.remoteIP(r)
	key := loginRateKey(ip, username)
	if !s.ipLimiter.Allowed(ip) || !s.loginLimiter.Allowed(key) {
		writeError(w, http.StatusTooManyRequests, "too many login attempts; try again later")
		return
	}

	admin, lookupErr := s.store.Admin(username)
	hash := dummyLoginHash
	knownUser := lookupErr == nil
	if knownUser {
		hash = []byte(admin.PasswordHash)
	}
	passwordErr := bcrypt.CompareHashAndPassword(hash, []byte(req.Password))
	invalidSize := username == "" || len(username) > maxLoginUsernameBytes || len(req.Password) > maxLoginPasswordBytes
	if invalidSize || !knownUser || passwordErr != nil {
		s.loginLimiter.Fail(key)
		s.ipLimiter.Fail(ip)
		time.Sleep(120 * time.Millisecond)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	s.loginLimiter.Success(key)
	s.ipLimiter.Success(ip)
	token, err := s.sessions.Create(admin.Username)
	if err != nil {
		writeError(w, 500, "session failure")
		return
	}
	s.setSessionCookie(w, r, token)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("xport_session"); err == nil {
		s.sessions.Delete(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name: "xport_session", Path: "/", MaxAge: -1, Expires: time.Unix(1, 0),
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.requestScheme(r) == "https",
	})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name: "xport_session", Value: token, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteStrictMode, Secure: s.requestScheme(r) == "https",
		MaxAge: int((24 * time.Hour).Seconds()),
	})
}

func (s *Server) require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("xport_session")
		if err != nil || !s.sessions.Valid(c.Value) {
			writeError(w, 401, "unauthorized")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !s.sameOrigin(r) {
			writeError(w, 403, "cross-origin request rejected")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	err := s.store.Ping()
	writeJSON(w, 200, map[string]any{"ok": err == nil, "database": err == nil, "time": time.Now().UnixMilli()})
}
func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	views, err := s.accounts.List()
	if err != nil {
		writeError(w, 500, "database error")
		return
	}
	enabled := 0
	for _, v := range views {
		if v.Enabled {
			enabled++
		}
	}
	writeJSON(w, 200, map[string]any{"system": sysinfo.Snapshot(), "accounts": map[string]int{"total": len(views), "enabled": enabled}, "xportVersion": buildinfo.Current})
}
func (s *Server) listAccounts(w http.ResponseWriter, r *http.Request) {
	v, e := s.accounts.List()
	if e != nil {
		writeError(w, 500, "database error")
		return
	}
	writeJSON(w, 200, map[string]any{"accounts": v})
}
func (s *Server) getAccount(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		writeError(w, 400, "invalid account id")
		return
	}
	v, e := s.accounts.Get(id)
	if e != nil {
		writeError(w, 404, e.Error())
		return
	}
	writeJSON(w, 200, v)
}
func (s *Server) createAccount(w http.ResponseWriter, r *http.Request) {
	var in accountcfg.Input
	if decodeJSON(w, r, &in) != nil {
		return
	}
	v, e := s.accounts.Create(in)
	if e != nil {
		writeError(w, 400, e.Error())
		return
	}
	writeJSON(w, 201, v)
}
func (s *Server) updateAccount(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		writeError(w, 400, "invalid account id")
		return
	}
	var in accountcfg.Input
	if decodeJSON(w, r, &in) != nil {
		return
	}
	v, e := s.accounts.Update(id, in)
	if e != nil {
		writeError(w, 400, e.Error())
		return
	}
	writeJSON(w, 200, v)
}
func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		writeError(w, 400, "invalid account id")
		return
	}
	if e = s.accounts.Delete(id); e != nil {
		writeError(w, 400, e.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
func (s *Server) cloneAccount(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		writeError(w, 400, "invalid account id")
		return
	}
	var req struct {
		Name string `json:"name"`
		Port int    `json:"port"`
	}
	if decodeJSON(w, r, &req) != nil {
		return
	}
	v, e := s.accounts.Clone(id, req.Name, req.Port)
	if e != nil {
		writeError(w, 400, e.Error())
		return
	}
	writeJSON(w, 201, v)
}
func (s *Server) shareAccount(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		writeError(w, 400, "invalid account id")
		return
	}
	a, e := s.accounts.Raw(id)
	if e != nil {
		writeError(w, 404, e.Error())
		return
	}
	uri, e := accountcfg.ShareURI(a, shareHost(r))
	if e != nil {
		writeError(w, 400, e.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"uri": uri})
}
func (s *Server) qrAccount(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil {
		writeError(w, 400, "invalid account id")
		return
	}
	a, e := s.accounts.Raw(id)
	if e != nil {
		writeError(w, 404, e.Error())
		return
	}
	uri, e := accountcfg.ShareURI(a, shareHost(r))
	if e != nil {
		writeError(w, 400, e.Error())
		return
	}
	code, e := qr.Encode(uri, qr.M)
	if e != nil {
		writeError(w, 400, "share link is too long for QR")
		return
	}
	code.Scale = 6
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(code.PNG())
}
func (s *Server) checkXrayUpdate(w http.ResponseWriter, r *http.Request) {
	if s.updater == nil {
		writeError(w, 503, "updater not configured")
		return
	}
	ctx, c := contextWithTimeout(r, 12*time.Second)
	defer c()
	info, _, e := s.updater.Check(ctx)
	if e != nil {
		writeError(w, 502, e.Error())
		return
	}
	writeJSON(w, 200, info)
}
func (s *Server) updateXray(w http.ResponseWriter, r *http.Request) {
	if s.updater == nil {
		writeError(w, 503, "updater not configured")
		return
	}
	ctx, c := contextWithTimeout(r, 2*time.Minute)
	defer c()
	info, e := s.updater.Update(ctx)
	if e != nil {
		writeError(w, 502, e.Error())
		return
	}
	writeJSON(w, 200, info)
}

func pathID(r *http.Request) (int64, error) { return strconv.ParseInt(r.PathValue("id"), 10, 64) }
func shareHost(r *http.Request) string {
	if h := strings.TrimSpace(r.URL.Query().Get("host")); h != "" {
		if len(h) > 512 {
			return ""
		}
		return h
	}
	host := saneHost(r.Host)
	if h, _, e := net.SplitHostPort(host); e == nil {
		return h
	}
	return host
}
func saneHost(v string) string {
	v = strings.TrimSpace(v)
	if len(v) > 512 || strings.ContainsAny(v, "\\/\r\n\t") {
		return ""
	}
	return v
}
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	return decodeJSONLimit(w, r, v, 1<<20)
}
func decodeJSONLimit(w http.ResponseWriter, r *http.Request, v any, limit int64) error {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		writeError(w, 400, "invalid request")
		return e
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		writeError(w, 400, "invalid request")
		if e == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return e
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}
func contextWithTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}

func loginRateKey(ip, username string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(username))))
	return ip + "|" + hex.EncodeToString(sum[:16])
}

func trustedProxyPrefixes() []netip.Prefix {
	out := []netip.Prefix{
		netip.MustParsePrefix("127.0.0.0/8"),
		netip.MustParsePrefix("::1/128"),
	}
	for _, raw := range strings.Split(os.Getenv("XPORT_TRUSTED_PROXIES"), ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if p, err := netip.ParsePrefix(raw); err == nil {
			out = append(out, p.Masked())
			continue
		}
		if a, err := netip.ParseAddr(raw); err == nil {
			out = append(out, netip.PrefixFrom(a, a.BitLen()))
		}
	}
	return out
}

func directRemoteIP(r *http.Request) string {
	if h, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr)); err == nil {
		return strings.Trim(h, "[]")
	}
	return strings.Trim(strings.TrimSpace(r.RemoteAddr), "[]")
}

func (s *Server) trustedProxy(ip string) bool {
	if i := strings.LastIndexByte(ip, '%'); i > 0 {
		ip = ip[:i]
	}
	a, err := netip.ParseAddr(strings.Trim(ip, "[]"))
	if err != nil {
		return false
	}
	for _, p := range s.trustedProxies {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

func (s *Server) remoteIP(r *http.Request) string {
	peer := directRemoteIP(r)
	if !s.trustedProxy(peer) {
		return peer
	}
	parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(parts) - 1; i >= 0; i-- {
		candidate := strings.Trim(strings.TrimSpace(parts[i]), "[]")
		if candidate == "" {
			continue
		}
		if _, err := netip.ParseAddr(strings.Split(candidate, "%")[0]); err != nil {
			continue
		}
		if s.trustedProxy(candidate) {
			continue
		}
		return candidate
	}
	if candidate := strings.Trim(strings.TrimSpace(r.Header.Get("X-Real-IP")), "[]"); candidate != "" {
		if _, err := netip.ParseAddr(strings.Split(candidate, "%")[0]); err == nil {
			return candidate
		}
	}
	return peer
}

func firstForwardedValue(v string) string {
	if i := strings.IndexByte(v, ','); i >= 0 {
		v = v[:i]
	}
	return strings.TrimSpace(v)
}

func (s *Server) requestScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if s.trustedProxy(directRemoteIP(r)) {
		if proto := strings.ToLower(firstForwardedValue(r.Header.Get("X-Forwarded-Proto"))); proto == "https" || proto == "http" {
			return proto
		}
	}
	return "http"
}

func (s *Server) requestHost(r *http.Request) string {
	if s.trustedProxy(directRemoteIP(r)) {
		if h := saneHost(firstForwardedValue(r.Header.Get("X-Forwarded-Host"))); h != "" {
			return h
		}
	}
	return saneHost(r.Host)
}

func (s *Server) sameOriginURL(raw string, r *http.Request) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && strings.EqualFold(u.Scheme, s.requestScheme(r)) && strings.EqualFold(u.Host, s.requestHost(r))
}

func (s *Server) sameOrigin(r *http.Request) bool {
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
		return s.sameOriginURL(origin, r)
	}
	if ref := strings.TrimSpace(r.Header.Get("Referer")); ref != "" {
		return s.sameOriginURL(ref, r)
	}
	site := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")))
	return site == "" || site == "same-origin" || site == "none"
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; object-src 'none'; frame-ancestors 'none'; form-action 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; script-src 'self'")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store, max-age=0")
			w.Header().Set("Pragma", "no-cache")
		}
		if s.requestScheme(r) == "https" {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(w, r)
	})
}
