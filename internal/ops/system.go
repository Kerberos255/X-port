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
	if service == "" {
		return "", errors.New("service is required")
	}
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
// Using systemd-run keeps the restart command outside xport.service's cgroup,
// so it is not killed as part of restarting the panel itself.
func ScheduleRestart(service string, delay time.Duration) error {
	if service == "" { return errors.New("service is required") }
	if delay < time.Second { delay = time.Second }
	unit := "xport-panel-restart-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "systemd-run", "--quiet", "--unit", unit, "--on-active", delay.String(), "/bin/systemctl", "restart", service).CombinedOutput()
	if ctx.Err() != nil { return ctx.Err() }
	if err != nil { return fmt.Errorf("schedule restart %s: %w: %s", service, err, strings.TrimSpace(string(out))) }
	return nil
}
