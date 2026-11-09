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

 "github.com/Kerberos255/X-port/internal/firewall"
 "github.com/Kerberos255/X-port/internal/model"
 "github.com/Kerberos255/X-port/internal/service"
 "github.com/Kerberos255/X-port/internal/store"
 "golang.org/x/crypto/bcrypt"
)

func externalRemoveFixture(t *testing.T, rule firewall.Rule, listening, discovery string) (http.Handler, *store.Store, string) {
 t.Helper()
 dir:=t.TempDir()
 removed:=filepath.Join(dir,"removed")
 t.Setenv("XPORT_TEST_REMOVED",removed)
 snap:=firewall.Snapshot{Backend:"firewalld",Active:true,Writable:true,Zones:[]string{"public"},Rules:[]firewall.Rule{rule}}
 first,err:=json.Marshal(snap);if err!=nil{t.Fatal(err)}
 snap.Rules=[]firewall.Rule{}
 empty,err:=json.Marshal(snap);if err!=nil{t.Fatal(err)}
 t.Setenv("XPORT_TEST_RULE_JSON",string(first))
 t.Setenv("XPORT_TEST_EMPTY_JSON",string(empty))
 t.Setenv("XPORT_TEST_LISTENERS",listening)
 t.Setenv("XPORT_TEST_DISCOVERY",discovery)
 script:=strings.Join([]string{
  "#!/bin/sh",
  "out=''",
  "for arg do case \"$arg\" in --property=StandardOutput=file:*) out=$(printf '%s' \"$arg\" | sed 's/^--property=StandardOutput=file://');; esac; done",
  "if [ -z \"$out\" ]; then case \" $* \" in *\" --on-active=120s \"*) exit 0;; esac; exit 96; fi",
  "exec 1>\"$out\"",
  "case \" $* \" in",
  " *\" firewall-inspect\"*) if [ -e \"$XPORT_TEST_REMOVED\" ]; then printf '%s' \"$XPORT_TEST_EMPTY_JSON\"; else printf '%s' \"$XPORT_TEST_RULE_JSON\"; fi ;;",
  " *\" firewall-listeners\"*) printf '%s' \"$XPORT_TEST_LISTENERS\" ;;",
  " *\" firewall-discovery\"*) printf '%s' \"$XPORT_TEST_DISCOVERY\" ;;",
  " *\" firewall-apply remove \"*) touch \"$XPORT_TEST_REMOVED\" ;;",
  " *) exit 96 ;;",
  "esac",
 },"\n")+"\n"
 for name,contents:=range map[string]string{
  "systemd-run":script,
  "firewall-cmd":"#!/bin/sh\nexit 0\n",
  "systemctl":"#!/bin/sh\nexit 0\n",
 }{
  if err:=os.WriteFile(filepath.Join(dir,name),[]byte(contents),0755);err!=nil{t.Fatal(err)}
 }
 t.Setenv("PATH",dir+string(os.PathListSeparator)+os.Getenv("PATH"))
 st,err:=store.Open(filepath.Join(dir,"x.db"));if err!=nil{t.Fatal(err)}
 hash,err:=bcrypt.GenerateFromPassword([]byte("ci-only-test-password"),bcrypt.MinCost);if err!=nil{t.Fatal(err)}
 if err:=st.SetAdmin("admin",string(hash));err!=nil{t.Fatal(err)}
 h:=New(st,service.NewAccounts(st,testApplier{}),nil,nil).Handler()
 creds,_:=json.Marshal(map[string]string{"username":"admin","password":"ci-only-test-password"})
 login:=httptest.NewRecorder()
 h.ServeHTTP(login,httptest.NewRequest(http.MethodPost,"http://panel/api/login",bytes.NewReader(creds)))
 if login.Code!=200{t.Fatalf("login failed: %d %s",login.Code,login.Body.String())}
 cookies:=login.Result().Cookies()
 if len(cookies)==0{t.Fatal("missing session cookie")}
 return h,st,cookies[0].String()
}

func externalRemoveRequest(t *testing.T,h http.Handler,cookie string,body any)*httptest.ResponseRecorder{
 t.Helper()
 data,err:=json.Marshal(body);if err!=nil{t.Fatal(err)}
 req:=httptest.NewRequest(http.MethodPost,"http://panel/api/firewall/external/remove",bytes.NewReader(data))
 req.Header.Set("Content-Type","application/json")
 req.Header.Set("Origin","http://panel")
 req.Header.Set("Cookie",cookie)
 rec:=httptest.NewRecorder()
 h.ServeHTTP(rec,req)
 return rec
}

func TestExternalFirewallDeleteRequiresConsentAndRevision(t *testing.T){
 source:=firewall.Rule{ID:"firewalld:public:53114/tcp",Port:53114,Protocol:"tcp",Zone:"public",
  Backend:"firewalld",Runtime:true,Permanent:true,Kind:"port",Action:"allow",Family:"both"}
 h,st,cookie:=externalRemoveFixture(t,source,"{}",`{"sshPorts":[22],"docker":[]}`)
 defer st.Close()
 rev:=firewallRuleRevision(source)
 for _,tc:=range []struct{name string;body any;code int}{
  {"consent",map[string]any{"id":source.ID,"revision":rev},400},
  {"stale",map[string]any{"id":source.ID,"revision":strings.Repeat("a",24),"externalAcknowledged":true},409},
 }{
  t.Run(tc.name,func(t *testing.T){
   res:=externalRemoveRequest(t,h,cookie,tc.body)
   if res.Code!=tc.code{t.Fatalf("status %d wanted %d: %s",res.Code,tc.code,res.Body.String())}
   if _,err:=os.Stat(os.Getenv("XPORT_TEST_REMOVED"));!os.IsNotExist(err){t.Fatalf("unexpected native deletion: %v",err)}
  })
 }
 res:=externalRemoveRequest(t,h,cookie,map[string]any{"id":source.ID,"revision":rev,"externalAcknowledged":true})
 if res.Code!=http.StatusAccepted{t.Fatalf("delete status %d: %s",res.Code,res.Body.String())}
 var pending struct{
  Pending bool `json:"pending"`
  ID string `json:"id"`
  Token string `json:"token"`
 }
 if err:=json.Unmarshal(res.Body.Bytes(),&pending);err!=nil{t.Fatal(err)}
 if !pending.Pending||pending.ID!=source.ID||len(pending.Token)!=32{t.Fatalf("invalid pending state: %+v",pending)}
 if _,err:=os.Stat(os.Getenv("XPORT_TEST_REMOVED"));err!=nil{t.Fatalf("native deletion not invoked: %v",err)}
 stored,ok,err:=st.Setting(firewallPendingKey(source.ID))
 if err!=nil||!ok||stored==""{t.Fatalf("pending recovery not persisted: %v %t",err,ok)}
 again:=externalRemoveRequest(t,h,cookie,map[string]any{"id":source.ID,"revision":rev,"externalAcknowledged":true})
 if again.Code!=http.StatusConflict{t.Fatalf("second deletion not blocked: %d %s",again.Code,again.Body.String())}
 data,_:=json.Marshal(map[string]string{"id":pending.ID,"token":pending.Token})
 req:=httptest.NewRequest(http.MethodPost,"http://panel/api/firewall/rules/confirm",bytes.NewReader(data))
 req.Header.Set("Content-Type","application/json")
 req.Header.Set("Origin","http://panel")
 req.Header.Set("Cookie",cookie)
 confirmation:=httptest.NewRecorder()
 h.ServeHTTP(confirmation,req)
 if confirmation.Code!=200{t.Fatalf("confirm status %d: %s",confirmation.Code,confirmation.Body.String())}
 raw,_,err:=st.Setting(firewallPendingKey(source.ID))
 if err!=nil||raw!=""{t.Fatalf("pending record not cleared after confirm: %s %v",raw,err)}
}

func TestExternalFirewallDeleteRefusesLiveAndComplexRules(t *testing.T){
 base:=firewall.Rule{ID:"firewalld:public:53114/tcp",Port:53114,Protocol:"tcp",
  Zone:"public",Backend:"firewalld",Runtime:true,Permanent:true,Kind:"port",Action:"allow",Family:"both"}
 for _,tc:=range []struct{name string;rule firewall.Rule;listeners,discovery string;status int}{
  {"listener",base,`{"53114":true}`,`{"sshPorts":[22],"docker":[]}`,409},
  {"docker",base,"{}",`{"sshPorts":[22],"docker":[{"port":53114,"protocol":"tcp"}]}`,409},
  {"ssh",base,"{}",`{"sshPorts":[53114],"docker":[]}`,409},
  {"rich",func()firewall.Rule{r:=base;r.Kind="rich";return r}(),"{}",`{"sshPorts":[22],"docker":[]}`,409},
  {"not-persistent",func()firewall.Rule{r:=base;r.Permanent=false;return r}(),"{}",`{"sshPorts":[22],"docker":[]}`,409},
  {"conflict",func()firewall.Rule{r:=base;r.Conflict=true;return r}(),"{}",`{"sshPorts":[22],"docker":[]}`,409},
  {"foreign-family",func()firewall.Rule{r:=base;r.Family="ipv6";return r}(),"{}",`{"sshPorts":[22],"docker":[]}`,409},
  {"foreign-backend",func()firewall.Rule{r:=base;r.Backend="ufw";return r}(),"{}",`{"sshPorts":[22],"docker":[]}`,409},
  {"identity",func()firewall.Rule{r:=base;r.ID="firewalld:public:53115/tcp";return r}(),"{}",`{"sshPorts":[22],"docker":[]}`,409},
 }{
  t.Run(tc.name,func(t *testing.T){
   h,st,cookie:=externalRemoveFixture(t,tc.rule,tc.listeners,tc.discovery);defer st.Close()
   res:=externalRemoveRequest(t,h,cookie,map[string]any{
    "id":tc.rule.ID,"revision":firewallRuleRevision(tc.rule),"externalAcknowledged":true,
   })
   if res.Code!=tc.status{t.Fatalf("status %d wanted %d: %s",res.Code,tc.status,res.Body.String())}
   if _,err:=os.Stat(os.Getenv("XPORT_TEST_REMOVED"));!os.IsNotExist(err){
    t.Fatalf("dangerous rule mutated: %v",err)
   }
  })
 }
}


// A configured but inactive Xray listener may be absent from ss. Removing its
// externally-created allow rule would break it when the account is enabled.
func TestExternalFirewallDeleteProtectsDormantXrayAccount(t *testing.T){
 source:=firewall.Rule{ID:"firewalld:public:53114/tcp",Port:53114,
  Protocol:"tcp",Zone:"public",Backend:"firewalld",
  Runtime:true,Permanent:true,Kind:"port",Action:"allow",Family:"both"}
 h,st,cookie:=externalRemoveFixture(t,source,"{}",`{"sshPorts":[22],"docker":[]}`)
 defer st.Close()
 err:=st.ReplaceAccounts([]model.Account{{
  ID:1,Name:"Dormant VLESS",Tag:"ci-dormant-inbound",Protocol:"vless",
  Port:53114,Enabled:false,
 }})
 if err!=nil{t.Fatal(err)}
 res:=externalRemoveRequest(t,h,cookie,map[string]any{
  "id":source.ID,"revision":firewallRuleRevision(source),"externalAcknowledged":true,
 })
 if res.Code!=http.StatusConflict{
  t.Fatalf("dormant Xray inbound must remain protected: %d %s",res.Code,res.Body.String())
 }
 if _,err:=os.Stat(os.Getenv("XPORT_TEST_REMOVED"));!os.IsNotExist(err){
  t.Fatalf("configured Xray port was deleted: %v",err)
 }
}
