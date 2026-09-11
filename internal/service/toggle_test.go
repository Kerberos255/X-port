package service

import (
	"path/filepath"
	"testing"

	"github.com/Kerberos255/X-port/internal/accountcfg"
	"github.com/Kerberos255/X-port/internal/store"
)

func TestSetEnabledPreservesProtocolJSON(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil { t.Fatal(err) }
	defer st.Close()
	fa := &fakeApply{}
	s := NewAccounts(st, fa)
	created, err := s.Create(accountcfg.Input{Name:"xhttp", Enabled:true, Port:24001, Protocol:"vless", Network:"xhttp", Security:"none", Path:"/test"})
	if err != nil { t.Fatal(err) }
	before, err := s.Raw(created.ID)
	if err != nil { t.Fatal(err) }

	view, err := s.SetEnabled(created.ID, false)
	if err != nil { t.Fatal(err) }
	if view.Enabled || view.DisabledReason != "manual" { t.Fatalf("unexpected disabled state: %+v", view) }
	disabled, err := s.Raw(created.ID)
	if err != nil { t.Fatal(err) }
	if disabled.SettingsJSON != before.SettingsJSON || disabled.StreamSettingsJSON != before.StreamSettingsJSON || disabled.SniffingJSON != before.SniffingJSON {
		t.Fatal("toggle changed protocol JSON")
	}

	view, err = s.SetEnabled(created.ID, true)
	if err != nil { t.Fatal(err) }
	if !view.Enabled || view.DisabledReason != "" { t.Fatalf("unexpected enabled state: %+v", view) }
	after, err := s.Raw(created.ID)
	if err != nil { t.Fatal(err) }
	if after.SettingsJSON != before.SettingsJSON || after.StreamSettingsJSON != before.StreamSettingsJSON || after.SniffingJSON != before.SniffingJSON {
		t.Fatal("re-enable changed protocol JSON")
	}
}
