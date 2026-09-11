package ops

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func writeExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0755); err != nil { t.Fatal(err) }
}

func TestVerifiedBinaryRestartScriptKeepsPreviousOnSuccess(t *testing.T) {
	dir := t.TempDir()
	fakeBin := filepath.Join(dir, "bin")
	if err := os.Mkdir(fakeBin, 0755); err != nil { t.Fatal(err) }
	writeExecutable(t, filepath.Join(fakeBin, "systemctl"), "#!/bin/sh\nexit 0\n")
	writeExecutable(t, filepath.Join(fakeBin, "sleep"), "#!/bin/sh\nexit 0\n")

	binary := filepath.Join(dir, "xport")
	backup := binary + ".previous"
	if err := os.WriteFile(binary, []byte("new"), 0755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(backup, []byte("old"), 0755); err != nil { t.Fatal(err) }

	cmd := exec.Command("/bin/sh", "-c", verifiedBinaryRestartScript(), "xport-self-update", "xport.service", binary, backup)
	cmd.Env = append(os.Environ(), "PATH="+fakeBin+":"+os.Getenv("PATH"))
	if out, err := cmd.CombinedOutput(); err != nil { t.Fatalf("script failed: %v: %s", err, out) }
	if got, _ := os.ReadFile(binary); string(got) != "new" { t.Fatalf("binary = %q", got) }
	if got, _ := os.ReadFile(backup); string(got) != "old" { t.Fatalf("backup = %q", got) }
}

func TestVerifiedBinaryRestartScriptRestoresPreviousOnFailure(t *testing.T) {
	dir := t.TempDir()
	fakeBin := filepath.Join(dir, "bin")
	if err := os.Mkdir(fakeBin, 0755); err != nil { t.Fatal(err) }
	writeExecutable(t, filepath.Join(fakeBin, "systemctl"), `#!/bin/sh
if [ "$1" = "is-active" ]; then exit 1; fi
exit 0
`)
	writeExecutable(t, filepath.Join(fakeBin, "sleep"), "#!/bin/sh\nexit 0\n")

	binary := filepath.Join(dir, "xport")
	backup := binary + ".previous"
	if err := os.WriteFile(binary, []byte("new"), 0755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(backup, []byte("old"), 0755); err != nil { t.Fatal(err) }

	cmd := exec.Command("/bin/sh", "-c", verifiedBinaryRestartScript(), "xport-self-update", "xport.service", binary, backup)
	cmd.Env = append(os.Environ(), "PATH="+fakeBin+":"+os.Getenv("PATH"))
	if out, err := cmd.CombinedOutput(); err != nil { t.Fatalf("script failed: %v: %s", err, out) }
	if got, _ := os.ReadFile(binary); string(got) != "old" { t.Fatalf("binary = %q", got) }
	if _, err := os.Stat(backup); !os.IsNotExist(err) { t.Fatalf("backup should be consumed during rollback, err=%v", err) }
}
