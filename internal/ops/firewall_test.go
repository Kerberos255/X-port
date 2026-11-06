package ops

import ("context";"testing")
func TestFirewallHelperRejectsInvalidArguments(t *testing.T){
 for _,tc:=range []struct{action string;port int;proto,zone string}{
  {"evil",80,"tcp","public"},{"add",0,"tcp","public"},{"add",22,"icmp","public"},{"add",80,"tcp","bad zone"},
 }{
  if err:=ApplyFirewall(context.Background(),tc.action,tc.port,tc.proto,tc.zone);err==nil{t.Errorf("accepted invalid args %+v",tc)}
 }
}
