package ops

import ("context";"testing")
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
