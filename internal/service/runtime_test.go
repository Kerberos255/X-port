package service

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Kerberos255/X-port/internal/accountcfg"
	"github.com/Kerberos255/X-port/internal/model"
	"github.com/Kerberos255/X-port/internal/store"
	"github.com/Kerberos255/X-port/internal/xray"
)

func TestMonthlyResetReenablesOnlyQuotaSuspendedAccount(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil { t.Fatal(err) }
	defer st.Close()
	a, err := accountcfg.New(accountcfg.Input{Name:"A", Enabled:true, Port:21001, Protocol:"vless", ServerName:"example.com", QuotaBytes:100, MonthlyReset:true},21001)
	if err != nil { t.Fatal(err) }
	if err := st.ReplaceAccounts([]model.Account{a}); err != nil { t.Fatal(err) }
	fa := &fakeApply{}
	svc := NewAccounts(st, fa)
	jan := time.Date(2026,1,31,23,59,0,0,time.Local)
	if err := svc.ApplyTraffic(map[string]xray.Traffic{a.Tag:{Up:120}}, jan); err != nil { t.Fatal(err) }
	got, _ := st.Accounts()
	if got[0].Enabled || got[0].DisabledReason != "quota" || got[0].UpBytes != 120 || got[0].AllTimeBytes != 120 { t.Fatalf("quota state: %+v", got[0]) }
	feb := time.Date(2026,2,1,0,1,0,0,time.Local)
	if err := svc.ApplyTraffic(nil, feb); err != nil { t.Fatal(err) }
	got, _ = st.Accounts()
	if !got[0].Enabled || got[0].DisabledReason != "" || got[0].UpBytes != 0 || got[0].DownBytes != 0 || got[0].AllTimeBytes != 120 || got[0].LastMonthlyReset != "2026-02" { t.Fatalf("monthly reset: %+v", got[0]) }
	if err := svc.ApplyTraffic(map[string]xray.Traffic{a.Tag:{Up:5}}, feb.Add(time.Hour)); err != nil { t.Fatal(err) }
	got, _ = st.Accounts()
	if got[0].UpBytes != 5 || got[0].AllTimeBytes != 125 { t.Fatalf("reset repeated: %+v", got[0]) }
}

func TestMonthlyResetDoesNotEnableManualDisable(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil { t.Fatal(err) }
	defer st.Close()
	a, err := accountcfg.New(accountcfg.Input{Name:"B", Enabled:false, Port:21002, Protocol:"vless", ServerName:"example.com", MonthlyReset:true},21002)
	if err != nil { t.Fatal(err) }
	a.UpBytes = 42
	a.AllTimeBytes = 42
	if err := st.ReplaceAccounts([]model.Account{a}); err != nil { t.Fatal(err) }
	svc := NewAccounts(st, &fakeApply{})
	if err := svc.ApplyTraffic(nil, time.Date(2026,3,1,8,0,0,0,time.Local)); err != nil { t.Fatal(err) }
	got, _ := st.Accounts()
	if got[0].Enabled || got[0].DisabledReason != "manual" || got[0].UpBytes != 0 || got[0].AllTimeBytes != 42 { t.Fatalf("manual disable changed: %+v", got[0]) }
}
