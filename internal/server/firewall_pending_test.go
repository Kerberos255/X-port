package server

import ("encoding/json";"testing";"time")
func TestPendingFirewallTokensAndSettings(t *testing.T){
 a,err:=newFirewallToken();if err!=nil||len(a)!=32{t.Fatalf("token=%q err=%v",a,err)}
 b,err:=newFirewallToken();if err!=nil||a==b{t.Fatal("pending token must be unique")}
 p:=firewallPending{ID:"firewalld:public:8443/tcp",Token:a,Unit:"xport-fw-restore-1234567890abcdef12345678",Deadline:time.Now().Add(time.Minute).UnixMilli()}
 data,_:=json.Marshal(p)
 rows:=pendingFromSettings(map[string]string{firewallPendingKey(p.ID):string(data),"other":"ignored","firewall_pending:bad":"not-json"})
 if len(rows)!=1||rows[0].ID!=p.ID||rows[0].Token!=a{t.Fatalf("pending rows=%+v",rows)}
}
