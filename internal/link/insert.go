package link

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"papertrail/internal/cli"
	"papertrail/internal/graph"
	"papertrail/internal/item"
	"papertrail/internal/store"
	"papertrail/internal/sync"
	"papertrail/internal/template"
)

const insertUsage = "pt insert <type> <title> --after <id> --before <id> [--priority 0-4] [--mode auto|hitl]"

// insertCmd is the insert command; Run is wired in init so the initializer stays cycle-free.
var insertCmd = cli.Command{
	Name:    "insert",
	Usage:   insertUsage,
	Summary: "Insert an item between two items in a dependency chain",
	Help:    "Both --after and --before are required full IDs or unique prefixes. The new item depends on --after, --before depends on the new item, and --before's direct edge to --after is dropped. The chain rule and the cycle refusal share one implementation.",
}

func init() {
	insertCmd.Run = runInsert
	cli.Register(insertCmd)
}

// insertResult is the structured result of a successful insert.
type insertResult struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	After  string `json:"after"`
	Before string `json:"before"`
}

func runInsert(e cli.Env, args []string) int {
	fs := cli.NewFlagSet(e, insertCmd)
	jf := &cli.JSONFlag{}
	fs.Var(jf, "json", "print the result as JSON")
	after := fs.String("after", "", "ID the new item depends on")
	before := fs.String("before", "", "ID that depends on the new item")
	priority := fs.Int("priority", 2, "priority 0-4, 0 highest")
	mode := fs.String("mode", item.ModeAuto, "auto or hitl")

	if code, done := cli.ParseFlags(e, fs, insertCmd, args); done {
		return code
	}
	rest := fs.Args()
	if len(rest) != 2 || *after == "" || *before == "" {
		return cli.UsageExit(e, cli.JSONEnabled(fs), insertCmd.Usage)
	}
	typ, title := rest[0], rest[1]

	root, code, ok := cli.RootFor(e, cli.JSONEnabled(fs))
	if !ok {
		return code
	}
	if *mode != item.ModeAuto && *mode != item.ModeHitl {
		return cli.PrintError(e, cli.JSONEnabled(fs), 1, fmt.Errorf("item: invalid mode %q", *mode))
	}
	if *priority < 0 || *priority > 4 {
		return cli.PrintError(e, cli.JSONEnabled(fs), 1, fmt.Errorf("item: invalid priority %d", *priority))
	}

	res, err := store.Scan(root)
	if err != nil {
		return cli.PrintError(e, cli.JSONEnabled(fs), 1, err)
	}
	ids, err := cli.ResolveTargets(root, []string{*after, *before})
	if err != nil {
		return cli.PrintError(e, cli.JSONEnabled(fs), 1, err)
	}
	a, b := ids[0], ids[1]

	newID, err := item.NewID(root)
	if err != nil {
		return cli.PrintError(e, cli.JSONEnabled(fs), 1, err)
	}
	nfm := item.Frontmatter{
		ID:        newID,
		Title:     title,
		Type:      typ,
		Status:    item.StatusOpen,
		Priority:  *priority,
		Mode:      *mode,
		Created:   time.Now().UTC(),
		DependsOn: []string{a},
	}

	// Rewire the whole planned graph through graph.ApplyInsert so the chain rule (N depends on A, B depends on N, B's direct A edge dropped) and the cycle refusal share one implementation.
	items := make([]item.Frontmatter, 0, len(res.Items)+1)
	for _, e2 := range res.Items {
		items = append(items, e2.Frontmatter)
	}
	items = append(items, nfm)
	planned, err := graph.ApplyInsert(items, a, b, newID)
	if err != nil {
		return cli.PrintError(e, cli.JSONEnabled(fs), 1, fmt.Errorf("insert: %w", err))
	}
	if len(graph.Build(planned).Cycles()) > 0 {
		return cli.PrintError(e, cli.JSONEnabled(fs), 1, errors.New("insert: link would create a dependency cycle"))
	}

	// Nothing has been written yet; validate and write all three files.
	if err := nfm.Validate(); err != nil {
		return cli.PrintError(e, cli.JSONEnabled(fs), 1, err)
	}
	var bfm item.Frontmatter
	bFound := false
	for _, fm := range planned {
		if fm.ID == b {
			bfm, bFound = fm, true
		}
	}
	if !bFound {
		return cli.PrintError(e, cli.JSONEnabled(fs), 1, fmt.Errorf("insert: %w: %q", graph.ErrUnknownItem, b))
	}
	if err := bfm.Validate(); err != nil {
		return cli.PrintError(e, cli.JSONEnabled(fs), 1, err)
	}

	body, err := template.Body(typ, title)
	if err != nil {
		return cli.PrintError(e, cli.JSONEnabled(fs), 1, err)
	}
	dir := filepath.Join(root, newID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return cli.PrintError(e, cli.JSONEnabled(fs), 1, fmt.Errorf("insert: mkdir %s: %w", dir, err))
	}
	path := filepath.Join(dir, item.Slug(title)+".md")
	if err := store.WriteItem(store.Entry{ID: newID, Path: path, Frontmatter: nfm}, []byte(body)); err != nil {
		return cli.PrintError(e, cli.JSONEnabled(fs), 1, err)
	}
	bEntry, ok := cli.EntryByID(res, b)
	if !ok {
		return cli.PrintError(e, cli.JSONEnabled(fs), 1, fmt.Errorf("insert: %w: %q", graph.ErrUnknownItem, b))
	}
	if err := store.WriteItem(store.Entry{ID: bEntry.ID, Path: bEntry.Path, Frontmatter: bfm}, nil); err != nil {
		return cli.PrintError(e, cli.JSONEnabled(fs), 1, err)
	}

	_, _ = sync.Run(root)

	out := insertResult{ID: newID, Path: path, After: a, Before: b}
	if cli.JSONEnabled(fs) {
		return cli.PrintJSON(e, out)
	}
	fmt.Fprintf(e.Stdout, "%s %s after %s before %s\n", newID, path, a, b)
	return 0
}
