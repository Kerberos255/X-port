package server

import (
 "bytes"
 "archive/zip"
 "strings"
 "encoding/json"
 "io"
 "net/http"
 "net/http/httptest"
 "path/filepath"
 "testing"
 "time"

 "golang.org/x/crypto/bcrypt"

 "github.com/Kerberos255/X-port/internal/accountcfg"
 "github.com/Kerberos255/X-port/internal/service"
 "github.com/Kerberos255/X-port/internal/store"
)

// Real handlers + temporary SQLite + real gzip files; no host services touched.
func TestBackupLifecycleHTTP(t *testing.T) {
 dir:=t.TempDir()
 st,err:=store.Open(filepath.Join(dir,"xport-test.sqlite"))
 if err!=nil{t.Fatal(err)}
 defer st.Close()
 hash,err:=bcrypt.GenerateFromPassword([]byte("test-password-only"),bcrypt.MinCost)
 if err!=nil{t.Fatal(err)}
 if err=st.SetAdmin("admin",string(hash));err!=nil{t.Fatal(err)}
 accounts:=service.NewAccounts(st,testApplier{})
 original,err:=accounts.Create(accountcfg.Input{
  Name:"backup-original",Enabled:true,Port:25881,ServerName:"example.com",
 })
 if err!=nil{t.Fatal(err)}
 h:=New(st,accounts,nil,nil).WithRuntime(RuntimeOptions{DataDir:dir}).Handler()
 request:=func(method,path string,body []byte,cookie *http.Cookie)*httptest.ResponseRecorder{
  req:=httptest.NewRequest(method,"http://panel"+path,bytes.NewReader(body))
  if cookie!=nil{req.AddCookie(cookie)}
  if body!=nil{req.Header.Set("Content-Type","application/json")}
  rec:=httptest.NewRecorder()
  h.ServeHTTP(rec,req)
  return rec
 }
 login:=func()*http.Cookie{
  rec:=request(http.MethodPost,"/api/login",[]byte(`{"username":"admin","password":"test-password-only"}`),nil)
  if rec.Code!=http.StatusOK{t.Fatalf("login: %d %s",rec.Code,rec.Body.String())}
  cookies:=rec.Result().Cookies()
  if len(cookies)==0{t.Fatal("login cookie missing")}
  return cookies[0]
 }
 cookie:=login()
 create:=request(http.MethodPost,"/api/backups",[]byte("{}"),cookie)
 if create.Code!=http.StatusCreated{t.Fatalf("create: %d %s",create.Code,create.Body.String())}
 var info struct{Name string `json:"name"`}
 if err=json.Unmarshal(create.Body.Bytes(),&info);err!=nil||info.Name==""{t.Fatalf("created backup info: %v %s",err,create.Body.String())}
 list:=request(http.MethodGet,"/api/backups",nil,cookie)
 if list.Code!=http.StatusOK||!bytes.Contains(list.Body.Bytes(),[]byte(info.Name)){
  t.Fatalf("backup not listed: %d %s",list.Code,list.Body.String())
 }
 downloaded:=request(http.MethodGet,"/api/backups/"+info.Name+"/download",nil,cookie)
 if downloaded.Code!=http.StatusOK||downloaded.Body.Len()==0{
  t.Fatalf("download: %d %s",downloaded.Code,downloaded.Body.String())
 }
 backupBytes:=append([]byte(nil),downloaded.Body.Bytes()...)
 bad:=request(http.MethodPost,"/api/backups/import",[]byte("invalid-gzip"),cookie)
 if bad.Code!=http.StatusBadRequest{t.Fatalf("invalid backup accepted: %d",bad.Code)}
 importedReq:=httptest.NewRequest(http.MethodPost,"http://panel/api/backups/import",bytes.NewReader(backupBytes))
 importedReq.AddCookie(cookie)
 importedReq.Header.Set("Content-Type","application/gzip")
 imported:=httptest.NewRecorder()
 h.ServeHTTP(imported,importedReq)
 if imported.Code!=http.StatusCreated{t.Fatalf("import: %d %s",imported.Code,imported.Body.String())}
 var importedInfo struct{Name string `json:"name"`}
 if err=json.Unmarshal(imported.Body.Bytes(),&importedInfo);err!=nil||importedInfo.Name==""||importedInfo.Name==info.Name{
  t.Fatalf("imported backup name invalid: %v %s",err,imported.Body.String())
 }
 if _,err=accounts.Create(accountcfg.Input{Name:"new-after-backup",Enabled:true,Port:25882,ServerName:"example.com"});err!=nil{t.Fatal(err)}
 before,_:=accounts.List()
 if len(before)!=2{t.Fatalf("expected 2 accounts before restore: %d",len(before))}
 time.Sleep(6*time.Millisecond) // Keep the pre-restore snapshot name distinct.
 // An invalid restore must not create a checkpoint or revoke a valid session.
 snapshotsBefore:=request(http.MethodGet,"/api/backups",nil,cookie)
 invalidRestore:=request(http.MethodPost,"/api/backups/missing.json.gz/restore",[]byte("{}"),cookie)
 if invalidRestore.Code!=http.StatusBadRequest{t.Fatalf("invalid restore: %d",invalidRestore.Code)}
 snapshotsAfter:=request(http.MethodGet,"/api/backups",nil,cookie)
 if snapshotsBefore.Body.String()!=snapshotsAfter.Body.String(){
  t.Fatal("invalid restore unexpectedly created another backup")
 }
 if live:=request(http.MethodGet,"/api/accounts",nil,cookie);live.Code!=http.StatusOK{
  t.Fatal("invalid restore invalidated session")
 }
 restored:=request(http.MethodPost,"/api/backups/"+info.Name+"/restore",[]byte("{}"),cookie)
 if restored.Code!=http.StatusOK||!bytes.Contains(restored.Body.Bytes(),[]byte("loginRequired")){
  t.Fatalf("restore: %d %s",restored.Code,restored.Body.String())
 }
 if old:=request(http.MethodGet,"/api/accounts",nil,cookie);old.Code!=http.StatusUnauthorized{
  t.Fatalf("old session survived restore: %d",old.Code)
 }
 freshCookie:=login()
 after,err:=accounts.List()
 if err!=nil||len(after)!=1||after[0].ID!=original.ID||after[0].Name!="backup-original"{
  t.Fatalf("restored accounts mismatch: %+v %v",after,err)
 }
 deletion:=request(http.MethodDelete,"/api/backups/"+info.Name,nil,freshCookie)
 if deletion.Code!=http.StatusOK{t.Fatalf("delete: %d %s",deletion.Code,deletion.Body.String())}
 if existing:=request(http.MethodGet,"/api/backups",nil,freshCookie);existing.Code!=http.StatusOK||
  bytes.Contains(existing.Body.Bytes(),[]byte(info.Name)){
  t.Fatalf("deleted backup still listed: %d %s",existing.Code,existing.Body.String())
 }
 again:=request(http.MethodDelete,"/api/backups/"+info.Name,nil,freshCookie)
 if again.Code!=http.StatusNotFound{t.Fatalf("missing backup DELETE expected 404, got %d",again.Code)}
 if r:=request(http.MethodGet,"/api/backups/"+importedInfo.Name+"/download",nil,freshCookie);r.Code!=http.StatusOK{
  t.Fatalf("imported backup unavailable: %d",r.Code)
 }
 if payload,err:=io.ReadAll(bytes.NewReader(backupBytes));err!=nil||len(payload)==0{t.Fatal("empty downloaded archive")}
}

func TestBackupExportAndUnauthenticatedOperationsHTTP(t *testing.T){
 dir:=t.TempDir()
 st,err:=store.Open(filepath.Join(dir,"account-share.sqlite"))
 if err!=nil{t.Fatal(err)}
 defer st.Close()
 hash,err:=bcrypt.GenerateFromPassword([]byte("test-export-password"),bcrypt.MinCost)
 if err!=nil{t.Fatal(err)}
 if err=st.SetAdmin("admin",string(hash));err!=nil{t.Fatal(err)}
 accounts:=service.NewAccounts(st,testApplier{})
 _,err=accounts.Create(accountcfg.Input{Name:"export-test",Enabled:true,Port:25883,ServerName:"example.com"})
 if err!=nil{t.Fatal(err)}
 h:=New(st,accounts,nil,nil).WithRuntime(RuntimeOptions{DataDir:dir}).Handler()
 get:=func(method,path string,cookie *http.Cookie)*httptest.ResponseRecorder{
  req:=httptest.NewRequest(method,"http://panel"+path,nil)
  if cookie!=nil{req.AddCookie(cookie)}
  rec:=httptest.NewRecorder()
  h.ServeHTTP(rec,req)
  return rec
 }
 unauth:=get(http.MethodGet,"/api/accounts/export?host=example.com",nil)
 if unauth.Code!=http.StatusUnauthorized{t.Fatalf("export not protected: %d",unauth.Code)}
 for _,tc:=range []struct{method,path string}{
  {http.MethodGet,"/api/backups"},
  {http.MethodPost,"/api/backups"},
  {http.MethodDelete,"/api/backups/example.json.gz"},
  {http.MethodPost,"/api/xray/restart"},
 }{
  rec:=get(tc.method,tc.path,nil)
  if rec.Code!=http.StatusUnauthorized{t.Errorf("%s %s unprotected: %d",tc.method,tc.path,rec.Code)}
 }
 login:=httptest.NewRequest(http.MethodPost,"http://panel/api/login",
  bytes.NewBufferString(`{"username":"admin","password":"test-export-password"}`))
 record:=httptest.NewRecorder()
 h.ServeHTTP(record,login)
 if record.Code!=http.StatusOK||len(record.Result().Cookies())==0{t.Fatalf("login: %d",record.Code)}
 cookie:=record.Result().Cookies()[0]
 export:=get(http.MethodGet,"/api/accounts/export?host=proxy.example.com",cookie)
 if export.Code!=http.StatusOK||!strings.Contains(export.Header().Get("Content-Type"),"application/zip"){
  t.Fatalf("export: %d %s",export.Code,export.Body.String())
 }
 zr,err:=zip.NewReader(bytes.NewReader(export.Body.Bytes()),int64(export.Body.Len()))
 if err!=nil{t.Fatal(err)}
 names:=map[string]bool{}
 foundLink:=false
 for _,entry:=range zr.File{
  names[entry.Name]=true
  if entry.Name=="links.txt"{
   body,err:=entry.Open()
   if err!=nil{t.Fatal(err)}
   contents,err:=io.ReadAll(body)
   body.Close()
   if err!=nil{t.Fatal(err)}
   foundLink=strings.Contains(string(contents),"vless://")&&strings.Contains(string(contents),"proxy.example.com")
  }
 }
 png:=false
 for name:=range names{if strings.HasPrefix(name,"qr/")&&strings.HasSuffix(name,".png"){png=true}}
 if !foundLink||!png{t.Fatalf("export ZIP missing valid links or QR image: %+v",names)}
 if rec:=get(http.MethodGet,"/api/accounts/export?host="+strings.Repeat("x",513),cookie);rec.Code!=http.StatusBadRequest{
  t.Fatalf("empty export hostname accepted: %d",rec.Code)
 }
 if rec:=get(http.MethodPost,"/api/xray/restart",cookie);rec.Code!=http.StatusServiceUnavailable{
  t.Fatalf("unconfigured Xray restart: %d",rec.Code)
 }
 if rec:=get(http.MethodPost,"/api/xport/restart",cookie);rec.Code!=http.StatusServiceUnavailable{
  t.Fatalf("unconfigured panel restart: %d",rec.Code)
 }
 if rec:=get(http.MethodGet,"/api/logs?service=xray",cookie);rec.Code!=http.StatusServiceUnavailable{
  t.Fatalf("unconfigured log access: %d",rec.Code)
 }
}
