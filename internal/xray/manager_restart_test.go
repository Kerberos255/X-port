package xray

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func fakeManagerRuntime(t *testing.T, systemctl string) (bin, config, log string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("systemctl integration fixtures require POSIX shell")
	}
	dir := t.TempDir()
	bin = filepath.Join(dir, "xray")
	config = filepath.Join(dir, "config.json")
	log = filepath.Join(dir, "calls")
	xrayScript := "#!/bin/sh\nif [ \"$1\" = run ] && [ \"$2\" = -test ]; then exit 0; fi\nexit 1\n"
	if err := os.WriteFile(bin, []byte(xrayScript), 0755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "systemctl")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"+systemctl), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	xrayRestartGate.Lock()
	xrayRestartGate.last = time.Time{}
	xrayRestartGate.Unlock()
	return bin, config, log
}

func currentManagerConfig(t *testing.T) []byte {
	t.Helper()
	cfg, err := BuildConfigWithBase(nil, 10085, "")
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestManagerApplyUnchangedConfigSkipsRestart(t *testing.T) {
	bin, cfg, log := fakeManagerRuntime(t,
		"echo \"$1\" >> \"$XPORT_TEST_LOG\"\nexit 0\n")
	t.Setenv("XPORT_TEST_LOG", log)
	current := currentManagerConfig(t)
	if err := os.WriteFile(cfg, current, 0640); err != nil {
		t.Fatal(err)
	}
	m := &Manager{BinaryPath: bin, ConfigPath: cfg, Service: "xport-xray.service", APIPort: 10085}
	if err := m.Apply(nil); err != nil {
		t.Fatal(err)
	}
	commands, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(commands), "restart") {
		t.Fatalf("unchanged save restarted Xray: %s", commands)
	}
}

func TestManagerApplyRateLimitedRestartRestoresPreviousConfig(t *testing.T) {
	fake := `echo "$1" >> "$XPORT_TEST_LOG"
case "$1" in
 restart)
  count=$(grep -c '^restart$' "$XPORT_TEST_LOG")
  if [ "$count" -eq 1 ]; then
   echo "Start request repeated too quickly" >&2
   exit 1
  fi ;;
 show) echo "start-limit-hit" ;;
esac
exit 0
`
	bin, cfg, log := fakeManagerRuntime(t, fake)
	t.Setenv("XPORT_TEST_LOG", log)
	previous := append(currentManagerConfig(t), '\n')
	if err := os.WriteFile(cfg, previous, 0640); err != nil {
		t.Fatal(err)
	}
	m := &Manager{BinaryPath: bin, ConfigPath: cfg, Service: "xport-xray.service", APIPort: 10085}
	err := m.Apply(nil)
	if err == nil || !strings.Contains(err.Error(), "previous configuration and service restored") {
		t.Fatalf("expected explicit rollback result, got %v", err)
	}
	restored, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(restored, previous) {
		t.Fatal("previous configuration was not restored")
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(calls), "restart\n") != 2 || !strings.Contains(string(calls), "reset-failed\n") {
		t.Fatalf("unexpected recovery calls: %s", calls)
	}
}

func TestManagerRollbackDoesNotClaimServiceRecoveredOnFailure(t *testing.T) {
	bin, cfg, log := fakeManagerRuntime(t,
		"echo \"$1\" >> \"$XPORT_TEST_LOG\"\nif [ \"$1\" = restart ]; then exit 1; fi\nexit 0\n")
	t.Setenv("XPORT_TEST_LOG", log)
	previous := append(currentManagerConfig(t), '\n')
	if err := os.WriteFile(cfg, previous, 0640); err != nil {
		t.Fatal(err)
	}
	m := &Manager{BinaryPath: bin, ConfigPath: cfg, Service: "xport-xray.service", APIPort: 10085}
	err := m.Apply(nil)
	if err == nil || !strings.Contains(err.Error(), "could not restart") {
		t.Fatalf("failed rollback must not be reported as successful: %v", err)
	}
	restored, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(restored, previous) {
		t.Fatal("previous bytes should remain available for manual recovery")
	}
}
