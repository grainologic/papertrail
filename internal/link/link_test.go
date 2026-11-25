package link

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"papertrail/internal/cli"
	"papertrail/internal/config"
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

func (p *project) run(t *testing.T, args ...string) map[string]any {
	t.Helper()
	args = append(append([]string{}, args...), "--json")
	var stdout, stderr strings.Builder
	code := cli.RunIn(p.dir, args, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("%v: exit %d, stderr %q", args, code, stderr.String())
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &doc); err != nil {
		t.Fatalf("%v: output is not JSON: %q", args, stdout.String())
	}
	return doc
}

func (p *project) runFail(t *testing.T, args ...string) {
	t.Helper()
	args = append(append([]string{}, args...), "--json")
	var stdout, stderr strings.Builder
	if code := cli.RunIn(p.dir, args, &stdout, &stderr); code == 0 {
		t.Fatalf("%v: want nonzero exit, stdout %q", args, stdout.String())
	}
}

func (p *project) create(t *testing.T, args ...string) string {
	t.Helper()
	out := p.run(t, append([]string{"create"}, args...)...)
	id, _ := out["id"].(string)
	if id == "" {
		t.Fatalf("create %v: no id in result %v", args, out)
	}
	return id
}

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

func TestLinkUpdatesBothSides(t *testing.T) {
	p := newProject(t)
	a := p.create(t, "task", "Alpha")
	b := p.create(t, "task", "Beta")
	c := p.create(t, "task", "Gamma")

	p.run(t, "link", a, "--depends-on", b)
	if got := p.links(t, a, "dependsOn"); len(got) != 1 || got[0] != b {
		t.Errorf("a dependsOn = %v, want [%s]", got, b)
	}

	p.run(t, "link", c, "--required-by", b)
	// required-by b means b depends on c.
	if got := p.links(t, b, "dependsOn"); len(got) != 1 || got[0] != c {
		t.Errorf("b dependsOn = %v, want [%s]", got, c)
	}

	p.run(t, "link", a, "--related-to", c)
	if got := p.links(t, a, "relatedTo"); len(got) != 1 || got[0] != c {
		t.Errorf("a relatedTo = %v, want [%s]", got, c)
	}
	if got := p.links(t, c, "relatedTo"); len(got) != 1 || got[0] != a {
		t.Errorf("c relatedTo = %v, want [%s]", got, a)
	}
}

func TestLinkRefusesCycle(t *testing.T) {
	p := newProject(t)
	a := p.create(t, "task", "Alpha")
	b := p.create(t, "task", "Beta", "--depends-on", a)

	before, err := os.ReadFile(p.itemPath(t, a))
	if err != nil {
		t.Fatal(err)
	}
	p.runFail(t, "link", a, "--depends-on", b)
	after, err := os.ReadFile(p.itemPath(t, a))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("refused link changed the item file")
	}
}

func TestUnlinkRemovesBothSides(t *testing.T) {
	p := newProject(t)
	a := p.create(t, "task", "Alpha")
	b := p.create(t, "task", "Beta")
	c := p.create(t, "task", "Gamma")
	d := p.create(t, "task", "Delta")

	p.run(t, "link", a, "--depends-on", b)
	p.run(t, "unlink", a, "--depends-on", b)
	if got := p.links(t, a, "dependsOn"); len(got) != 0 {
		t.Errorf("a dependsOn after unlink = %v, want empty", got)
	}

	// required-by d on c: c depends on d; unlinking removes c -> d.
	p.run(t, "link", c, "--required-by", d)
	p.run(t, "unlink", c, "--required-by", d)
	if got := p.links(t, c, "dependsOn"); len(got) != 0 {
		t.Errorf("c dependsOn after unlink = %v, want empty", got)
	}

	p.run(t, "link", a, "--related-to", b)
	p.run(t, "unlink", a, "--related-to", b)
	if got := p.links(t, a, "relatedTo"); len(got) != 0 {
		t.Errorf("a relatedTo after unlink = %v, want empty", got)
	}
	if got := p.links(t, b, "relatedTo"); len(got) != 0 {
		t.Errorf("b relatedTo after unlink = %v, want empty", got)
	}
}

func TestInsertBetween(t *testing.T) {
	p := newProject(t)
	a := p.create(t, "task", "Alpha")
	b := p.create(t, "task", "Beta", "--depends-on", a)

	out := p.run(t, "insert", "task", "Middle", "--after", a, "--before", b)
	n, _ := out["id"].(string)
	if n == "" {
		t.Fatalf("insert result missing id: %v", out)
	}
	if got := p.links(t, n, "dependsOn"); len(got) != 1 || got[0] != a {
		t.Errorf("n dependsOn = %v, want [%s]", got, a)
	}
	if got := p.links(t, b, "dependsOn"); len(got) != 1 || got[0] != n {
		t.Errorf("b dependsOn = %v, want [%s] with no direct %s", got, n, a)
	}
}

func TestInsertRefusesCycleAndBadArgs(t *testing.T) {
	p := newProject(t)
	a := p.create(t, "task", "Alpha")
	c := p.create(t, "task", "Charlie", "--depends-on", a)
	b := p.create(t, "task", "Beta")
	p.run(t, "link", b, "--depends-on", c)

	// n -> b -> c -> a -> n is a cycle.
	p.runFail(t, "insert", "task", "Loop", "--after", b, "--before", a)
	// Missing one endpoint is a usage error.
	p.runFail(t, "insert", "task", "NoAfter", "--before", b)
	p.runFail(t, "insert", "task", "SameEnds", "--after", a, "--before", a)
	p.runFail(t, "insert", "nosuchtype", "Bad", "--after", a, "--before", b)
}
