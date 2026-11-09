package service

import (
	"github.com/Kerberos255/X-port/internal/accountcfg"
	"github.com/Kerberos255/X-port/internal/model"
	"github.com/Kerberos255/X-port/internal/store"
	"path/filepath"
	"testing"
)

type fakeApply struct {
	calls int
	fail  bool
	last  []model.Account
}

func (f *fakeApply) Apply(a []model.Account) error {
	f.calls++
	f.last = append([]model.Account(nil), a...)
	if f.fail {
		return errFake
	}
	return nil
}

type fakeErr string

func (e fakeErr) Error() string { return string(e) }

var errFake = fakeErr("apply failed")

func TestCRUDAndClone(t *testing.T) {
	st, e := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer st.Close()
	fa := &fakeApply{}
	s := NewAccounts(st, fa)
	a, e := s.Create(accountcfg.Input{Name: "A", Enabled: true, Port: 21001, ServerName: "a.example"})
	if e != nil {
		t.Fatal(e)
	}
	if a.ID != 1 {
		t.Fatalf("first id = %d, want 1", a.ID)
	}
	c, e := s.Clone(a.ID, "B", 21002)
	if e != nil {
		t.Fatal(e)
	}
	if c.ID != 2 || c.Port != 21002 || c.Credential == a.Credential {
		t.Fatalf("bad clone: %+v", c)
	}
	a.Name = "A2"
	_, e = s.Update(a.ID, accountcfg.Input{Name: "A2", Enabled: true, Port: 21001, Credential: a.Credential, Flow: a.Flow, Network: a.Network, Security: a.Security, ServerName: a.ServerName, Dest: a.Dest, PrivateKey: a.PrivateKey, ShortID: a.ShortID})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Delete(c.ID); e != nil {
		t.Fatal(e)
	}
	reused, e := s.Create(accountcfg.Input{Name: "C", Enabled: true, Port: 21003, ServerName: "c.example"})
	if e != nil {
		t.Fatal(e)
	}
	if reused.ID != c.ID {
		t.Fatalf("reused id = %d, want deleted id %d", reused.ID, c.ID)
	}
	list, e := s.List()
	if e != nil || len(list) != 2 || list[0].Name != "A2" || list[1].Name != "C" {
		t.Fatalf("%+v %v", list, e)
	}
}

func TestCloneReusesLowestIDGap(t *testing.T) {
	st, e := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer st.Close()
	fa := &fakeApply{}
	s := NewAccounts(st, fa)
	a, _ := s.Create(accountcfg.Input{Name: "A", Enabled: true, Port: 21101, ServerName: "a.example"})
	b, _ := s.Create(accountcfg.Input{Name: "B", Enabled: true, Port: 21102, ServerName: "b.example"})
	_, _ = s.Create(accountcfg.Input{Name: "C", Enabled: true, Port: 21103, ServerName: "c.example"})
	if e := s.Delete(b.ID); e != nil {
		t.Fatal(e)
	}
	clone, e := s.Clone(a.ID, "A copy", 21104)
	if e != nil {
		t.Fatal(e)
	}
	if clone.ID != b.ID {
		t.Fatalf("clone id = %d, want lowest gap %d", clone.ID, b.ID)
	}
}

func TestApplyFailureDoesNotPersist(t *testing.T) {
	st, _ := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	defer st.Close()
	fa := &fakeApply{fail: true}
	s := NewAccounts(st, fa)
	_, e := s.Create(accountcfg.Input{Name: "A", Enabled: true, Port: 21001, ServerName: "a.example"})
	if e == nil {
		t.Fatal("expected failure")
	}
	list, _ := s.List()
	if len(list) != 0 {
		t.Fatal("persisted failed mutation")
	}
}

func TestRestoreSnapshotRollsBackDatabaseWhenXrayApplyFails(t *testing.T) {
 st,err:=store.Open(filepath.Join(t.TempDir(),"rollback.sqlite"))
 if err!=nil{t.Fatal(err)}
 defer st.Close()
 applier:=&fakeApply{}
 accounts:=NewAccounts(st,applier)
 if _,err=accounts.Create(accountcfg.Input{Name:"saved",Enabled:true,Port:24001,ServerName:"example.com"});err!=nil{t.Fatal(err)}
 previous,err:=st.Snapshot()
 if err!=nil{t.Fatal(err)}
 if _,err=accounts.Create(accountcfg.Input{Name:"newer",Enabled:true,Port:24002,ServerName:"example.com"});err!=nil{t.Fatal(err)}
 applier.fail=true
 if err=accounts.RestoreSnapshot(previous);err==nil{t.Fatal("restore should fail when Xray cannot apply")}
 actual,err:=accounts.List()
 if err!=nil{t.Fatal(err)}
 if len(actual)!=2||actual[0].Name!="saved"||actual[1].Name!="newer"{
  t.Fatalf("failed restore modified original database: %+v",actual)
 }
}

func TestUnchangedSaveAfterPanelUpgradeNeverAppliesXray(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "unchanged.sqlite"))
	if err != nil { t.Fatal(err) }
	defer st.Close()
	apply := &fakeApply{}
	s := NewAccounts(st, apply)
	created, err := s.Create(accountcfg.Input{
		Name:"original",Enabled:true,Port:26101,Protocol:"vless",ServerName:"www.example.com",
	})
	if err != nil { t.Fatal(err) }
	before, err := s.Raw(created.ID)
	if err != nil { t.Fatal(err) }
	calls := apply.calls
	_,err=s.Update(created.ID,accountcfg.Input{
		Name:created.Name, Enabled:created.Enabled, Port:created.Port,
		Protocol:created.Protocol, Credential:created.Credential, Flow:created.Flow,
		Network:created.Network, Security:created.Security,
		ServerName:created.ServerName,Dest:created.Dest,
		PrivateKey:created.PrivateKey,ShortID:created.ShortID,
		QuotaBytes:created.QuotaBytes,ExpiryTime:created.ExpiryTime,MonthlyReset:created.MonthlyReset,
	})
	if err != nil {t.Fatal(err)}
	if apply.calls != calls {t.Fatalf("unchanged account restarted Xray: %d -> %d",calls,apply.calls)}
	after,err:=s.Raw(created.ID)
	if err!=nil||after!=before {t.Fatalf("unchanged save touched persistent account: %+v",err)}
	adv,err:=s.Advanced(created.ID)
	if err!=nil{t.Fatal(err)}
	_,err=s.UpdateAdvanced(created.ID,adv)
	if err!=nil{t.Fatal(err)}
	if apply.calls != calls {t.Fatal("unchanged advanced editor restarted Xray")}
	expert,err:=s.Expert(created.ID)
	if err!=nil{t.Fatal(err)}
	_,err=s.UpdateExpert(created.ID,expert)
	if err!=nil{t.Fatal(err)}
	if apply.calls != calls {t.Fatal("unchanged expert editor restarted Xray")}
	modified:=accountcfg.Input{
		Name:"renamed",Enabled:true,Port:created.Port,Protocol:created.Protocol,
		Credential:created.Credential,Flow:created.Flow,Network:created.Network,
		Security:created.Security,ServerName:created.ServerName,Dest:created.Dest,
		PrivateKey:created.PrivateKey,ShortID:created.ShortID,
	}
	updated,err:=s.Update(created.ID,modified)
	if err!=nil{t.Fatal(err)}
	if apply.calls!=calls+1||updated.Name!="renamed"||updated.Credential!=created.Credential{
		t.Fatalf("real edit was ignored or credentials rotated: calls=%d %+v",apply.calls,updated)
	}
}

func TestAccountStateEqualityIgnoresJSONKeyOrderButNotCredentials(t *testing.T) {
	left:=model.Account{ID:1,Port:21001,SettingsJSON:`{"a":1,"b":{"x":true}}`,
		StreamSettingsJSON:`{"security":"reality"}`, SniffingJSON:`{}`, UpdatedAt:12}
	right:=left
	right.SettingsJSON=`{"b":{"x":true},"a":1}`
	right.UpdatedAt=13
	if !accountStateEqual(left,right){t.Fatal("equivalent serialized config should not trigger restart")}
	right.SettingsJSON=`{"b":{"x":false},"a":1}`
	if accountStateEqual(left,right){t.Fatal("material config changes must still apply")}
}
