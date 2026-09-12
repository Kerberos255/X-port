package service

import (
	"testing"

	"github.com/Kerberos255/X-port/internal/accountcfg"
	"github.com/Kerberos255/X-port/internal/model"
)

func TestEditorViewCurrentMethodAliases(t *testing.T) {
	a := model.Account{
		Name:         "modern-ws",
		Enabled:      true,
		Port:         26001,
		Protocol:     "vless",
		SettingsJSON: `{"clients":[{"id":"11111111-1111-4111-8111-111111111111"}],"decryption":"none"}`,
		StreamSettingsJSON: `{"method":"websocket","security":"tls","wsSettings":{"path":"/ws","headers":{"Host":"cdn.example.com"}}}`,
	}
	v, err := editorView(a)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Editable || v.Network != "ws" || v.Security != "tls" || v.Path != "/ws" || v.Host != "cdn.example.com" {
		t.Fatalf("modern websocket view mismatch: %+v", v)
	}

	a.StreamSettingsJSON = `{"method":"mkcp","security":"none"}`
	v, err = editorView(a)
	if err != nil {
		t.Fatal(err)
	}
	if v.Editable {
		t.Fatalf("unsupported imported transport must be read-only: %+v", v)
	}
}

func TestEditorUpdateCanClearVisionWhenLeavingRaw(t *testing.T) {
	a, err := accountcfg.New(accountcfg.Input{
		Name: "vision", Enabled: true, Port: 26002, Protocol: "vless",
		Network: "tcp", Security: "reality", ServerName: "example.com", Dest: "example.com:443",
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	before, err := accountcfg.ToView(a)
	if err != nil || before.Flow != "xtls-rprx-vision" {
		t.Fatalf("expected Vision source account: %+v err=%v", before, err)
	}
	in := accountcfg.Input{
		Name: a.Name, Enabled: true, Port: a.Port, Protocol: "vless",
		Network: "ws", Security: "tls", Flow: "",
	}
	normalizeEditorInput(&in)
	base := sanitizeEditableBaseForExplicitClears(a, in)
	updated, err := accountcfg.Update(base, in)
	if err != nil {
		t.Fatal(err)
	}
	v, err := accountcfg.ToView(updated)
	if err != nil {
		t.Fatal(err)
	}
	if v.Network != "ws" || v.Security != "tls" || v.Flow != "" {
		t.Fatalf("RAW -> WebSocket must clear Vision cleanly: %+v", v)
	}
}

func TestEditorUpdateCanClearVisibleTransportFields(t *testing.T) {
	a, err := accountcfg.New(accountcfg.Input{
		Name: "ws", Enabled: true, Port: 26003, Protocol: "vmess",
		Network: "ws", Security: "tls", Host: "old.example.com", Path: "/old",
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	in := accountcfg.Input{
		Name: a.Name, Enabled: true, Port: a.Port, Protocol: "vmess",
		Network: "ws", Security: "tls", Host: "", Path: "",
	}
	normalizeEditorInput(&in)
	base := sanitizeEditableBaseForExplicitClears(a, in)
	updated, err := accountcfg.Update(base, in)
	if err != nil {
		t.Fatal(err)
	}
	v, err := accountcfg.ToView(updated)
	if err != nil {
		t.Fatal(err)
	}
	if v.Host != "" || v.Path != "" {
		t.Fatalf("visible WebSocket fields were merged back after clearing: %+v", v)
	}
}

func TestBuiltInEditorProtocolTransportMatrix(t *testing.T) {
	transports := []string{"tcp", "xhttp", "grpc", "ws", "httpupgrade"}
	protocols := []string{"vless", "vmess", "trojan", "shadowsocks"}
	port := 26100
	for _, protocol := range protocols {
		for _, network := range transports {
			securities := []string{"none", "tls"}
			if (protocol == "vless" || protocol == "trojan") && (network == "tcp" || network == "xhttp" || network == "grpc") {
				securities = append(securities, "reality")
			}
			for _, security := range securities {
				port++
				name := protocol + "-" + network + "-" + security
				in := accountcfg.Input{Name: name, Enabled: true, Port: port, Protocol: protocol, Network: network, Security: security}
				if security == "reality" {
					in.ServerName = "example.com"
					in.Dest = "example.com:443"
				}
				normalizeEditorInput(&in)
				a, err := accountcfg.New(in, 0)
				if err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				v, err := editorView(a)
				if err != nil {
					t.Fatalf("%s view: %v", name, err)
				}
				if !v.Editable || v.Network != network || v.Security != security {
					t.Fatalf("%s round-trip mismatch: %+v", name, v)
				}
			}
		}
	}

	for i, protocol := range []string{"socks", "http"} {
		in := accountcfg.Input{Name: protocol, Enabled: true, Port: 26900 + i, Protocol: protocol}
		normalizeEditorInput(&in)
		a, err := accountcfg.New(in, 0)
		if err != nil {
			t.Fatalf("%s: %v", protocol, err)
		}
		v, err := editorView(a)
		if err != nil || !v.Editable || v.Network != "" || v.Security != "none" {
			t.Fatalf("%s editor view mismatch: %+v err=%v", protocol, v, err)
		}
	}
}

func TestNormalizeEditorInputAliases(t *testing.T) {
	cases := map[string]string{"raw": "tcp", "tcp": "tcp", "websocket": "ws", "ws": "ws", "splithttp": "xhttp", "xhttp": "xhttp"}
	for inNetwork, want := range cases {
		in := accountcfg.Input{Protocol: "vless", Network: inNetwork, Security: "none"}
		normalizeEditorInput(&in)
		if in.Network != want {
			t.Fatalf("network %q -> %q, want %q", inNetwork, in.Network, want)
		}
	}
}
