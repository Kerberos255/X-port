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
	if _,ok:=cfg["dns"];!ok{t.Fatal("dns lost")};outs:=cfg["outbounds"].([]any);if len(outs)!=1||outs[0].(map[string]any)["tag"]!="my-direct"{t.Fatalf("outbounds changed: %#v",outs)}
	ins:=cfg["inbounds"].([]any);if len(ins)!=2{t.Fatalf("old inbounds leaked: %#v",ins)}
	routing:=cfg["routing"].(map[string]any);if routing["domainStrategy"]!="IPIfNonMatch"{t.Fatal("routing option lost")};rules:=routing["rules"].([]any);if len(rules)!=2{t.Fatalf("expected one API rule plus custom rule: %#v",rules)}
	api:=cfg["api"].(map[string]any);if api["tag"]!="api"{t.Fatal("api section not replaced")}
}
