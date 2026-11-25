package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"papertrail/internal/item"
)

// writeItem creates one item folder with one .md file.
func writeItem(t *testing.T, root, id, title string, extraMD []string) {
	t.Helper()
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nid: " + id + "\ntitle: " + title + "\ntype: task\nstatus: open\npriority: 2\ncreated: 2026-01-01T00:00:00Z\n---\n# " + title + "\n\nBody that should not be read.\n"
	for _, name := range append([]string{itemFileName(id)}, extraMD...) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func itemFileName(id string) string { return id + ".md" }

func TestScan(t *testing.T) {
	root := t.TempDir()
	writeItem(t, root, "aaaaaaaa", "One", nil)
	writeItem(t, root, "bbbbbbbb", "Two", nil)

	got, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 2 || len(got.Findings) != 0 {
		t.Fatalf("got %d items, %d findings: %+v", len(got.Items), len(got.Findings), got)
	}
	if got.Items[0].ID != "aaaaaaaa" || got.Items[1].ID != "bbbbbbbb" {
		t.Fatalf("unexpected order: %+v", got.Items)
	}
	if got.Items[0].Frontmatter.Title != "One" || got.Items[0].ID != "aaaaaaaa" {
		t.Fatalf("bad entry: %+v", got.Items[0])
	}
}

func TestScanSameWithAndWithoutCache(t *testing.T) {
	root := t.TempDir()
	writeItem(t, root, "aaaaaaaa", "One", nil)
	writeItem(t, root, "bbbbbbbb", "Two", nil)
	writeItem(t, root, "cccccccc", "Broken", nil)
	// Break the third item's frontmatter.
	path := filepath.Join(root, "cccccccc", itemFileName("cccccccc"))
	if err := os.WriteFile(path, []byte("---\nid: cccccccc\ntitle: Broken\ntype: bogus\nstatus: open\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	first, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 {
		t.Fatalf("expected 2 valid items, got %d", len(first.Items))
	}
	var invalid []Finding
	for _, f := range first.Findings {
		if strings.Contains(f.Reason, item.ErrInvalidType.Error()) {
			invalid = append(invalid, f)
		}
	}
	if len(invalid) != 1 {
		t.Fatalf("expected one invalid-type finding, got %+v", first.Findings)
	}

	// Second scan goes through the cache and must give the same result.
	second, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("cached scan differs:\nfirst  %+v\nsecond %+v", first, second)
	}
}

func TestScanCorruptCache(t *testing.T) {
	root := t.TempDir()
	writeItem(t, root, "aaaaaaaa", "One", nil)
	if err := os.WriteFile(cachePath(root), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 || got.Items[0].ID != "aaaaaaaa" {
		t.Fatalf("corrupt cache broke the scan: %+v", got)
	}
	// The cache is rebuilt.
	if _, err := os.Stat(cachePath(root)); err != nil {
		t.Fatalf("cache not rebuilt: %v", err)
	}
}

func TestScanChangedFileIsReread(t *testing.T) {
	root := t.TempDir()
	writeItem(t, root, "aaaaaaaa", "One", nil)
	if _, err := Scan(root); err != nil {
		t.Fatal(err)
	}
	newBody := "---\nid: aaaaaaaa\ntitle: Renamed\ntype: bug\nstatus: closed\npriority: 0\ncreated: 2026-01-02T00:00:00Z\n---\n"
	path := filepath.Join(root, "aaaaaaaa", itemFileName("aaaaaaaa"))
	// Change size and force a new mtime; filesystems stamp at second granularity, and equal size alone must not hide the change.
	if err := os.WriteFile(path, []byte(newBody), 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Hour)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	got, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 || got.Items[0].Frontmatter.Title != "Renamed" {
		t.Fatalf("changed file not reread: %+v", got.Items)
	}
}

func TestScanFolderFindings(t *testing.T) {
	root := t.TempDir()
	writeItem(t, root, "aaaaaaaa", "One", []string{"extra.md"})
	if err := os.MkdirAll(filepath.Join(root, "bbbbbbbb"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 0 || len(got.Findings) != 2 {
		t.Fatalf("expected 0 items and 2 findings, got %+v", got)
	}
	if !strings.Contains(got.Findings[0].Reason, ErrSeveralMDFile.Error()) ||
		!strings.Contains(got.Findings[1].Reason, ErrNoMDFile.Error()) {
		t.Fatalf("bad finding reasons: %+v", got.Findings)
	}
}

func TestReadItemDirectLookup(t *testing.T) {
	root := t.TempDir()
	writeItem(t, root, "aaaaaaaa", "One", nil)
	writeItem(t, root, "bbbbbbbb", "Two", nil)
	if _, err := Scan(root); err != nil {
		t.Fatal(err)
	}
	e, err := ReadItem(root, "bbbbbbbb")
	if err != nil {
		t.Fatal(err)
	}
	if e.ID != "bbbbbbbb" || e.Frontmatter.Title != "Two" {
		t.Fatalf("bad ReadItem result: %+v", e)
	}
	// Unknown ID and odd folders.
	if _, err := ReadItem(root, "zzzzzzzz"); err == nil {
		t.Fatal("expected error for unknown id")
	}
}

func TestReadItemNoCacheFile(t *testing.T) {
	root := t.TempDir()
	writeItem(t, root, "aaaaaaaa", "One", nil)
	e, err := ReadItem(root, "aaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	if e.ID != "aaaaaaaa" {
		t.Fatalf("bad entry: %+v", e)
	}
}

// TestScanRaceHammer runs Scan from many goroutines at once; under -race it catches data races in the worker pool and cache handling.
func TestScanRaceHammer(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 10; i++ {
		writeItem(t, root, fmt.Sprintf("aaaaaa%02d", i), fmt.Sprintf("Item %d", i), nil)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := Scan(root); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	// The cache file must still be valid JSON after concurrent writes.
	var c cache
	data, err := os.ReadFile(cachePath(root))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatalf("cache corrupted by concurrent scans: %v", err)
	}
}

func TestRunBounded(t *testing.T) {
	seen := make([]int, 5)
	runBounded(5, func(i int) { seen[i] = i + 1 })
	for i, v := range seen {
		if v != i+1 {
			t.Fatalf("runBounded skipped index %d", i)
		}
	}
	runBounded(0, func(i int) { t.Error("called with n=0") })
}
