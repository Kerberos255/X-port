package xray

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// All Xray apply, core-update and geodata paths share this gate. Avoid more
// than four voluntary starts in the default systemd 10-second window; leave
// headroom for a genuine unexpected restart without increasing StartLimitBurst.
var xrayRestartGate struct {
	sync.Mutex
	last time.Time
}

const minXrayRestartSpacing = 3 * time.Second

func restartAndVerify(service string) error {
	xrayRestartGate.Lock()
	defer xrayRestartGate.Unlock()

	if wait := minXrayRestartSpacing - time.Since(xrayRestartGate.last); wait > 0 {
		time.Sleep(wait)
	}
	xrayRestartGate.last = time.Now()
	out, err := run(12*time.Second, "systemctl", "restart", service)
	if err != nil {
		return fmt.Errorf("restart %s: %w: %s", service, err, strings.TrimSpace(out))
	}

	// The first "active" response is not enough: a process that immediately
	// crashes can briefly be active before systemd begins its failure loop.
	deadline := time.Now().Add(6 * time.Second)
	stableSince := time.Time{}
	for time.Now().Before(deadline) {
		if _, err := run(2*time.Second, "systemctl", "is-active", "--quiet", service); err == nil {
			if stableSince.IsZero() {
				stableSince = time.Now()
			}
			if time.Since(stableSince) >= 1200*time.Millisecond {
				return nil
			}
		} else {
			stableSince = time.Time{}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return errors.New("Xray service did not stay active after restart")
}

// Reset a consumed start-limit only while restoring a known previous config.
// Never clear systemd's failure counter during a normal apply or crash loop.
func clearStartLimitForRollback(service string, cause error) error {
	limited := strings.Contains(strings.ToLower(cause.Error()), "start request repeated too quickly") ||
		strings.Contains(strings.ToLower(cause.Error()), "start-limit-hit")
	if !limited {
		out, err := run(2*time.Second, "systemctl", "show", service, "--property=Result", "--value")
		limited = err == nil && strings.Contains(strings.TrimSpace(out), "start-limit-hit")
	}
	if !limited {
		return nil
	}
	out, err := run(5*time.Second, "systemctl", "reset-failed", service)
	if err != nil {
		return fmt.Errorf("reset start-limit for %s: %w: %s", service, err, strings.TrimSpace(out))
	}
	return nil
}
