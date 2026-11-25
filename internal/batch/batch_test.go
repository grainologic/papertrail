package batch_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"papertrail/internal/cli"
	"papertrail/internal/config"
	"papertrail/internal/item"
)

// project is one initialized papertrail project in a temp directory.
type project struct {
	dir  string
	root string
}

func newProject(t *testing.T) *project {
	t.Helper()
	p := &project{dir: t.TempDir()}
	var stdout, stderr strings.Builder
	if code := cli.RunIn(p.dir, []string{"init"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init: exit %d, stderr %q", code, stderr.String())
	}
	cfg, err := config.Find(p.dir)
	if err != nil {
		t.Fatalf("config.Find: %v", err)
	}
	p.root = cfg.Root
	return p
}

// run executes batch with a JSON payload written to a file in the project.
// It returns the exit code and both output streams.
func (p *project) run(t *testing.T, payload string, args ...string) (int, string, string) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "batch.json")
	if payload != "" {
		if err := os.WriteFile(file, []byte(payload), 0o644); err != nil {
			t.Fatalf("write batch file: %v", err)
		}
	}
	var stdout, stderr strings.Builder
	code := cli.RunIn(p.dir, append(append([]string{"batch", file}, args...), "--json"), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// existing creates one item through pt create and returns its ID.
func (p *project) existing(t *testing.T) string {
	t.Helper()
	var stdout, stderr strings.Builder
	if code := cli.RunIn(p.dir, []string{"create", "task", "Existing", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("create: exit %d, stderr %q", code, stderr.String())
	}
	cfg, err := config.Find(p.dir)
	if err != nil {
		t.Fatalf("config.Find: %v", err)
	}
	res, err := item.Parse(mustRead(t, firstMD(t, cfg.Root, "Existing")))
	if err != nil {
		t.Fatalf("parse existing: %v", err)
	}
	return res.Frontmatter.ID
}

// firstMD returns the item file path whose content contains the title.
func firstMD(t *testing.T, root, title string) string {
	t.Helper()
	entries, err := filepath.Glob(filepath.Join(root, "*", "*.md"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	for _, path := range entries {
		data, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(data), "# "+title) {
			return path
		}
	}
	t.Fatalf("no item with title %q under %s", title, root)
	return ""
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

// itemIDs returns the full IDs of every item under root by parsing files.
func itemIDs(t *testing.T, root string) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(root, "*", "*.md"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	var ids []string
	for _, path := range paths {
		parsed, err := item.Parse(mustRead(t, path))
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ids = append(ids, parsed.Frontmatter.ID)
	}
	return ids
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func frontmatterOf(t *testing.T, root, id string) item.Frontmatter {
	t.Helper()
	paths, _ := filepath.Glob(filepath.Join(root, id, "*.md"))
	if len(paths) != 1 {
		t.Fatalf("item %s: %d md files", id, len(paths))
	}
	parsed, err := item.Parse(mustRead(t, paths[0]))
	if err != nil {
		t.Fatalf("parse %s: %v", paths[0], err)
	}
	return parsed.Frontmatter
}

func TestBatchCreatesAllAndDeletesFile(t *testing.T) {
	p := newProject(t)
	existingID := p.existing(t)

	payload := `[
		{"key": "a", "type": "task", "title": "First", "priority": 1,
		 "dependsOn": ["b"], "requiredBy": ["` + existingID + `"], "relatedTo": ["` + existingID + `"]},
		{"key": "b", "type": "bug", "title": "Second", "mode": "hitl",
		 "relatedTo": ["a"]}
	]`
	file := filepath.Join(t.TempDir(), "batch.json")
	if err := os.WriteFile(file, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr strings.Builder
	code := cli.RunIn(p.dir, []string{"batch", file, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("batch: exit %d, stderr %q", code, stderr.String())
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("batch file still exists after success")
	}

	ids := itemIDs(t, p.root)
	if len(ids) != 3 {
		t.Fatalf("want 3 items, got %v", ids)
	}
	var first, second string
	for _, id := range ids {
		fm := frontmatterOf(t, p.root, id)
		switch fm.Title {
		case "First":
			first = id
		case "Second":
			second = id
		}
	}
	if first == "" || second == "" {
		t.Fatalf("batch items not created: %v", ids)
	}
	firstFM := frontmatterOf(t, p.root, first)
	if len(firstFM.DependsOn) != 1 || firstFM.DependsOn[0] != second {
		t.Fatalf("first dependsOn = %v, want [%s]", firstFM.DependsOn, second)
	}
	if firstFM.Priority != 1 {
		t.Fatalf("first priority = %d, want 1", firstFM.Priority)
	}
	if firstFM.Mode != item.ModeAuto {
		t.Fatalf("first mode = %q, want auto", firstFM.Mode)
	}
	secondFM := frontmatterOf(t, p.root, second)
	if secondFM.Mode != item.ModeHitl {
		t.Fatalf("second mode = %q, want hitl", secondFM.Mode)
	}
	// requiredBy on the existing item resolves to it depending on first;
	// relatedTo is symmetric.
	if len(secondFM.DependsOn) != 0 {
		t.Fatalf("second dependsOn = %v, want empty", secondFM.DependsOn)
	}
	existingFM := frontmatterOf(t, p.root, existingID)
	if len(existingFM.DependsOn) != 1 || existingFM.DependsOn[0] != first {
		t.Fatalf("existing dependsOn = %v, want [%s]", existingFM.DependsOn, first)
	}
	// Each entry keeps its own relatedTo declaration; one that points at the existing item is mirrored onto that item by symmetry.
	if !contains(secondFM.RelatedTo, first) {
		t.Fatalf("second relatedTo = %v, want to contain %s", secondFM.RelatedTo, first)
	}
	if !contains(existingFM.RelatedTo, first) {
		t.Fatalf("existing relatedTo = %v, want to contain %s", existingFM.RelatedTo, first)
	}
}

func TestBatchDryRunWritesNothingAndKeepsFile(t *testing.T) {
	p := newProject(t)
	p.existing(t)
	file := filepath.Join(t.TempDir(), "batch.json")
	payload := `[{"key": "a", "type": "task", "title": "Planned"}]`
	if err := os.WriteFile(file, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr strings.Builder
	code := cli.RunIn(p.dir, []string{"batch", file, "--dry-run", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("dry-run: exit %d, stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"title": "Planned"`) {
		t.Fatalf("plan missing planned title: %q", stdout.String())
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("dry-run deleted the input file: %v", err)
	}
	if ids := itemIDs(t, p.root); len(ids) != 1 {
		t.Fatalf("dry-run wrote items: %v", ids)
	}
}

func TestBatchRejectsBadInputWithoutWriting(t *testing.T) {
	cases := []struct {
		name    string
		payload string
	}{
		{"malformed json", `[{"key": "a"`},
		{"unknown key", `[{"key": "a", "type": "task", "title": "A", "dependsOn": ["nope"]}]`},
		{"duplicate key", `[
			{"key": "a", "type": "task", "title": "A"},
			{"key": "a", "type": "task", "title": "B"}
		]`},
		{"cycle", `[
			{"key": "a", "type": "task", "title": "A", "dependsOn": ["b"]},
			{"key": "b", "type": "task", "title": "B", "dependsOn": ["a"]}
		]`},
		{"unknown type", `[{"key": "a", "type": "nope", "title": "A"}]`},
		{"invalid priority", `[{"key": "a", "type": "task", "title": "A", "priority": 9}]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := newProject(t)
			p.existing(t)
			code, stdout, stderr := p.run(t, tc.payload)
			if code == 0 {
				t.Fatalf("want nonzero exit, stdout %q", stdout)
			}
			if !strings.Contains(stdout, `"error"`) {
				t.Fatalf("missing JSON error report: stdout %q stderr %q", stdout, stderr)
			}
			if ids := itemIDs(t, p.root); len(ids) != 1 {
				t.Fatalf("invalid batch wrote items: %v", ids)
			}
		})
	}
}

func TestBatchFileKeptOnFailure(t *testing.T) {
	p := newProject(t)
	code, _, _ := p.run(t, `[{"key": "a", "type": "task", "title": "A", "dependsOn": ["nope"]}]`)
	if code == 0 {
		t.Fatal("want nonzero exit")
	}
	// p.run writes into t.TempDir of that call; verify via a direct rerun.
	file := filepath.Join(t.TempDir(), "batch.json")
	if err := os.WriteFile(file, []byte(`[{"key": "a", "type": "task", "title": "A", "dependsOn": ["nope"]}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr strings.Builder
	if code := cli.RunIn(p.dir, []string{"batch", file, "--json"}, &stdout, &stderr); code == 0 {
		t.Fatal("want nonzero exit")
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("input file removed on failure: %v", err)
	}
}

func TestBatchUsageErrors(t *testing.T) {
	p := newProject(t)
	var stdout, stderr strings.Builder
	for _, args := range [][]string{
		{"batch"},
		{"batch", "a", "b"},
		{"batch"},
	} {
		if code := cli.RunIn(p.dir, args, &stdout, &stderr); code != 2 {
			t.Fatalf("%v: exit %d, want 2 (stderr %q)", args, code, stderr.String())
		}
	}
	if code := cli.RunIn(p.dir, []string{"batch", "--json"}, &stdout, &stderr); code != 2 || !strings.Contains(stdout.String(), `"error"`) {
		t.Fatalf("batch --json: exit %d, stdout %q", code, stdout.String())
	}
}

func TestBatchTwoEntriesMixedLinksSameExistingItem(t *testing.T) {
	p := newProject(t)
	existingID := p.existing(t)
	payload := `[
		{"key": "a", "type": "task", "title": "A", "relatedTo": ["` + existingID + `"]},
		{"key": "b", "type": "task", "title": "B", "requiredBy": ["` + existingID + `"]}
	]`
	code, stdout, stderr := p.run(t, payload)
	if code != 0 {
		t.Fatalf("batch: exit %d, stdout %q stderr %q", code, stdout, stderr)
	}
	ids := itemIDs(t, p.root)
	var aID, bID string
	for _, id := range ids {
		fm := frontmatterOf(t, p.root, id)
		switch fm.Title {
		case "A":
			aID = id
		case "B":
			bID = id
		}
	}
	if aID == "" || bID == "" {
		t.Fatalf("batch items not created: %v", ids)
	}
	existingFM := frontmatterOf(t, p.root, existingID)
	if !contains(existingFM.RelatedTo, aID) {
		t.Fatalf("existing relatedTo = %v, want to contain %s", existingFM.RelatedTo, aID)
	}
	if !contains(existingFM.DependsOn, bID) {
		t.Fatalf("existing dependsOn = %v, want to contain %s", existingFM.DependsOn, bID)
	}
}

func TestBatchRejectsEmptySlugTitle(t *testing.T) {
	p := newProject(t)
	code, _, _ := p.run(t, `[{"key": "a", "type": "task", "title": "!!!"}]`)
	if code == 0 {
		t.Fatal("want nonzero exit for empty-slug title")
	}
	if ids := itemIDs(t, p.root); len(ids) != 0 {
		t.Fatalf("invalid title wrote items: %v", ids)
	}
}
