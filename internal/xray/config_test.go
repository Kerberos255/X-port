package xray

import (
	"encoding/json"
	"github.com/Kerberos255/X-port/internal/model"
	"testing"
)

func TestBuildConfig(t *testing.T) {
	a := []model.Account{{Name: "A", Enabled: true, Port: 21001, Protocol: "vless", SettingsJSON: `{"clients":[{"id":"x"}],"decryption":"none"}`, StreamSettingsJSON: `{}`, SniffingJSON: `{}`, Tag: "a"}}
	b, err := BuildConfig(a, 10085)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if json.Unmarshal(b, &cfg) != nil {
		t.Fatal("invalid json")
	}
	if len(cfg["inbounds"].([]any)) != 2 {
		t.Fatal("expected account and API inbound")
	}
}
func TestBuildConfigRejectsPortConflict(t *testing.T) {
	a := []model.Account{{Name: "A", Enabled: true, Port: 10085, Protocol: "vless", SettingsJSON: `{}`, StreamSettingsJSON: `{}`, SniffingJSON: `{}`, Tag: "a"}}
	if _, err := BuildConfig(a, 10085); err == nil {
		t.Fatal("expected conflict")
	}
}
