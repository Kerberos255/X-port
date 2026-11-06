package server

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
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
 Paused bool `json:"paused,omitempty"`
 PausedDrift bool `json:"pausedDrift,omitempty"`
 CanToggle bool `json:"canToggle"`
 Revision string `json:"revision,omitempty"`
}
type firewallView struct {
 firewall.Snapshot
 Rules []firewallViewRule `json:"rules"`
 Pending []firewallPending `json:"pending,omitempty"`
}

// Account association is by numeric port AND the actual Xray transport.
// Unknown settings are not guessed into a potentially incorrect account.
func accountFirewallTransports(a model.Account)[]string{
 protocol:=strings.ToLower(strings.TrimSpace(a.Protocol))
 if protocol=="shadowsocks"{
  var settings struct{Network string `json:"network"`}
  if json.Unmarshal([]byte(a.SettingsJSON),&settings)!=nil{return nil}
  network:=strings.ToLower(strings.TrimSpace(settings.Network))
  switch network{
  case "tcp,udp","udp,tcp":return []string{"tcp","udp"}
  case "tcp","udp":return []string{network}
  default:return nil
  }
 }
 if protocol=="socks"{
  var settings struct{UDP bool `json:"udp"`}
  if json.Unmarshal([]byte(a.SettingsJSON),&settings)!=nil{return nil}
  if settings.UDP{return []string{"tcp","udp"}}
  return []string{"tcp"}
 }
 var stream struct{Network string `json:"network"`}
 if a.StreamSettingsJSON!="" {
  if err:=json.Unmarshal([]byte(a.StreamSettingsJSON),&stream);err!=nil{return nil}
 }
 network:=strings.ToLower(strings.TrimSpace(stream.Network))
 if network==""{network="tcp"}
 switch network{
 case "tcp","raw","ws","websocket","grpc","http","h2","httpupgrade","xhttp":return []string{"tcp"}
 case "mkcp","kcp","quic":return []string{"udp"}
 default:return nil
 }
}
func accountFirewallMatches(accounts []model.Account)map[string][]model.Account{
 found:=make(map[string][]model.Account)
 for _,a:=range accounts{
  for _,protocol:=range accountFirewallTransports(a){
   if !firewall.ValidRule(a.Port,protocol){continue}
   key:=fmt.Sprintf("%s/%d",protocol,a.Port)
   found[key]=append(found[key],a)
  }
 }
 return found
}

// Revision is a bounded, content-derived precondition token. It prevents
// using a stale Web snapshot after another control plane has changed the rule.
// Identical delete/recreate races cannot be distinguished by firewalld's API.
func firewallRuleRevision(rule firewall.Rule)string{
 raw:=fmt.Sprintf("%s|%d|%s|%s|%s|%s|%s|%t|%t|%t",
  rule.ID,rule.Port,rule.Protocol,rule.Zone,rule.Kind,rule.Action,rule.Family,
  rule.Runtime,rule.Permanent,rule.Conflict)
 value:=sha256.Sum256([]byte(raw))
 return hex.EncodeToString(value[:12])
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
 // One socket snapshot drives both protocol-specific usage and the
 // conservative port-level protection label. Avoid two systemd helper calls
 // that could disagree if a listener starts or stops between queries.
 sockets,socketError:=ops.FirewallSockets(ctx,s.store.Dir())
 listeningError:=socketError
 listening:=make(map[int]bool)
 for key:=range sockets{
  slash:=strings.IndexByte(key,'/')
  if slash<0{continue}
  port,err:=strconv.Atoi(key[slash+1:])
  if err==nil&&port>0&&port<=65535{listening[port]=true}
 }
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
   AccountAmbiguous:ambiguous,Usage:usage,Revision:firewallRuleRevision(rr),
    CanToggle:settings["firewall_owned:"+rr.ID]=="v1"&&!rr.Conflict&&
      rr.Port!=22&&rr.Port!=s.firewallGuardPort()&&
      (rr.Kind=="port"||rr.Kind=="rich")&&
      (!protected||(xray&&!ambiguous&&account.Enabled)),
  })
 }
 // Persistently paused allowances are not active firewalld rules.
 // Reconstruct from saved metadata and mark any externally restored drift.
 for key,raw:=range settings{
  if !strings.HasPrefix(key,"firewall_paused:")||raw==""{continue}
  var paused firewall.Rule
  if json.Unmarshal([]byte(raw),&paused)!=nil||paused.ID==""||
     key!=firewallPausedKey(paused.ID)||paused.Backend!="firewalld"||
     paused.Kind!="port"||paused.Action!="allow"||
     !firewall.ValidZone(paused.Zone)||!firewall.ValidRule(paused.Port,paused.Protocol)||
     settings["firewall_owned:"+paused.ID]!="v1"{continue}
  existing:=-1
  conflicting:=false
  for i,entry:=range rules{
   if entry.ID==paused.ID{existing=i}
   if entry.Port==paused.Port&&entry.Protocol==paused.Protocol&&entry.Zone==paused.Zone{
    conflicting=true
   }
  }
  if existing>=0{
   rules[existing].PausedDrift=true
   rules[existing].Conflict=true
   rules[existing].CanToggle=false
   continue
  }
  paused.Runtime=false;paused.Permanent=false
  account:=bySocket[fmt.Sprintf("%s/%d",paused.Protocol,paused.Port)]
  name:="";var accountID int64;enabled:=false
  if len(account)==1{name=account[0].Name;accountID=account[0].ID;enabled=account[0].Enabled}
  usage:="unknown"
  if socketError==nil{usage="unused";if sockets[fmt.Sprintf("%s/%d",paused.Protocol,paused.Port)]{usage="used"}}
  rules=append(rules,firewallViewRule{
   Rule:paused,Purpose:settings["firewall_purpose:"+paused.ID],
   PurposeSource:"manual",Note:settings["firewall_note:"+paused.ID],
   Protected:paused.Port==22||paused.Port==s.firewallGuardPort(),
   Managed:true,Xray:len(account)>0,AccountID:accountID,AccountName:name,
   AccountEnabled:enabled,AccountAmbiguous:len(account)>1,Usage:usage,
   Paused:true,Revision:firewallRuleRevision(paused),CanToggle:!conflicting&&paused.Port!=22&&paused.Port!=s.firewallGuardPort(),
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
   var rollback error
   if in.Action=="resume-drop"{rollback=ops.ApplySimpleDrop(rbctx,s.store.Dir(),"remove",in.Port,in.Protocol,in.Zone)}else{rollback=ops.ApplyFirewall(rbctx,s.store.Dir(),"remove",in.Port,in.Protocol,in.Zone)}
   done()
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

type firewallControlRequest struct{
 Action string `json:"action"`
 ID string `json:"id"`
 Port int `json:"port"`
 Protocol string `json:"protocol"`
 Zone string `json:"zone"`
}
// firewallControl is deliberately separate from the legacy add/remove API.
// It only accepts narrow firewalld rules, with pre-armed external recovery
// for any operation that can remove connectivity.
func (s *Server) firewallControl(w http.ResponseWriter,r *http.Request){
 var in firewallControlRequest
 if decodeJSONLimit(w,r,&in,4<<10)!=nil{return}
 if !firewall.ValidRule(in.Port,in.Protocol)||!firewall.ValidZone(in.Zone){
  writeError(w,400,"invalid firewall port/protocol/zone");return
 }
 switch in.Action {
 case "add-drop","remove-drop","pause-allow","resume-allow","pause-drop","resume-drop":
 default:writeError(w,400,"unsupported firewall control action");return
 }
 allowID:=firewall.RuleID("firewalld",in.Zone,in.Port,in.Protocol)
 dropID:=fmt.Sprintf("firewalld:%s:rich:%d/%s:deny:both",in.Zone,in.Port,in.Protocol)
 id:=allowID
 if in.Action=="add-drop"||in.Action=="remove-drop"||in.Action=="pause-drop"||in.Action=="resume-drop"{id=dropID}
 if id!=in.ID{writeError(w,409,"rule changed or stale selection");return}
 s.firewallMu.Lock();defer s.firewallMu.Unlock()
 ctx,cancel:=context.WithTimeout(r.Context(),100*time.Second);defer cancel()
 snap,err:=ops.InspectFirewall(ctx,s.store.Dir())
 if err!=nil{writeError(w,502,"cannot inspect firewall before rule change");return}
 if !snap.Writable||snap.Backend!="firewalld"{
  writeError(w,409,"firewalld read-only; cannot change policies");return
 }
 if snap.OpaqueRichZones[in.Zone]{
  writeError(w,409,"zone contains complex rich rules; edit through original manager");return
 }
 zoneOK:=false
 for _,z:=range snap.Zones{if z==in.Zone{zoneOK=true;break}}
 if !zoneOK{writeError(w,409,"unknown firewalld zone");return}
 for _,key:=range []string{allowID,dropID}{
  if err:=s.ensureNoPending(key);err!=nil{writeError(w,409,err.Error());return}
 }
 target:=firewall.Rule{}
 matching:=0
 // Never operate on a rule that is mixed with a contradictory policy,
 // or another zone's similarly named port.
 for _,rr:=range snap.Rules{
  if rr.Port!=in.Port||rr.Protocol!=in.Protocol{continue}
  if rr.Zone!=in.Zone{
   writeError(w,409,"same port is configured in another zone; use original manager");return
  }
  matching++
  if rr.ID==id{target=rr}
  if rr.Conflict{
   writeError(w,409,"overlapping firewall policies require manual reconciliation");return
  }
 }
 if matching>1{writeError(w,409,"ambiguous firewall rules");return}
 if in.Port==22||in.Port==s.firewallGuardPort(){
  writeError(w,403,"SSH and panel management ports are protected");return
 }
 ownedValue,_,err:=s.store.Setting("firewall_owned:"+id)
 if err!=nil{writeError(w,500,"cannot check rule ownership");return}
 if in.Action!="add-drop"&&ownedValue!="v1"{
  writeError(w,403,"only X-port owned rules can be controlled");return
 }
 // A DROP rule is not a harmless inverse of ALLOW. Never add one to a
 // listening port, including a panel, SSH or a Docker-proxy process.
 if in.Action=="add-drop"||in.Action=="pause-allow"||in.Action=="resume-drop"{
  listeners,err:=ops.FirewallListeners(ctx,s.store.Dir())
  if err!=nil{writeError(w,409,"cannot verify live listeners; unsafe change blocked");return}
  if listeners[in.Port]{
   if in.Action=="add-drop"||in.Action=="resume-drop"{
    writeError(w,409,"active listener detected; DROP activation blocked");return
   }
   // Pause a live Xray rule only when it unambiguously belongs to one
   // enabled Xray account. Other listening services stay protected.
   accounts,err:=s.store.Accounts()
   if err!=nil{writeError(w,500,"cannot verify account ownership");return}
   key:=fmt.Sprintf("%s/%d",in.Protocol,in.Port)
   candidates:=accountFirewallMatches(accounts)[key]
   if len(candidates)!=1||!candidates[0].Enabled{
    writeError(w,409,"non-Xray or ambiguous listening port protected");return
   }
  }
 }
 if in.Action=="resume-allow"||in.Action=="resume-drop"{
  raw,ok,err:=s.store.Setting(firewallPausedKey(id))
  if err!=nil{writeError(w,500,"cannot load paused rule");return}
  if !ok||raw==""{writeError(w,409,"no paused rule to resume");return}
  var paused firewall.Rule
  if json.Unmarshal([]byte(raw),&paused)!=nil||paused.ID!=id||
     ((in.Action=="resume-allow"&&(paused.Kind!="port"||paused.Action!="allow"))||
      (in.Action=="resume-drop"&&(paused.Kind!="rich"||paused.Action!="deny"||paused.Family!="both")))||paused.Port!=in.Port||
     paused.Protocol!=in.Protocol||paused.Zone!=in.Zone||paused.Backend!="firewalld"{
   writeError(w,409,"paused rule snapshot invalid");return
  }
  if matching!=0{writeError(w,409,"rule already exists or was changed externally");return}
  var applyErr error
  if in.Action=="resume-drop"{applyErr=ops.ApplySimpleDrop(ctx,s.store.Dir(),"add",in.Port,in.Protocol,in.Zone)}else{applyErr=ops.ApplyFirewall(ctx,s.store.Dir(),"add",in.Port,in.Protocol,in.Zone)}
  if applyErr!=nil{
   writeError(w,409,"resume failed: "+applyErr.Error());return
  }
  if err:=s.store.SetSetting(firewallPausedKey(id),"");err!=nil{
   rbctx,done:=context.WithTimeout(context.Background(),35*time.Second)
   rollback:=ops.ApplyFirewall(rbctx,s.store.Dir(),"remove",in.Port,in.Protocol,in.Zone);done()
   writeError(w,500,fmt.Sprintf("resume metadata failed; rollback: %v",rollback));return
  }
  s.firewallAudit(in.Action,id)
  writeJSON(w,200,map[string]bool{"ok":true});return
 }
 switch in.Action{
 case "add-drop":
  if matching!=0{writeError(w,409,"port is already governed by another policy");return}
  target=firewall.Rule{ID:id,Port:in.Port,Protocol:in.Protocol,Zone:in.Zone,
   Backend:"firewalld",Kind:"rich",Action:"deny",Family:"both"}
 case "remove-drop","pause-drop":
  if matching!=1||target.ID!=id||target.Kind!="rich"||target.Action!="deny"||
     target.Family!="both"||!target.Runtime||!target.Permanent{
   writeError(w,409,"DROP rule changed or is not a complete dual-family policy");return
  }
  if in.Action=="pause-drop"{
   raw,_,err:=s.store.Setting(firewallPausedKey(id))
   if err!=nil{writeError(w,500,"cannot inspect paused metadata");return}
   if raw!=""{writeError(w,409,"rule already has paused metadata");return}
  }
 case "pause-allow":
  if matching!=1||target.ID!=id||target.Kind!="port"||target.Action!="allow"||
     !target.Runtime||!target.Permanent{
   writeError(w,409,"allow rule changed or runtime/permanent differ");return
  }
  raw,_,err:=s.store.Setting(firewallPausedKey(id))
  if err!=nil{writeError(w,500,"cannot inspect paused metadata");return}
  if raw!=""{writeError(w,409,"rule already has paused metadata");return}
 }
 var unit string
 var deadline int64
 var kind string
 switch in.Action{
 case "add-drop":
  unit,deadline,err=ops.ScheduleFirewallDropUndo(ctx,"remove",in.Port,in.Protocol,in.Zone)
  kind="add-drop"
 case "remove-drop","pause-drop":
  unit,deadline,err=ops.ScheduleFirewallDropUndo(ctx,"add",in.Port,in.Protocol,in.Zone)
  kind=in.Action
 case "pause-allow":
  unit,deadline,err=ops.ScheduleFirewallRestore(ctx,"firewalld",in.Port,in.Protocol,in.Zone)
  kind="pause-allow"
 }
 if err!=nil{writeError(w,409,"cannot arm native 120-second recovery: "+err.Error());return}
 if kind=="pause-allow"{
  err=ops.ApplyFirewall(ctx,s.store.Dir(),"remove",in.Port,in.Protocol,in.Zone)
 }else if kind=="add-drop"{
  err=ops.ApplySimpleDrop(ctx,s.store.Dir(),"add",in.Port,in.Protocol,in.Zone)
 }else{
  err=ops.ApplySimpleDrop(ctx,s.store.Dir(),"remove",in.Port,in.Protocol,in.Zone)
 }
 if err!=nil{
  // Even a failed permanent update might have changed the runtime rule.
  // Leave the independent timer armed rather than assuming full rollback.
  writeError(w,409,"policy operation failed; recovery remains armed: "+err.Error());return
 }
 pending,err:=s.firewallPendingSaveKind(id,unit,deadline,kind,target)
 if err!=nil{
  writeError(w,500,"rule changed but pending confirmation could not be saved; recovery remains armed");return
 }
 s.firewallAudit(kind+"-pending",id)
 writeJSON(w,http.StatusAccepted,map[string]any{
  "ok":true,"pending":true,"id":id,"token":pending.Token,"deadline":pending.Deadline,"kind":kind,
 })
}
