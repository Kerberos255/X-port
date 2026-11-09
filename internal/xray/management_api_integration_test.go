
package xray

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net"
    "net/http"
    "net/http/httptest"
    "net/url"
    "os"
    "os/exec"
    "path/filepath"
    "strconv"
    "strings"
    "testing"
    "time"

    "github.com/Kerberos255/X-port/internal/model"
)

// This opt-in integration test runs only in the disposable CI runner with an
// official SHA-256-verified Xray executable. No production ports are touched.
func TestManagementAPIBoundaryWithRealXray(t *testing.T) {
    binary := os.Getenv("XPORT_INTEGRATION_XRAY")
    if binary == "" { t.Skip("requires disposable real Xray executable") }

    normal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        _, _ = io.WriteString(w, "normal-lan-access")
    }))
    defer normal.Close()
    protected := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        _, _ = io.WriteString(w, "protected-management-api")
    }))
    defer protected.Close()
    protectedPort := protected.Listener.Addr().(*net.TCPAddr).Port

    reserve := func() int {
        listener, err := net.Listen("tcp", "127.0.0.1:0")
        if err != nil { t.Fatal(err) }
        port := listener.Addr().(*net.TCPAddr).Port
        _ = listener.Close()
        return port
    }
    socksPort, httpPort := reserve(), reserve()
    for socksPort == httpPort || socksPort == protectedPort || httpPort == protectedPort {
        socksPort, httpPort = reserve(), reserve()
    }
    accounts := []model.Account{
        {Name:"integration socks",Enabled:true,Listen:"127.0.0.1",Port:socksPort,
            Protocol:"socks", SettingsJSON:"{\"auth\":\"password\",\"accounts\":[{\"user\":\"tester\",\"pass\":\"test-password\"}],\"udp\":false}",StreamSettingsJSON:"{}",SniffingJSON:"{}",Tag:"ci-socks"},
        {Name:"integration http",Enabled:true,Listen:"127.0.0.1",Port:httpPort,
            Protocol:"http", SettingsJSON:"{\"accounts\":[{\"user\":\"tester\",\"pass\":\"test-password\"}],\"allowTransparent\":false}",StreamSettingsJSON:"{}",SniffingJSON:"{}",Tag:"ci-http"},
    }
    config, err := BuildConfigWithBase(accounts, protectedPort, "")
    if err != nil { t.Fatal(err) }

    // The protected test HTTP server occupies the management port. Replace
    // only Xray's actual API listener with that service; retain its security
    // rules, outbound behavior and both real Xray proxy inbounds unchanged.
    var cfg map[string]any
    if err = json.Unmarshal(config,&cfg);err != nil {t.Fatal(err)}
    inbounds := cfg["inbounds"].([]any)
    cfg["inbounds"] = inbounds[:len(inbounds)-1]
    delete(cfg,"api")
    routing := cfg["routing"].(map[string]any)
    routingRules := routing["rules"].([]any)
    routing["rules"] = routingRules[1:]
    config,err = json.Marshal(cfg)
    if err != nil {t.Fatal(err)}

    path := filepath.Join(t.TempDir(),"xray.json")
    if err := os.WriteFile(path,config,0600);err != nil {t.Fatal(err)}
    env := append(os.Environ(),"XRAY_LOCATION_ASSET="+filepath.Dir(binary))
    checkCtx,checkCancel := context.WithTimeout(context.Background(),15*time.Second)
    check := exec.CommandContext(checkCtx,binary,"run","-test","-config",path)
    check.Env = env
    output,checkErr := check.CombinedOutput()
    checkCancel()
    if checkErr != nil {t.Fatalf("real Xray rejected generated config: %v %s",checkErr,output)}

    ctx,cancel := context.WithCancel(context.Background())
    defer cancel()
    process := exec.CommandContext(ctx,binary,"run","-config",path)
    process.Env = env
    logFile,err := os.CreateTemp(t.TempDir(),"xray-runtime-*")
    if err != nil {t.Fatal(err)}
    process.Stdout,process.Stderr = logFile,logFile
    if err := process.Start();err != nil {t.Fatal(err)}
    defer func(){cancel();_ = process.Wait();_ = logFile.Close()}()
    ready := false
    addr := net.JoinHostPort("127.0.0.1",strconv.Itoa(socksPort))
    for i:=0;i<100;i++ {
        c,dialErr:=net.DialTimeout("tcp",addr,100*time.Millisecond)
        if dialErr==nil {_=c.Close();ready=true;break}
        time.Sleep(100*time.Millisecond)
    }
    if !ready {
        logs,_:=os.ReadFile(logFile.Name())
        t.Fatalf("Xray did not start on disposable test port: %s",logs)
    }
    for _,testCase:=range []struct{name,scheme string;port int}{
        {"SOCKS5","socks5h",socksPort},{"HTTP","http",httpPort},
    } {
        t.Run(testCase.name,func(t *testing.T){
            proxyURL:=&url.URL{Scheme:testCase.scheme,Host:net.JoinHostPort("127.0.0.1",strconv.Itoa(testCase.port)),
                User:url.UserPassword("tester","test-password")}
            transport:=&http.Transport{Proxy:http.ProxyURL(proxyURL),DisableKeepAlives:true}
            defer transport.CloseIdleConnections()
            client:=&http.Client{Transport:transport,Timeout:4*time.Second}

            resp,err:=client.Get(normal.URL)
            if err!=nil {t.Fatalf("ordinary LAN proxy traffic broke: %v",err)}
            contents,_:=io.ReadAll(io.LimitReader(resp.Body,512))
            _=resp.Body.Close()
            if resp.StatusCode!=200 || string(contents)!="normal-lan-access" {
                t.Fatalf("ordinary traffic not preserved: %d %s",resp.StatusCode,contents)
            }

            for _,target:=range []string{
                protected.URL,
                fmt.Sprintf("http://localhost:%d/",protectedPort),
                fmt.Sprintf("http://xport-loopback-ci.test:%d/",protectedPort),
            } {
                resp,err=client.Get(target)
                if err==nil {
                    contents,_=io.ReadAll(io.LimitReader(resp.Body,512))
                    _=resp.Body.Close()
                    if strings.Contains(string(contents),"protected-management-api") {
                        t.Fatalf("proxy reached the protected management port via %s",target)
                    }
                }
            }
        })
    }
}
