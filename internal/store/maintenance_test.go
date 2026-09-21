package store

import (
	"path/filepath"
	"testing"

	"github.com/Kerberos255/X-port/internal/model"
)

func TestAddTrafficBatch(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	accounts := []model.Account{
		{Name: "A", Port: 21001, Protocol: "vless", SettingsJSON: "{}", StreamSettingsJSON: "{}", SniffingJSON: "{}", Tag: "a"},
		{Name: "B", Port: 21002, Protocol: "vless", SettingsJSON: "{}", StreamSettingsJSON: "{}", SniffingJSON: "{}", Tag: "b"},
	}
	if err := st.ReplaceAccounts(accounts); err != nil {
		t.Fatal(err)
	}
	if err := st.AddTrafficBatch([]TrafficDelta{{Tag: "a", Up: 10, Down: 20}, {Tag: "b", Up: 3, Down: 4}}); err != nil {
		t.Fatal(err)
	}
	got, err := st.Accounts()
	if err != nil {
		t.Fatal(err)
	}
	if got[0].UpBytes != 10 || got[0].DownBytes != 20 || got[0].AllTimeBytes != 30 {
		t.Fatalf("account A traffic = %+v", got[0])
	}
	if got[1].UpBytes != 3 || got[1].DownBytes != 4 || got[1].AllTimeBytes != 7 {
		t.Fatalf("account B traffic = %+v", got[1])
	}
}

func TestAddTrafficBatchRejectsNegativeWithoutPartialWrite(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.ReplaceAccounts([]model.Account{{Name: "A", Port: 21001, Protocol: "vless", SettingsJSON: "{}", StreamSettingsJSON: "{}", SniffingJSON: "{}", Tag: "a"}}); err != nil {
		t.Fatal(err)
	}
	if err := st.AddTrafficBatch([]TrafficDelta{{Tag: "a", Up: 10}, {Tag: "a", Down: -1}}); err == nil {
		t.Fatal("expected negative traffic error")
	}
	got, err := st.Accounts()
	if err != nil {
		t.Fatal(err)
	}
	if got[0].UpBytes != 0 || got[0].DownBytes != 0 || got[0].AllTimeBytes != 0 {
		t.Fatalf("traffic changed after rejected batch: %+v", got[0])
	}
}
