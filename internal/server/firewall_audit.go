package server

import (
 "encoding/json"
 "time"
)

type firewallAuditEntry struct{
 Time int64 `json:"time"`
 Action string `json:"action"`
 Rule string `json:"rule"`
}

// A small bounded audit tail lives with the panel settings backup. This
// metadata does not include credentials or remote client addresses.
func (s *Server) firewallAudit(action,rule string){
 raw,_,err:=s.store.Setting("firewall_audit");if err!=nil{return}
 var events []firewallAuditEntry
 if raw!=""{_ = json.Unmarshal([]byte(raw),&events)}
 events=append(events,firewallAuditEntry{Time:time.Now().UnixMilli(),Action:action,Rule:rule})
 if len(events)>100{events=events[len(events)-100:]}
 if data,err:=json.Marshal(events);err==nil{_ = s.store.SetSetting("firewall_audit",string(data))}
}
