package xray

import (
	"errors"
	"fmt"
	"os"
	"time"
)

type RollbackInfo struct {
	Current   string `json:"current"`
	Previous  string `json:"previous"`
	Available bool   `json:"available"`
}

func (u *Updater) CheckRollback() RollbackInfo {
	current := binaryVersion(u.BinaryPath)
	previous := binaryVersion(u.BinaryPath + ".previous")
	return RollbackInfo{Current: current, Previous: previous, Available: previous != ""}
}

// Rollback swaps the installed core with the .previous binary left by the last
// successful update. The previous core is validated against the current config
// before any files are moved. If restart fails, the current core is restored.
// On success the former current core becomes .previous, so the operation can be
// reversed again if needed.
func (u *Updater) Rollback() (RollbackInfo, error) {
	info := u.CheckRollback()
	if !info.Available {
		return info, errors.New("no previous Xray version is available")
	}
	previousPath := u.BinaryPath + ".previous"
	if u.ConfigPath != "" {
		if _, err := os.Stat(u.ConfigPath); err == nil {
			if out, err := run(10*time.Second, previousPath, "run", "-test", "-config", u.ConfigPath); err != nil {
				return info, fmt.Errorf("previous Xray rejected current config: %v: %s", err, out)
			}
	}

	currentTmp := u.BinaryPath + ".rollback-current"
	_ = os.Remove(currentTmp)
	if err := os.Rename(u.BinaryPath, currentTmp); err != nil {
		return info, err
	}
	restoreCurrent := func() {
		_ = os.Remove(u.BinaryPath)
		_ = os.Rename(currentTmp, u.BinaryPath)
		if u.Service != "" {
			_ = restartAndVerify(u.Service)
		}
	}
	if err := os.Rename(previousPath, u.BinaryPath); err != nil {
		restoreCurrent()
		return info, err
	}
	if u.Service != "" {
		if err := restartAndVerify(u.Service); err != nil {
			_ = os.Rename(u.BinaryPath, previousPath)
			restoreCurrent()
			return info, fmt.Errorf("Xray rollback failed; current version was restored: %w", err)
		}
	}
	if err := os.Rename(currentTmp, previousPath); err != nil {
		return u.CheckRollback(), fmt.Errorf("rollback succeeded but could not preserve the replaced core as previous: %w", err)
	}
	return u.CheckRollback(), nil
}
