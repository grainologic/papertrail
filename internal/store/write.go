package store

import (
	"fmt"
	"os"
	"path/filepath"

	"papertrail/internal/item"
)

// WriteItem rewrites an item file from its frontmatter. A nil body reads the existing body from the file so a frontmatter rewrite preserves it. The write goes through a temp sibling that never ends in .md (a crash then leaves no stray second md file) and an atomic rename. The file is chmod 0644 after the write because CreateTemp creates it 0600.
func WriteItem(e Entry, body []byte) error {
	if err := e.Frontmatter.Validate(); err != nil {
		return fmt.Errorf("store: %s: %w", e.Path, err)
	}
	if body == nil {
		raw, err := os.ReadFile(e.Path)
		if err != nil {
			return fmt.Errorf("store: read %s: %w", e.Path, err)
		}
		parsed, err := item.Parse(raw)
		if err != nil {
			return fmt.Errorf("store: %s: %w", e.Path, err)
		}
		body = parsed.Body
	}
	data, err := e.Frontmatter.Encode(body)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(e.Path), ".item-*")
	if err != nil {
		return fmt.Errorf("store: temp: %w", err)
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if err := firstErr(werr, cerr); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("store: write %s: %w", e.Path, err)
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("store: chmod %s: %w", e.Path, err)
	}
	if err := os.Rename(tmp.Name(), e.Path); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("store: rename %s: %w", e.Path, err)
	}
	return nil
}

// WriteRaw atomically replaces a file's bytes without parsing or re-encoding them. Section set uses it so only the section's lines change.
func WriteRaw(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".item-*")
	if err != nil {
		return fmt.Errorf("store: temp: %w", err)
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if err := firstErr(werr, cerr); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("store: write %s: %w", path, err)
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("store: chmod %s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("store: rename %s: %w", path, err)
	}
	return nil
}

func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
