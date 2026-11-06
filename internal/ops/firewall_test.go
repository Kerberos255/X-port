package ops

import ("context";"testing";"os";"path/filepath";"time";"strings")
func TestFirewallHelperRejectsInvalidArguments(t *testing.T){
 for _,tc:=range []struct{action string;port int;proto,zone string}{
  {"evil",80,"tcp","public"},{"add",0,"tcp","public"},{"add",22,"icmp","public"},{"add",80,"tcp","bad zone"},
 }{
  if err:=ApplyFirewall(context.Background(),t.TempDir(),tc.action,tc.port,tc.proto,tc.zone);err==nil{t.Errorf("accepted invalid args %+v",tc)}
 }
}

func TestFirewallRollbackRejectsUntrustedUnit(t *testing.T){
 if err:=CancelFirewallRestore(context.Background(),"other.service");err==nil{t.Fatal("untrusted unit should be rejected")}
 if err:=CancelFirewallRestore(context.Background(),"xport-fw-restore-../etc");err==nil{t.Fatal("path should be rejected")}
}
func TestFirewallRollbackRejectsInvalidArguments(t *testing.T){
 for _,tc:=range []struct{backend string;port int;proto,zone string}{
  {"nftables",8443,"tcp",""},{"ufw",0,"tcp",""},{"firewalld",8443,"tcp","bad zone"},{"ufw",8443,"tcp","public"},
 }{
  if _,_,err:=ScheduleFirewallRestore(context.Background(),tc.backend,tc.port,tc.proto,tc.zone);err==nil{t.Errorf("invalid rollback accepted: %+v",tc)}
 }
}

func TestFirewallRestoreSchedulingAndCancellation(t *testing.T){
 dir:=t.TempDir()
 script:=`#!/bin/sh
case "$1" in
 --quiet) exit 0;;
 is-active) exit 0;;
 stop) exit 0;;
 *) exit 1;;
esac
`
 for _,name:=range []string{"systemd-run","systemctl","firewall-cmd","ufw"}{
  if err:=os.WriteFile(filepath.Join(dir,name),[]byte(script),0755);err!=nil{t.Fatal(err)}
 }
 t.Setenv("PATH",dir)
 unit,deadline,err:=ScheduleFirewallRestore(context.Background(),"firewalld",2053,"tcp","public")
 if err!=nil{t.Fatal(err)}
 if !restoreUnitRE.MatchString(unit){t.Fatalf("unexpected unit: %s",unit)}
 if deadline<=time.Now().UnixMilli(){t.Fatalf("invalid deadline: %d",deadline)}
 if err:=CancelFirewallRestore(context.Background(),unit);err!=nil{t.Fatal(err)}
}
// Systemd must store recovery commands that remain valid when the X-port
// executable is downgraded or removed during the 120-second window.
func TestFirewallRestoreTimerUsesNativeUtilities(t *testing.T){
 dir:=t.TempDir()
 logFile:=filepath.Join(dir,"timer-arguments")
 t.Setenv("XPORT_TEST_TIMER_LOG",logFile)
 systemdScript:="#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$XPORT_TEST_TIMER_LOG\"\n"
 if err:=os.WriteFile(filepath.Join(dir,"systemd-run"),[]byte(systemdScript),0755);err!=nil{t.Fatal(err)}
 for _,tool:=range []string{"firewall-cmd","ufw"}{
  if err:=os.WriteFile(filepath.Join(dir,tool),[]byte("#!/bin/sh\nexit 0\n"),0755);err!=nil{t.Fatal(err)}
 }
 t.Setenv("PATH",dir)
 for _,tc:=range []struct{backend,zone,proto,expected string}{
  {"firewalld","public","tcp","--permanent"},
  {"ufw","","udp","allow"},
 }{
  unit,_,err:=ScheduleFirewallRestore(context.Background(),tc.backend,53334,tc.proto,tc.zone)
  if err!=nil{t.Fatal(err)}
  if !restoreUnitRE.MatchString(unit){t.Fatal("timer unit name invalid")}
  raw,err:=os.ReadFile(logFile);if err!=nil{t.Fatal(err)}
  args:=string(raw)
  tool:=map[string]string{"firewalld":"firewall-cmd","ufw":"ufw"}[tc.backend]
  if !strings.Contains(args,"/bin/sh\n-c\n") ||
     !strings.Contains(args,"--on-active=120s") ||
     !strings.Contains(args,filepath.Join(dir,tool)) ||
     !strings.Contains(args,tc.expected){
    t.Fatalf("%s lacks independent native recovery command: %s",tc.backend,args)
  }
  if strings.Contains(args,"\nfirewall-restore\n"){t.Fatalf("timer depends on panel subcommand: %s",args)}
 }
}

func TestFirewallRestoreCannotConfirmExpiredTimer(t *testing.T){
 dir:=t.TempDir()
 if err:=os.WriteFile(filepath.Join(dir,"systemctl"),[]byte("#!/bin/sh\nexit 3\n"),0755);err!=nil{t.Fatal(err)}
 t.Setenv("PATH",dir)
 if err:=CancelFirewallRestore(context.Background(),"xport-fw-restore-0123456789abcdef01234567");err==nil{t.Fatal("expired timer must not confirm")}
}

func TestFirewallHelperUsesPrivateOutputFileWithoutPipe(t *testing.T){
 work:=t.TempDir()
 bin:=filepath.Join(work,"bin")
 dataDir:=filepath.Join(work,"data")
 if err:=os.MkdirAll(bin,0700);err!=nil{t.Fatal(err)}
 if err:=os.MkdirAll(dataDir,0700);err!=nil{t.Fatal(err)}
 script:=`#!/bin/sh
out=''
for arg do
 case "$arg" in
  --pipe) exit 87;;
  --property=StandardOutput=file:*) out=${arg#--property=StandardOutput=file:};;
esac
done
test -n "$out" || exit 91
case " $* " in
 *" firewall-inspect"*) printf '{"backend":"firewalld","active":true,"writable":true,"rules":[]}' > "$out";;
 *" firewall-listeners"*) printf '{"443":true}' > "$out";;
 *) exit 98;;
esac
`
 if err:=os.WriteFile(filepath.Join(bin,"systemd-run"),[]byte(script),0755);err!=nil{t.Fatal(err)}
 t.Setenv("PATH",bin)
 snap,err:=InspectFirewall(context.Background(),dataDir)
 if err!=nil||snap.Backend!="firewalld"||!snap.Writable{t.Fatalf("inspect=%+v err=%v",snap,err)}
 ports,err:=FirewallListeners(context.Background(),dataDir)
 if err!=nil||!ports[443]{t.Fatalf("ports=%v err=%v",ports,err)}
 left,err:=filepath.Glob(filepath.Join(dataDir,".xport-fw-result-*"))
 if err!=nil||len(left)!=0{t.Fatalf("helper result files not removed: %v %v",left,err)}
}
