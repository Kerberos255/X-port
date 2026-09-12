#!/usr/bin/env python3
from pathlib import Path


def read(path):
    return Path(path).read_text(encoding="utf-8")


def write(path, text):
    Path(path).write_text(text, encoding="utf-8")


def replace(path, old, new, count=1):
    text = read(path)
    n = text.count(old)
    if n < count:
        raise SystemExit(f"{path}: expected at least {count} occurrence(s), found {n}: {old[:80]!r}")
    text = text.replace(old, new, count)
    write(path, text)


def replace_between(path, start, end, replacement):
    text = read(path)
    a = text.find(start)
    if a < 0:
        raise SystemExit(f"{path}: start marker not found: {start!r}")
    b = text.find(end, a)
    if b < 0:
        raise SystemExit(f"{path}: end marker not found: {end!r}")
    write(path, text[:a] + replacement.rstrip() + "\n\n" + text[b:])


# --- Web/auth middleware -------------------------------------------------
path = "internal/server/server.go"
replace(path, '"context"\n\t"crypto/subtle"\n\t"encoding/json"', '"context"\n\t"crypto/sha256"\n\t"encoding/hex"\n\t"encoding/json"')
replace(path, '"net/http"\n\t"net/url"', '"net/http"\n\t"net/netip"\n\t"net/url"\n\t"os"')
replace(path, '\tmanager      *xray.Manager\n}', '\tmanager         *xray.Manager\n\ttrustedProxies  []netip.Prefix\n}')
replace(path, '\t\tipLimiter: auth.NewLoginLimiter(24, 30*time.Minute),\n\t}', '\t\tipLimiter: auth.NewLoginLimiter(24, 30*time.Minute),\n\t\ttrustedProxies: trustedProxyPrefixes(),\n\t}')
replace(path, 'mux.HandleFunc("POST /api/logout", s.logout)', 'mux.Handle("POST /api/logout", s.require(http.HandlerFunc(s.logout)))')
replace(path, 'return securityHeaders(handler)', 'return s.securityHeaders(handler)')

login_block = r'''var dummyLoginHash = func() []byte {
	hash, err := bcrypt.GenerateFromPassword([]byte("xport-login-timing-sentinel"), bcrypt.DefaultCost)
	if err != nil { panic(err) }
	return hash
}()

const (
	maxLoginUsernameBytes = 128
	maxLoginPasswordBytes = 1024
	maxLoginBodyBytes     = 16 << 10
)

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct { Username string `json:"username"`; Password string `json:"password"` }
	if decodeJSONLimit(w, r, &req, maxLoginBodyBytes) != nil { return }
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
	if knownUser { hash = []byte(admin.PasswordHash) }
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
	if err != nil { writeError(w, 500, "session failure"); return }
	s.setSessionCookie(w, r, token)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("xport_session"); err == nil { s.sessions.Delete(c.Value) }
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
		if err != nil || !s.sessions.Valid(c.Value) { writeError(w, 401, "unauthorized"); return }
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !s.sameOrigin(r) {
			writeError(w, 403, "cross-origin request rejected")
			return
		}
		next.ServeHTTP(w, r)
	})
}'''
replace_between(path, 'func (s *Server) login', 'func (s *Server) health', login_block)

helpers = r'''func pathID(r *http.Request)(int64,error){return strconv.ParseInt(r.PathValue("id"),10,64)}
func shareHost(r *http.Request)string{if h:=strings.TrimSpace(r.URL.Query().Get("host"));h!=""{if len(h)>512{return ""};return h};host:=saneHost(r.Host);if h,_,e:=net.SplitHostPort(host);e==nil{return h};return host}
func saneHost(v string) string { v=strings.TrimSpace(v); if len(v)>512 || strings.ContainsAny(v,"\\/\r\n\t") { return "" }; return v }
func decodeJSON(w http.ResponseWriter,r *http.Request,v any)error{return decodeJSONLimit(w,r,v,1<<20)}
func decodeJSONLimit(w http.ResponseWriter,r *http.Request,v any,limit int64)error{d:=json.NewDecoder(http.MaxBytesReader(w,r.Body,limit));d.DisallowUnknownFields();if e:=d.Decode(v);e!=nil{writeError(w,400,"invalid request");return e};if d.More(){writeError(w,400,"invalid request");return &json.SyntaxError{Offset:0}};return nil}
func writeJSON(w http.ResponseWriter,status int,v any){w.Header().Set("Content-Type","application/json; charset=utf-8");w.WriteHeader(status);_=json.NewEncoder(w).Encode(v)}
func writeError(w http.ResponseWriter,status int,msg string){writeJSON(w,status,map[string]any{"error":msg})}
func contextWithTimeout(r *http.Request,d time.Duration)(context.Context,context.CancelFunc){return context.WithTimeout(r.Context(),d)}

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
		if raw == "" { continue }
		if p, err := netip.ParsePrefix(raw); err == nil { out = append(out, p.Masked()); continue }
		if a, err := netip.ParseAddr(raw); err == nil { out = append(out, netip.PrefixFrom(a, a.BitLen())) }
	}
	return out
}

func directRemoteIP(r *http.Request) string {
	if h, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr)); err == nil { return strings.Trim(h, "[]") }
	return strings.Trim(strings.TrimSpace(r.RemoteAddr), "[]")
}

func (s *Server) trustedProxy(ip string) bool {
	if i := strings.LastIndexByte(ip, '%'); i > 0 { ip = ip[:i] }
	a, err := netip.ParseAddr(strings.Trim(ip, "[]"))
	if err != nil { return false }
	for _, p := range s.trustedProxies { if p.Contains(a) { return true } }
	return false
}

func (s *Server) remoteIP(r *http.Request) string {
	peer := directRemoteIP(r)
	if !s.trustedProxy(peer) { return peer }
	parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(parts)-1; i >= 0; i-- {
		candidate := strings.Trim(strings.TrimSpace(parts[i]), "[]")
		if candidate == "" { continue }
		if _, err := netip.ParseAddr(strings.Split(candidate, "%")[0]); err != nil { continue }
		if s.trustedProxy(candidate) { continue }
		return candidate
	}
	if candidate := strings.Trim(strings.TrimSpace(r.Header.Get("X-Real-IP")), "[]"); candidate != "" {
		if _, err := netip.ParseAddr(strings.Split(candidate, "%")[0]); err == nil { return candidate }
	}
	return peer
}

func firstForwardedValue(v string) string {
	if i := strings.IndexByte(v, ','); i >= 0 { v = v[:i] }
	return strings.TrimSpace(v)
}

func (s *Server) requestScheme(r *http.Request) string {
	if r.TLS != nil { return "https" }
	if s.trustedProxy(directRemoteIP(r)) {
		if proto := strings.ToLower(firstForwardedValue(r.Header.Get("X-Forwarded-Proto"))); proto == "https" || proto == "http" { return proto }
	}
	return "http"
}

func (s *Server) requestHost(r *http.Request) string {
	if s.trustedProxy(directRemoteIP(r)) {
		if h := saneHost(firstForwardedValue(r.Header.Get("X-Forwarded-Host"))); h != "" { return h }
	}
	return saneHost(r.Host)
}

func (s *Server) sameOriginURL(raw string, r *http.Request) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && strings.EqualFold(u.Scheme, s.requestScheme(r)) && strings.EqualFold(u.Host, s.requestHost(r))
}

func (s *Server) sameOrigin(r *http.Request) bool {
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" { return s.sameOriginURL(origin, r) }
	if ref := strings.TrimSpace(r.Header.Get("Referer")); ref != "" { return s.sameOriginURL(ref, r) }
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
		if s.requestScheme(r) == "https" { w.Header().Set("Strict-Transport-Security", "max-age=31536000") }
		next.ServeHTTP(w, r)
	})
}'''
text = read(path)
a = text.find('func pathID')
if a < 0:
    raise SystemExit(f"{path}: pathID marker not found")
write(path, text[:a] + helpers + "\n")

# --- Settings/session rotation ------------------------------------------
path = "internal/server/operations.go"
needle = '\tin.AdminUsername=strings.TrimSpace(in.AdminUsername);in.PanelListen=strings.TrimSpace(in.PanelListen);in.PanelBasePath=normalizePanelBasePathSetting(in.PanelBasePath);in.PanelCertFile=strings.TrimSpace(in.PanelCertFile);in.PanelKeyFile=strings.TrimSpace(in.PanelKeyFile);in.PanelDomain=strings.TrimSpace(in.PanelDomain)\n'
replacement = needle + '''\tif err:=validateSettingsInput(in);err!=nil{writeError(w,400,err.Error());return}\n'''
replace(path, needle, replacement)
old = '\tif in.AdminUsername!=currentUser{s.sessions.RenameUser(currentUser,in.AdminUsername)};restartRequired:=oldPanel!=in.PanelListen||oldBase!=in.PanelBasePath||oldCert!=in.PanelCertFile||oldKey!=in.PanelKeyFile;writeJSON(w,200,map[string]any{"ok":true,"restartRequired":restartRequired,"xrayApiApplied":apiChanged})\n'
new = '''\tif adminChanging{\n\t\ts.sessions.Clear()\n\t\ttoken,err:=s.sessions.Create(in.AdminUsername);if err!=nil{writeError(w,500,"settings saved but session rotation failed; sign in again");return}\n\t\ts.setSessionCookie(w,r,token)\n\t}\n\trestartRequired:=oldPanel!=in.PanelListen||oldBase!=in.PanelBasePath||oldCert!=in.PanelCertFile||oldKey!=in.PanelKeyFile\n\twriteJSON(w,200,map[string]any{"ok":true,"restartRequired":restartRequired,"xrayApiApplied":apiChanged,"sessionRotated":adminChanging})\n'''
replace(path, old, new)
append = r'''

func validateSettingsInput(in settingsInput) error {
	checks := []struct{name, value string; max int}{
		{"admin username", in.AdminUsername, 128}, {"panel listen", in.PanelListen, 256},
		{"panel base path", in.PanelBasePath, 256}, {"panel certificate path", in.PanelCertFile, 4096},
		{"panel private key path", in.PanelKeyFile, 4096}, {"panel domain", in.PanelDomain, 253},
		{"default REALITY SNI", in.DefaultRealitySNI, 512}, {"default REALITY target", in.DefaultRealityDest, 1024},
		{"current password", in.CurrentPassword, 4096}, {"new password", in.NewPassword, 4096},
	}
	for _, c := range checks {
		if len(c.value) > c.max { return fmt.Errorf("%s is too long", c.name) }
		if strings.ContainsRune(c.value, '\x00') || strings.ContainsAny(c.value, "\r\n") { return fmt.Errorf("%s contains invalid control characters", c.name) }
	}
	if in.PanelDomain != "" && strings.ContainsAny(in.PanelDomain, "/\\?#@ \t") { return errors.New("invalid panel domain") }
	return nil
}
'''
write(path, read(path).rstrip() + append + "\n")

# --- Account input bounds / enums ---------------------------------------
path = "internal/accountcfg/account.go"
replace(path, 'func New(in Input, fallbackPort int) (model.Account, error) {\n\tin.Name = strings.TrimSpace(in.Name)', 'func New(in Input, fallbackPort int) (model.Account, error) {\n\tnormalizeEditableInput(&in)\n\tin.Name = strings.TrimSpace(in.Name)')
replace(path, '\tapplyDefaults(&in)\n\tsettings, stream, err := buildProtocol(in)', '\tapplyDefaults(&in)\n\tif err := validateEditableInput(in); err != nil { return model.Account{}, err }\n\tsettings, stream, err := buildProtocol(in)', 1)
replace(path, 'func Update(a model.Account, in Input) (model.Account, error) {\n\tin.Name = strings.TrimSpace(in.Name)', 'func Update(a model.Account, in Input) (model.Account, error) {\n\tnormalizeEditableInput(&in)\n\tin.Name = strings.TrimSpace(in.Name)')
replace(path, '\tmergeMissing(&in, oldView)\n\tsettings, stream, err := updateProtocolJSON(a, in)', '\tmergeMissing(&in, oldView)\n\tnormalizeEditableInput(&in)\n\tif err := validateEditableInput(in); err != nil { return model.Account{}, err }\n\tsettings, stream, err := updateProtocolJSON(a, in)')
replace(path, '\tif name == "" {\n\t\tname = a.Name + " copy"\n\t}\n\tclone := a', '\tif name == "" {\n\t\tname = a.Name + " copy"\n\t}\n\tif len(name) > 80 { return model.Account{}, errors.New("name is too long") }\n\tclone := a')
marker = 'func validPort(port int) error {'
validation = r'''func normalizeEditableInput(in *Input) {
	in.Name = strings.TrimSpace(in.Name)
	in.Protocol = strings.ToLower(strings.TrimSpace(in.Protocol))
	in.Network = strings.ToLower(strings.TrimSpace(in.Network))
	in.Security = strings.ToLower(strings.TrimSpace(in.Security))
	in.Method = strings.ToLower(strings.TrimSpace(in.Method))
	in.Credential = strings.TrimSpace(in.Credential)
	in.Username = strings.TrimSpace(in.Username)
	in.ServerName = strings.TrimSpace(in.ServerName)
	in.Dest = strings.TrimSpace(in.Dest)
	in.PrivateKey = strings.TrimSpace(in.PrivateKey)
	in.ShortID = strings.TrimSpace(in.ShortID)
	in.Host = strings.TrimSpace(in.Host)
	in.Path = strings.TrimSpace(in.Path)
	in.ServiceName = strings.TrimSpace(in.ServiceName)
}

func validateEditableInput(in Input) error {
	checks := []struct{name, value string; max int}{
		{"name", in.Name, 80}, {"protocol", in.Protocol, 32}, {"credential", in.Credential, 256},
		{"username", in.Username, 256}, {"password", in.Password, 4096}, {"server password", in.ServerPassword, 4096},
		{"client security", in.ClientSecurity, 64}, {"method", in.Method, 128}, {"flow", in.Flow, 128},
		{"network", in.Network, 32}, {"security", in.Security, 32}, {"server name", in.ServerName, 512},
		{"target", in.Dest, 1024}, {"private key", in.PrivateKey, 256}, {"short id", in.ShortID, 128},
		{"host", in.Host, 512}, {"path", in.Path, 2048}, {"service name", in.ServiceName, 512},
	}
	for _, c := range checks {
		if len(c.value) > c.max { return fmt.Errorf("%s is too long", c.name) }
		if strings.ContainsRune(c.value, '\x00') || strings.ContainsAny(c.value, "\r\n") { return fmt.Errorf("%s contains invalid control characters", c.name) }
	}
	if in.Name == "" { return errors.New("name is required") }
	if in.QuotaBytes < 0 { return errors.New("quotaBytes must not be negative") }
	if in.ExpiryTime < 0 { return errors.New("expiryTime must not be negative") }
	switch in.Protocol {
	case "vless", "vmess", "trojan", "shadowsocks", "socks", "http":
	default: return fmt.Errorf("protocol %q is not supported by the built-in editor", in.Protocol)
	}
	if in.Protocol != "socks" && in.Protocol != "http" {
		switch in.Network {
		case "tcp", "raw", "ws", "websocket", "grpc", "httpupgrade", "xhttp":
		default: return fmt.Errorf("network %q is not supported by the built-in editor", in.Network)
		}
		switch in.Security {
		case "none", "tls":
		case "reality": if in.Protocol != "vless" && in.Protocol != "trojan" { return errors.New("REALITY is supported only for VLESS and Trojan") }
		default: return fmt.Errorf("security %q is not supported by the built-in editor", in.Security)
		}
	}
	if in.Protocol == "vmess" && in.ClientSecurity != "" {
		switch strings.ToLower(in.ClientSecurity) {
		case "auto", "aes-128-gcm", "chacha20-poly1305", "none", "zero":
		default: return fmt.Errorf("VMess client security %q is not supported", in.ClientSecurity)
		}
	}
	return nil
}

'''
text = read(path)
pos = text.find(marker)
if pos < 0: raise SystemExit(f"{path}: validPort marker missing")
write(path, text[:pos] + validation + text[pos:])

# --- Expert editor limits / overflow ------------------------------------
path = "internal/service/expert.go"
replace(path, 'func (s *Accounts) UpdateExpert(id int64, in ExpertConfig) (ExpertConfig, error) {\n\ts.mu.Lock()', 'func (s *Accounts) UpdateExpert(id int64, in ExpertConfig) (ExpertConfig, error) {\n\tif err := validateExpertConfig(in); err != nil { return ExpertConfig{}, err }\n\ts.mu.Lock()')
replace(path, 'out.XHTTPScMaxBufferedPosts = int64(expertUint(x["scMaxBufferedPosts"]))', 'out.XHTTPScMaxBufferedPosts = expertInt64(x["scMaxBufferedPosts"])')
replace(path, 'func expertUint(v any) uint64 { switch n := v.(type) { case float64: if n > 0 { return uint64(n) }; case json.Number: u, _ := n.Int64(); if u > 0 { return uint64(u) } }; return 0 }', 'func expertUint(v any) uint64 { switch n := v.(type) { case float64: if n > 0 && n <= float64(^uint64(0)) { return uint64(n) }; case json.Number: u, _ := n.Int64(); if u > 0 { return uint64(u) } }; return 0 }\nfunc expertInt64(v any) int64 { u:=expertUint(v); if u > uint64(1<<63-1) { return int64(1<<63-1) }; return int64(u) }')
append = r'''

func validateExpertConfig(in ExpertConfig) error {
	if len(in.FallbacksJSON) > 128<<10 { return errors.New("fallbacks JSON is too large") }
	if len(in.HTTPHeaderJSON) > 64<<10 { return errors.New("HTTP camouflage header JSON is too large") }
	if len(in.TLSCertificatesJSON) > 256<<10 { return errors.New("TLS certificates JSON is too large") }
	if len(in.RealityServerNames) > 64 || len(in.RealityShortIDs) > 64 || len(in.TLSALPN) > 64 { return errors.New("expert list contains too many entries") }
	checks := []struct{name, value string; max int}{
		{"REALITY target", in.RealityTarget, 1024}, {"REALITY private key", in.RealityPrivateKey, 256},
		{"REALITY min client version", in.RealityMinClientVer, 128}, {"REALITY max client version", in.RealityMaxClientVer, 128},
		{"REALITY ML-DSA seed", in.RealityMLDSA65Seed, 4096}, {"TLS min version", in.TLSMinVersion, 64},
		{"TLS max version", in.TLSMaxVersion, 64}, {"TLS cipher suites", in.TLSCipherSuites, 4096},
		{"XHTTP mode", in.XHTTPMode, 64}, {"XHTTP padding", in.XHTTPXPaddingBytes, 256},
		{"XHTTP each-post bytes", in.XHTTPScMaxEachPostBytes, 256}, {"XHTTP stream-up seconds", in.XHTTPScStreamUpServerSecs, 256},
		{"XHTTP uplink method", in.XHTTPUplinkHTTPMethod, 32},
	}
	for _, c := range checks {
		if len(c.value) > c.max { return fmt.Errorf("%s is too long", c.name) }
		if strings.ContainsRune(c.value, '\x00') || strings.ContainsAny(c.value, "\r\n") { return fmt.Errorf("%s contains invalid control characters", c.name) }
	}
	for _, list := range [][]string{in.RealityServerNames, in.RealityShortIDs, in.TLSALPN} {
		for _, v := range list { if len(v) > 1024 || strings.ContainsRune(v, '\x00') || strings.ContainsAny(v, "\r\n") { return errors.New("expert list entry is invalid") } }
	}
	if in.RealityXver > 2 { return errors.New("REALITY xver must be between 0 and 2") }
	if in.XHTTPScMaxBufferedPosts < 0 { return errors.New("XHTTP scMaxBufferedPosts must not be negative") }
	return nil
}
'''
write(path, read(path).rstrip() + append + "\n")

# --- Migration allowlist -------------------------------------------------
write("internal/migrate/xray_config.go", r'''package migrate

import (
	"encoding/json"
	"fmt"
	"os"
)

var preservedGlobalXraySections = map[string]struct{}{
	"log": {}, "dns": {}, "routing": {}, "policy": {}, "outbounds": {},
	"transport": {}, "fakedns": {},
}

// ReadGlobalXrayConfig preserves only global sections that X-port can safely
// carry forward. Listener/API/metrics/reverse/observatory sections are rebuilt
// or intentionally omitted so a legacy config cannot silently introduce a new
// management listener or unrelated active behavior during migration.
func ReadGlobalXrayConfig(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil { return "", err }
	if len(b) > 16<<20 { return "", fmt.Errorf("Xray config is unexpectedly large") }
	var cfg map[string]any
	if err := json.Unmarshal(b, &cfg); err != nil { return "", fmt.Errorf("parse Xray config: %w", err) }
	out := make(map[string]any, len(preservedGlobalXraySections))
	for key := range preservedGlobalXraySections {
		if value, ok := cfg[key]; ok { out[key] = value }
	}
	encoded, err := json.Marshal(out)
	if err != nil { return "", err }
	return string(encoded), nil
}
''')
write("internal/migrate/xray_config_test.go", r'''package migrate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReadGlobalXrayConfigAllowlist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	raw := `{"log":{"loglevel":"warning"},"dns":{"servers":["1.1.1.1"]},"routing":{"domainStrategy":"AsIs"},"outbounds":[{"protocol":"freedom"}],"transport":{},"fakedns":[],"inbounds":[{"port":1}],"api":{"tag":"api"},"stats":{},"metrics":{"listen":"0.0.0.0:11111"},"reverse":{"bridges":[]},"observatory":{"subjectSelector":[""]}}`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil { t.Fatal(err) }
	got, err := ReadGlobalXrayConfig(path)
	if err != nil { t.Fatal(err) }
	var cfg map[string]any
	if err := json.Unmarshal([]byte(got), &cfg); err != nil { t.Fatal(err) }
	for _, keep := range []string{"log", "dns", "routing", "outbounds", "transport", "fakedns"} {
		if _, ok := cfg[keep]; !ok { t.Fatalf("expected %s to be preserved: %s", keep, got) }
	}
	for _, drop := range []string{"inbounds", "api", "stats", "metrics", "reverse", "observatory"} {
		if _, ok := cfg[drop]; ok { t.Fatalf("expected %s to be removed: %s", drop, got) }
	}
}
''')

# --- Safer service command boundary -------------------------------------
write("internal/ops/system.go", r'''package ops

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var systemdUnitRE = regexp.MustCompile(`^[A-Za-z0-9_.@:-]+$`)

func validSystemdUnit(service string) bool { return service != "" && len(service) <= 255 && systemdUnitRE.MatchString(service) }
func validManagedPath(path string) bool { return filepath.IsAbs(path) && filepath.Clean(path) == path && !strings.ContainsRune(path, '\x00') }

func Journal(service string, lines int) (string, error) {
	if !validSystemdUnit(service) { return "", errors.New("invalid service name") }
	if lines < 1 { lines = 200 }
	if lines > 2000 { lines = 2000 }
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "journalctl", "-u", service, "-n", strconv.Itoa(lines), "--no-pager", "-o", "short-iso").CombinedOutput()
	if ctx.Err() != nil { return "", ctx.Err() }
	if err != nil { return "", fmt.Errorf("journalctl %s: %w: %s", service, err, strings.TrimSpace(string(out))) }
	return string(out), nil
}

func RestartService(service string) error {
	if !validSystemdUnit(service) { return errors.New("invalid service name") }
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "systemctl", "restart", service).CombinedOutput()
	if ctx.Err() != nil { return ctx.Err() }
	if err != nil { return fmt.Errorf("restart %s: %w: %s", service, err, strings.TrimSpace(string(out))) }
	return nil
}

func ScheduleRestart(service string, delay time.Duration) error {
	if !validSystemdUnit(service) { return errors.New("invalid service name") }
	if delay < time.Second { delay = time.Second }
	unit := "xport-panel-restart-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "systemd-run", "--quiet", "--unit", unit, "--on-active="+delay.String(), "/bin/systemctl", "restart", service).CombinedOutput()
	if ctx.Err() != nil { return ctx.Err() }
	if err != nil { return fmt.Errorf("schedule restart %s: %w: %s", service, err, strings.TrimSpace(string(out))) }
	return nil
}

func verifiedBinaryRestartScript() string {
	return `service="$1"; bin="$2"; bak="$3"; ` +
		`if systemctl restart "$service"; then ` +
		`i=0; while [ "$i" -lt 10 ]; do sleep 1; systemctl is-active --quiet "$service" || break; i=$((i+1)); done; ` +
		`if [ "$i" -eq 10 ]; then exit 0; fi; fi; ` +
		`if [ ! -f "$bak" ]; then exit 1; fi; ` +
		`rm -f "$bin"; mv "$bak" "$bin"; systemctl restart "$service"`
}

func ScheduleVerifiedBinaryRestart(service, binary, backup string, delay time.Duration) error {
	if !validSystemdUnit(service) { return errors.New("invalid service name") }
	if !validManagedPath(binary) || !validManagedPath(backup) { return errors.New("invalid binary or backup path") }
	if backup != binary+".previous" { return errors.New("backup path must be the managed .previous generation") }
	if delay < time.Second { delay = time.Second }
	unit := "xport-self-update-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	// The shell program is constant; service/binary/backup are positional arguments,
	// never interpolated into shell source, and are validated above.
	out, err := exec.CommandContext(ctx, "systemd-run", "--quiet", "--unit", unit, "--on-active="+delay.String(), "/bin/sh", "-c", verifiedBinaryRestartScript(), "xport-self-update", service, binary, backup).CombinedOutput()
	if ctx.Err() != nil { return ctx.Err() }
	if err != nil { return fmt.Errorf("schedule verified restart %s: %w: %s", service, err, strings.TrimSpace(string(out))) }
	return nil
}
''')

# --- Linux disk arithmetic ----------------------------------------------
path = "internal/system/info_linux.go"
replace(path, '\ttotal := st.Blocks * uint64(st.Bsize)\n\tfree := st.Bavail * uint64(st.Bsize)', '\tif st.Bsize <= 0 { return 0, 0 }\n\tblockSize := uint64(st.Bsize)\n\ttotal := st.Blocks * blockSize\n\tfree := st.Bavail * blockSize')

# --- UI: no preset admin + PROXY warning --------------------------------
path = "web/dist/index.html"
replace(path, '<input id="login-user" autocomplete="username" value="admin" required>', '<input id="login-user" autocomplete="username" maxlength="128" required>')
replace(path, '<input id="login-pass" type="password" autocomplete="current-password" required>', '<input id="login-pass" type="password" autocomplete="current-password" maxlength="1024" required>')
path = "web/dist/v3.js"
replace(path, 'let common=`<label class="expert-check full"><input id="expert-proxy" type="checkbox" ${d.acceptProxyProtocol?\'checked\':\'\'}>接收 PROXY Protocol</label>`', 'let common=`<label class="expert-check full"><input id="expert-proxy" type="checkbox" ${d.acceptProxyProtocol?\'checked\':\'\'}>接收 PROXY Protocol<small class="field-hint">仅在可信 L4 反向代理后开启；不要直接暴露给不受信任客户端。</small></label>`')

# --- systemd low-risk sandboxing ----------------------------------------
hardening = '''NoNewPrivileges=true\nPrivateTmp=true\nPrivateDevices=true\nProtectHome=read-only\nProtectKernelTunables=true\nProtectKernelModules=true\nProtectControlGroups=true\nRestrictSUIDSGID=true\nRestrictRealtime=true\nLockPersonality=true\nSystemCallArchitectures=native'''
path = "scripts/install.sh"
replace(path, 'NoNewPrivileges=true\nPrivateTmp=true\nProtectHome=read-only', hardening, 2)
path = "cmd/xport/manager.go"
replace(path, 'NoNewPrivileges=true\nPrivateTmp=true\nProtectHome=read-only', hardening, 2)

# --- Pin first-party Actions to reviewed commits -------------------------
for path in [".github/workflows/ci.yml", ".github/workflows/release.yml"]:
    replace(path, 'uses: actions/checkout@v4', 'uses: actions/checkout@11d5960a326750d5838078e36cf38b85af677262 # v4')
    replace(path, 'uses: actions/setup-go@v5', 'uses: actions/setup-go@40f1582b2485089dde7abd97c1529aa768e1baff # v5')

# --- Security regression tests ------------------------------------------
write("internal/auth/limiter_test.go", r'''package auth

import (
	"fmt"
	"testing"
	"time"
)

func TestLoginLimiterCapacityIsBounded(t *testing.T) {
	l := newLoginLimiter(3, time.Hour, 8)
	for i := 0; i < 100; i++ { l.Fail(fmt.Sprintf("key-%d", i)) }
	l.mu.Lock(); n := len(l.entries); l.mu.Unlock()
	if n > 8 { t.Fatalf("limiter grew past capacity: %d", n) }
}

func TestLoginLimiterExpiresBuckets(t *testing.T) {
	l := newLoginLimiter(1, 5*time.Millisecond, 8)
	l.Fail("a")
	if l.Allowed("a") { t.Fatal("expected a to be limited") }
	time.Sleep(10*time.Millisecond)
	if !l.Allowed("a") { t.Fatal("expected expired bucket to be allowed") }
}
''')

write("internal/accountcfg/security_test.go", r'''package accountcfg

import (
	"strings"
	"testing"
)

func TestInputValidationBoundsAndEnums(t *testing.T) {
	if _, err := New(Input{Name: strings.Repeat("a", 81), Enabled: true, Port: 20001, Protocol: "vless", ServerName: "example.com"}, 0); err == nil { t.Fatal("expected long name to fail") }
	if _, err := New(Input{Name: "a", Enabled: true, Port: 20001, Protocol: "vless", Network: "made-up", ServerName: "example.com"}, 0); err == nil { t.Fatal("expected unknown network to fail") }
	if _, err := New(Input{Name: "a", Enabled: true, Port: 20001, Protocol: "vmess", Security: "reality"}, 0); err == nil { t.Fatal("expected VMess REALITY to fail") }
	if _, err := New(Input{Name: "a\r\nb", Enabled: true, Port: 20001, Protocol: "vless", ServerName: "example.com"}, 0); err == nil { t.Fatal("expected control characters to fail") }
}
''')

write("internal/server/security_test.go", r'''package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
	"github.com/Kerberos255/X-port/internal/service"
	"github.com/Kerberos255/X-port/internal/store"
)

func newSecurityTestServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "x.db")); if err != nil { t.Fatal(err) }
	t.Cleanup(func(){ _ = st.Close() })
	hash, _ := bcrypt.GenerateFromPassword([]byte("correct-horse-battery"), bcrypt.DefaultCost)
	if err := st.SetAdmin("admin", string(hash)); err != nil { t.Fatal(err) }
	s := New(st, service.NewAccounts(st, testApplier{}), nil, nil)
	return s, s.Handler()
}

func loginSecurityTest(t *testing.T, h http.Handler) *http.Cookie {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username":"admin","password":"correct-horse-battery"})
	req := httptest.NewRequest(http.MethodPost, "http://panel/api/login", bytes.NewReader(body))
	rec := httptest.NewRecorder(); h.ServeHTTP(rec, req)
	if rec.Code != 200 { t.Fatalf("login failed: %d %s", rec.Code, rec.Body.String()) }
	return rec.Result().Cookies()[0]
}

func TestLoginRejectsOversizedUsernameWithoutLargeLimiterKey(t *testing.T) {
	s, h := newSecurityTestServer(t)
	body, _ := json.Marshal(map[string]string{"username":strings.Repeat("x", 5000),"password":"bad"})
	req := httptest.NewRequest(http.MethodPost, "http://panel/api/login", bytes.NewReader(body)); req.RemoteAddr="203.0.113.9:1234"
	rec := httptest.NewRecorder(); h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized { t.Fatalf("got %d", rec.Code) }
	s.loginLimiter.mu.Lock()
	defer s.loginLimiter.mu.Unlock()
	for key := range s.loginLimiter.entries { if len(key) > 96 { t.Fatalf("unexpected long limiter key: %d", len(key)) } }
}

func TestUnsafeAPICrossOriginRejectedAndNoStore(t *testing.T) {
	_, h := newSecurityTestServer(t)
	cookie := loginSecurityTest(t, h)
	req := httptest.NewRequest(http.MethodPost, "http://panel/api/backups", nil); req.AddCookie(cookie); req.Header.Set("Origin","https://evil.example")
	rec := httptest.NewRecorder(); h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden { t.Fatalf("got %d", rec.Code) }
	if !strings.Contains(rec.Header().Get("Cache-Control"), "no-store") { t.Fatalf("missing no-store: %q", rec.Header().Get("Cache-Control")) }
}

func TestForwardedHeadersIgnoredFromUntrustedPeer(t *testing.T) {
	s, _ := newSecurityTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "http://panel/", nil); req.RemoteAddr="203.0.113.2:5555"; req.Header.Set("X-Forwarded-For","198.51.100.7"); req.Header.Set("X-Forwarded-Proto","https")
	if got:=s.remoteIP(req); got!="203.0.113.2" { t.Fatalf("remoteIP trusted spoofed header: %q", got) }
	if got:=s.requestScheme(req); got!="http" { t.Fatalf("scheme trusted spoofed header: %q", got) }
}
''')

print("security hardening patch applied")
