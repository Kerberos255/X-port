package xray

import (
	"encoding/json"
	"testing"

	"github.com/Kerberos255/X-port/internal/model"
)

func testAccount(port int) model.Account { return model.Account{Name:"A",Enabled:true,Port:port,Protocol:"vless",SettingsJSON:`{"clients":[{"id":"x"}],"decryption":"none"}`,StreamSettingsJSON:`{}`,SniffingJSON:`{}`,Tag:"a"} }

func TestBuildConfig(t *testing.T) {
	b,err:=BuildConfig([]model.Account{testAccount(21001)},10085);if err!=nil{t.Fatal(err)};var cfg map[string]any;if json.Unmarshal(b,&cfg)!=nil{t.Fatal("invalid json")};if len(cfg["inbounds"].([]any))!=2{t.Fatal("expected account and API inbound")}
	policy:=cfg["policy"].(map[string]any);system:=policy["system"].(map[string]any)
	if system["statsInboundUplink"]!=true||system["statsInboundDownlink"]!=true{t.Fatalf("inbound stats not enabled: %#v",system)}
	if _,ok:=system["statsOutboundUplink"];ok{t.Fatalf("outbound stats should not be injected: %#v",system)}
	if _,ok:=policy["levels"];ok{t.Fatalf("user stats levels should not be injected: %#v",policy)}
}
func TestBuildConfigRejectsPortConflict(t *testing.T) { if _,err:=BuildConfig([]model.Account{testAccount(10085)},10085);err==nil{t.Fatal("expected conflict")} }
func TestBuildConfigWithBasePreservesGlobals(t *testing.T) {
	base:=`{"dns":{"servers":["1.1.1.1"]},"outbounds":[{"protocol":"freedom","tag":"my-direct"}],"routing":{"domainStrategy":"IPIfNonMatch","rules":[{"type":"field","domain":["example.com"],"outboundTag":"my-direct"},{"type":"field","inboundTag":["api"],"outboundTag":"api"}]},"inbounds":[{"port":9999,"protocol":"http","tag":"old"}],"api":{"tag":"old-api"},"stats":{"old":true}}`
	b,err:=BuildConfigWithBase([]model.Account{testAccount(21001)},10085,base);if err!=nil{t.Fatal(err)};var cfg map[string]any;if err:=json.Unmarshal(b,&cfg);err!=nil{t.Fatal(err)}
	if _,ok:=cfg["dns"];!ok{t.Fatal("dns lost")};outs:=cfg["outbounds"].([]any);if len(outs)!=2||outs[0].(map[string]any)["tag"]!="my-direct"{t.Fatalf("outbounds changed: %#v",outs)}
	ins:=cfg["inbounds"].([]any);if len(ins)!=2{t.Fatalf("old inbounds leaked: %#v",ins)}
	routing:=cfg["routing"].(map[string]any);if routing["domainStrategy"]!="IPIfNonMatch"{t.Fatal("routing option lost")};rules:=routing["rules"].([]any);if len(rules)!=4{t.Fatalf("expected API, two security rules, and custom rule: %#v",rules)}
	api:=cfg["api"].(map[string]any);if api["tag"]!="api"{t.Fatal("api section not replaced")}
}

func TestManagementAPIProtectionKeepsCustomLANRules(t *testing.T) {
	accounts := []model.Account{testAccount(21001), testAccount(21002)}
	accounts[0].Protocol, accounts[0].Tag = "socks", "socks-account"
	accounts[1].Protocol, accounts[1].Tag = "http", "http-account"
	data, err := BuildConfigWithBase(accounts, 12085, "{\"outbounds\":[{\"protocol\":\"freedom\",\"tag\":\"direct\",\"settings\":{\"finalRules\":[{\"action\":\"allow\",\"ip\":[\"geoip:private\"]}]}}],\"routing\":{\"rules\":[{\"type\":\"field\",\"domain\":[\"domain:example.org\"],\"outboundTag\":\"direct\"}]}}")
	if err != nil { t.Fatal(err) }
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil { t.Fatal(err) }
	outbounds := cfg["outbounds"].([]any)
	if len(outbounds) != 2 || outbounds[0].(map[string]any)["tag"] != "direct" ||
		outbounds[1].(map[string]any)["tag"] != managementAPIBlockTag {
		t.Fatalf("original outbound order must be preserved: %#v",outbounds)
	}
	finalRules := outbounds[0].(map[string]any)["settings"].(map[string]any)["finalRules"].([]any)
	if len(finalRules) != 2 || finalRules[1].(map[string]any)["action"] != "allow" {
		t.Fatalf("user's existing LAN allowance must be preserved: %#v",finalRules)
	}
	guard := finalRules[0].(map[string]any)
	if guard["action"] != "block" || guard["port"] != "12085" {
		t.Fatalf("only the configured management port may be blocked: %#v",guard)
	}
	rules := cfg["routing"].(map[string]any)["rules"].([]any)
	if len(rules) != 4 || rules[0].(map[string]any)["outboundTag"] != "api" ||
		rules[3].(map[string]any)["outboundTag"] != "direct" {
		t.Fatalf("API and user routing order must remain intact: %#v",rules)
	}
	for _, raw := range rules[1:3] {
		rule := raw.(map[string]any)
		if rule["outboundTag"] != managementAPIBlockTag || rule["port"] != "12085" || rule["network"] != "tcp" {
			t.Fatalf("guard is broader than management API: %#v",rule)
		}
		tags := rule["inboundTag"].([]any)
		if len(tags)!=2 || tags[0]!="socks-account" || tags[1]!="http-account" {
			t.Fatalf("only client inbounds should be guarded: %#v",rule)
		}
	}
	if rules[1].(map[string]any)["ip"] == nil || rules[2].(map[string]any)["domain"] == nil {
		t.Fatalf("loopback IP and localhost aliases must be guarded: %#v",rules)
	}
}

func TestManagementAPIProtectionRejectsOutboundCollision(t *testing.T) {
	_, err := BuildConfigWithBase(nil, 10085, "{\"outbounds\":[{\"protocol\":\"freedom\",\"tag\":\"xport-management-api-block\"}]}")
	if err == nil { t.Fatal("management safety tag collision accepted") }
}
