package ops

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func Journal(service string, lines int) (string, error) {
	if service == "" { return "", errors.New("service is required") }
	if lines < 1 { lines = 200 }
	if lines > 2000 { lines = 2000 }
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "journalctl", "-u", service, "-n", strconv.Itoa(lines), "--no-pager", "-o", "short-iso").CombinedOutput()
	if ctx.Err() != nil { return "", ctx.Err() }
	if err != nil { return "", fmt.Errorf("journalctl %s: %w: %s", service, err, strings.TrimSpace(string(out))) }
	return string(out), nil
}

func RestartService(service string) error {
	if service == "" { return errors.New("service is required") }
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "systemctl", "restart", service).CombinedOutput()
	if ctx.Err() != nil { return ctx.Err() }
	if err != nil { return fmt.Errorf("restart %s: %w: %s", service, err, strings.TrimSpace(string(out))) }
	return nil
}

// ScheduleRestart asks PID 1 to create a transient unit which restarts the
// panel after the current HTTP response has had time to leave this process.
func ScheduleRestart(service string, delay time.Duration) error {
	if service == "" { return errors.New("service is required") }
	if delay < time.Second { delay = time.Second }
	unit := "xport-panel-restart-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "systemd-run", "--quiet", "--unit", unit, "--on-active="+delay.String(), "/bin/systemctl", "restart", service).CombinedOutput()
	if ctx.Err() != nil { return ctx.Err() }
	if err != nil { return fmt.Errorf("schedule restart %s: %w: %s", service, err, strings.TrimSpace(string(out))) }
	return nil
}

// ScheduleVerifiedBinaryRestart is used after staging a new X-port binary.
// The transient unit is outside xport.service's cgroup, so it survives the
// restart. If the new service does not stay active, it restores the previous
// binary and starts the old version again.
func ScheduleVerifiedBinaryRestart(service, binary, backup string, delay time.Duration) error {
	if service == "" || binary == "" || backup == "" { return errors.New("service, binary and backup are required") }
	if delay < time.Second { delay = time.Second }
	unit := "xport-self-update-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	script := `service="$1"; bin="$2"; bak="$3"; ` +
		`if systemctl restart "$service" && sleep 3 && systemctl is-active --quiet "$service"; then rm -f "$bak"; exit 0; fi; ` +
		`rm -f "$bin"; mv "$bak" "$bin"; systemctl restart "$service"`
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "systemd-run", "--quiet", "--unit", unit, "--on-active="+delay.String(), "/bin/sh", "-c", script, "xport-self-update", service, binary, backup).CombinedOutput()
	if ctx.Err() != nil { return ctx.Err() }
	if err != nil { return fmt.Errorf("schedule verified restart %s: %w: %s", service, err, strings.TrimSpace(string(out))) }
	return nil
}
