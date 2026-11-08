package server

import (
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "path/filepath"
 "testing"

 "github.com/Kerberos255/X-port/internal/service"
 "github.com/Kerberos255/X-port/internal/store"
)

func TestPanelVersionExposesRotatingInstanceWithoutSession(t *testing.T) {
 st,err:=store.Open(filepath.Join(t.TempDir(),"instance.sqlite"))
 if err!=nil{t.Fatal(err)}
 defer st.Close()
 accounts:=service.NewAccounts(st,nil)
 version:=func(s *Server)(string,string){
  rec:=httptest.NewRecorder()
  s.Handler().ServeHTTP(rec,httptest.NewRequest(http.MethodGet,"http://panel/api/xport/version",nil))
  if rec.Code!=http.StatusOK{t.Fatalf("unauthenticated version: %d %s",rec.Code,rec.Body.String())}
  var data struct{Version string `json:"version"`;Instance string `json:"instance"`}
  if err:=json.Unmarshal(rec.Body.Bytes(),&data);err!=nil{t.Fatal(err)}
  if data.Version==""||len(data.Instance)<16{t.Fatalf("incomplete version probe: %+v",data)}
  return data.Version,data.Instance
 }
 a:=New(st,accounts,nil,nil)
 v1,instance1:=version(a)
 vAgain,instanceAgain:=version(a)
 if vAgain!=v1||instanceAgain!=instance1{t.Fatalf("instance not stable within one process: %q vs %q",instance1,instanceAgain)}
 b:=New(st,accounts,nil,nil)
 v2,instance2:=version(b)
 if v2!=v1{t.Fatalf("version changed across identical processes: %q vs %q",v1,v2)}
 if instance1==instance2{t.Fatal("new process reused the previous instance ID")}
}
