package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kerberos255/X-port/internal/service"
	"github.com/Kerberos255/X-port/internal/store"
	"golang.org/x/crypto/bcrypt"
)

func newSecurityTestServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	hash, _ := bcrypt.GenerateFromPassword([]byte("correct-horse-battery"), bcrypt.DefaultCost)
	if err := st.SetAdmin("admin", string(hash)); err != nil {
		t.Fatal(err)
	}
	s := New(st, service.NewAccounts(st, testApplier{}), nil, nil)
	return s, s.Handler()
}

func loginSecurityTest(t *testing.T, h http.Handler) *http.Cookie {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "correct-horse-battery"})
	req := httptest.NewRequest(http.MethodPost, "http://panel/api/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("login failed: %d %s", rec.Code, rec.Body.String())
	}
	return rec.Result().Cookies()[0]
}

func TestLoginRejectsOversizedUsernameWithoutLargeLimiterKey(t *testing.T) {
	_, h := newSecurityTestServer(t)
	username := strings.Repeat("x", 5000)
	body, _ := json.Marshal(map[string]string{"username": username, "password": "bad"})
	req := httptest.NewRequest(http.MethodPost, "http://panel/api/login", bytes.NewReader(body))
	req.RemoteAddr = "203.0.113.9:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", rec.Code)
	}
	if key := loginRateKey("203.0.113.9", username); len(key) > 96 {
		t.Fatalf("unexpected long limiter key: %d", len(key))
	}
}

func TestUnsafeAPICrossOriginRejectedAndNoStore(t *testing.T) {
	_, h := newSecurityTestServer(t)
	cookie := loginSecurityTest(t, h)
	req := httptest.NewRequest(http.MethodPost, "http://panel/api/backups", nil)
	req.AddCookie(cookie)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("missing no-store: %q", rec.Header().Get("Cache-Control"))
	}
}

func TestForwardedHeadersIgnoredFromUntrustedPeer(t *testing.T) {
	s, _ := newSecurityTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "http://panel/", nil)
	req.RemoteAddr = "203.0.113.2:5555"
	req.Header.Set("X-Forwarded-For", "198.51.100.7")
	req.Header.Set("X-Forwarded-Proto", "https")
	if got := s.remoteIP(req); got != "203.0.113.2" {
		t.Fatalf("remoteIP trusted spoofed header: %q", got)
	}
	if got := s.requestScheme(req); got != "http" {
		t.Fatalf("scheme trusted spoofed header: %q", got)
	}
}
