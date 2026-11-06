package server

import (
 "context"
 "crypto/rand"
 "crypto/subtle"
 "encoding/hex"
 "encoding/json"
 "errors"
 "net/http"
 "strings"
 "time"

 "github.com/Kerberos255/X-port/internal/firewall"
 "github.com/Kerberos255/X-port/internal/ops"
)

type firewallPending struct {
 ID string `json:"id"`
 Token string `json:"token"`
 Unit string `json:"unit"`
 Deadline int64 `json:"deadline"`
 Kind string `json:"kind,omitempty"`
 Rule firewall.Rule `json:"rule,omitempty"`
}
func firewallPendingKey(id string)string{return "firewall_pending:"+id}
func newFirewallToken()(string,error){
 b:=make([]byte,16)
 if _,err:=rand.Read(b);err!=nil{return "",err}
 return hex.EncodeToString(b),nil
}
func (s *Server) firewallPendingSave(id,unit string,deadline int64)(firewallPending,error){
 return s.firewallPendingSaveKind(id,unit,deadline,"remove-allow",firewall.Rule{})
}
func (s *Server) firewallPendingSaveKind(id,unit string,deadline int64,kind string,rule firewall.Rule)(firewallPending,error){
 token,err:=newFirewallToken();if err!=nil{return firewallPending{},err}
 if id==""||unit==""||deadline<=0{return firewallPending{},errors.New("invalid pending firewall action")}
 entry:=firewallPending{ID:id,Token:token,Unit:unit,Deadline:deadline,Kind:kind,Rule:rule}
 raw,err:=json.Marshal(entry);if err!=nil{return firewallPending{},err}
 err=s.store.SetSetting(firewallPendingKey(id),string(raw))
 return entry,err
}
func firewallPausedKey(id string)string{return "firewall_paused:"+id}
func pendingFromSettings(settings map[string]string)[]firewallPending{
 var results []firewallPending
 for key,value:=range settings{
  if !strings.HasPrefix(key,"firewall_pending:")||value==""{continue}
  var p firewallPending
  if json.Unmarshal([]byte(value),&p)!=nil||p.ID==""||p.Unit==""{continue}
  results=append(results,p)
 }
 return results
}
func (s *Server) firewallRuleConfirm(w http.ResponseWriter,r *http.Request){
 var in struct{ID string `json:"id"`;Token string `json:"token"`}
 if decodeJSONLimit(w,r,&in,2<<10)!=nil{return}
 if len(in.ID)>140||len(in.Token)!=32{writeError(w,400,"invalid confirmation");return}
 s.firewallMu.Lock();defer s.firewallMu.Unlock()
 raw,ok,err:=s.store.Setting(firewallPendingKey(in.ID))
 if err!=nil{writeError(w,500,"could not read pending action");return}
 if !ok||raw==""{writeError(w,404,"pending action not found");return}
 var p firewallPending
 if err=json.Unmarshal([]byte(raw),&p);err!=nil||p.ID!=in.ID||subtle.ConstantTimeCompare([]byte(p.Token),[]byte(in.Token))!=1{
  writeError(w,403,"invalid pending action token");return
 }
 if time.Now().UnixMilli()>=p.Deadline{writeError(w,409,"confirmation expired; automatic restoration may have started");return}
 ctx,cancel:=context.WithTimeout(r.Context(),35*time.Second);defer cancel()
 snap,err:=ops.InspectFirewall(ctx,s.store.Dir())
 if err!=nil{writeError(w,502,"cannot verify firewall before confirmation");return}
 expectsPresent:=p.Kind=="add-drop"
 correct:=func(snapshot firewall.Snapshot)bool{
  present:=false
  for _,rule:=range snapshot.Rules{
   if rule.ID!=p.ID{continue}
   if p.Kind=="add-drop"&&(rule.Action!="deny"||rule.Kind!="rich"||rule.Family!="both"||!rule.Runtime||!rule.Permanent||rule.Conflict){return false}
   present=true
  }
  return present==expectsPresent
 }
 if !correct(snap){writeError(w,409,"rule state changed; confirmation rejected");return}
 // Persist any resume metadata before disabling automatic recovery. If this
 // DB operation fails the timer remains armed and the old state is restored.
 stagedKey,stagedValue:="",""
 if p.Kind=="pause-allow"{
  if p.Rule.ID!=p.ID||p.Rule.Kind!="port"||p.Rule.Action!="allow"{writeError(w,409,"invalid paused rule snapshot");return}
  body,err:=json.Marshal(p.Rule)
  if err!=nil{writeError(w,500,"cannot encode paused rule");return}
  stagedKey,stagedValue=firewallPausedKey(p.ID),string(body)
 }else if p.Kind=="add-drop"{
  stagedKey,stagedValue="firewall_owned:"+p.ID,"v1"
 }
 if stagedKey!=""{
  if err:=s.store.SetSetting(stagedKey,stagedValue);err!=nil{writeError(w,500,"cannot persist confirmation; recovery remains armed");return}
 }
 if err:=ops.CancelFirewallRestore(ctx,p.Unit);err!=nil{
  if stagedKey!=""{_ = s.store.SetSetting(stagedKey,"")}
  writeError(w,409,err.Error());return
 }
 verified,err:=ops.InspectFirewall(ctx,s.store.Dir())
 if err!=nil{writeError(w,502,"recovery cancelled, but final rule state could not be verified");return}
 if !correct(verified){writeError(w,409,"rule changed during confirmation; refresh before retrying");return}
 // Pausing retains ownership and note, whereas permanent deletion drops it.
 switch p.Kind{
 case "add-drop","pause-allow":
  if err:=s.store.SetSetting(firewallPendingKey(p.ID),"");err!=nil{writeError(w,500,"could not clear pending state");return}
  s.firewallAudit(p.Kind,p.ID)
 case "","remove-allow","remove-drop":
  for _,prefix:=range []string{"firewall_owned:","firewall_note:","firewall_purpose:","firewall_pending:","firewall_paused:"}{
   if err:=s.store.SetSetting(prefix+p.ID,"");err!=nil{writeError(w,500,"recovery cancelled but metadata cleanup failed");return}
  }
  s.firewallAudit("remove",p.ID)
 default:
  writeError(w,409,"unrecognized pending action");return
 }
 writeJSON(w,200,map[string]bool{"ok":true})
}
func (s *Server) ensureNoPending(id string)error{
 raw,_,err:=s.store.Setting(firewallPendingKey(id));if err!=nil{return err}
 if raw!=""{
  var p firewallPending
  if json.Unmarshal([]byte(raw),&p)==nil&&p.Deadline>time.Now().UnixMilli(){
   return errors.New("rule has a pending removal confirmation")
  }
 }
 return nil
}
