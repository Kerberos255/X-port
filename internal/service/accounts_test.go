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
