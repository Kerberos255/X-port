package service

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Kerberos255/X-port/internal/accountcfg"
	"github.com/Kerberos255/X-port/internal/model"
	"github.com/Kerberos255/X-port/internal/store"
	"github.com/Kerberos255/X-port/internal/xray"
)

type Accounts struct {
	store   *store.Store
	applier xray.Applier
	mu      sync.Mutex
}

func NewAccounts(st *store.Store, applier xray.Applier) *Accounts {
	return &Accounts{store: st, applier: applier}
}

func (s *Accounts) List() ([]accountcfg.View, error) {
	accounts, err := s.store.Accounts()
	if err != nil {
		return nil, err
	}
	out := make([]accountcfg.View, 0, len(accounts))
	for _, a := range accounts {
		out = append(out, editorPublicView(a))
	}
	return out, nil
}

func (s *Accounts) Get(id int64) (accountcfg.View, error) {
	a, err := s.raw(id)
	if err != nil {
		return accountcfg.View{}, err
	}
	return editorView(a)
}

func (s *Accounts) Raw(id int64) (model.Account, error) { return s.raw(id) }

func (s *Accounts) Create(in accountcfg.Input) (accountcfg.View, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	old, err := s.store.Accounts()
	if err != nil {
		return accountcfg.View{}, err
	}
	normalizeEditorInput(&in)
	if err := s.prepareInput(&in, old); err != nil {
		return accountcfg.View{}, err
	}
	if err := uniquePort(old, in.Port, 0); err != nil {
		return accountcfg.View{}, err
	}
	if err := ensurePortFree(in.Port); err != nil {
		return accountcfg.View{}, err
	}
	a, err := accountcfg.New(in, in.Port)
	if err != nil {
		return accountcfg.View{}, err
	}
	initializeMonthlyCycle(&a, false)
	next := append(copyAccounts(old), a)
	if err := s.applyAndPersist(old, next); err != nil {
		return accountcfg.View{}, err
	}
	saved, err := s.findByPort(in.Port)
	if err != nil {
		return accountcfg.View{}, err
	}
	return editorView(saved)
}

func (s *Accounts) Update(id int64, in accountcfg.Input) (accountcfg.View, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	old, err := s.store.Accounts()
	if err != nil {
		return accountcfg.View{}, err
	}
	idx := indexID(old, id)
	if idx < 0 {
		return accountcfg.View{}, errors.New("account not found")
	}
	normalizeEditorInput(&in)
	if in.Port == 0 {
		in.Port = old[idx].Port
	}
	if err := uniquePort(old, in.Port, id); err != nil {
		return accountcfg.View{}, err
	}
	if in.Port != old[idx].Port {
		if err := ensurePortFree(in.Port); err != nil {
			return accountcfg.View{}, err
		}
	}

	base := sanitizeEditableBaseForExplicitClears(old[idx], in)
	a, err := accountcfg.Update(base, in)
	if err != nil {
		return accountcfg.View{}, err
	}
	if !old[idx].Enabled && !a.Enabled {
		a.DisabledReason = old[idx].DisabledReason
	}
	initializeMonthlyCycle(&a, !old[idx].MonthlyReset)
	next := copyAccounts(old)
	next[idx] = a
	if err := s.applyAndPersist(old, next); err != nil {
		return accountcfg.View{}, err
	}
	saved, err := s.findByPort(a.Port)
	if err != nil {
		return accountcfg.View{}, err
	}
	return editorView(saved)
}

func (s *Accounts) Delete(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, err := s.store.Accounts()
	if err != nil {
		return err
	}
	idx := indexID(old, id)
	if idx < 0 {
		return errors.New("account not found")
	}
	next := append(copyAccounts(old[:idx]), old[idx+1:]...)
	return s.applyAndPersist(old, next)
}

func (s *Accounts) Clone(id int64, name string, port int) (accountcfg.View, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, err := s.store.Accounts()
	if err != nil {
		return accountcfg.View{}, err
	}
	idx := indexID(old, id)
	if idx < 0 {
		return accountcfg.View{}, errors.New("account not found")
	}
	if port == 0 {
		minPort, maxPort, apiPort := s.portDefaults()
		port = nextFreePort(old, minPort, maxPort, apiPort)
		if port == 0 {
			return accountcfg.View{}, fmt.Errorf("no automatic port available in %d-%d", minPort, maxPort)
		}
	} else if err := ensurePortFree(port); err != nil {
		return accountcfg.View{}, err
	}
	if err := uniquePort(old, port, 0); err != nil {
		return accountcfg.View{}, err
	}
	a, err := accountcfg.Clone(old[idx], name, port)
	if err != nil {
		return accountcfg.View{}, err
	}
	initializeMonthlyCycle(&a, true)
	next := append(copyAccounts(old), a)
	if err := s.applyAndPersist(old, next); err != nil {
		return accountcfg.View{}, err
	}
	saved, err := s.findByPort(port)
	if err != nil {
		return accountcfg.View{}, err
	}
	return editorView(saved)
}

func initializeMonthlyCycle(a *model.Account, newlyEnabled bool) {
	if a.MonthlyReset && (newlyEnabled || a.LastMonthlyReset == "") {
		a.LastMonthlyReset = time.Now().Format("2006-01")
	}
}

func (s *Accounts) applyAndPersist(old, next []model.Account) error {
	if s.applier == nil {
		return errors.New("Xray applier is not configured")
	}
	if err := s.applier.Apply(next); err != nil {
		return err
	}
	if err := s.store.ReplaceAccounts(next); err != nil {
		rollback := s.applier.Apply(old)
		if rollback != nil {
			return fmt.Errorf("database update failed: %v; Xray rollback also failed: %v", err, rollback)
		}
		return fmt.Errorf("database update failed; Xray was rolled back: %w", err)
	}
	return nil
}

func (s *Accounts) raw(id int64) (model.Account, error) {
	a, err := s.store.Accounts()
	if err != nil {
		return model.Account{}, err
	}
	idx := indexID(a, id)
	if idx < 0 {
		return model.Account{}, errors.New("account not found")
	}
	return a[idx], nil
}

func (s *Accounts) findByPort(port int) (model.Account, error) {
	a, err := s.store.Accounts()
	if err != nil {
		return model.Account{}, err
	}
	for _, v := range a {
		if v.Port == port {
			return v, nil
		}
	}
	return model.Account{}, errors.New("account not found after save")
}

func copyAccounts(in []model.Account) []model.Account {
	out := make([]model.Account, len(in))
	copy(out, in)
	return out
}

func indexID(a []model.Account, id int64) int {
	for i, v := range a {
		if v.ID == id {
			return i
		}
	}
	return -1
}

func uniquePort(a []model.Account, port int, exceptID int64) error {
	for _, v := range a {
		if v.Port == port && v.ID != exceptID {
			return fmt.Errorf("port %d is already used by %q", port, v.Name)
		}
	}
	return nil
}
