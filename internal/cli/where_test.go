package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"papertrail/internal/config"
)

func TestWherePrintsRoot(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	var stdout, stderr strings.Builder
	if code := Run([]string{"init"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init: exit %d, stderr %q", code, stderr.String())
	}
	want := filepath.Join(dir, config.DefaultRoot)

	stdout.Reset()
	if code := Run([]string{"where"}, &stdout, &stderr); code != 0 {
		t.Fatalf("where: exit %d, stderr %q", code, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != want {
		t.Errorf("where = %q, want %q", got, want)
	}

	stdout.Reset()
	if code := Run([]string{"where", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("where --json: exit %d, stderr %q", code, stderr.String())
	}
	var got struct {
		Root string `json:"root"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &got); err != nil {
		t.Fatalf("where --json: %v", err)
	}
	if got.Root != want {
		t.Errorf("where --json root = %q, want %q", got.Root, want)
	}
}

func TestWhereNoConfig(t *testing.T) {
	t.Chdir(t.TempDir())
	var stdout, stderr strings.Builder
	if code := Run([]string{"where"}, &stdout, &stderr); code != 1 {
		t.Fatalf("where: exit = %d, want 1; stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "no .papertrail/config.toml") {
		t.Errorf("stderr = %q, want missing-config error", stderr.String())
	}
}
