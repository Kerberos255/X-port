package server

import (
 "bytes"
 "encoding/json"
 "fmt"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "golang.org/x/crypto/bcrypt"
 "github.com/Kerberos255/X-port/internal/firewall"
 "github.com/Kerberos255/X-port/internal/model"
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
  "out=''",
  "for arg do case \"$arg\" in --property=StandardOutput=file:*) out=${arg#--property=StandardOutput=file:};; esac; done",
  "test -n \"$out\" || exit 96",
  "exec 1>\"$out\"",
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

 // Upcoming policy/edit/toggle verbs must never fall back to the old
 // "allow" mutation path before their own guarded transactions exist.
 for _,action:=range []string{"deny","drop","toggle","pause","resume","edit","update"}{
  body:=strings.NewReader(fmt.Sprintf(`{"action":%q,"port":53114,"protocol":"tcp","zone":"public"}`,action))
  req:=httptest.NewRequest(http.MethodPost,"http://panel/api/firewall/rules",body)
  req.Header.Set("Origin","http://panel")
  req.Header.Set("Content-Type","application/json")
  req.AddCookie(cookies[0])
  res:=httptest.NewRecorder()
  h.ServeHTTP(res,req)
  if res.Code!=http.StatusBadRequest{t.Fatalf("%s must remain unsupported: %d %s",action,res.Code,res.Body.String())}
  if _,err:=os.Stat(marker);!os.IsNotExist(err){t.Fatalf("%s unexpectedly changed firewall: %v",action,err)}
 }
}

func TestFirewallReadMapsAccountsAndListenerUsage(t *testing.T) {
 dir:=t.TempDir()
 script:=strings.Join([]string{
  "#!/bin/sh",
  "out=''",
  "for arg do case \"$arg\" in --property=StandardOutput=file:*) out=${arg#--property=StandardOutput=file:};; esac; done",
  "test -n \"$out\" || exit 96",
  "exec 1>\"$out\"",
  "case \" $* \" in",
  "  *\" firewall-inspect\"*)",
  "    printf '%s' '{\"backend\":\"firewalld\",\"active\":true,\"writable\":true,\"zones\":[\"public\"],\"rules\":[{\"id\":\"firewalld:public:2053/tcp\",\"port\":2053,\"protocol\":\"tcp\",\"zone\":\"public\",\"backend\":\"firewalld\",\"runtime\":true,\"permanent\":true},{\"id\":\"firewalld:public:8443/tcp\",\"port\":8443,\"protocol\":\"tcp\",\"zone\":\"public\",\"backend\":\"firewalld\",\"runtime\":true,\"permanent\":true},{\"id\":\"firewalld:public:2053/udp\",\"port\":2053,\"protocol\":\"udp\",\"zone\":\"public\",\"backend\":\"firewalld\",\"runtime\":true,\"permanent\":true}]}'",
  "    ;;",
  "  *\" firewall-listeners\"*) printf '%s' '{\"2053\":true}' ;;",
  "  *\" firewall-sockets\"*)",
  "    if [ \"${XPORT_TEST_SOCKET_ERROR:-}\" = 1 ]; then exit 98; fi",
  "    printf '%s' '{\"tcp/2053\":true}' ;;",
  "  *) exit 97 ;;",
  "esac",
 },"\n")+"\n"
 if err:=os.WriteFile(filepath.Join(dir,"systemd-run"),[]byte(script),0755);err!=nil{t.Fatal(err)}
 t.Setenv("PATH",dir+string(os.PathListSeparator)+os.Getenv("PATH"))
 st,err:=store.Open(filepath.Join(dir,"x.db"));if err!=nil{t.Fatal(err)};defer st.Close()
 err=st.ReplaceAccounts([]model.Account{
  {ID:1,Name:"Active inbound",Enabled:true,Port:2053,Protocol:"vless",Tag:"test-inbound-active"},
  {ID:2,Name:"Paused inbound",Enabled:false,Port:8443,Protocol:"trojan",Tag:"test-inbound-disabled"},
 })
 if err!=nil{t.Fatal(err)}
 hash,err:=bcrypt.GenerateFromPassword([]byte("ci-test-only-pass"),bcrypt.MinCost);if err!=nil{t.Fatal(err)}
 if err:=st.SetAdmin("admin",string(hash));err!=nil{t.Fatal(err)}
 h:=New(st,service.NewAccounts(st,testApplier{}),nil,nil).Handler()
 credential,_:=json.Marshal(map[string]string{"username":"admin","password":"ci-test-only-pass"})
 login:=httptest.NewRecorder()
 h.ServeHTTP(login,httptest.NewRequest(http.MethodPost,"http://panel/api/login",bytes.NewReader(credential)))
 if login.Code!=http.StatusOK{t.Fatalf("login: %d %s",login.Code,login.Body.String())}
 cookies:=login.Result().Cookies();if len(cookies)==0{t.Fatal("no session")}
 req:=httptest.NewRequest(http.MethodGet,"http://panel/api/firewall",nil)
 req.AddCookie(cookies[0])
 response:=httptest.NewRecorder();h.ServeHTTP(response,req)
 if response.Code!=http.StatusOK{t.Fatalf("firewall: %d %s",response.Code,response.Body.String())}
 var payload struct{Rules []struct{
  ID string `json:"id"`
  AccountID int64 `json:"accountId"`
  AccountName string `json:"accountName"`
  AccountEnabled bool `json:"accountEnabled"`
  Usage string `json:"usage"`
  Protected bool `json:"protected"`
 }}
 if err:=json.Unmarshal(response.Body.Bytes(),&payload);err!=nil{t.Fatal(err)}
 if len(payload.Rules)!=3{t.Fatalf("rules: %+v",payload.Rules)}
 byPort:=map[string]struct{ID int64;Name,Usage string;Enabled,Protected bool}{
  "firewalld:public:2053/tcp":{1,"Active inbound","used",true,true},
  "firewalld:public:8443/tcp":{2,"Paused inbound","unused",false,false},
  "firewalld:public:2053/udp":{0,"","unused",false,true},
 }
 for _,rule:=range payload.Rules{
  want,ok:=byPort[rule.ID]
  if !ok||rule.AccountID!=want.ID||rule.AccountName!=want.Name||
     rule.AccountEnabled!=want.Enabled||rule.Usage!=want.Usage||
     rule.Protected!=want.Protected{t.Fatalf("unexpected rule: %+v",rule)}
 }

 // An ss failure must never make ports appear idle and safe to remove.
 t.Setenv("XPORT_TEST_SOCKET_ERROR","1")
 unknown:=httptest.NewRecorder()
 h.ServeHTTP(unknown,req)
 if unknown.Code!=http.StatusOK{t.Fatalf("socket failure GET: %d %s",unknown.Code,unknown.Body.String())}
 var fallback struct{Rules []struct{
  Usage string `json:"usage"`
  Protected bool `json:"protected"`
 }}
 if err:=json.Unmarshal(unknown.Body.Bytes(),&fallback);err!=nil{t.Fatal(err)}
 if len(fallback.Rules)!=3{t.Fatalf("fallback rules: %+v",fallback.Rules)}
 for _,item:=range fallback.Rules{
  if item.Usage!="unknown"||!item.Protected{t.Fatalf("failed socket inspection must be unknown and protected: %+v",item)}
 }

 // A confirmed paused DROP is absent from firewalld but must remain
 // visible and resumable in X-port's read model. This caught the CentOS
 // E2E mismatch after the real DROP was confirmed and then paused.
 t.Setenv("XPORT_TEST_SOCKET_ERROR","")
 paused:=firewall.Rule{
  ID:"firewalld:public:rich:53342/tcp:deny:both",Port:53342,Protocol:"tcp",
  Zone:"public",Backend:"firewalld",Kind:"rich",Action:"deny",Family:"both",
  Runtime:true,Permanent:true,
 }
 encoded,err:=json.Marshal(paused);if err!=nil{t.Fatal(err)}
 if err:=st.SetSetting("firewall_owned:"+paused.ID,"v1");err!=nil{t.Fatal(err)}
 if err:=st.SetSetting("firewall_paused:"+paused.ID,string(encoded));err!=nil{t.Fatal(err)}
 pausedResponse:=httptest.NewRecorder()
 h.ServeHTTP(pausedResponse,req)
 if pausedResponse.Code!=http.StatusOK{t.Fatalf("paused read: %d %s",pausedResponse.Code,pausedResponse.Body.String())}
 var view struct{Rules []struct{
  ID string `json:"id"`
  Action string `json:"action"`
  Kind string `json:"kind"`
  Family string `json:"family"`
  Paused bool `json:"paused"`
  Managed bool `json:"managed"`
  Runtime bool `json:"runtime"`
  Permanent bool `json:"permanent"`
  CanToggle bool `json:"canToggle"`
 }}
 if err:=json.Unmarshal(pausedResponse.Body.Bytes(),&view);err!=nil{t.Fatal(err)}
 count:=0
 for _,rule:=range view.Rules{
  if rule.ID!=paused.ID{continue}
  count++
  if !rule.Paused||!rule.Managed||rule.Runtime||rule.Permanent||
     !rule.CanToggle||rule.Kind!="rich"||rule.Action!="deny"||rule.Family!="both"{
   t.Fatalf("paused DROP must be visible and inactive: %+v",rule)
  }
 }
 if count!=1{t.Fatalf("paused DROP absent or duplicated: count=%d rules=%+v",count,view.Rules)}
}

func TestAccountFirewallTransportAndPortCollisions(t *testing.T){
 accounts:=[]model.Account{
  {ID:1,Name:"TCP-only",Port:10401,Protocol:"vless",StreamSettingsJSON:`{"network":"xhttp"}`},
  {ID:2,Name:"UDP-only",Port:10401,Protocol:"vmess",StreamSettingsJSON:`{"network":"quic"}`},
  {ID:3,Name:"second TCP",Port:10401,Protocol:"trojan",StreamSettingsJSON:`{"network":"tcp"}`},
  {ID:4,Name:"unrecognized transport",Port:20000,Protocol:"vless",StreamSettingsJSON:`{"network":"future-protocol"}`},
  {ID:5,Name:"malformed transport",Port:20001,Protocol:"vless",StreamSettingsJSON:"{"},
  {ID:6,Name:"SS both",Port:20002,Protocol:"shadowsocks",SettingsJSON:`{"network":"tcp,udp"}`},
  {ID:7,Name:"Socks UDP on",Port:20003,Protocol:"socks",SettingsJSON:`{"udp":true}`},
  {ID:8,Name:"SS unknown",Port:20004,Protocol:"shadowsocks",SettingsJSON:`{"network":"future"}`},
 }
 got:=accountFirewallMatches(accounts)
 if len(got["tcp/10401"])!=2||len(got["udp/10401"])!=1{t.Fatalf("wrong network mapping: %+v",got)}
 if got["udp/10401"][0].ID!=2{t.Fatalf("wrong UDP account: %+v",got["udp/10401"])}
 if got["tcp/20000"]!=nil||got["tcp/20001"]!=nil{t.Fatalf("unknown transport was guessed: %+v",got)}
 for _,port:=range []int{20002,20003}{
  for _,proto:=range []string{"tcp","udp"}{
   key:=fmt.Sprintf("%s/%d",proto,port)
   if len(got[key])!=1{t.Fatalf("multi-transport account missing %s: %+v",key,got)}
  }
 }
 if got["tcp/20004"]!=nil||got["udp/20004"]!=nil{t.Fatalf("unknown SS network guessed: %+v",got)}
}
