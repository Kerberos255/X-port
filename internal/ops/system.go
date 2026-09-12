package ops

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var systemdUnitRE = regexp.MustCompile(`^[A-Za-z0-9_.@:-]+$`)

func validSystemdUnit(service string) bool {
	return service != "" && len(service) <= 255 && systemdUnitRE.MatchString(service)
}
func validManagedPath(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path && !strings.ContainsRune(path, '\x00')
}

func Journal(service string, lines int) (string, error) {
	if !validSystemdUnit(service) {
		return "", errors.New("invalid service name")
	}
	if lines < 1 {
		lines = 200
	}
	if lines > 2000 {
		lines = 2000
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "journalctl", "-u", service, "-n", strconv.Itoa(lines), "--no-pager", "-o", "short-iso").CombinedOutput()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		return "", fmt.Errorf("journalctl %s: %w: %s", service, err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func RestartService(service string) error {
	if !validSystemdUnit(service) {
		return errors.New("invalid service name")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "systemctl", "restart", service).CombinedOutput()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return fmt.Errorf("restart %s: %w: %s", service, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func ScheduleRestart(service string, delay time.Duration) error {
	if !validSystemdUnit(service) {
		return errors.New("invalid service name")
	}
	if delay < time.Second {
		delay = time.Second
	}
	unit := "xport-panel-restart-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "systemd-run", "--quiet", "--unit", unit, "--on-active="+delay.String(), "/bin/systemctl", "restart", service).CombinedOutput()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return fmt.Errorf("schedule restart %s: %w: %s", service, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func verifiedBinaryRestartScript() string {
	return `service="$1"; bin="$2"; bak="$3"; ` +
		`if systemctl restart "$service"; then ` +
		`i=0; while [ "$i" -lt 10 ]; do sleep 1; systemctl is-active --quiet "$service" || break; i=$((i+1)); done; ` +
		`if [ "$i" -eq 10 ]; then exit 0; fi; fi; ` +
		`if [ ! -f "$bak" ]; then exit 1; fi; ` +
		`rm -f "$bin"; mv "$bak" "$bin"; systemctl restart "$service"`
}

func ScheduleVerifiedBinaryRestart(service, binary, backup string, delay time.Duration) error {
	if !validSystemdUnit(service) {
		return errors.New("invalid service name")
	}
	if !validManagedPath(binary) || !validManagedPath(backup) {
		return errors.New("invalid binary or backup path")
	}
	if backup != binary+".previous" {
		return errors.New("backup path must be the managed .previous generation")
	}
	if delay < time.Second {
		delay = time.Second
	}
	unit := "xport-self-update-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	// The shell program is constant; service/binary/backup are positional arguments,
	// never interpolated into shell source, and are validated above.
	out, err := exec.CommandContext(ctx, "systemd-run", "--quiet", "--unit", unit, "--on-active="+delay.String(), "/bin/sh", "-c", verifiedBinaryRestartScript(), "xport-self-update", service, binary, backup).CombinedOutput()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return fmt.Errorf("schedule verified restart %s: %w: %s", service, err, strings.TrimSpace(string(out)))
	}
	return nil
}
