package store

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestWriteItemLeavesExactlyOneMDAnd0644 verifies the atomic item write never leaves a second .md behind (the temp file must not carry the .md suffix) and that the written file is world-readable (0644), not the 0600 CreateTemp default.
func TestWriteItemLeavesExactlyOneMDAnd0644(t *testing.T) {
	root := t.TempDir()
	writeItem(t, root, "aaaaaaaa", "One", nil)

	res, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteItem(res.Items[0], nil); err != nil {
		t.Fatal(err)
	}

	var mds []string
	for _, name := range listDir(t, filepath.Join(root, "aaaaaaaa")) {
		if filepath.Ext(name) == ".md" {
			mds = append(mds, name)
		}
	}
	if len(mds) != 1 {
		t.Fatalf("folder has %d md files: %v, want exactly one", len(mds), mds)
	}

	if runtime.GOOS == "windows" {
		// Windows reports no unix permission bits; the 0644 check is unix-only.
		return
	}
	info, err := os.Stat(filepath.Join(root, "aaaaaaaa", "aaaaaaaa.md"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("mode = %v, want 0644", info.Mode().Perm())
	}
}

func listDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}
