package ops

import ("context";"testing";"os";"path/filepath";"time")
func TestFirewallHelperRejectsInvalidArguments(t *testing.T){
 for _,tc:=range []struct{action string;port int;proto,zone string}{
  {"evil",80,"tcp","public"},{"add",0,"tcp","public"},{"add",22,"icmp","public"},{"add",80,"tcp","bad zone"},
 }{
  if err:=ApplyFirewall(context.Background(),tc.action,tc.port,tc.proto,tc.zone);err==nil{t.Errorf("accepted invalid args %+v",tc)}
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
 for _,name:=range []string{"systemd-run","systemctl"}{
  if err:=os.WriteFile(filepath.Join(dir,name),[]byte(script),0755);err!=nil{t.Fatal(err)}
 }
 t.Setenv("PATH",dir)
 unit,deadline,err:=ScheduleFirewallRestore(context.Background(),"firewalld",2053,"tcp","public")
 if err!=nil{t.Fatal(err)}
 if !restoreUnitRE.MatchString(unit){t.Fatalf("unexpected unit: %s",unit)}
 if deadline<=time.Now().UnixMilli(){t.Fatalf("invalid deadline: %d",deadline)}
 if err:=CancelFirewallRestore(context.Background(),unit);err!=nil{t.Fatal(err)}
}
func TestFirewallRestoreCannotConfirmExpiredTimer(t *testing.T){
 dir:=t.TempDir()
 if err:=os.WriteFile(filepath.Join(dir,"systemctl"),[]byte("#!/bin/sh\nexit 3\n"),0755);err!=nil{t.Fatal(err)}
 t.Setenv("PATH",dir)
 if err:=CancelFirewallRestore(context.Background(),"xport-fw-restore-0123456789abcdef01234567");err==nil{t.Fatal("expired timer must not confirm")}
}
