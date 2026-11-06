package ops

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "os/exec"
 "strconv"
 "strings"
 "time"

 "github.com/Kerberos255/X-port/internal/firewall"
)

// The persistent Web process keeps its strict read-only filesystem and socket
// policy. Each firewall operation runs in a short-lived systemd unit whose
// command is restricted to X-port's validated CLI subcommands.
func firewallHelper(ctx context.Context,args ...string)(string,error){
 if len(args)==0{return "",errors.New("missing firewall helper command")}
 exe,err:=os.Executable();if err!=nil{return "",err}
 if !validManagedPath(exe){return "",errors.New("invalid executable path")}
 unit:="xport-firewall-"+strconv.FormatInt(time.Now().UnixNano(),10)
 cctx,cancel:=context.WithTimeout(ctx,70*time.Second);defer cancel()
 argv:=[]string{"--quiet","--wait","--collect","--pipe","--unit="+unit,
  "--property=Type=exec","--property=NoNewPrivileges=yes",exe}
 argv=append(argv,args...)
 out,err:=exec.CommandContext(cctx,"systemd-run",argv...).CombinedOutput()
 if len(out)>512<<10{return "",errors.New("firewall helper output too large")}
 if err!=nil{
  if cctx.Err()!=nil{return "",cctx.Err()}
  return "",fmt.Errorf("firewall helper failed: %w: %s",err,strings.TrimSpace(string(out)))
 }
 return strings.TrimSpace(string(out)),nil
}
func ApplyFirewall(ctx context.Context,action string,port int,proto,zone string)error{
 if action!="add"&&action!="remove"{return errors.New("invalid action")}
 if !firewall.ValidRule(port,proto){return errors.New("invalid port/protocol")}
 if zone!=""&&!firewall.ValidZone(zone){return errors.New("invalid zone")}
 _,err:=firewallHelper(ctx,"firewall-apply",action,strconv.Itoa(port),proto,zone)
 return err
}
func InspectFirewall(ctx context.Context)(firewall.Snapshot,error){
 out,err:=firewallHelper(ctx,"firewall-inspect");if err!=nil{return firewall.Snapshot{},err}
 var snap firewall.Snapshot
 if err=json.Unmarshal([]byte(out),&snap);err!=nil{return snap,fmt.Errorf("invalid firewall helper reply: %w",err)}
 return snap,nil
}
func NativeFirewallRules(ctx context.Context)(string,error){
 return firewallHelper(ctx,"firewall-raw")
}
func FirewallListeners(ctx context.Context)(map[int]bool,error){
 out,err:=firewallHelper(ctx,"firewall-listeners");if err!=nil{return nil,err}
 var ports map[int]bool
 if err=json.Unmarshal([]byte(out),&ports);err!=nil{return nil,fmt.Errorf("invalid listening port data: %w",err)}
 return ports,nil
}
