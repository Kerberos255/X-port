package service

import (
	"path/filepath"
	"testing"

	"github.com/Kerberos255/X-port/internal/accountcfg"
	"github.com/Kerberos255/X-port/internal/model"
	"github.com/Kerberos255/X-port/internal/store"
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

func TestEditorPersistedRealityTransportSecurityTransitions(t *testing.T) {
 st,err:=store.Open(filepath.Join(t.TempDir(),"editor-reality.sqlite"))
 if err!=nil {t.Fatal(err)}
 defer st.Close()
 svc:=NewAccounts(st,&fakeApply{})
 initial,err:=svc.Create(accountcfg.Input{
  Name:"Account", Enabled:true, Port:26950, Protocol:"vless",
  Network:"xhttp",Security:"none", Path:"/first",
 })
 if err!=nil {t.Fatal(err)}
 current,err:=svc.Update(initial.ID,accountcfg.Input{
  Name:"Reality",Enabled:true,Port:initial.Port,Protocol:"vless",
  Network:"xhttp",Security:"reality",
  ServerName:"one.example.com",Dest:"one.example.com:443",
 })
 if err!=nil {t.Fatal(err)}
 if current.Network!="xhttp"||current.Security!="reality"||current.PrivateKey==""||current.ShortID=="" {
  t.Fatalf("REALITY update incomplete: %+v",current)
 }
 expert,err:=svc.Expert(initial.ID)
 if err!=nil {t.Fatal(err)}
 if !expert.SupportsReality||!expert.SupportsXHTTP||expert.RealityPublicKey==""{
  t.Fatalf("REALITY expert section missing: %+v",expert)
 }
 expert.RealityServerNames=[]string{"one.example.com","two.example.com"}
 expert.RealityShortIDs=[]string{current.ShortID,"1234abcd"}
 if _,err=svc.UpdateExpert(initial.ID,expert);err!=nil {t.Fatal(err)}
 updated,err:=svc.Update(initial.ID,accountcfg.Input{
  Name:"Renamed",Enabled:true,Port:initial.Port,Protocol:"vless",
  Network:"xhttp",Security:"reality",
 })
 if err!=nil {t.Fatal(err)}
 again,err:=svc.Expert(updated.ID)
 if err!=nil {t.Fatal(err)}
 if len(again.RealityServerNames)!=2||again.RealityServerNames[1]!="two.example.com"||
  len(again.RealityShortIDs)!=2||again.RealityShortIDs[1]!="1234abcd" {
  t.Fatalf("ordinary edit destroyed advanced REALITY arrays: %+v",again)
 }
 cloned,err:=svc.Clone(initial.ID,"Clone",26951)
 if err!=nil {t.Fatal(err)}
 cloneExpert,err:=svc.Expert(cloned.ID)
 if err!=nil {t.Fatal(err)}
 if cloned.PrivateKey==updated.PrivateKey||cloned.ShortID==updated.ShortID||
  cloneExpert.RealityPublicKey==again.RealityPublicKey||
  len(cloneExpert.RealityShortIDs)!=2 {
  t.Fatalf("REALITY clone shared secrets or lost Short ID slots")
 }
 // A transport/security switch must leave Vision cleared and the saved
 // configuration editable, without mutating the original clone.
 changed,err:=svc.Update(initial.ID,accountcfg.Input{
  Name:"Websocket",Enabled:true,Port:initial.Port,Protocol:"vless",
  Network:"ws",Security:"tls",Host:"host.example.com",Path:"/ws",
 })
 if err!=nil {t.Fatal(err)}
 if changed.Flow!=""||changed.Network!="ws"||changed.Security!="tls"{
  t.Fatalf("security downgrade retained incompatible Vision: %+v",changed)
 }
 before,err:=svc.Raw(initial.ID)
 if err!=nil {t.Fatal(err)}
 if _,err=svc.Update(initial.ID,accountcfg.Input{
  Name:"invalid",Enabled:true,Port:initial.Port,Protocol:"vless",
  Network:"ws",Security:"reality",ServerName:"bad.example.com",
 });err==nil {t.Fatal("WebSocket + REALITY must be rejected")}
 after,err:=svc.Raw(initial.ID)
 if err!=nil {t.Fatal(err)}
 if before.StreamSettingsJSON!=after.StreamSettingsJSON||
  before.SettingsJSON!=after.SettingsJSON {t.Fatal("invalid transition modified persisted account")}
}
