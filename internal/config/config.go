// Package config locates and creates the papertrail configuration at .papertrail/config.toml. The file holds a single key, root, the directory where items live, resolved relative to the workspace root that contains the .papertrail directory.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const (
	// ConfigDir is the workspace configuration directory name.
	ConfigDir = ".papertrail"
	// ConfigFile is the configuration file name inside ConfigDir.
	ConfigFile = "config.toml"
	// DefaultRoot is the item root used when the root key is absent.
	DefaultRoot = ".papertrail"

	indexFile    = "INDEX.md"
	frontierFile = "FRONTIER.md"
	healthFile   = "HEALTH.md"
)

// ErrNoConfig is returned when no .papertrail/config.toml is found walking up from the starting directory.
var ErrNoConfig = errors.New("config: no .papertrail/config.toml found")

// ErrAlreadyInitialized is returned when the configuration already exists.
var ErrAlreadyInitialized = errors.New("config: already initialized")

// Config is the loaded configuration with the item root resolved to an absolute path.
type Config struct {
	Root string
}

// Find walks up from fromDir (typically the current directory) to the filesystem root looking for .papertrail/config.toml.
func Find(fromDir string) (Config, error) {
	dir, err := filepath.Abs(fromDir)
	if err != nil {
		return Config{}, fmt.Errorf("config: resolve %s: %w", fromDir, err)
	}
	for {
		path := filepath.Join(dir, ConfigDir, ConfigFile)
		if _, err := os.Stat(path); err == nil {
			return load(path, dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return Config{}, ErrNoConfig
		}
		dir = parent
	}
}

func load(path, workspaceRoot string) (Config, error) {
	var file struct {
		Root string `toml:"root"`
	}
	if _, err := toml.DecodeFile(path, &file); err != nil {
		return Config{}, fmt.Errorf("config: %s: %w", path, err)
	}
	root := file.Root
	if root == "" {
		root = DefaultRoot
	}
	absRoot, err := filepath.Abs(filepath.Join(workspaceRoot, root))
	if err != nil {
		return Config{}, fmt.Errorf("config: resolve root %q: %w", root, err)
	}
	return Config{Root: absRoot}, nil
}

// Init creates .papertrail/config.toml, the item root directory, and the three generated index files. It fails when the configuration already exists.
func Init(dir string) error {
	workspaceRoot, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("config: resolve %s: %w", dir, err)
	}
	configDir := filepath.Join(workspaceRoot, ConfigDir)
	configPath := filepath.Join(configDir, ConfigFile)
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("%w: %s", ErrAlreadyInitialized, configPath)
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("config: create %s: %w", configDir, err)
	}
	if err := os.WriteFile(configPath, []byte("root = \""+DefaultRoot+"/\"\n"), 0o644); err != nil {
		return fmt.Errorf("config: write %s: %w", configPath, err)
	}
	rootDir := filepath.Join(workspaceRoot, DefaultRoot)
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return fmt.Errorf("config: create %s: %w", rootDir, err)
	}
	for _, name := range []string{indexFile, frontierFile, healthFile} {
		path := filepath.Join(rootDir, name)
		if _, err := os.Stat(path); err == nil {
			continue
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			return fmt.Errorf("config: write %s: %w", path, err)
		}
	}
	return nil
}
