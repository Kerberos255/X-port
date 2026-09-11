package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/Kerberos255/X-port/internal/accountcfg"
	"github.com/Kerberos255/X-port/internal/model"
	"github.com/Kerberos255/X-port/internal/service"
	"github.com/Kerberos255/X-port/internal/store"
)

type testApplier struct{}

func (testApplier) Apply([]model.Account) error { return nil }

func TestLoginShareAndQR(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	hash, _ := bcrypt.GenerateFromPassword([]byte("correct-horse-battery"), bcrypt.DefaultCost)
	if err := st.SetAdmin("admin", string(hash)); err != nil {
		t.Fatal(err)
	}
	accounts := service.NewAccounts(st, testApplier{})
	a, err := accounts.Create(accountcfg.Input{Name: "Alpha", Enabled: true, Port: 21001, ServerName: "www.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	h := New(st, accounts, nil, nil).Handler()
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "correct-horse-battery"})
	req := httptest.NewRequest(http.MethodPost, "http://panel/api/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("login: %d %s", rec.Code, rec.Body.String())
	}
	cookie := rec.Result().Cookies()[0]
	req = httptest.NewRequest(http.MethodGet, "http://panel/api/accounts/"+itoa(a.ID)+"/share?host=proxy.example.com", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "vless://") {
		t.Fatalf("share: %d %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "http://panel/api/accounts/"+itoa(a.ID)+"/qr?host=proxy.example.com", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || !bytes.HasPrefix(rec.Body.Bytes(), []byte("\x89PNG")) {
		t.Fatalf("qr: %d %q", rec.Code, rec.Body.Bytes()[:min(8, rec.Body.Len())])
	}
}
func itoa(v int64) string {
	const digits = "0123456789"
	if v == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = digits[v%10]
		v /= 10
	}
	return string(b[i:])
}
