package service

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/Kerberos255/X-port/internal/model"
	"github.com/Kerberos255/X-port/internal/store"
)

func TestAdvancedProxyProtocolPreservesImportedXHTTP(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil { t.Fatal(err) }
	defer st.Close()
	now := time.Now().UnixMilli()
	original := model.Account{
		ID: 1, Name: "xhttp", Enabled: true, Port: 40836, Protocol: "vless",
		SettingsJSON: `{"clients":[{"id":"ca11e8f4-d83d-4bc5-a000-000000000001","email":"x@test"}],"decryption":"none"}`,
		StreamSettingsJSON: `{"network":"xhttp","security":"none","xhttpSettings":{"path":"/vlessxhttptest","host":"","mode":"packet-up","scMaxBufferedPosts":30,"xPaddingBytes":"100-1000"}}`,
		SniffingJSON: `{}`, Tag: "in-40836", CreatedAt: now, UpdatedAt: now,
	}
	if err := st.ReplaceAccounts([]model.Account{original}); err != nil { t.Fatal(err) }
	fa := &fakeApply{}
	svc := NewAccounts(st, fa)

	out, err := svc.UpdateAdvanced(1, AdvancedConfig{AcceptProxyProtocol: true})
	if err != nil { t.Fatal(err) }
	if !out.AcceptProxyProtocol || out.Network != "xhttp" { t.Fatalf("unexpected advanced view: %+v", out) }

	saved, err := svc.Raw(1)
	if err != nil { t.Fatal(err) }
	var stream map[string]any
	if err := json.Unmarshal([]byte(saved.StreamSettingsJSON), &stream); err != nil { t.Fatal(err) }
	xhttp := stream["xhttpSettings"].(map[string]any)
	if xhttp["mode"] != "packet-up" || xhttp["path"] != "/vlessxhttptest" || xhttp["xPaddingBytes"] != "100-1000" {
		t.Fatalf("imported XHTTP settings changed: %s", saved.StreamSettingsJSON)
	}
	sock := stream["sockopt"].(map[string]any)
	if sock["acceptProxyProtocol"] != true { t.Fatalf("proxy protocol not enabled: %s", saved.StreamSettingsJSON) }
}

func TestAdvancedFallbacksRequireTCPAndTLS(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil { t.Fatal(err) }
	defer st.Close()
	now := time.Now().UnixMilli()
	a := model.Account{
		ID: 1, Name: "xhttp", Enabled: true, Port: 40836, Protocol: "vless",
		SettingsJSON: `{"clients":[{"id":"ca11e8f4-d83d-4bc5-a000-000000000001"}],"decryption":"none"}`,
		StreamSettingsJSON: `{"network":"xhttp","security":"none","xhttpSettings":{"path":"/"}}`,
		SniffingJSON: `{}`, Tag: "in-40836", CreatedAt: now, UpdatedAt: now,
	}
	if err := st.ReplaceAccounts([]model.Account{a}); err != nil { t.Fatal(err) }
	svc := NewAccounts(st, &fakeApply{})
	if _, err := svc.UpdateAdvanced(1, AdvancedConfig{FallbacksJSON: `[{"dest":80}]`}); err == nil {
		t.Fatal("expected fallbacks validation failure")
	}
}
