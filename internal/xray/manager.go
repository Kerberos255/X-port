package xray

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Kerberos255/X-port/internal/model"
)

type Applier interface{ Apply([]model.Account) error }

type Manager struct {
	mu             sync.Mutex
	BinaryPath     string
	ConfigPath     string
	Service        string
	APIPort        int
	BaseConfigJSON string
}

func (m *Manager) Apply(accounts []model.Account) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.applyLocked(accounts)
}

func (m *Manager) ApplyWithAPIPort(accounts []model.Account, port int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	previous := m.APIPort
	m.APIPort = port
	if err := m.applyLocked(accounts); err != nil {
		m.APIPort = previous
		return err
	}
	return nil
}

func (m *Manager) applyLocked(accounts []model.Account) error {
	cfg, err := BuildConfigWithBase(accounts, m.APIPort, m.BaseConfigJSON)
	if err != nil {
		return err
	}
	if m.BinaryPath == "" || m.ConfigPath == "" {
		return errors.New("Xray runtime paths are not configured")
	}
	if err := os.MkdirAll(filepath.Dir(m.ConfigPath), 0750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(m.ConfigPath), ".xport-config-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0640); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(cfg); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if out, err := run(8*time.Second, m.BinaryPath, "run", "-test", "-config", tmpPath); err != nil {
		return fmt.Errorf("Xray config validation failed: %v: %s", err, strings.TrimSpace(out))
	}

	previous, readErr := os.ReadFile(m.ConfigPath)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return fmt.Errorf("read existing Xray configuration: %w", readErr)
	}
	// Saving identical settings must not consume another systemd start-limit slot.
	// If the service is down, a no-op save can still recover the existing config.
	if readErr == nil && bytes.Equal(cfg, previous) {
		if m.Service == "" {
			return nil
		}
		if _, err := run(2*time.Second, "systemctl", "is-active", "--quiet", m.Service); err == nil {
			return nil
		}
		return restartAndVerify(m.Service)
	}

	if err := os.Rename(tmpPath, m.ConfigPath); err != nil {
		return err
	}
	if m.Service == "" {
		return nil
	}
	applyErr := restartAndVerify(m.Service)
	if applyErr == nil {
		return nil
	}
	if readErr != nil {
		return fmt.Errorf("Xray restart failed (%w); no previous configuration exists to restore", applyErr)
	}

	// Restore the previous bytes atomically. The old implementation claimed a
	// successful rollback even if either the file write or restart failed.
	if err := restoreConfig(m.ConfigPath, previous); err != nil {
		return fmt.Errorf("Xray restart failed (%v); restoring previous configuration also failed: %w", applyErr, err)
	}
	if err := clearStartLimitForRollback(m.Service, applyErr); err != nil {
		return fmt.Errorf("Xray restart failed (%v); previous configuration restored but systemd recovery failed: %w", applyErr, err)
	}
	if err := restartAndVerify(m.Service); err != nil {
		return fmt.Errorf("Xray restart failed (%v); previous configuration restored but Xray could not restart: %w", applyErr, err)
	}
	return fmt.Errorf("Xray restart failed (%v); previous configuration and service restored", applyErr)
}

func restoreConfig(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".xport-rollback-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0640); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func run(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	b, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if ctx.Err() != nil {
		return string(b), ctx.Err()
	}
	return string(b), err
}
