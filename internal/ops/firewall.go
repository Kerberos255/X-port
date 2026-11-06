package ops

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "os"
 "os/exec"
 "path/filepath"
 "strconv"
 "strings"
 "time"

 "github.com/Kerberos255/X-port/internal/firewall"
)

const maxFirewallHelperOutput = 512 << 10

// The persistent Web process retains its strict systemd sandbox. A transient
// systemd service runs the same validated X-port CLI outside that sandbox.
//
// systemd-run --pipe cannot reliably pass Go os/exec's anonymous pipe FDs
// across hardened CentOS systemd/SELinux service boundaries. Instead, systemd
// opens a mode-0600 file already reserved in the panel's writable data
// directory and the caller reads it after --wait confirms the helper exited.
// No systemd unit needs to be rewritten and no host-level writable /etc access
// is granted to the persistent Web process.
func firewallHelper(ctx context.Context, dataDir string, args ...string) (string, error) {
 if len(args)==0 { return "", errors.New("missing firewall helper command") }
 if !filepath.IsAbs(dataDir) { return "", errors.New("firewall result directory must be absolute") }
 exe,err:=os.Executable()
 if err!=nil { return "",err }
 if !validManagedPath(exe){return "",errors.New("invalid executable path")}

 file,err:=os.CreateTemp(dataDir,".xport-fw-result-")
 if err!=nil { return "",fmt.Errorf("cannot reserve firewall result file: %w",err) }
 path:=file.Name()
 if err:=file.Close();err!=nil{_ = os.Remove(path);return "",err}
 defer os.Remove(path)

 unit:="xport-firewall-"+strconv.FormatInt(time.Now().UnixNano(),10)
 cctx,cancel:=context.WithTimeout(ctx,70*time.Second)
 defer cancel()
 argv:=[]string{
  "--quiet","--wait","--collect","--unit="+unit,
  "--property=Type=exec","--property=NoNewPrivileges=yes",
  "--property=StandardOutput=file:"+path,exe,
 }
 argv=append(argv,args...)
 out,err:=exec.CommandContext(cctx,"systemd-run",argv...).CombinedOutput()
 if len(out)>maxFirewallHelperOutput { return "",errors.New("firewall helper status too large") }
 if err!=nil{
  if cctx.Err()!=nil {return "",cctx.Err()}
  return "",fmt.Errorf("firewall helper failed: %w: %s",err,strings.TrimSpace(string(out)))
 }
 f,err:=os.Open(path)
 if err!=nil{return "",fmt.Errorf("cannot read firewall helper output: %w",err)}
 defer f.Close()
 data,err:=io.ReadAll(io.LimitReader(f,maxFirewallHelperOutput+1))
 if err!=nil{return "",fmt.Errorf("cannot read firewall helper output: %w",err)}
 if len(data)>maxFirewallHelperOutput{return "",errors.New("firewall helper output too large")}
 return strings.TrimSpace(string(data)),nil
}

func ApplyFirewall(ctx context.Context, dataDir,action string,port int,proto,zone string)error{
 if action!="add"&&action!="remove"{return errors.New("invalid action")}
 if !firewall.ValidRule(port,proto){return errors.New("invalid port/protocol")}
 if zone!=""&&!firewall.ValidZone(zone){return errors.New("invalid zone")}
 _,err:=firewallHelper(ctx,dataDir,"firewall-apply",action,strconv.Itoa(port),proto,zone)
 return err
}
// ApplySimpleDrop is intentionally separate from normal port allowances.
// It is not exposed as an unchecked Web action.
func ApplySimpleDrop(ctx context.Context,dataDir,action string,port int,proto,zone string)error{
 if action!="add"&&action!="remove"{return errors.New("invalid DROP action")}
 if !firewall.ValidRule(port,proto)||!firewall.ValidZone(zone){return errors.New("invalid DROP port/protocol/zone")}
 _,err:=firewallHelper(ctx,dataDir,"firewall-policy",action,strconv.Itoa(port),proto,zone)
 return err
}
func InspectFirewall(ctx context.Context, dataDir string)(firewall.Snapshot,error){
 out,err:=firewallHelper(ctx,dataDir,"firewall-inspect")
 if err!=nil{return firewall.Snapshot{},err}
 var snap firewall.Snapshot
 if err=json.Unmarshal([]byte(out),&snap);err!=nil{return snap,fmt.Errorf("invalid firewall helper reply: %w",err)}
 return snap,nil
}
func NativeFirewallRules(ctx context.Context, dataDir string)(string,error){
 return firewallHelper(ctx,dataDir,"firewall-raw")
}
func FirewallListeners(ctx context.Context, dataDir string)(map[int]bool,error){
 out,err:=firewallHelper(ctx,dataDir,"firewall-listeners")
 if err!=nil{return nil,err}
 var ports map[int]bool
 if err=json.Unmarshal([]byte(out),&ports);err!=nil{return nil,fmt.Errorf("invalid listening port data: %w",err)}
 return ports,nil
}
func FirewallSockets(ctx context.Context, dataDir string)(map[string]bool,error){
 out,err:=firewallHelper(ctx,dataDir,"firewall-sockets")
 if err!=nil{return nil,err}
 var sockets map[string]bool
 if err=json.Unmarshal([]byte(out),&sockets);err!=nil{return nil,fmt.Errorf("invalid listening socket data: %w",err)}
 if sockets==nil{return nil,errors.New("empty listening socket reply")}
 return sockets,nil
}
