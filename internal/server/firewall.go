package server

import (
 "context"
 "encoding/json"
 "fmt"
 "log"
 "net"
 "net/http"
 "strconv"
 "strings"
 "time"
 "github.com/Kerberos255/X-port/internal/firewall"
 "github.com/Kerberos255/X-port/internal/model"
 "github.com/Kerberos255/X-port/internal/ops"
)

type firewallViewRule struct {
 firewall.Rule
 Purpose string `json:"purpose"`
 PurposeSource string `json:"purposeSource"`
 Note string `json:"note"`
 Protected bool `json:"protected"`
 Managed bool `json:"managed"`
 Xray bool `json:"xray"`
 AccountID int64 `json:"accountId,omitempty"`
 AccountName string `json:"accountName,omitempty"`
 AccountEnabled bool `json:"accountEnabled"`
 Usage string `json:"usage"`
 AccountAmbiguous bool `json:"accountAmbiguous,omitempty"`
}
type firewallView struct {
 firewall.Snapshot
 Rules []firewallViewRule `json:"rules"`
 Pending []firewallPending `json:"pending,omitempty"`
}

// Account association is by both port and actual Xray transport, never by
// numeric port alone. Unknown/custom transports stay unassociated.
func accountFirewallTransport(a model.Account) string {
 var stream struct{Network string `json:"network"`}
 if a.StreamSettingsJSON!="" {
  if err:=json.Unmarshal([]byte(a.StreamSettingsJSON),&stream);err!=nil{return ""}
 }
 network:=strings.ToLower(strings.TrimSpace(stream.Network))
 if network==""{network="tcp"}
 switch network{
 case "tcp","raw","ws","websocket","grpc","http","h2","httpupgrade","xhttp": return "tcp"
 case "mkcp","kcp","quic": return "udp"
 default: return ""
 }
}
func accountFirewallMatches(accounts []model.Account)map[string][]model.Account{
 found:=make(map[string][]model.Account)
 for _,a:=range accounts{
  protocol:=accountFirewallTransport(a)
  if protocol==""||!firewall.ValidRule(a.Port,protocol){continue}
  key:=fmt.Sprintf("%s/%d",protocol,a.Port)
  found[key]=append(found[key],a)
 }
 return found
}

func (s *Server) firewallGuardPort() int {
 listen:=settingString(s.store,"panel_listen","127.0.0.1:8080")
 _,p,err:=net.SplitHostPort(listen);if err!=nil{return 8080}
 n,err:=strconv.Atoi(p);if err!=nil||n<1||n>65535{return 8080}
 return n
}
func (s *Server) firewallRead(w http.ResponseWriter,r *http.Request){
 ctx,cancel:=context.WithTimeout(r.Context(),15*time.Second);defer cancel()
 snap,err:=ops.InspectFirewall(ctx,s.store.Dir());if err!=nil{log.Printf("firewall inspection unavailable: %v",err);writeError(w,502,"failed to inspect firewall rules");return}
 w.Header().Set("Cache-Control","no-store")
 accounts,err:=s.store.Accounts();if err!=nil{writeError(w,500,"database error");return}
 settings,err:=s.store.Settings();if err!=nil{writeError(w,500,"database error");return}
 bySocket:=accountFirewallMatches(accounts)
 listening,listeningError:=ops.FirewallListeners(ctx,s.store.Dir())
 sockets,socketError:=ops.FirewallSockets(ctx,s.store.Dir())
 rules:=make([]firewallViewRule,0,len(snap.Rules))
 for _,rr:=range snap.Rules {
  purpose,xray:="未识别",false
  key:=fmt.Sprintf("%s/%d",rr.Protocol,rr.Port)
  candidates:=bySocket[key]
  xray=len(candidates)>0
  ambiguous:=len(candidates)>1
  var account model.Account
  if len(candidates)==1{account=candidates[0];purpose="Xray · "+strings.ToUpper(account.Protocol)}
  if ambiguous{purpose="多个 Xray 账号关联同一端口（需检查）"}
  source:="unknown"
  if xray{source="xray"}
  protected:=rr.Port==22||rr.Port==s.firewallGuardPort()||listeningError!=nil||listening[rr.Port]
  usage:="unknown"
  if socketError==nil{
   usage="unused"
   if sockets[key]{usage="used"}
  }
  if !xray&&rr.Port==22{purpose="SSH（常见端口，未核实）";source="assumed"}
  if !xray&&rr.Port==s.firewallGuardPort(){purpose="X-port 管理面板";source="panel"}
  if custom:=settings["firewall_purpose:"+rr.ID];custom!=""{purpose=custom;source="manual"}
  rules=append(rules,firewallViewRule{
   Rule:rr,Purpose:purpose,PurposeSource:source,
   Note:settings["firewall_note:"+rr.ID],Protected:protected,
   Managed:settings["firewall_owned:"+rr.ID]=="v1",Xray:xray,
   AccountID:account.ID,AccountName:account.Name,AccountEnabled:account.Enabled,
   AccountAmbiguous:ambiguous,Usage:usage,
  })
 }
 writeJSON(w,200,firewallView{Snapshot:snap,Rules:rules,Pending:pendingFromSettings(settings)})
}
func (s *Server) firewallNote(w http.ResponseWriter,r *http.Request){
 var in struct{ID string `json:"id"`; Note string `json:"note"`}
 if decodeJSONLimit(w,r,&in,4<<10)!=nil{return}
 note:=strings.TrimSpace(in.Note)
 if len(note)>360||len([]rune(note))>120{writeError(w,400,"note is too long");return}
 s.firewallMu.Lock();defer s.firewallMu.Unlock()
 ctx,cancel:=context.WithTimeout(r.Context(),15*time.Second);defer cancel()
 snap,err:=ops.InspectFirewall(ctx,s.store.Dir());if err!=nil{writeError(w,502,"failed to inspect firewall rules");return}
 found:=false
 for _,rr:=range snap.Rules{if rr.ID==in.ID{found=true;break}}
 if !found{writeError(w,404,"firewall rule was not found");return}
 if err:=s.store.SetSetting("firewall_note:"+in.ID,note);err!=nil{writeError(w,500,"could not save note");return}
 writeJSON(w,200,map[string]string{"note":note})
}
func (s *Server) firewallRuleChange(w http.ResponseWriter,r *http.Request){
 var in struct{
  Action string `json:"action"`
  Port int `json:"port"`
  Protocol string `json:"protocol"`
  Zone string `json:"zone"`
 }
 if decodeJSONLimit(w,r,&in,4<<10)!=nil{return}
 if !firewall.ValidRule(in.Port,in.Protocol)||(in.Action!="add"&&in.Action!="remove"){
  writeError(w,400,"invalid firewall rule");return
 }
 s.firewallMu.Lock();defer s.firewallMu.Unlock()
 ctx,cancel:=context.WithTimeout(r.Context(),100*time.Second);defer cancel()
 snap,err:=ops.InspectFirewall(ctx,s.store.Dir())
 if err!=nil{writeError(w,502,"could not inspect firewall");return}
 if !snap.Writable{writeError(w,409,"read-only firewall: "+snap.UnsafeReason);return}
 if snap.Backend!="firewalld"&&snap.Backend!="ufw"{writeError(w,409,"unsupported firewall backend");return}
 if snap.Backend=="ufw"&&in.Zone!=""{writeError(w,400,"UFW does not support zones");return}
 if snap.Backend=="firewalld"&&!firewall.ValidZone(in.Zone){writeError(w,400,"invalid zone");return}
 id:=firewall.RuleID(snap.Backend,in.Zone,in.Port,in.Protocol)
 if err:=s.ensureNoPending(id);err!=nil{writeError(w,409,err.Error());return}
 var restoreUnit string
 var restoreDeadline int64
 if in.Action=="remove"{
  value,ok,err:=s.store.Setting("firewall_owned:"+id)
  if err!=nil{writeError(w,500,"could not check rule ownership");return}
  if !ok||value!="v1"{writeError(w,403,"only X-port created rules can be deleted");return}
  if in.Port==22||in.Port==s.firewallGuardPort(){writeError(w,403,"management port protected");return}
  ports,err:=ops.FirewallListeners(ctx,s.store.Dir())
  if err!=nil{writeError(w,409,"cannot verify listeners; deletion blocked");return}
  if ports[in.Port]{writeError(w,409,"port has an active listener; stop it first");return}
  restoreUnit,restoreDeadline,err=ops.ScheduleFirewallRestore(ctx,snap.Backend,in.Port,in.Protocol,in.Zone)
  if err!=nil{writeError(w,409,"could not arm connection recovery: "+err.Error());return}
 }
 if err:=ops.ApplyFirewall(ctx,s.store.Dir(),in.Action,in.Port,in.Protocol,in.Zone);err!=nil{
  // Keep the independent restore armed: a failed operation might have changed runtime rules partially.
  writeError(w,409,"firewall operation failed: "+err.Error());return
 }
 if in.Action=="add"{
  if err:=s.store.SetSetting("firewall_owned:"+id,"v1");err!=nil{
   rbctx,done:=context.WithTimeout(context.Background(),35*time.Second)
   rollback:=ops.ApplyFirewall(rbctx,s.store.Dir(),"remove",in.Port,in.Protocol,in.Zone);done()
   writeError(w,500,fmt.Sprintf("ownership persistence failed; rollback: %v",rollback));return
  }
  s.firewallAudit("add",id)
  writeJSON(w,200,map[string]bool{"ok":true});return
 }
 pending,err:=s.firewallPendingSave(id,restoreUnit,restoreDeadline)
 if err!=nil{
  writeError(w,500,"rule temporarily removed; recovery timer remains armed because confirmation state could not be saved");return
 }
 s.firewallAudit("remove-pending",id)
 writeJSON(w,http.StatusAccepted,map[string]any{"ok":true,"pending":true,"id":id,"token":pending.Token,"deadline":pending.Deadline})
}
func (s *Server) firewallRaw(w http.ResponseWriter,r *http.Request){
 ctx,cancel:=context.WithTimeout(r.Context(),12*time.Second);defer cancel()
 raw,err:=ops.NativeFirewallRules(ctx,s.store.Dir());if err!=nil{writeError(w,502,"could not read native firewall rules");return}
 w.Header().Set("Cache-Control","no-store")
 writeJSON(w,200,map[string]string{"ruleset":raw})
}

func (s *Server) firewallPurpose(w http.ResponseWriter,r *http.Request){
 var in struct{ID string `json:"id"`; Purpose string `json:"purpose"`}
 if decodeJSONLimit(w,r,&in,4<<10)!=nil{return}
 purpose:=strings.TrimSpace(in.Purpose)
 if len(purpose)>180||len([]rune(purpose))>60{writeError(w,400,"purpose is too long");return}
 s.firewallMu.Lock();defer s.firewallMu.Unlock()
 ctx,cancel:=context.WithTimeout(r.Context(),15*time.Second);defer cancel()
 snap,err:=ops.InspectFirewall(ctx,s.store.Dir());if err!=nil{writeError(w,502,"failed to inspect firewall rules");return}
 found:=false;for _,rr:=range snap.Rules{if rr.ID==in.ID{found=true;break}}
 if !found{writeError(w,404,"firewall rule was not found");return}
 if err:=s.store.SetSetting("firewall_purpose:"+in.ID,purpose);err!=nil{writeError(w,500,"could not save purpose");return}
 writeJSON(w,200,map[string]string{"purpose":purpose})
}
