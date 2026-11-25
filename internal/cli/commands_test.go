package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"papertrail/internal/config"
)

// project is one initialized papertrail project in a temp directory.
type project struct {
	dir  string // starting directory for config lookup
	root string // item root
}

// newProject initializes a project in a fresh temp directory.
func newProject(t *testing.T) *project {
	t.Helper()
	p := &project{dir: t.TempDir()}
	var stdout, stderr strings.Builder
	if code := RunIn(p.dir, []string{"init"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init: exit %d, stderr %q", code, stderr.String())
	}
	cfg, err := config.Find(p.dir)
	if err != nil {
		t.Fatalf("config.Find: %v", err)
	}
	p.root = cfg.Root
	return p
}

// run executes one command, failing the test on a nonzero exit, and returns the parsed JSON result.
func (p *project) run(t *testing.T, args ...string) map[string]any {
	t.Helper()
	args = append(append([]string{}, args...), "--json")
	var stdout, stderr strings.Builder
	code := RunIn(p.dir, args, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("%v: exit %d, stderr %q", args, code, stderr.String())
	}
	var doc any
	if err := json.Unmarshal([]byte(stdout.String()), &doc); err != nil {
		t.Fatalf("%v: output is not JSON: %q", args, stdout.String())
	}
	if m, ok := doc.(map[string]any); ok {
		return m
	}
	return map[string]any{"_": doc}
}

// runFail executes one command expecting a nonzero exit.
func (p *project) runFail(t *testing.T, args ...string) {
	t.Helper()
	args = append(append([]string{}, args...), "--json")
	var stdout, stderr strings.Builder
	if code := RunIn(p.dir, args, &stdout, &stderr); code == 0 {
		t.Fatalf("%v: want nonzero exit, stdout %q", args, stdout.String())
	}
}

// create makes an item and returns its full ID.
func (p *project) create(t *testing.T, args ...string) string {
	t.Helper()
	out := p.run(t, append([]string{"create"}, args...)...)
	id, _ := out["id"].(string)
	if id == "" {
		t.Fatalf("create %v: no id in result %v", args, out)
	}
	return id
}

// itemPath returns the item file path for an ID.
func (p *project) itemPath(t *testing.T, id string) string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(p.root, id))
	if err != nil {
		t.Fatalf("read item dir %s: %v", id, err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			return filepath.Join(p.root, id, e.Name())
		}
	}
	t.Fatalf("no .md in %s", filepath.Join(p.root, id))
	return ""
}

// links reads one link field from an item's frontmatter.
func (p *project) links(t *testing.T, id, field string) []string {
	t.Helper()
	data, err := os.ReadFile(p.itemPath(t, id))
	if err != nil {
		t.Fatalf("read %s: %v", id, err)
	}
	var values []string
	in := false
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, field+":") {
			in = true
			continue
		}
		if in {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "- ") {
				values = append(values, strings.TrimPrefix(trimmed, "- "))
				continue
			}
			break
		}
	}
	return values
}

func TestCreateWithLinksWritesBothSides(t *testing.T) {
	p := newProject(t)
	a := p.create(t, "feature", "First")
	// b depends on a; c is required by b (b depends on c); d relates to a.
	b := p.create(t, "task", "Second", "--depends-on", a)
	c := p.create(t, "task", "Third", "--required-by", b)
	d := p.create(t, "task", "Fourth", "--related-to", a)

	if got := p.links(t, b, "dependsOn"); len(got) != 2 || got[0] != a || got[1] != c {
		t.Errorf("b dependsOn = %v, want [%s %s]", got, a, c)
	}
	if got := p.links(t, a, "relatedTo"); len(got) != 1 || got[0] != d {
		t.Errorf("a relatedTo = %v, want [%s]", got, d)
	}
	if got := p.links(t, d, "relatedTo"); len(got) != 1 || got[0] != a {
		t.Errorf("d relatedTo = %v, want [%s]", got, a)
	}
}

func TestCreateRefusesBadTargets(t *testing.T) {
	p := newProject(t)
	p.runFail(t, "create", "task", "Delta", "--depends-on", "zzzzzzzz")
	p.runFail(t, "create", "task", "Epsilon", "--mode", "sometimes")
	p.runFail(t, "create", "task", "Zeta", "--priority", "9")
	p.runFail(t, "create", "nosuchtype", "Eta")
}

func TestDeletePreviewAndForce(t *testing.T) {
	p := newProject(t)
	a := p.create(t, "task", "Alpha")
	b := p.create(t, "task", "Beta", "--depends-on", a)

	out := p.run(t, "delete", a)
	if out["wouldRemove"] != true {
		t.Errorf("preview result = %v, want wouldRemove", out)
	}
	if _, err := os.Stat(p.itemPath(t, a)); err != nil {
		t.Error("preview removed the item folder")
	}

	out = p.run(t, "delete", a, "-y")
	if out["removed"] != true {
		t.Errorf("delete result = %v, want removed", out)
	}
	if _, err := os.Stat(filepath.Join(p.root, a)); !os.IsNotExist(err) {
		t.Errorf("item folder still exists: %v", err)
	}
	if got := p.links(t, b, "dependsOn"); len(got) != 0 {
		t.Errorf("b dependsOn after delete = %v, want empty", got)
	}
}

func TestUpdateAndUnknownID(t *testing.T) {
	p := newProject(t)
	a := p.create(t, "task", "Alpha")

	p.run(t, "update", a, "--status", "closed")
	data, err := os.ReadFile(p.itemPath(t, a))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "status: closed") {
		t.Errorf("frontmatter = %q, want status: closed", data)
	}

	p.runFail(t, "update", "zzzzzzzz", "--status", "open")
	p.runFail(t, "update", a, "--status", "nope")
	p.runFail(t, "show", "zzzzzzzz")
	p.runFail(t, "delete", "zzzzzzzz")
}

func TestListFilters(t *testing.T) {
	p := newProject(t)
	p.create(t, "task", "Alpha")
	b := p.create(t, "bug", "Beta")

	var stdout strings.Builder
	if code := RunIn(p.dir, []string{"list", "--type", "bug", "--json"}, &stdout, nil); code != 0 {
		t.Fatalf("list: exit %d", code)
	}
	if !strings.Contains(stdout.String(), b) {
		t.Errorf("list output missing %s", b)
	}
	p.runFail(t, "list", "--status", "nonsense")
	p.runFail(t, "list", "--type", "nonsense")
}

func TestShowListsDependenciesWithStatus(t *testing.T) {
	p := newProject(t)
	a := p.create(t, "task", "Alpha")
	b := p.create(t, "bug", "Beta", "--depends-on", a)
	out := p.run(t, "show", b[:4])
	deps, _ := out["dependencies"].([]any)
	if len(deps) != 1 {
		t.Fatalf("dependencies = %v, want one", deps)
	}
	d := deps[0].(map[string]any)
	if d["id"] != a || d["status"] != "open" || d["field"] != "dependsOn" {
		t.Errorf("dependency = %v, want id %s, status open, field dependsOn", d, a)
	}
}

func TestFrontierExcludesBlockedByOpenDependency(t *testing.T) {
	p := newProject(t)
	a := p.create(t, "task", "Alpha")
	b := p.create(t, "task", "Beta", "--depends-on", a)

	var stdout strings.Builder
	if code := RunIn(p.dir, []string{"frontier"}, &stdout, nil); code != 0 {
		t.Fatalf("frontier: exit %d", code)
	}
	if strings.Contains(stdout.String(), b) {
		t.Errorf("frontier contains %s whose dependency %s is open", b, a)
	}
	if !strings.Contains(stdout.String(), a) {
		t.Errorf("frontier missing %s", a)
	}

	// Closing the dependency brings b onto the frontier.
	p.run(t, "update", a, "--status", "closed")
	stdout.Reset()
	if code := RunIn(p.dir, []string{"frontier"}, &stdout, nil); code != 0 {
		t.Fatalf("frontier: exit %d", code)
	}
	if !strings.Contains(stdout.String(), b) {
		t.Errorf("frontier missing %s after its dependency closed", b)
	}
}

func TestSyncCommandJSONParses(t *testing.T) {
	p := newProject(t)
	p.create(t, "task", "Alpha")
	p.run(t, "sync")
}

func TestEveryCommandAcceptsJSON(t *testing.T) {
	p := newProject(t)
	a := p.create(t, "task", "Alpha")
	for _, args := range [][]string{
		{"list"},
		{"show", a[:4]},
		{"frontier"},
		{"sync"},
	} {
		var stdout strings.Builder
		if code := RunIn(p.dir, append(append([]string{}, args...), "--json"), &stdout, nil); code != 0 {
			t.Errorf("%v: exit %d", args, code)
			continue
		}
		trimmed := strings.TrimSpace(stdout.String())
		if !strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[") {
			t.Errorf("%v: output not JSON: %q", args, trimmed)
		}
	}
}
