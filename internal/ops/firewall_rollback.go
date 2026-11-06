package ops

import (
 "context"
 "crypto/rand"
 "encoding/hex"
 "errors"
 "fmt"
 "path/filepath"
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

 // Recovery is intentionally independent of the installed X-port binary.
 // The helper can be upgraded, rolled back, or removed while this timer waits.
 utility:="firewall-cmd"
 if backend=="ufw"{utility="ufw"}
 executable,err:=exec.LookPath(utility)
 if err!=nil{return "",0,fmt.Errorf("recovery utility unavailable: %w",err)}
 if !filepath.IsAbs(executable){return "",0,errors.New("invalid recovery utility path")}
 var script string
 var worker []string
 if backend=="firewalld"{
  script="set -eu\n\"$1\" \"--zone=$2\" \"--add-port=$3/$4\"\n\"$1\" --permanent \"--zone=$2\" \"--add-port=$3/$4\""
  worker=[]string{"/bin/sh","-c",script,"xport-recovery",executable,zone,strconv.Itoa(port),proto}
 }else{
  script="set -eu\n\"$1\" allow \"$2/$3\""
  worker=[]string{"/bin/sh","-c",script,"xport-recovery",executable,strconv.Itoa(port),proto}
 }
 bytes:=make([]byte,12);if _,err:=rand.Read(bytes);err!=nil{return "",0,err}
 unit:="xport-fw-restore-"+hex.EncodeToString(bytes)
 cctx,cancel:=context.WithTimeout(ctx,12*time.Second);defer cancel()
 argv:=[]string{"--quiet","--unit="+unit,"--on-active=120s",
  "--property=Type=exec","--property=NoNewPrivileges=yes"}
 argv=append(argv,worker...)
 out,err:=exec.CommandContext(cctx,"systemd-run",argv...).CombinedOutput()
 if err!=nil{return "",0,fmt.Errorf("could not schedule firewall restore: %w: %s",err,strings.TrimSpace(string(out)))}
 return unit,time.Now().Add(FirewallRestoreAfter).UnixMilli(),nil
}
func CancelFirewallRestore(ctx context.Context,unit string)error{
 if !restoreUnitRE.MatchString(unit){return errors.New("invalid recovery unit")}
 cctx,cancel:=context.WithTimeout(ctx,10*time.Second);defer cancel()
 // A timer that already fired cannot be safely treated as cancelled.
 activeErr:=exec.CommandContext(cctx,"systemctl","is-active","--quiet",unit+".timer").Run()
 if activeErr!=nil{return errors.New("recovery timer is no longer active; it may have fired")}
 out,err:=exec.CommandContext(cctx,"systemctl","stop",unit+".timer").CombinedOutput()
 if err!=nil{return fmt.Errorf("cannot cancel firewall recovery: %w: %s",err,strings.TrimSpace(string(out)))}
 return nil
}
