package service

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Kerberos255/X-port/internal/accountcfg"
	"github.com/Kerberos255/X-port/internal/store"
)

func TestExpertPreservesUnknownXHTTPSettings(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil { t.Fatal(err) }
	defer st.Close()
	fa := &fakeApply{}
	s := NewAccounts(st, fa)
	v, err := s.Create(accountcfg.Input{Name:"A", Enabled:true, Port:21001, Protocol:"vless", Network:"xhttp", Security:"reality", ServerName:"example.com", Dest:"example.com:443"})
	if err != nil { t.Fatal(err) }
	accounts, err := st.Accounts(); if err != nil { t.Fatal(err) }
	var stream map[string]any
	if err := json.Unmarshal([]byte(accounts[0].StreamSettingsJSON), &stream); err != nil { t.Fatal(err) }
	x := stream["xhttpSettings"].(map[string]any)
	x["customFutureField"] = "keep-me"
	x["mode"] = "auto"
	b, _ := json.Marshal(stream); accounts[0].StreamSettingsJSON = string(b)
	if err := st.ReplaceAccounts(accounts); err != nil { t.Fatal(err) }

	ex, err := s.Expert(v.ID); if err != nil { t.Fatal(err) }
	if !ex.SupportsReality || !ex.SupportsXHTTP || ex.RealityPublicKey == "" { t.Fatalf("bad expert view: %+v", ex) }
	ex.XHTTPMode = "packet-up"
	ex.XHTTPXPaddingBytes = "100-1000"
	if _, err := s.UpdateExpert(v.ID, ex); err != nil { t.Fatal(err) }
	a, err := s.Raw(v.ID); if err != nil { t.Fatal(err) }
	if err := json.Unmarshal([]byte(a.StreamSettingsJSON), &stream); err != nil { t.Fatal(err) }
	x = stream["xhttpSettings"].(map[string]any)
	if x["customFutureField"] != "keep-me" || x["mode"] != "packet-up" { t.Fatalf("future field lost or mode not updated: %#v", x) }
}

func TestExpertRejectsFallbacksOutsideRawTLS(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite")); if err != nil { t.Fatal(err) }
	defer st.Close(); s := NewAccounts(st, &fakeApply{})
	v, err := s.Create(accountcfg.Input{Name:"A", Enabled:true, Port:21001, Protocol:"vless", Network:"xhttp", Security:"none"})
	if err != nil { t.Fatal(err) }
	ex, err := s.Expert(v.ID); if err != nil { t.Fatal(err) }
	ex.FallbacksJSON = `[{"dest":80}]`
	if _, err := s.UpdateExpert(v.ID, ex); err == nil { t.Fatal("expected fallback validation error") }
}
