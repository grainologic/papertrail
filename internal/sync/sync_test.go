package sync_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"papertrail/internal/sync"
)

// fmStr renders a valid item file: frontmatter plus a body whose heading matches the title.
func fmStr(id, title, status string, priority int, mode, created string, deps []string) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "id: %s\n", id)
	fmt.Fprintf(&b, "title: %s\n", title)
	b.WriteString("type: task\n")
	fmt.Fprintf(&b, "status: %s\n", status)
	fmt.Fprintf(&b, "priority: %d\n", priority)
	if mode != "" {
		fmt.Fprintf(&b, "mode: %s\n", mode)
	}
	fmt.Fprintf(&b, "created: %s\n", created)
	if len(deps) > 0 {
		b.WriteString("dependsOn:\n")
		for _, d := range deps {
			fmt.Fprintf(&b, "  - %s\n", d)
		}
	}
	b.WriteString("---\n")
	fmt.Fprintf(&b, "# %s\n\n## Notes\n", title)
	return b.String()
}

func writeItem(t *testing.T, root, id, name, content string) string {
	t.Helper()
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// captureStderr runs fn with stderr redirected to a pipe and returns what the function wrote there.
func captureStderr(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() { os.Stderr = old }()
	runErr := fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	return string(out), runErr
}

// tableIDs extracts the item ids from a generated markdown table, in row order.
func tableIDs(t *testing.T, content string) []string {
	t.Helper()
	var ids []string
	for _, line := range strings.Split(content, "\n") {
		if !strings.HasPrefix(line, "| [") {
			continue
		}
		id := strings.TrimPrefix(line, "| [")
		if i := strings.Index(id, "]"); i >= 0 {
			id = id[:i]
		}
		ids = append(ids, id)
	}
	return ids
}

func TestHealRequiredByAndReportRemoved(t *testing.T) {
	root := t.TempDir()
	writeItem(t, root, "aaaaaaaa", "alpha.md", fmStr("aaaaaaaa", "Alpha", "open", 2, "", "2024-01-01T00:00:00Z", []string{"bbbbbbbb"}))
	// Beta carries a hand-written requiredBy entry that nothing derives.
	writeItem(t, root, "bbbbbbbb", "beta.md", `---
id: bbbbbbbb
title: Beta
type: task
status: open
priority: 2
created: 2024-01-01T00:00:00Z
requiredBy:
  - xxxxxxxx
---
# Beta

## Notes
`)

	stderr, err := captureStderr(t, func() error { _, err := sync.Run(root); return err })
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(stderr, "removed requiredBy link xxxxxxxx from bbbbbbbb") {
		t.Fatalf("removed link not reported, stderr: %q", stderr)
	}
	beta := readFile(t, filepath.Join(root, "bbbbbbbb", "beta.md"))
	if !strings.Contains(beta, "requiredBy:") || !strings.Contains(beta, "aaaaaaaa") {
		t.Fatalf("beta.md missing healed requiredBy:\n%s", beta)
	}
	if strings.Contains(beta, "xxxxxxxx") {
		t.Fatalf("beta.md still carries removed link:\n%s", beta)
	}
	alpha := readFile(t, filepath.Join(root, "aaaaaaaa", "alpha.md"))
	if strings.Contains(alpha, "requiredBy") {
		t.Fatalf("alpha.md gained an unexpected requiredBy:\n%s", alpha)
	}
}

func TestSlugRenameAndHeadingRewrite(t *testing.T) {
	root := t.TempDir()
	oldPath := writeItem(t, root, "aaaaaaaa", "old-name.md", `---
id: aaaaaaaa
title: New Title
type: task
status: open
priority: 2
created: 2024-01-01T00:00:00Z
---
# Old Title

## Notes
`)

	if _, err := sync.Run(root); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("stale file still exists at %s", oldPath)
	}
	renamed := filepath.Join(root, "aaaaaaaa", "new-title.md")
	if _, err := os.Stat(renamed); err != nil {
		t.Fatalf("renamed file missing: %v", err)
	}
	content := readFile(t, renamed)
	if !strings.Contains(content, "# New Title") || strings.Contains(content, "# Old Title") {
		t.Fatalf("heading not rewritten:\n%s", content)
	}
}

func TestGeneratedFilesColumnsAndOrdering(t *testing.T) {
	root := t.TempDir()
	writeItem(t, root, "cccccccc", "gamma.md", fmStr("cccccccc", "Gamma", "open", 0, "auto", "2024-01-03T00:00:00Z", nil))
	writeItem(t, root, "aaaaaaaa", "alpha.md", fmStr("aaaaaaaa", "Alpha", "open", 1, "hitl", "2024-01-01T00:00:00Z", nil))
	writeItem(t, root, "dddddddd", "delta.md", fmStr("dddddddd", "Delta", "in_progress", 2, "auto", "2024-01-02T00:00:00Z", []string{"bbbbbbbb"}))
	writeItem(t, root, "bbbbbbbb", "beta.md", fmStr("bbbbbbbb", "Beta", "closed", 0, "auto", "2024-01-02T00:00:00Z", nil))

	if _, err := sync.Run(root); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	index := readFile(t, filepath.Join(root, "INDEX.md"))
	if !strings.Contains(index, "| mode |") {
		t.Fatalf("INDEX missing mode column:\n%s", index)
	}
	gotIndex := tableIDs(t, index)
	wantIndex := []string{"cccccccc", "aaaaaaaa", "dddddddd", "bbbbbbbb"}
	if strings.Join(gotIndex, ",") != strings.Join(wantIndex, ",") {
		t.Fatalf("INDEX order = %v, want %v\n%s", gotIndex, wantIndex, index)
	}
	if !strings.Contains(index, "| [dddddddd](dddddddd/delta.md) | Delta | task | in_progress | 2 | auto | bbbbbbbb |") {
		t.Fatalf("INDEX row missing dependsOn column:\n%s", index)
	}
	if !strings.Contains(index, "| [cccccccc](cccccccc/gamma.md) |") {
		t.Fatalf("INDEX id link missing:\n%s", index)
	}

	frontier := readFile(t, filepath.Join(root, "FRONTIER.md"))
	if !strings.Contains(frontier, "| mode |") {
		t.Fatalf("FRONTIER missing mode column:\n%s", frontier)
	}
	gotFrontier := tableIDs(t, frontier)
	wantFrontier := []string{"cccccccc", "aaaaaaaa", "dddddddd"}
	if strings.Join(gotFrontier, ",") != strings.Join(wantFrontier, ",") {
		t.Fatalf("FRONTIER order = %v, want %v\n%s", gotFrontier, wantFrontier, frontier)
	}

	health := readFile(t, filepath.Join(root, "HEALTH.md"))
	if health != "# HEALTH\n\nNo findings.\n" {
		t.Fatalf("HEALTH not clean:\n%q", health)
	}
}

func TestSecondPassWritesNoFiles(t *testing.T) {
	root := t.TempDir()
	writeItem(t, root, "aaaaaaaa", "alpha.md", fmStr("aaaaaaaa", "Alpha", "open", 2, "", "2024-01-01T00:00:00Z", nil))
	writeItem(t, root, "bbbbbbbb", "beta.md", fmStr("bbbbbbbb", "Beta", "open", 2, "", "2024-01-02T00:00:00Z", nil))

	if _, err := sync.Run(root); err != nil {
		t.Fatalf("first Run returned error: %v", err)
	}
	before := snapshot(root)
	if _, err := sync.Run(root); err != nil {
		t.Fatalf("second Run returned error: %v", err)
	}
	for path, info := range snapshot(root) {
		prev, ok := before[path]
		if !ok {
			t.Fatalf("second pass created %s", path)
		}
		if !prev.sameAs(info) {
			t.Fatalf("second pass rewrote %s", path)
		}
	}
}

// fileState is the content and modtime of one file under the item root.
type fileState struct {
	content string
	mod     int64
}

// sameAs reports whether two snapshots of the same file are identical, modtime included, so a rewrite is caught even when bytes match.
func (a fileState) sameAs(b fileState) bool {
	return a.content == b.content && a.mod == b.mod
}

// snapshot records every non-directory file under root except the store cache, which the store rewrites on every scan by design.
func snapshot(root string) map[string]fileState {
	out := map[string]fileState{}
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Base(path) == "cache.json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		out[path] = fileState{content: string(data), mod: info.ModTime().UnixNano()}
		return nil
	})
	return out
}

func TestInvalidFrontmatterUntouchedAndExcluded(t *testing.T) {
	root := t.TempDir()
	badYAML := writeItem(t, root, "aaaaaaaa", "alpha.md", "---\nid: [unclosed\ntitle: Alpha\n---\n")
	badType := writeItem(t, root, "bbbbbbbb", "beta.md", `---
id: bbbbbbbb
title: Beta
type: bogus
status: open
priority: 2
created: 2024-01-01T00:00:00Z
---
# Beta
`)
	writeItem(t, root, "cccccccc", "gamma.md", fmStr("cccccccc", "Gamma", "open", 2, "", "2024-01-01T00:00:00Z", nil))

	beforeYAML, err := os.ReadFile(badYAML)
	if err != nil {
		t.Fatal(err)
	}
	beforeType, err := os.ReadFile(badType)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sync.Run(root); err == nil {
		t.Fatalf("Run returned nil, want an error")
	}
	afterYAML, err := os.ReadFile(badYAML)
	if err != nil {
		t.Fatal(err)
	}
	afterType, err := os.ReadFile(badType)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforeYAML, afterYAML) {
		t.Fatalf("malformed item file was modified:\n%s", afterYAML)
	}
	if !bytes.Equal(beforeType, afterType) {
		t.Fatalf("invalid-type item file was modified:\n%s", afterType)
	}

	index := readFile(t, filepath.Join(root, "INDEX.md"))
	if strings.Contains(index, "aaaaaaaa") || strings.Contains(index, "bbbbbbbb") {
		t.Fatalf("erroring items present in INDEX:\n%s", index)
	}
	if !strings.Contains(index, "cccccccc") {
		t.Fatalf("valid item missing from INDEX:\n%s", index)
	}
	health := readFile(t, filepath.Join(root, "HEALTH.md"))
	if !strings.Contains(health, "aaaaaaaa") || !strings.Contains(health, "bbbbbbbb") {
		t.Fatalf("erroring items missing from HEALTH:\n%s", health)
	}
}

func TestWarningsDoNotFailRun(t *testing.T) {
	root := t.TempDir()
	writeItem(t, root, "aaaaaaaa", "alpha.md", fmStr("aaaaaaaa", "Alpha", "open", 2, "", "2024-01-01T00:00:00Z", nil))
	writeItem(t, root, "bbbbbbbb", "beta.md", fmStr("bbbbbbbb", "Beta", "in_progress", 2, "", "2024-01-01T00:00:00Z", []string{"aaaaaaaa"}))

	if _, err := sync.Run(root); err != nil {
		t.Fatalf("stale-status warning failed the run: %v", err)
	}
	health := readFile(t, filepath.Join(root, "HEALTH.md"))
	if !strings.Contains(health, "in_progress but dependency aaaaaaaa is open") {
		t.Fatalf("warning missing from HEALTH:\n%s", health)
	}
}

func TestCycleAndSeveralMDFilesAreErrors(t *testing.T) {
	root := t.TempDir()
	writeItem(t, root, "aaaaaaaa", "alpha.md", fmStr("aaaaaaaa", "Alpha", "open", 2, "", "2024-01-01T00:00:00Z", []string{"bbbbbbbb"}))
	writeItem(t, root, "bbbbbbbb", "beta.md", fmStr("bbbbbbbb", "Beta", "open", 2, "", "2024-01-01T00:00:00Z", []string{"aaaaaaaa"}))
	writeItem(t, root, "cccccccc", "gamma.md", fmStr("cccccccc", "Gamma", "open", 2, "", "2024-01-01T00:00:00Z", nil))
	writeItem(t, root, "cccccccc", "extra.md", fmStr("cccccccc", "Gamma", "open", 2, "", "2024-01-01T00:00:00Z", nil))

	if _, err := sync.Run(root); err == nil {
		t.Fatalf("Run returned nil, want errors for cycle and duplicate md files")
	}
	health := readFile(t, filepath.Join(root, "HEALTH.md"))
	if !strings.Contains(health, "dependency cycle") {
		t.Fatalf("cycle missing from HEALTH:\n%s", health)
	}
	if !strings.Contains(health, "several") {
		t.Fatalf("several-md finding missing from HEALTH:\n%s", health)
	}
	index := readFile(t, filepath.Join(root, "INDEX.md"))
	for _, id := range []string{"aaaaaaaa", "bbbbbbbb", "cccccccc"} {
		if strings.Contains(index, "["+id+"]") {
			t.Fatalf("erroring item %s present in INDEX:\n%s", id, index)
		}
	}
}

func TestIdMismatchAndDanglingLinksAreErrors(t *testing.T) {
	root := t.TempDir()
	// Folder name does not match the frontmatter id.
	writeItem(t, root, "zzzzzzzz", "alpha.md", fmStr("aaaaaaaa", "Alpha", "open", 2, "", "2024-01-01T00:00:00Z", nil))
	// dependsOn points at an item that does not exist.
	writeItem(t, root, "eeeeeeee", "echo.md", fmStr("eeeeeeee", "Echo", "open", 2, "", "2024-01-01T00:00:00Z", []string{"ffffffff"}))

	if _, err := sync.Run(root); err == nil {
		t.Fatalf("Run returned nil, want id mismatch and dangling link errors")
	}
	health := readFile(t, filepath.Join(root, "HEALTH.md"))
	if !strings.Contains(health, "does not match folder name") {
		t.Fatalf("id mismatch missing from HEALTH:\n%s", health)
	}
	if !strings.Contains(health, "link to missing id ffffffff") {
		t.Fatalf("dangling link missing from HEALTH:\n%s", health)
	}
	index := readFile(t, filepath.Join(root, "INDEX.md"))
	for _, id := range []string{"aaaaaaaa", "eeeeeeee"} {
		if strings.Contains(index, "["+id+"]") {
			t.Fatalf("erroring item %s present in INDEX:\n%s", id, index)
		}
	}
}

func TestSyncCRLFItemHealsAndIndexes(t *testing.T) {
	root := t.TempDir()
	lf := fmStr("aaaaaaaa", "Alpha", "open", 2, "", "2024-01-01T00:00:00Z", []string{"bbbbbbbb"})
	writeItem(t, root, "aaaaaaaa", "alpha.md", strings.ReplaceAll(lf, "\n", "\r\n"))
	writeItem(t, root, "bbbbbbbb", "beta.md", fmStr("bbbbbbbb", "Beta", "open", 2, "", "2024-01-01T00:00:00Z", nil))
	res, err := sync.Run(root)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("CRLF item produced error findings: %+v", res.Errors)
	}
	idx := readFile(t, filepath.Join(root, "INDEX.md"))
	if !strings.Contains(idx, "aaaaaaaa") {
		t.Fatalf("CRLF item missing from INDEX:\n%s", idx)
	}
	beta := readFile(t, filepath.Join(root, "bbbbbbbb", "beta.md"))
	if !strings.Contains(beta, "aaaaaaaa") {
		t.Fatalf("requiredBy not healed from CRLF dependency:\n%s", beta)
	}
}

func TestSyncCRLFHeadingStable(t *testing.T) {
	root := t.TempDir()
	lf := fmStr("aaaaaaaa", "Alpha", "open", 2, "", "2024-01-01T00:00:00Z", nil)
	crlf := strings.ReplaceAll(lf, "\n", "\r\n")
	crlf = strings.Replace(crlf, "# Alpha\r\n", "# Wrong\r\n", 1)
	path := writeItem(t, root, "aaaaaaaa", "alpha.md", crlf)
	if _, err := sync.Run(root); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, path)
	if !strings.Contains(got, "# Alpha\r\n") {
		t.Fatalf("heading rewrite lost CRLF:\n%q", got)
	}
	if _, err := sync.Run(root); err != nil {
		t.Fatal(err)
	}
	if readFile(t, path) != got {
		t.Fatal("second sync rewrote a stable CRLF heading")
	}
}
