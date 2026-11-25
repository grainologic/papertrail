package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSyncReportsScanFailure verifies sync does not swallow the scan error: a root that is a regular file fails the scan and the command exits nonzero instead of printing sync ok.
func TestSyncReportsScanFailure(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "afile.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	configDir := filepath.Join(dir, ".papertrail")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "root = \"afile.txt\"\n"
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr strings.Builder
	code := RunIn(dir, []string{"sync", "--json"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("sync: want nonzero exit, stdout %q", stdout.String())
	}
	if strings.Contains(stdout.String(), "sync ok") {
		t.Errorf("sync printed ok on scan failure")
	}
	if code != 1 {
		t.Errorf("sync: exit %d, want 1", code)
	}
}
