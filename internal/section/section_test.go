package section

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"papertrail/internal/cli"
)

const demoItem = `---
id: ab12cd34
title: demo
type: task
status: open
priority: 2
created: 2026-01-01T00:00:00Z
---

# demo

overview text

## Goal

build the thing

## Acceptance Criteria

- it works

### edge cases

- empty input

## Notes

written down
`

// project is one papertrail workspace in a temp directory with a single item.
type project struct {
	dir   string    // starting directory for config lookup
	path  string    // the item file
	stdin io.Reader // optional input for section set
}

func newProject(t *testing.T) *project {
	t.Helper()
	dir := t.TempDir()
	root := filepath.Join(dir, ".papertrail")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config.toml"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	itemDir := filepath.Join(root, "ab12cd34")
	if err := os.MkdirAll(itemDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(itemDir, "demo.md")
	if err := os.WriteFile(path, []byte(demoItem), 0o644); err != nil {
		t.Fatal(err)
	}
	return &project{dir: dir, path: path}
}

// run invokes a registered command like the wiring stage does.
func (p *project) run(t *testing.T, name string, args ...string) (int, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	cmd, ok := cli.Lookup(name)
	if !ok {
		t.Fatalf("command %q not registered", name)
	}
	var stdout, stderr bytes.Buffer
	env := cli.Env{Dir: p.dir, Stdout: &stdout, Stderr: &stderr}
	if p.stdin != nil {
		env.Stdin = p.stdin
	}
	code := cmd.Run(env, args)
	return code, &stdout, &stderr
}

func (p *project) file(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(p.path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSectionGetIncludesSubheadings(t *testing.T) {
	p := newProject(t)
	code, out, stderr := p.run(t, "section get", "ab12cd34", "Acceptance", "Criteria")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
	want := "\n- it works\n\n### edge cases\n\n- empty input\n\n"
	if out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}
}

func TestSectionSetReplacesOnlySectionBytes(t *testing.T) {
	p := newProject(t)
	before := p.file(t)
	p.stdin = strings.NewReader("new goal\n")
	code, out, stderr := p.run(t, "section set", "ab12cd34", "Goal")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
	_ = out
	after := p.file(t)
	want := strings.Replace(before, "## Goal\n\nbuild the thing\n\n", "## Goal\nnew goal\n", 1)
	if after != want {
		t.Errorf("file changed outside the section:\n%s", after)
	}
}

func TestSectionSetFromFile(t *testing.T) {
	p := newProject(t)
	before := p.file(t)
	src := filepath.Join(t.TempDir(), "replacement.md")
	if err := os.WriteFile(src, []byte("from file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := p.run(t, "section set", "ab12cd34", "Goal", "--file", src)
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
	want := strings.Replace(before, "## Goal\n\nbuild the thing\n\n", "## Goal\nfrom file\n", 1)
	if after := p.file(t); after != want {
		t.Errorf("got:\n%s\nwant:\n%s", after, want)
	}
	// The source file must be left in place.
	if _, err := os.Stat(src); err != nil {
		t.Errorf("source file was removed: %v", err)
	}
}

func TestSectionSetMissingFileErrors(t *testing.T) {
	p := newProject(t)
	code, _, stderr := p.run(t, "section set", "ab12cd34", "Goal", "--file", "does-not-exist.md")
	if code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "read") {
		t.Errorf("stderr %q, want a read failure", stderr.String())
	}
}

func TestSectionSetEmptyReplacement(t *testing.T) {
	p := newProject(t)
	before := p.file(t)
	p.stdin = strings.NewReader("")
	code, _, stderr := p.run(t, "section set", "ab12cd34", "Notes")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
	if strings.Contains(p.file(t), "written down") {
		t.Errorf("section not cleared:\n%s", p.file(t))
	}
	if before == p.file(t) {
		t.Error("file unchanged")
	}
}

func TestSectionMissingHeadingErrors(t *testing.T) {
	p := newProject(t)
	code, _, stderr := p.run(t, "section get", "ab12cd34", "Nonexistent")
	if code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "no heading") {
		t.Errorf("stderr %q", stderr.String())
	}
}

func TestSectionAmbiguousHeadingErrors(t *testing.T) {
	p := newProject(t)
	// A level 3 heading whose text repeats a level 2 heading is ambiguous:
	// exact text matches twice.
	item := strings.Replace(demoItem, "## Notes\n\nwritten down\n",
		"### Goal\n\nsub note\n\n## Notes\n\nwritten down\n", 1)
	if err := os.WriteFile(p.path, []byte(item), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := p.run(t, "section get", "ab12cd34", "Goal")
	if code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "matches more than once") {
		t.Errorf("stderr %q", stderr.String())
	}
}

func TestSectionGetUnknownIDErrors(t *testing.T) {
	p := newProject(t)
	code, _, stderr := p.run(t, "section get", "zzzzzzzz", "Goal")
	if code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
	if stderr.String() == "" {
		t.Error("no error reported")
	}
}

func TestSectionUsageErrors(t *testing.T) {
	p := newProject(t)
	code, _, _ := p.run(t, "section get", "ab12cd34")
	if code != 2 {
		t.Errorf("exit %d, want 2", code)
	}
}

func TestSectionJSON(t *testing.T) {
	p := newProject(t)
	code, out, stderr := p.run(t, "section get", "--json", "ab12cd34", "Goal")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(out.String()), &doc); err != nil {
		t.Fatalf("output is not JSON: %q", out.String())
	}
	if doc["id"] != "ab12cd34" || doc["heading"] != "Goal" {
		t.Errorf("got %v", doc)
	}
	wantContent := "\nbuild the thing\n\n"
	if doc["content"] != wantContent {
		t.Errorf("content %q, want %q", doc["content"], wantContent)
	}
}

func TestSectionIgnoresHeadingsInsideCodeFences(t *testing.T) {
	p := newProject(t)
	content := strings.Replace(demoItem, "build the thing\n", "build the thing\n\n```\n## Acceptance Criteria\n```\n", 1)
	if err := os.WriteFile(p.path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := p.run(t, "section get", "ab12cd34", "Goal")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
	if !strings.Contains(out.String(), "```") {
		t.Errorf("Goal section truncated at fenced heading:\n%s", out.String())
	}
	code, out, stderr = p.run(t, "section get", "ab12cd34", "Acceptance", "Criteria")
	if code != 0 {
		t.Fatalf("fenced heading made match ambiguous: exit %d, stderr %q", code, stderr.String())
	}
	if !strings.Contains(out.String(), "it works") {
		t.Errorf("real Acceptance Criteria section missing:\n%s", out.String())
	}
}

func TestHelpSectionCommands(t *testing.T) {
	for _, args := range [][]string{
		{"help", "section", "get"},
		{"help", "section", "set"},
		{"section", "set", "--help"},
		{"section", "get", "-h"},
	} {
		var stdout, stderr strings.Builder
		code := cli.RunIn(t.TempDir(), args, &stdout, &stderr)
		if code != 0 {
			t.Errorf("%v: exit %d, stderr %q", args, code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "usage:") {
			t.Errorf("%v: missing usage in stdout %q", args, stdout.String())
		}
	}
}
