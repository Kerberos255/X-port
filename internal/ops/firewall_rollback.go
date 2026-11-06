package ops

import (
 "context"
 "crypto/rand"
 "encoding/hex"
 "errors"
 "fmt"
 "os"
 "os/exec"
 "regexp"
 "strconv"
 "strings"
 "time"

 "github.com/Kerberos255/X-port/internal/firewall"
)

const FirewallRestoreAfter = 120 * time.Second
var restoreUnitRE = regexp.MustCompile(`^xport-fw-restore-[a-f0-9]{24}$`)

// ScheduleFirewallRestore installs an external systemd timer *before* removing
// a managed rule. The recovery task outlives the Web process and its session.
func ScheduleFirewallRestore(ctx context.Context,backend string,port int,proto,zone string)(string,int64,error){
 if !firewall.ValidRule(port,proto){return "",0,errors.New("invalid port")}
 if backend!="firewalld"&&backend!="ufw"{return "",0,errors.New("unsupported backend")}
 if backend=="firewalld"&&!firewall.ValidZone(zone){return "",0,errors.New("invalid firewalld zone")}
 if backend=="ufw"&&zone!=""{return "",0,errors.New("UFW does not use zones")}
 exe,err:=os.Executable();if err!=nil{return "",0,err}
 if !validManagedPath(exe){return "",0,errors.New("invalid executable")}
 bytes:=make([]byte,12);if _,err:=rand.Read(bytes);err!=nil{return "",0,err}
 unit:="xport-fw-restore-"+hex.EncodeToString(bytes)
 cctx,cancel:=context.WithTimeout(ctx,12*time.Second);defer cancel()
 cmd:=exec.CommandContext(cctx,"systemd-run","--quiet","--unit="+unit,
  "--on-active=120s","--property=Type=exec","--property=NoNewPrivileges=yes",
  exe,"firewall-restore",backend,strconv.Itoa(port),proto,zone)
 out,err:=cmd.CombinedOutput()
 if err!=nil{return "",0,fmt.Errorf("could not schedule firewall restore: %w: %s",err,strings.TrimSpace(string(out)))}
 return unit,time.Now().Add(FirewallRestoreAfter).UnixMilli(),nil
}
func CancelFirewallRestore(ctx context.Context,unit string)error{
 if !restoreUnitRE.MatchString(unit){return errors.New("invalid recovery unit")}
 cctx,cancel:=context.WithTimeout(ctx,10*time.Second);defer cancel()
 out,err:=exec.CommandContext(cctx,"systemctl","stop",unit+".timer").CombinedOutput()
 if err!=nil{return fmt.Errorf("cannot cancel firewall recovery: %w: %s",err,strings.TrimSpace(string(out)))}
 return nil
}
