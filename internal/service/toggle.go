package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/Kerberos255/X-port/internal/accountcfg"
)

// SetEnabled toggles an account without rebuilding its protocol/settings JSON.
// This is safe for migrated accounts whose advanced Xray configuration is not
// representable by the built-in editor.
func (s *Accounts) SetEnabled(id int64, enabled bool) (accountcfg.View, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := s.store.Accounts()
	if err != nil {
		return accountcfg.View{}, err
	}
	idx := indexID(current, id)
	if idx < 0 {
		return accountcfg.View{}, errors.New("account not found")
	}
	if current[idx].Enabled == enabled {
		return accountcfg.PublicView(current[idx]), nil
	}

	next := copyAccounts(current)
	a := &next[idx]
	now := time.Now().UnixMilli()
	if enabled {
		if a.ExpiryTime > 0 && now >= a.ExpiryTime {
			return accountcfg.View{}, errors.New("account has expired")
		}
		if a.QuotaBytes > 0 && a.UpBytes+a.DownBytes >= a.QuotaBytes {
			return accountcfg.View{}, fmt.Errorf("traffic quota is exhausted; reset traffic or raise the quota first")
		}
		a.Enabled = true
		a.DisabledReason = ""
	} else {
		a.Enabled = false
		a.DisabledReason = "manual"
	}
	a.UpdatedAt = now

	if err := s.persistRuntimeState(current, next, true); err != nil {
		return accountcfg.View{}, err
	}
	return accountcfg.PublicView(*a), nil
}
