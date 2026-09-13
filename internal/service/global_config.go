package service

import (
	"errors"
	"fmt"

	"github.com/Kerberos255/X-port/internal/model"
	"github.com/Kerberos255/X-port/internal/xray"
)

type globalConfigApplier interface {
	xray.Applier
	ApplyWithBase([]model.Account, string) error
}

func (s *Accounts) GlobalXrayConfig() ([]byte, error) {
	base, ok, err := s.store.Setting("xray_global_config")
	if err != nil {
		return nil, err
	}
	if !ok {
		base = ""
	}
	return xray.EditableGlobalConfig(base)
}

func (s *Accounts) UpdateGlobalXrayConfig(raw string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	applier, ok := s.applier.(globalConfigApplier)
	if !ok {
		return nil, errors.New("Xray global configuration applier is not configured")
	}
	oldBase, exists, err := s.store.Setting("xray_global_config")
	if err != nil {
		return nil, err
	}
	if !exists {
		oldBase = ""
	}
	merged, normalized, err := xray.MergeEditableGlobalConfig(oldBase, raw)
	if err != nil {
		return nil, err
	}
	accounts, err := s.store.Accounts()
	if err != nil {
		return nil, err
	}
	if err := applier.ApplyWithBase(accounts, merged); err != nil {
		return nil, err
	}
	if err := s.store.SetSetting("xray_global_config", merged); err != nil {
		rollbackErr := applier.ApplyWithBase(accounts, oldBase)
		if rollbackErr != nil {
			return nil, fmt.Errorf("save global Xray config failed: %v; runtime rollback also failed: %v", err, rollbackErr)
		}
		return nil, fmt.Errorf("save global Xray config failed; runtime configuration was rolled back: %w", err)
	}
	return normalized, nil
}
