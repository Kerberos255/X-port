package xray

import (
	"encoding/json"
	"fmt"

	"github.com/Kerberos255/X-port/internal/model"
)

type inbound struct {
	Listen         string          `json:"listen,omitempty"`
	Port           int             `json:"port"`
	Protocol       string          `json:"protocol"`
	Settings       json.RawMessage `json:"settings"`
	StreamSettings json.RawMessage `json:"streamSettings,omitempty"`
	Tag            string          `json:"tag"`
	Sniffing       json.RawMessage `json:"sniffing,omitempty"`
}

func BuildConfig(accounts []model.Account, apiPort int) ([]byte, error) {
	return BuildConfigWithBase(accounts, apiPort, "")
}

// BuildConfigWithBase rebuilds all managed inbounds while retaining safe global
// Xray sections migrated from the previous panel (DNS/routing/outbounds/etc.).
// X-port always owns api, stats, inbounds and the stats policy switches.
func BuildConfigWithBase(accounts []model.Account, apiPort int, baseJSON string) ([]byte, error) {
	inbounds := make([]inbound, 0, len(accounts)+1)
	seenPorts := map[int]bool{}
	seenTags := map[string]bool{}
	for _, a := range accounts {
		if !a.Enabled { continue }
		if a.Port < 1 || a.Port > 65535 { return nil, fmt.Errorf("account %q has invalid port %d",a.Name,a.Port) }
		if seenPorts[a.Port] { return nil, fmt.Errorf("duplicate port %d",a.Port) }
		if a.Tag=="" { return nil,fmt.Errorf("account %q has empty tag",a.Name) }
		if seenTags[a.Tag] { return nil,fmt.Errorf("duplicate tag %q",a.Tag) }
		seenPorts[a.Port],seenTags[a.Tag]=true,true
		if !json.Valid([]byte(a.SettingsJSON)){return nil,fmt.Errorf("account %q has invalid settings JSON",a.Name)}
		if !json.Valid([]byte(a.StreamSettingsJSON)){return nil,fmt.Errorf("account %q has invalid stream settings JSON",a.Name)}
		if !json.Valid([]byte(a.SniffingJSON)){return nil,fmt.Errorf("account %q has invalid sniffing JSON",a.Name)}
		inbounds=append(inbounds,inbound{Listen:a.Listen,Port:a.Port,Protocol:a.Protocol,Settings:json.RawMessage(a.SettingsJSON),StreamSettings:json.RawMessage(a.StreamSettingsJSON),Tag:a.Tag,Sniffing:json.RawMessage(a.SniffingJSON)})
	}
	if apiPort<=0{apiPort=10085};if seenPorts[apiPort]{return nil,fmt.Errorf("Xray API port %d conflicts with an account",apiPort)}
	inbounds=append(inbounds,inbound{Listen:"127.0.0.1",Port:apiPort,Protocol:"dokodemo-door",Settings:json.RawMessage(`{"address":"127.0.0.1"}`),Tag:"api"})

	cfg:=map[string]any{}
	if baseJSON!="" {
		if err:=json.Unmarshal([]byte(baseJSON),&cfg);err!=nil{return nil,fmt.Errorf("invalid migrated global Xray config: %w",err)}
	}
	if _,ok:=cfg["log"];!ok{cfg["log"]=map[string]any{"loglevel":"warning"}}
	if _,ok:=cfg["outbounds"];!ok{cfg["outbounds"]=[]any{map[string]any{"protocol":"freedom","tag":"direct"},map[string]any{"protocol":"blackhole","tag":"blocked"}}}
	cfg["api"]=map[string]any{"tag":"api","services":[]string{"HandlerService","LoggerService","StatsService"}}
	cfg["stats"]=map[string]any{}
	cfg["inbounds"]=inbounds

	policy:=mapObject(cfg["policy"]);system:=mapObject(policy["system"])
	// X-port persists traffic by inbound tag. Do not enable outbound/user
	// counters unless the preserved user policy explicitly asks for them.
	system["statsInboundUplink"],system["statsInboundDownlink"]=true,true
	policy["system"]=system
	cfg["policy"]=policy

	routing:=mapObject(cfg["routing"]);oldRules,_:=routing["rules"].([]any);rules:=make([]any,0,len(oldRules)+1)
	rules=append(rules,map[string]any{"type":"field","inboundTag":[]string{"api"},"outboundTag":"api"})
	for _,r:=range oldRules{if !isAPIRule(r){rules=append(rules,r)}}
	routing["rules"]=rules;cfg["routing"]=routing
	return json.MarshalIndent(cfg,"","  ")
}

func mapObject(v any) map[string]any { if m,ok:=v.(map[string]any);ok&&m!=nil{return m};return map[string]any{} }
func isAPIRule(v any) bool { m,ok:=v.(map[string]any);if !ok||m["outboundTag"]!="api"{return false};tags,ok:=m["inboundTag"].([]any);if ok{for _,t:=range tags{if t=="api"{return true}}};if tags2,ok:=m["inboundTag"].([]string);ok{for _,t:=range tags2{if t=="api"{return true}}};return false }
