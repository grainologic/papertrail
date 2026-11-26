package cli

import (
	"encoding/json"
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

// TestSyncFindingsExitZero guards the uniform contract: a pass with an error finding still reports the finding but exits 0, in both JSON and plain output.
func TestSyncFindingsExitZero(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr strings.Builder
	if code := RunIn(dir, []string{"init"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init: exit %d, stderr %q", code, stderr.String())
	}
	stdout.Reset()
	if code := RunIn(dir, []string{"create", "task", "Solo", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("create: exit %d, stderr %q", code, stderr.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &created); err != nil {
		t.Fatalf("create json: %v", err)
	}
	root := filepath.Join(dir, ".papertrail")
	if err := os.Rename(filepath.Join(root, created.ID), filepath.Join(root, "wrongname")); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	if code := RunIn(dir, []string{"sync", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("sync --json: exit %d, stderr %q", code, stderr.String())
	}
	var report struct {
		Errors []string `json:"errors"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &report); err != nil {
		t.Fatalf("sync json: %v", err)
	}
	if len(report.Errors) == 0 {
		t.Fatalf("sync --json: want error finding reported, got %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := RunIn(dir, []string{"sync"}, &stdout, &stderr); code != 0 {
		t.Fatalf("sync: exit %d, stderr %q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "pt sync: error:") {
		t.Errorf("sync plain: want error finding on stderr, got %q", stderr.String())
	}
}
