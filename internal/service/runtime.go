package service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Kerberos255/X-port/internal/accountcfg"
	"github.com/Kerberos255/X-port/internal/model"
	"github.com/Kerberos255/X-port/internal/store"
	"github.com/Kerberos255/X-port/internal/xray"
)

// ApplyTraffic persists the Xray deltas, applies monthly per-account reset policy,
// and enforces expiry/quota state. Monthly reset clears only the current-period
// counters: AllTimeBytes intentionally remains lifetime traffic.
func (s *Accounts) ApplyTraffic(stats map[string]xray.Traffic, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for tag, t := range stats {
		if err := s.store.AddTraffic(tag, t.Up, t.Down); err != nil {
			return err
		}
	}
	current, err := s.store.Accounts()
	if err != nil {
		return err
	}
	next := copyAccounts(current)
	dbChanged, configChanged := false, false
	nowMS := now.UnixMilli()
	monthKey := now.Format("2006-01")

	for i := range next {
		a := &next[i]

		if now.Day() == 1 && a.MonthlyReset && a.LastMonthlyReset != monthKey {
			a.UpBytes, a.DownBytes = 0, 0
			a.LastMonthlyReset = monthKey
			a.UpdatedAt = nowMS
			dbChanged = true
			// Only restore accounts that X-port itself suspended for quota. Manual
			// disables stay disabled, and expired accounts are handled below.
			if !a.Enabled && a.DisabledReason == "quota" && (a.ExpiryTime == 0 || nowMS < a.ExpiryTime) {
				a.Enabled = true
				a.DisabledReason = ""
				configChanged = true
			}
		}

		expired := a.ExpiryTime > 0 && nowMS >= a.ExpiryTime
		overQuota := a.QuotaBytes > 0 && a.UpBytes+a.DownBytes >= a.QuotaBytes
		switch {
		case expired:
			if a.Enabled {
				a.Enabled = false
				configChanged = true
			}
			if a.DisabledReason != "expiry" {
				a.DisabledReason = "expiry"
				dbChanged = true
			}
		case overQuota:
			if a.Enabled {
				a.Enabled = false
				configChanged = true
			}
			if a.DisabledReason != "quota" {
				a.DisabledReason = "quota"
				dbChanged = true
			}
		}
		if dbChanged {
			a.UpdatedAt = nowMS
		}
	}

	if !dbChanged && !configChanged {
		return nil
	}
	if configChanged {
		if s.applier == nil {
			return errors.New("Xray applier is not configured")
		}
		if err := s.applier.Apply(next); err != nil {
			return fmt.Errorf("apply automatic account state: %w", err)
		}
	}
	if err := s.store.ReplaceAccounts(next); err != nil {
		if configChanged {
			_ = s.applier.Apply(current)
		}
		return fmt.Errorf("persist automatic account state: %w", err)
	}
	return nil
}

func (s *Accounts) ResetTraffic(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.store.ResetTraffic(id)
}

func (s *Accounts) RestoreSnapshot(snapshot store.Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.applier == nil {
		return errors.New("Xray applier is not configured")
	}
	old, err := s.store.Snapshot()
	if err != nil {
		return err
	}
	if err := s.store.ReplaceSnapshot(snapshot); err != nil {
		return fmt.Errorf("restore database failed: %w", err)
	}
	if err := s.applier.Apply(snapshot.Accounts); err != nil {
		_ = s.store.ReplaceSnapshot(old)
		_ = s.applier.Apply(old.Accounts)
		return fmt.Errorf("backup Xray validation failed; database and Xray were rolled back: %w", err)
	}
	return nil
}

func (s *Accounts) prepareInput(in *accountcfg.Input, accounts []model.Account) error {
	if in.Protocol == "" {
		in.Protocol = "vless"
	}
	if in.Port == 0 {
		minPort, maxPort, apiPort := s.portDefaults()
		in.Port = nextFreePort(accounts, minPort, maxPort, apiPort)
		if in.Port == 0 {
			return fmt.Errorf("no automatic port available in %d-%d", minPort, maxPort)
		}
	}
	protocol := strings.ToLower(strings.TrimSpace(in.Protocol))
	if (protocol == "vless" || protocol == "trojan") && (in.Security == "" || in.Security == "reality") {
		if strings.TrimSpace(in.ServerName) == "" {
			if v, ok, _ := s.store.Setting("default_reality_sni"); ok {
				in.ServerName = strings.TrimSpace(v)
			}
		}
		if strings.TrimSpace(in.Dest) == "" {
			if v, ok, _ := s.store.Setting("default_reality_dest"); ok {
				in.Dest = strings.TrimSpace(v)
			}
		}
	}
	return nil
}

func (s *Accounts) portDefaults() (int, int, int) {
	minPort := settingInt(s.store, "port_min", 20000)
	maxPort := settingInt(s.store, "port_max", 60000)
	apiPort := settingInt(s.store, "xray_api_port", 10085)
	if minPort < 1 || minPort > 65535 {
		minPort = 20000
	}
	if maxPort < minPort || maxPort > 65535 {
		maxPort = 60000
	}
	return minPort, maxPort, apiPort
}

func settingInt(st *store.Store, key string, fallback int) int {
	v, ok, err := st.Setting(key)
	if err != nil || !ok {
		return fallback
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return fallback
	}
	return n
}
