package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"papertrail/internal/config"
)

func TestRunInitCreatesLayout(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	var stdout, stderr strings.Builder
	code := Run([]string{"init"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	cfg, err := config.Find(dir)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	for _, name := range []string{config.ConfigFile, "INDEX.md", "FRONTIER.md", "HEALTH.md"} {
		path := filepath.Join(dir, config.ConfigDir, name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s missing: %v", name, err)
		}
	}
	_ = cfg
}

func TestRunErrors(t *testing.T) {
	t.Chdir(t.TempDir())
	var stdout, stderr strings.Builder
	if code := Run(nil, &stdout, &stderr); code == 0 {
		t.Error("no args: want nonzero exit")
	}
	if code := Run([]string{"bogus"}, &stdout, &stderr); code == 0 {
		t.Error("unknown command: want nonzero exit")
	}
	stderr.Reset()
	if code := Run([]string{"init", "extra"}, &stdout, &stderr); code == 0 {
		t.Error("init with args: want nonzero exit")
	}
	stderr.Reset()
	if code := Run([]string{"init"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init: exit = %d, stderr = %q", code, stderr.String())
	}
	if code := Run([]string{"init"}, &stdout, &stderr); code == 0 {
		t.Error("second init: want nonzero exit")
	}
	if !strings.Contains(stderr.String(), "already initialized") {
		t.Errorf("stderr = %q, want already-initialized error", stderr.String())
	}
}
