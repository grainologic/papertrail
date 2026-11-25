package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFindFindsConfigFromSubdirectory(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, ".papertrail")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, ConfigFile), []byte("root = \".papertrail/\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "deep", "deeper")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg, err := Find(sub)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if cfg.Root != filepath.Join(root, ".papertrail") {
		t.Errorf("Root = %q, want %q", cfg.Root, filepath.Join(root, ".papertrail"))
	}
}

func TestFindDefaultsRootWhenKeyMissing(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, ".papertrail")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, ConfigFile), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Find(root)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if cfg.Root != configDir {
		t.Errorf("Root = %q, want %q", cfg.Root, configDir)
	}
}

func TestFindErrorsWithoutConfig(t *testing.T) {
	if _, err := Find(t.TempDir()); !errors.Is(err, ErrNoConfig) {
		t.Errorf("err = %v, want ErrNoConfig", err)
	}
}

func TestFindErrorsOnMalformedConfig(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, ".papertrail")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, ConfigFile), []byte("root = 12@3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Find(root); err == nil {
		t.Error("want error for malformed config")
	}
}

func TestInitCreatesConfigRootAndGeneratedFiles(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	cfg, err := Find(dir)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	for _, name := range []string{ConfigFile, "INDEX.md", "FRONTIER.md", "HEALTH.md"} {
		path := filepath.Join(cfg.Root, name)
		if name == ConfigFile {
			path = filepath.Join(dir, ConfigDir, ConfigFile)
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s missing: %v", name, err)
		}
	}
	if err := Init(dir); !errors.Is(err, ErrAlreadyInitialized) {
		t.Errorf("second Init err = %v, want ErrAlreadyInitialized", err)
	}
}
