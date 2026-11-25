package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"papertrail/internal/item"
	"papertrail/internal/store"
	"papertrail/internal/sync"
	"papertrail/internal/template"
)

const createUsage = "pt create <type> <title> [--priority 0-4] [--mode auto|hitl] [--depends-on ID]... [--required-by ID]... [--related-to ID]..."

// createCmd is the create command; Run is wired in init so the initializer stays cycle-free.
var createCmd = Command{
	Name:    "create",
	Usage:   createUsage,
	Summary: "Create a new work item",
	Help:    "The type is one of bug, chore, debate, feature, feedback, milestone, question, research, spike, task. The title becomes the file name. --priority is 0-4 with 0 highest and 2 the default; --mode is auto or hitl. Every --depends-on, --required-by, and --related-to value is a full ID or a unique prefix. Every link is resolved and the cycle check runs before anything is written, so a bad link or a dependency cycle leaves the project untouched.",
}

func init() {
	createCmd.Run = runCreate
	Register(createCmd)
}

// createResult is the structured result of a successful create.
type createResult struct {
	ID    string `json:"id"`
	Path  string `json:"path"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

func runCreate(env Env, args []string) int {
	fs := NewFlagSet(env, createCmd)
	jf := &JSONFlag{}
	fs.Var(jf, "json", "print the result as JSON")
	priority := fs.Int("priority", 2, "priority 0-4, 0 highest")
	mode := fs.String("mode", item.ModeAuto, "auto or hitl")
	dependsOn := repeatString(fs, "depends-on", "ID this item depends on")
	requiredBy := repeatString(fs, "required-by", "ID that depends on this item")
	relatedTo := repeatString(fs, "related-to", "ID to relate symmetrically")

	if code, done := ParseFlags(env, fs, createCmd, args); done {
		return code
	}
	rest := fs.Args()
	if len(rest) != 2 {
		return UsageExit(env, jf.enabled, createUsage)
	}
	typ, title := rest[0], rest[1]

	root, code, ok := RootFor(env, jf.enabled)
	if !ok {
		return code
	}

	if *mode != item.ModeAuto && *mode != item.ModeHitl {
		return PrintError(env, jf.enabled, 1, fmt.Errorf("item: invalid mode %q", *mode))
	}
	if *priority < 0 || *priority > 4 {
		return PrintError(env, jf.enabled, 1, fmt.Errorf("item: invalid priority %d", *priority))
	}

	// Resolve every link target before writing anything so a bad prefix or a cycle leaves the project untouched.
	res, err := store.Scan(root)
	if err != nil {
		return PrintError(env, jf.enabled, 1, err)
	}
	deps, err := ResolveTargets(root, *dependsOn)
	if err != nil {
		return PrintError(env, jf.enabled, 1, err)
	}
	reqs, err := ResolveTargets(root, *requiredBy)
	if err != nil {
		return PrintError(env, jf.enabled, 1, err)
	}
	rels, err := ResolveTargets(root, *relatedTo)
	if err != nil {
		return PrintError(env, jf.enabled, 1, err)
	}

	id, err := item.NewID(root)
	if err != nil {
		return PrintError(env, jf.enabled, 1, err)
	}
	fm := item.Frontmatter{
		ID:        id,
		Title:     title,
		Type:      typ,
		Status:    item.StatusOpen,
		Priority:  *priority,
		Mode:      *mode,
		Created:   time.Now().UTC(),
		DependsOn: deps,
		RelatedTo: rels,
	}
	if err := fm.Validate(); err != nil {
		return PrintError(env, jf.enabled, 1, err)
	}

	// Cycle check over the graph including the new item and the requiredBy links the targets will gain.
	items := make([]item.Frontmatter, 0, len(res.Items)+1)
	for _, e := range res.Items {
		other := e.Frontmatter
		for _, req := range reqs {
			if other.ID == req {
				other.DependsOn = unique(append(other.DependsOn, id))
			}
		}
		items = append(items, other)
	}
	items = append(items, fm)
	if cycled, err := CycleIn(items, id); err != nil {
		return PrintError(env, jf.enabled, 1, err)
	} else if cycled {
		return PrintError(env, jf.enabled, 1, errors.New("create: link would create a dependency cycle"))
	}

	body, err := template.Body(typ, title)
	if err != nil {
		return PrintError(env, jf.enabled, 1, err)
	}
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return PrintError(env, jf.enabled, 1, fmt.Errorf("create: mkdir %s: %w", dir, err))
	}
	path := filepath.Join(dir, item.Slug(title)+".md")
	if err := store.WriteItem(store.Entry{ID: id, Path: path, Frontmatter: fm}, []byte(body)); err != nil {
		return PrintError(env, jf.enabled, 1, err)
	}

	// requiredBy is written into the target's authoritative dependsOn; the sync pass derives requiredBy from it. relatedTo is written on both sides.
	for _, e := range res.Items {
		other := e.Frontmatter
		changed := false
		for _, req := range reqs {
			if other.ID == req && !containsID(other.DependsOn, id) {
				other.DependsOn = append(other.DependsOn, id)
				changed = true
			}
		}
		if containsID(rels, other.ID) {
			other.RelatedTo = unique(append(other.RelatedTo, id))
			changed = true
		}
		if !changed {
			continue
		}
		if err := store.WriteItem(store.Entry{ID: other.ID, Path: e.Path, Frontmatter: other}, nil); err != nil {
			return PrintError(env, jf.enabled, 1, err)
		}
	}

	_, _ = sync.Run(root)
	out := createResult{ID: id, Path: path, Title: title, Type: typ}
	if jf.enabled {
		return PrintJSON(env, out)
	}
	fmt.Fprintf(env.Stdout, "%s %s\n", out.ID, out.Path)
	return 0
}

func repeatString(fs *flag.FlagSet, name, usage string) *[]string {
	var values []string
	fs.Var(&repeatValue{values: &values}, name, usage)
	return &values
}

// repeatValue collects repeated string flag values.
type repeatValue struct {
	values *[]string
}

func (r *repeatValue) String() string { return fmt.Sprint(*r.values) }
func (r *repeatValue) Set(s string) error {
	*r.values = append(*r.values, s)
	return nil
}

func unique(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func containsID(values []string, id string) bool {
	for _, v := range values {
		if v == id {
			return true
		}
	}
	return false
}
