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
