package server

import (
 "bytes"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "testing"

 "github.com/Kerberos255/X-port/internal/accountcfg"
)

type maintenanceResponseProbe struct {
 *httptest.ResponseRecorder
 s *Server
 successReleased bool
}

func (p *maintenanceResponseProbe) WriteHeader(status int) {
 if status >= 200 && status < 300 {
  if p.s.maintenanceMu.TryLock() {
   p.successReleased = true
   p.s.maintenanceMu.Unlock()
  }
 }
 p.ResponseRecorder.WriteHeader(status)
}

func TestAccountMutationsReleaseMaintenanceBeforeSuccessResponse(t *testing.T) {
 s, h := newSecurityTestServer(t)
 cookie := loginSecurityTest(t, h)
 request := func(method, path string, body any, want int) []byte {
  t.Helper()
  raw, err := json.Marshal(body)
  if err != nil { t.Fatal(err) }
  req := httptest.NewRequest(method, "http://panel"+path, bytes.NewReader(raw))
  req.AddCookie(cookie)
  req.Header.Set("Origin", "http://panel")
  req.Header.Set("Content-Type", "application/json")
  rec := &maintenanceResponseProbe{ResponseRecorder: httptest.NewRecorder(), s: s}
  h.ServeHTTP(rec, req)
  if rec.Code != want {
   t.Fatalf("%s %s: got %d, want %d: %s",method,path,rec.Code,want,rec.Body.String())
  }
  if !rec.successReleased {
   t.Fatalf("%s %s responded before releasing maintenance mutex", method,path)
  }
  return rec.Body.Bytes()
 }
 input := accountcfg.Input{Name:"ci-maintenance", Port:53981,
  Enabled:true, Protocol:"vless", Network:"tcp", Security:"none"}
 createdBytes := request(http.MethodPost, "/api/accounts", input, http.StatusCreated)
 var created struct{ ID int64 `json:"id"` }
 if err := json.Unmarshal(createdBytes, &created); err != nil || created.ID == 0 {
  t.Fatalf("unexpected creation response: %s: %v", createdBytes, err)
 }
 path := "/api/accounts/" + itoa(created.ID)
 request(http.MethodPut, path, input, http.StatusOK)
 config, err := s.accounts.Expert(created.ID)
 if err != nil { t.Fatal(err) }
 request(http.MethodPut, path+"/expert", config, http.StatusOK)
}
