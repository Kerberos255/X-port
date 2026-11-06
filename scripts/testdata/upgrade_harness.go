// This test-only program exercises the same updater.Stage and external
// systemd restart supervisor used by the authenticated Web update endpoint.
// Its release API is an in-process test server, never a production override.
package main

import (
 "context"
 "crypto/sha256"
 "encoding/json"
 "fmt"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "runtime"
 "strconv"
 "time"

 "github.com/Kerberos255/X-port/internal/ops"
 "github.com/Kerberos255/X-port/internal/selfupdate"
)

func main(){
 if len(os.Args)!=6{fatal("usage: harness CURRENT TARGET LIVE_BINARY CANDIDATE SERVICE")}
 current,target,live,candidate,service:=os.Args[1],os.Args[2],os.Args[3],os.Args[4],os.Args[5]
 data,err:=os.ReadFile(candidate);if err!=nil{fatal(err.Error())}
 sum:=sha256.Sum256(data)
 asset:="xport-linux-"+runtime.GOARCH
 var server *httptest.Server
 server=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.URL.Path=="/asset"{
   w.Header().Set("Content-Type","application/octet-stream")
   _,_=w.Write(data)
   return
  }
  if r.URL.Path!="/releases"{http.NotFound(w,r);return}
  items:=[]map[string]any{{
   "tag_name":"v"+target,"draft":false,"prerelease":false,
   "assets":[]map[string]any{{"name":asset,"url":server.URL+"/asset","size":len(data),"digest":fmt.Sprintf("sha256:%x",sum)}},
  }}
  if err:=json.NewEncoder(w).Encode(items);err!=nil{fmt.Fprintln(os.Stderr,err)}
 }))
 defer server.Close()
 updater:=selfupdate.Updater{CurrentVersion:current,BinaryPath:live,ReleasesURL:server.URL+"/releases",Client:server.Client()}
 ctx,cancel:=context.WithTimeout(context.Background(),90*time.Second);defer cancel()
 info,previous,err:=updater.Stage(ctx)
 if err!=nil{fatal("upgrade staging failed: "+err.Error())}
 if previous!=filepath.Clean(live)+".previous"||info.Current!=target{
  fatal("unexpected staged version or backup path: "+strconv.Quote(info.Current))
 }
 if err:=ops.ScheduleVerifiedBinaryRestart(service,live,previous,time.Second);err!=nil{
  _=updater.Restore(previous)
  fatal("schedule restart failed: "+err.Error())
 }
 fmt.Printf("staged %s -> %s with watched restart, backup preserved\n",current,target)
}
func fatal(s string){fmt.Fprintln(os.Stderr,s);os.Exit(1)}
