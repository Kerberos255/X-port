package accountcfg

import (
	"net/url"
	"strings"
	"testing"

	"github.com/Kerberos255/X-port/internal/model"
)

func TestNewCloneShareReality(t *testing.T) {
	a, err := New(Input{Name: "Alpha", Enabled: true, Port: 21001, Protocol: "vless", ServerName: "www.example.com", Dest: "www.example.com:443"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	v, err := ToView(a)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Editable || v.Credential == "" || v.PrivateKey == "" || v.ShortID == "" {
		t.Fatalf("incomplete view: %+v", v)
	}
	uri, err := ShareURI(a, "proxy.example.net")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "vless" || !strings.Contains(u.Host, ":21001") || u.Query().Get("pbk") == "" {
		t.Fatalf("bad uri: %s", uri)
	}
	clone, err := Clone(a, "Beta", 21002)
	if err != nil {
		t.Fatal(err)
	}
	cv, _ := ToView(clone)
	if cv.Credential == v.Credential || clone.Port != 21002 || clone.ID != 0 {
		t.Fatalf("clone not isolated: %+v", cv)
	}
}

func TestCommonProtocolEditorsShareAndClone(t *testing.T) {
	tests := []struct{ name, protocol, security, wantPrefix string }{
		{"vmess", "vmess", "none", "vmess://"},
		{"trojan", "trojan", "reality", "trojan://"},
		{"shadowsocks", "shadowsocks", "none", "ss://"},
		{"socks", "socks", "none", "socks://"},
		{"http", "http", "none", "http://"},
	}
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := Input{Name: tc.name, Enabled: true, Port: 22000 + i, Protocol: tc.protocol, Security: tc.security}
			if tc.security == "reality" {
				in.ServerName = "www.example.com"
				in.Dest = "www.example.com:443"
			}
			a, err := New(in, 0)
			if err != nil {
				t.Fatal(err)
			}
			v, err := ToView(a)
			if err != nil {
				t.Fatal(err)
			}
			if !v.Editable {
				t.Fatalf("not editable: %+v", v)
			}
			uri, err := ShareURI(a, "proxy.example.net")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(uri, tc.wantPrefix) {
				t.Fatalf("unexpected share %q", uri)
			}
			clone, err := Clone(a, tc.name+"-copy", 23000+i)
			if err != nil {
				t.Fatal(err)
			}
			cv, err := ToView(clone)
			if err != nil {
				t.Fatal(err)
			}
			if clone.Port == a.Port {
				t.Fatal("port was not changed")
			}
			switch tc.protocol {
			case "vless", "vmess":
				if cv.Credential == v.Credential {
					t.Fatal("UUID was reused")
				}
			case "trojan", "shadowsocks":
				if cv.Password == v.Password && cv.ServerPassword == v.ServerPassword {
					t.Fatal("password was reused")
				}
			case "socks", "http":
				if cv.Password == v.Password {
					t.Fatal("password was reused")
				}
			}
		})
	}
}

func TestPublicViewRedactsCredentials(t *testing.T) {
	a, err := New(Input{Name: "Secret", Enabled: true, Port: 24001, Protocol: "vless", ServerName: "example.com"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	full, _ := ToView(a)
	pub := PublicView(a)
	if full.Credential == "" || full.PrivateKey == "" {
		t.Fatal("test account missing secret")
	}
	if pub.Credential != "" || pub.PrivateKey != "" || pub.Password != "" || pub.ServerPassword != "" {
		t.Fatalf("secret leaked: %+v", pub)
	}
}

func TestNextPort(t *testing.T) {
	a, _ := New(Input{Name: "A", Enabled: true, Port: 20000, ServerName: "a.example"}, 0)
	if got := NextPort([]model.Account{a}); got != 20001 {
		t.Fatalf("got %d", got)
	}
}

func TestVLESSVisionDefaultsOnlyForRawTLSOrReality(t *testing.T) {
	raw, err := New(Input{Name: "raw", Enabled: true, Port: 25001, Protocol: "vless", Network: "raw", Security: "reality", ServerName: "example.com", Dest: "example.com:443"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	rawView, err := ToView(raw)
	if err != nil {
		t.Fatal(err)
	}
	if rawView.Flow != "xtls-rprx-vision" {
		t.Fatalf("RAW+REALITY should default Vision, got %q", rawView.Flow)
	}

	xhttp, err := New(Input{Name: "xhttp", Enabled: true, Port: 25002, Protocol: "vless", Network: "xhttp", Security: "reality", ServerName: "example.com", Dest: "example.com:443"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	xhttpView, err := ToView(xhttp)
	if err != nil {
		t.Fatal(err)
	}
	if xhttpView.Flow != "" {
		t.Fatalf("XHTTP must not default Vision without VLESS Encryption, got %q", xhttpView.Flow)
	}
}

func TestRejectsUnsupportedRealityAndVisionCombinations(t *testing.T) {
	if _, err := New(Input{Name: "ws-reality", Enabled: true, Port: 25101, Protocol: "vless", Network: "ws", Security: "reality", ServerName: "example.com", Dest: "example.com:443"}, 0); err == nil {
		t.Fatal("expected WebSocket+REALITY to be rejected")
	}
	if _, err := New(Input{Name: "xhttp-vision", Enabled: true, Port: 25102, Protocol: "vless", Network: "xhttp", Security: "reality", Flow: "xtls-rprx-vision", ServerName: "example.com", Dest: "example.com:443"}, 0); err == nil {
		t.Fatal("expected XHTTP+Vision to be rejected by built-in editor")
	}
}

func TestVLESSEncryptionImportIsReadOnly(t *testing.T) {
	a := model.Account{
		Name:               "encrypted-import",
		Enabled:            true,
		Port:               25201,
		Protocol:           "vless",
		SettingsJSON:       `{"clients":[{"id":"11111111-1111-4111-8111-111111111111"}],"decryption":"example-encryption"}`,
		StreamSettingsJSON: `{"network":"xhttp","security":"none","xhttpSettings":{"path":"/"}}`,
	}
	v, err := ToView(a)
	if err != nil {
		t.Fatal(err)
	}
	if v.Editable {
		t.Fatal("VLESS Encryption import must remain read-only until a dedicated editor exists")
	}
}
