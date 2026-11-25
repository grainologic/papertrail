// Package batch implements the pt batch command: create many items from one JSON array read from a file or stdin. Every link resolves to an existing item ID or the key of another entry in the array. The whole array is validated before anything is written, and the input file is deleted only after every item is written.
package batch

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"papertrail/internal/cli"
	"papertrail/internal/config"
	"papertrail/internal/graph"
	"papertrail/internal/item"
	"papertrail/internal/store"
	"papertrail/internal/sync"
	"papertrail/internal/template"
)

const usage = "pt batch <file|-> [--dry-run] [--json]"

// batchCmd is the batch command; Run is wired in init so the initializer stays cycle-free.
var batchCmd = cli.Command{
	Name:    "batch",
	Usage:   usage,
	Summary: "Create many items from a JSON array",
	Help:    "The argument is a JSON file path or - for stdin. Each array entry has key, type, title and optional priority, mode, dependsOn, requiredBy, relatedTo. Links resolve by full ID, a unique prefix, or the key of another entry in the array. The whole array is validated before anything is written, and the input file is deleted only after every item is written.",
}

func init() {
	batchCmd.Run = run
	cli.Register(batchCmd)
}

// input is one entry of the batch JSON array.
type input struct {
	Key        string   `json:"key"`
	Type       string   `json:"type"`
	Title      string   `json:"title"`
	Priority   *int     `json:"priority"`
	Mode       string   `json:"mode"`
	DependsOn  []string `json:"dependsOn"`
	RequiredBy []string `json:"requiredBy"`
	RelatedTo  []string `json:"relatedTo"`
}

// planned is one validated entry with its generated ID and resolved links.
type planned struct {
	Key        string   `json:"key"`
	Type       string   `json:"type"`
	Title      string   `json:"title"`
	ID         string   `json:"id"`
	Priority   int      `json:"priority"`
	Mode       string   `json:"mode"`
	DependsOn  []string `json:"dependsOn"`
	RequiredBy []string `json:"requiredBy"`
	RelatedTo  []string `json:"relatedTo"`
}

func run(env cli.Env, args []string) int {
	fs := cli.NewFlagSet(env, batchCmd)
	jf := &cli.JSONFlag{}
	fs.Var(jf, "json", "print the result as JSON")
	dryRun := fs.Bool("dry-run", false, "validate and print the plan without writing")
	if code, done := cli.ParseFlags(env, fs, batchCmd, args); done {
		return code
	}
	asJSON := cli.JSONEnabled(fs)
	rest := fs.Args()
	if len(rest) != 1 {
		return cli.UsageExit(env, asJSON, usage)
	}
	source := rest[0]

	data, err := readSource(source, env.Stdin)
	if err != nil {
		return cli.PrintError(env, asJSON, 1, err)
	}

	var entries []input
	if err := json.Unmarshal(data, &entries); err != nil {
		return cli.PrintError(env, asJSON, 1, fmt.Errorf("batch: invalid JSON: %w", err))
	}

	cfg, err := config.Find(env.Dir)
	if err != nil {
		return cli.PrintError(env, asJSON, 1, err)
	}

	plan, updates, err := planBatch(cfg.Root, entries)
	if err != nil {
		return cli.PrintError(env, asJSON, 1, err)
	}

	if *dryRun {
		return printPlan(env, asJSON, plan)
	}

	created, err := write(cfg.Root, plan, updates)
	if err != nil {
		return cli.PrintError(env, asJSON, 1, err)
	}

	_, _ = sync.Run(cfg.Root)

	if source != "-" {
		if err := os.Remove(source); err != nil {
			return cli.PrintError(env, asJSON, 1, fmt.Errorf("batch: remove %s: %w", source, err))
		}
	}

	return printResult(env, asJSON, cfg.Root, created)
}

func readSource(source string, stdin io.Reader) ([]byte, error) {
	if source == "-" {
		return io.ReadAll(stdin)
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return nil, fmt.Errorf("batch: read %s: %w", source, err)
	}
	return data, nil
}

// planBatch validates every entry and resolves all links against the scanned store and the array's own keys. It returns the planned items and the frontmatter updates existing items will receive. Nothing is written.
func planBatch(root string, entries []input) ([]planned, map[string]item.Frontmatter, error) {
	res, err := store.Scan(root)
	if err != nil {
		return nil, nil, err
	}
	known := make([]string, 0, len(res.Items))
	for _, e := range res.Items {
		known = append(known, e.Frontmatter.ID)
	}

	keyIndex := make(map[string]int)
	for i, in := range entries {
		if in.Key == "" {
			continue
		}
		if _, dup := keyIndex[in.Key]; dup {
			return nil, nil, fmt.Errorf("batch: duplicate key %q", in.Key)
		}
		keyIndex[in.Key] = i
	}

	updates := make(map[string]item.Frontmatter)
	now := time.Now().UTC()
	reserved := make(map[string]bool, len(entries))
	plan := make([]planned, 0, len(entries))
	for i, in := range entries {
		p := planned{
			Key:        in.Key,
			Type:       in.Type,
			Title:      in.Title,
			DependsOn:  in.DependsOn,
			RequiredBy: in.RequiredBy,
			RelatedTo:  in.RelatedTo,
		}
		if in.Type == "" {
			return nil, nil, fmt.Errorf("batch: entry %d: type is required", i)
		}
		if in.Title == "" {
			return nil, nil, fmt.Errorf("batch: entry %d: title is required", i)
		}
		p.Priority = 2
		if in.Priority != nil {
			if *in.Priority < 0 || *in.Priority > 4 {
				return nil, nil, fmt.Errorf("batch: entry %q: invalid priority %d", in.Key, *in.Priority)
			}
			p.Priority = *in.Priority
		}
		p.Mode = item.ModeAuto
		if in.Mode != "" {
			if in.Mode != item.ModeAuto && in.Mode != item.ModeHitl {
				return nil, nil, fmt.Errorf("batch: entry %q: invalid mode %q", in.Key, in.Mode)
			}
			p.Mode = in.Mode
		}
		id, err := uniqueID(root, reserved)
		if err != nil {
			return nil, nil, err
		}
		reserved[id] = true
		p.ID = id
		plan = append(plan, p)
	}

	for i := range plan {
		deps, err := resolveAll(keyIndex, plan, known, plan[i].DependsOn, plan[i].Key)
		if err != nil {
			return nil, nil, err
		}
		reqs, err := resolveAll(keyIndex, plan, known, plan[i].RequiredBy, plan[i].Key)
		if err != nil {
			return nil, nil, err
		}
		rels, err := resolveAll(keyIndex, plan, known, plan[i].RelatedTo, plan[i].Key)
		if err != nil {
			return nil, nil, err
		}
		plan[i].DependsOn, plan[i].RequiredBy, plan[i].RelatedTo = deps, reqs, rels
	}

	// Fold all edges into the shared fms slice first so multiple entries linking the same existing item accumulate rather than overwrite. requiredBy becomes dependsOn on the target so cycle check sees one field.
	fms := make([]item.Frontmatter, 0, len(res.Items)+len(plan))
	for _, e := range res.Items {
		fms = append(fms, e.Frontmatter)
	}
	byID := make(map[string]int, len(plan))
	for i := range plan {
		byID[plan[i].ID] = len(fms)
		fms = append(fms, item.Frontmatter{
			ID:        plan[i].ID,
			Title:     plan[i].Title,
			Type:      plan[i].Type,
			Status:    item.StatusOpen,
			Priority:  plan[i].Priority,
			Mode:      plan[i].Mode,
			Created:   now,
			DependsOn: unique(plan[i].DependsOn),
			RelatedTo: unique(plan[i].RelatedTo),
		})
	}
	get := func(id string) *item.Frontmatter {
		if idx, ok := byID[id]; ok {
			return &fms[idx]
		}
		for j := range fms {
			if fms[j].ID == id {
				return &fms[j]
			}
		}
		return nil
	}
	// Existing items that receive an edge become updates, recorded by ID so the merged frontmatter is read back out of fms exactly once.
	touched := make(map[string]bool)
	addEdge := func(target string, mutate func(*item.Frontmatter)) bool {
		fm := get(target)
		if fm == nil {
			return false
		}
		mutate(fm)
		if _, internal := byID[target]; !internal {
			touched[target] = true
		}
		return true
	}
	for i := range plan {
		for _, id := range plan[i].RequiredBy {
			if !addEdge(id, func(fm *item.Frontmatter) {
				fm.DependsOn = unique(append(fm.DependsOn, plan[i].ID))
			}) {
				return nil, nil, fmt.Errorf("batch: entry %q: requiredBy target %s missing", plan[i].Key, id)
			}
		}
		for _, id := range plan[i].RelatedTo {
			if !addEdge(id, func(fm *item.Frontmatter) {
				fm.RelatedTo = unique(append(fm.RelatedTo, plan[i].ID))
			}) {
				return nil, nil, fmt.Errorf("batch: entry %q: relatedTo target %s missing", plan[i].Key, id)
			}
		}
	}
	for id := range touched {
		updates[id] = *get(id)
	}

	// Validate the whole combined graph before anything is written.
	g := graph.Build(fms)
	for _, fm := range fms {
		if err := fm.Validate(); err != nil {
			return nil, nil, fmt.Errorf("batch: %s: %w", fm.ID, err)
		}
	}
	if cycles := g.Cycles(); len(cycles) > 0 {
		return nil, nil, fmt.Errorf("batch: link would create a dependency cycle: %v", cycles[0])
	}

	// Copy the final dependsOn, including edges derived from requiredBy, back onto the plan so writing stores authoritative fields.
	for i := range plan {
		plan[i].DependsOn = fms[byID[plan[i].ID]].DependsOn
	}
	return plan, updates, nil
}

// resolveAll resolves every link of one field to a full item ID.
func resolveAll(keyIndex map[string]int, plan []planned, known []string, links []string, key string) ([]string, error) {
	out := make([]string, 0, len(links))
	for _, link := range links {
		if idx, ok := keyIndex[link]; ok {
			out = append(out, plan[idx].ID)
			continue
		}
		id, err := item.ResolvePrefix(known, link)
		if err != nil {
			return nil, fmt.Errorf("batch: entry %q: link %q: %w", key, link, err)
		}
		out = append(out, id)
	}
	return out, nil
}

// uniqueID generates an ID that collides with neither an existing folder nor another entry in the same batch.
func uniqueID(root string, reserved map[string]bool) (string, error) {
	for {
		id, err := item.NewID(root)
		if err != nil {
			return "", err
		}
		if !reserved[id] {
			return id, nil
		}
	}
}

func write(root string, plan []planned, updates map[string]item.Frontmatter) ([]planned, error) {
	for _, p := range plan {
		dir := filepath.Join(root, p.ID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("batch: mkdir %s: %w", dir, err)
		}
		body, err := template.Body(p.Type, p.Title)
		if err != nil {
			return nil, err
		}
		fm := item.Frontmatter{
			ID:        p.ID,
			Title:     p.Title,
			Type:      p.Type,
			Status:    item.StatusOpen,
			Priority:  p.Priority,
			Mode:      p.Mode,
			Created:   time.Now().UTC(),
			DependsOn: p.DependsOn,
			RelatedTo: p.RelatedTo,
		}
		path := filepath.Join(dir, item.Slug(p.Title)+".md")
		if err := store.WriteItem(store.Entry{ID: p.ID, Path: path, Frontmatter: fm}, []byte(body)); err != nil {
			return nil, fmt.Errorf("batch: write %s: %w", path, err)
		}
	}
	for id, fm := range updates {
		e, err := store.ReadItem(root, id)
		if err != nil {
			return nil, err
		}
		raw, err := os.ReadFile(e.Path)
		if err != nil {
			return nil, fmt.Errorf("batch: read %s: %w", e.Path, err)
		}
		parsed, err := item.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("batch: %s: %w", e.Path, err)
		}
		if err := store.WriteItem(store.Entry{ID: id, Path: e.Path, Frontmatter: fm}, parsed.Body); err != nil {
			return nil, fmt.Errorf("batch: write %s: %w", e.Path, err)
		}
	}
	return plan, nil
}

func printPlan(env cli.Env, asJSON bool, plan []planned) int {
	if asJSON {
		return cli.PrintJSON(env, plan)
	}
	for _, p := range plan {
		fmt.Fprintf(env.Stdout, "create %s %q -> %s (dependsOn %v, requiredBy %v, relatedTo %v)\n",
			p.Type, p.Title, p.ID, p.DependsOn, p.RequiredBy, p.RelatedTo)
	}
	return 0
}

func printResult(env cli.Env, asJSON bool, root string, created []planned) int {
	if asJSON {
		rows := make([]map[string]string, 0, len(created))
		for _, p := range created {
			rows = append(rows, map[string]string{
				"key":  p.Key,
				"id":   p.ID,
				"path": filepath.Join(root, p.ID, item.Slug(p.Title)+".md"),
			})
		}
		return cli.PrintJSON(env, rows)
	}
	for _, p := range created {
		fmt.Fprintf(env.Stdout, "%s %s\n", p.ID, filepath.Join(root, p.ID, item.Slug(p.Title)+".md"))
	}
	return 0
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
