package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kerberos255/X-port/internal/service"
 "github.com/Kerberos255/X-port/internal/buildinfo"
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

func TestVersionProbeAuthenticatedAndPendingGuard(t *testing.T){
 s,h:=newSecurityTestServer(t)
 request:=func(path string,cookie *http.Cookie)(int,map[string]any,string){
  req:=httptest.NewRequest(http.MethodGet,"http://panel"+path,nil)
  if cookie!=nil{req.AddCookie(cookie)}
  rr:=httptest.NewRecorder();h.ServeHTTP(rr,req)
  var out map[string]any
  _=json.Unmarshal(rr.Body.Bytes(),&out)
  return rr.Code,out,rr.Header().Get("Cache-Control")
 }
 // Session storage is process-local. The public version and random process
 // generation must be readable after restart, without disclosing firewall state.
 code,v,cache:=request("/api/xport/version",nil)
 instance,ok:=v["instance"].(string)
 if code!=200||v["version"]!=buildinfo.Current||!ok||len(instance)!=32||
  len(v)!=2||!strings.Contains(cache,"no-store"){
  t.Fatalf("public version=%d %v cache=%q",code,v,cache)
 }
 if code,_,_:=request("/api/xport/update-state",nil);code!=http.StatusUnauthorized{
  t.Fatalf("pending-state endpoint must require login: %d",code)
 }
 cookie:=loginSecurityTest(t,h)
 state:=func() map[string]any{
  code,result,cache:=request("/api/xport/update-state",cookie)
  if code!=200||!strings.Contains(cache,"no-store")||len(result)!=1{
   t.Fatalf("guard=%d %v cache=%q",code,result,cache)
  }
  return result
 }
 if result:=state();result["firewallPending"]!=false{t.Fatalf("initial pending: %v",result)}
 if err:=s.store.SetSetting("firewall_pending:test","unparseable-but-still-pending");err!=nil{t.Fatal(err)}
 if result:=state();result["firewallPending"]!=true{t.Fatalf("invalid metadata must fail closed: %v",result)}
 if err:=s.store.SetSetting("firewall_pending:test","");err!=nil{t.Fatal(err)}
 if result:=state();result["firewallPending"]!=false{t.Fatalf("pending must clear: %v",result)}
 future,_:=json.Marshal(firewallPending{ID:"test",Unit:"xport-ci.timer",Deadline:time.Now().Add(60*time.Second).UnixMilli()})
 if err:=s.store.SetSetting("firewall_pending:test",string(future));err!=nil{t.Fatal(err)}
 if result:=state();result["firewallPending"]!=true{t.Fatalf("active recovery blocks update: %v",result)}
 expired,_:=json.Marshal(firewallPending{ID:"test",Unit:"xport-ci.timer",Deadline:time.Now().Add(-3*time.Minute).UnixMilli()})
 if err:=s.store.SetSetting("firewall_pending:test",string(expired));err!=nil{t.Fatal(err)}
 if result:=state();result["firewallPending"]!=false{t.Fatalf("old recovery metadata must expire: %v",result)}
 // Even a logged-out client sees only the version, never pending metadata.
 code,v,_=request("/api/xport/version",nil)
 if code!=200||len(v)!=1||v["version"]!=buildinfo.Current{t.Fatalf("state leaked in public version: %v",v)}
}

func TestUpdatePanelRejectsPendingWithoutStaging(t *testing.T){
 s,h:=newSecurityTestServer(t)
 s.panelService="xport.service"
 raw,_:=json.Marshal(firewallPending{ID:"firewalld:public:test",Unit:"xport-test.timer",
  Deadline:time.Now().Add(2*time.Minute).UnixMilli()})
 if err:=s.store.SetSetting("firewall_pending:test",string(raw));err!=nil{t.Fatal(err)}
 cookie:=loginSecurityTest(t,h)
 request:=httptest.NewRequest(http.MethodPost,"http://panel/api/xport/update",
  strings.NewReader("{}"))
 request.AddCookie(cookie)
 request.Header.Set("Origin","http://panel")
 request.Header.Set("Content-Type","application/json")
 rec:=httptest.NewRecorder()
 h.ServeHTTP(rec,request)
 if rec.Code!=http.StatusConflict||!strings.Contains(rec.Body.String(),"firewall"){
  t.Fatalf("pending firewall must reject update before Stage: %d %s",rec.Code,rec.Body.String())
 }
}
