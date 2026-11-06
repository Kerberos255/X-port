package server

import (
 "context"
 "net"
 "net/http"
 "strconv"
 "strings"
 "time"
 "github.com/Kerberos255/X-port/internal/firewall"
)

type firewallViewRule struct {
 firewall.Rule
 Purpose string `json:"purpose"`
 PurposeSource string `json:"purposeSource"`
 Note string `json:"note"`
 Protected bool `json:"protected"`
 Xray bool `json:"xray"`
}
type firewallView struct {
 firewall.Snapshot
 Rules []firewallViewRule `json:"rules"`
}

func (s *Server) firewallGuardPort() int {
 listen:=settingString(s.store,"panel_listen","127.0.0.1:8080")
 _,p,err:=net.SplitHostPort(listen);if err!=nil{return 8080}
 n,err:=strconv.Atoi(p);if err!=nil||n<1||n>65535{return 8080}
 return n
}
func (s *Server) firewallRead(w http.ResponseWriter,r *http.Request){
 ctx,cancel:=context.WithTimeout(r.Context(),15*time.Second);defer cancel()
 snap,err:=firewall.Inspect(ctx);if err!=nil{writeError(w,502,"failed to inspect firewall rules");return}
 w.Header().Set("Cache-Control","no-store")
 accounts,err:=s.store.Accounts();if err!=nil{writeError(w,500,"database error");return}
 settings,err:=s.store.Settings();if err!=nil{writeError(w,500,"database error");return}
 byPort:=make(map[int]string,len(accounts))
 for _,a:=range accounts{byPort[a.Port]="Xray · "+strings.ToUpper(a.Protocol)}
 rules:=make([]firewallViewRule,0,len(snap.Rules))
 for _,rr:=range snap.Rules {
  purpose,xray:="未识别",false
  if val,ok:=byPort[rr.Port];ok {purpose,xray=val,true}
  source:="unknown"
  if xray{source="xray"}
  protected:=rr.Port==22||rr.Port==s.firewallGuardPort()
  if !xray&&rr.Port==22{purpose="SSH（常见端口，未核实）";source="assumed"}
  if !xray&&rr.Port==s.firewallGuardPort(){purpose="X-port 管理面板";source="panel"}
  if custom:=settings["firewall_purpose:"+rr.ID];custom!=""{purpose=custom;source="manual"}
  rules=append(rules,firewallViewRule{Rule:rr,Purpose:purpose,PurposeSource:source,Note:settings["firewall_note:"+rr.ID],Protected:protected,Xray:xray})
 }
 writeJSON(w,200,firewallView{Snapshot:snap,Rules:rules})
}
func (s *Server) firewallNote(w http.ResponseWriter,r *http.Request){
 var in struct{ID string `json:"id"`; Note string `json:"note"`}
 if decodeJSONLimit(w,r,&in,4<<10)!=nil{return}
 note:=strings.TrimSpace(in.Note)
 if len(note)>360||len([]rune(note))>120{writeError(w,400,"note is too long");return}
 ctx,cancel:=context.WithTimeout(r.Context(),15*time.Second);defer cancel()
 snap,err:=firewall.Inspect(ctx);if err!=nil{writeError(w,502,"failed to inspect firewall rules");return}
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
 if !firewall.ValidRule(in.Port,in.Protocol)||(in.Action!="add"&&in.Action!="remove"){writeError(w,400,"invalid firewall rule");return}
 if in.Action=="remove"&&(in.Port==22||in.Port==s.firewallGuardPort()){writeError(w,403,"protected management port cannot be removed");return}
 ctx,cancel:=context.WithTimeout(r.Context(),20*time.Second);defer cancel()
 if err:=firewall.Change(ctx,in.Action,in.Port,in.Protocol,in.Zone);err!=nil{writeError(w,409,"firewall rule was not changed: "+err.Error());return}
 writeJSON(w,200,map[string]bool{"ok":true})
}
func (s *Server) firewallRaw(w http.ResponseWriter,r *http.Request){
 ctx,cancel:=context.WithTimeout(r.Context(),12*time.Second);defer cancel()
 raw,err:=firewall.Raw(ctx);if err!=nil{writeError(w,502,"could not read native firewall rules");return}
 w.Header().Set("Cache-Control","no-store")
 writeJSON(w,200,map[string]string{"ruleset":raw})
}

func (s *Server) firewallPurpose(w http.ResponseWriter,r *http.Request){
 var in struct{ID string `json:"id"`; Purpose string `json:"purpose"`}
 if decodeJSONLimit(w,r,&in,4<<10)!=nil{return}
 purpose:=strings.TrimSpace(in.Purpose)
 if len(purpose)>180||len([]rune(purpose))>60{writeError(w,400,"purpose is too long");return}
 ctx,cancel:=context.WithTimeout(r.Context(),15*time.Second);defer cancel()
 snap,err:=firewall.Inspect(ctx);if err!=nil{writeError(w,502,"failed to inspect firewall rules");return}
 found:=false;for _,rr:=range snap.Rules{if rr.ID==in.ID{found=true;break}}
 if !found{writeError(w,404,"firewall rule was not found");return}
 if err:=s.store.SetSetting("firewall_purpose:"+in.ID,purpose);err!=nil{writeError(w,500,"could not save purpose");return}
 writeJSON(w,200,map[string]string{"purpose":purpose})
}
