package server

import (
 "bytes"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "golang.org/x/crypto/bcrypt"
 "github.com/Kerberos255/X-port/internal/service"
 "github.com/Kerberos255/X-port/internal/store"
)

// External firewalld rules must not become implicitly owned by X-port.
func TestExternalFirewallRuleCannotBeDeletedThroughWeb(t *testing.T) {
 dir:=t.TempDir()
 marker:=filepath.Join(dir,"unexpected-firewall-write")
 t.Setenv("XPORT_TEST_APPLY_MARKER",marker)
 script:=strings.Join([]string{
  "#!/bin/sh",
  "case \" $* \" in",
  "  *\" firewall-inspect\"*)",
  "    printf '{\"backend\":\"firewalld\",\"active\":true,\"writable\":true,\"zones\":[\"public\"],\"rules\":[{\"id\":\"firewalld:public:53114/tcp\",\"port\":53114,\"protocol\":\"tcp\",\"zone\":\"public\",\"backend\":\"firewalld\",\"runtime\":true,\"permanent\":true}]}'",
  "    ;;",
  "  *\" firewall-listeners\"*)",
  "    printf '{\"53114\":true}'",
  "    ;;",
  "  *\" firewall-apply\"*)",
  "    touch \"$XPORT_TEST_APPLY_MARKER\"; exit 95",
  "    ;;",
  "  *) exit 96 ;;",
  "esac",
 },"\n")+"\n"
 if err:=os.WriteFile(filepath.Join(dir,"systemd-run"),[]byte(script),0755);err!=nil{t.Fatal(err)}
 t.Setenv("PATH",dir+string(os.PathListSeparator)+os.Getenv("PATH"))
 st,err:=store.Open(filepath.Join(dir,"x.db"));if err!=nil{t.Fatal(err)}
 defer st.Close()
 hash,err:=bcrypt.GenerateFromPassword([]byte("ci-test-only-pass"),bcrypt.MinCost)
 if err!=nil{t.Fatal(err)}
 if err:=st.SetAdmin("admin",string(hash));err!=nil{t.Fatal(err)}
 h:=New(st,service.NewAccounts(st,testApplier{}),nil,nil).Handler()
 credential,_:=json.Marshal(map[string]string{"username":"admin","password":"ci-test-only-pass"})
 login:=httptest.NewRecorder()
 h.ServeHTTP(login,httptest.NewRequest(http.MethodPost,"http://panel/api/login",bytes.NewReader(credential)))
 if login.Code!=http.StatusOK{t.Fatalf("login: %d %s",login.Code,login.Body.String())}
 cookies:=login.Result().Cookies()
 if len(cookies)==0{t.Fatal("no session cookie")}

 postRemove:=func() *httptest.ResponseRecorder{
  body:=strings.NewReader("{\"action\":\"remove\",\"port\":53114,\"protocol\":\"tcp\",\"zone\":\"public\"}")
  req:=httptest.NewRequest(http.MethodPost,"http://panel/api/firewall/rules",body)
  req.Header.Set("Origin","http://panel")
  req.Header.Set("Content-Type","application/json")
  req.AddCookie(cookies[0])
  rr:=httptest.NewRecorder()
  h.ServeHTTP(rr,req)
  return rr
 }
 // An independent manager added this port. No X-port ownership record exists.
 rec:=postRemove()
 if rec.Code!=http.StatusForbidden{t.Fatalf("external rule must be forbidden: %d %s",rec.Code,rec.Body.String())}
 // The listener safeguard applies even if metadata incorrectly says managed.
 if err:=st.SetSetting("firewall_owned:firewalld:public:53114/tcp","v1");err!=nil{t.Fatal(err)}
 rec=postRemove()
 if rec.Code!=http.StatusConflict{t.Fatalf("listening port must be blocked: %d %s",rec.Code,rec.Body.String())}
 if _,err:=os.Stat(marker);!os.IsNotExist(err){t.Fatalf("write attempted despite Web rejection: %v",err)}
}
