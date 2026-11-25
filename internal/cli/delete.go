package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"papertrail/internal/item"
	"papertrail/internal/store"
	"papertrail/internal/sync"
)

const deleteUsage = "pt delete <id> [-y]"

// deleteCmd is the delete command; Run is wired in init so the initializer stays cycle-free.
var deleteCmd = Command{
	Name:    "delete",
	Usage:   deleteUsage,
	Summary: "Delete an item and remove links to it",
	Help:    "Without -y the command only reports which item and links would change. With -y it removes the item directory and strips the ID from every item that links to it.",
}

func init() {
	deleteCmd.Run = runDelete
	Register(deleteCmd)
}

// deleteResult is the structured result of delete. Without -y the command only reports; with -y it removes and repairs.
type deleteResult struct {
	ID          string   `json:"id"`
	Path        string   `json:"path"`
	WouldRemove bool     `json:"wouldRemove,omitempty"`
	Removed     bool     `json:"removed,omitempty"`
	References  []string `json:"references"`
}

func runDelete(env Env, args []string) int {
	fs := NewFlagSet(env, deleteCmd)
	jf := &JSONFlag{}
	fs.Var(jf, "json", "print the result as JSON")
	yes := fs.Bool("y", false, "perform the deletion")

	if code, done := ParseFlags(env, fs, deleteCmd, args); done {
		return code
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return UsageExit(env, jf.enabled, deleteUsage)
	}

	root, code, ok := RootFor(env, jf.enabled)
	if !ok {
		return code
	}
	res, err := store.Scan(root)
	if err != nil {
		return PrintError(env, jf.enabled, 1, err)
	}
	id, err := item.ResolvePrefix(idsOf(res), rest[0])
	if err != nil {
		return PrintError(env, jf.enabled, 1, err)
	}
	e, ok := EntryByID(res, id)
	if !ok {
		return PrintError(env, jf.enabled, 1, fmt.Errorf("item: unknown id %q", id))
	}

	// Items referencing the ID in any link field, in scan order.
	var refs []store.Entry
	for _, other := range res.Items {
		if other.Frontmatter.ID == id {
			continue
		}
		if references(other.Frontmatter, id) {
			refs = append(refs, other)
		}
	}
	refIDs := make([]string, len(refs))
	for i, r := range refs {
		refIDs[i] = r.Frontmatter.ID
	}

	if !*yes {
		out := deleteResult{ID: id, Path: e.Path, WouldRemove: true, References: refIDs}
		if jf.enabled {
			return PrintJSON(env, out)
		}
		fmt.Fprintf(env.Stdout, "would remove %s\n", e.Path)
		for _, r := range refs {
			fmt.Fprintf(env.Stdout, "would strip %s from %s\n", id, r.Frontmatter.ID)
		}
		return 0
	}

	dir := filepath.Dir(e.Path)
	if err := os.RemoveAll(dir); err != nil {
		return PrintError(env, jf.enabled, 1, fmt.Errorf("delete: remove %s: %w", dir, err))
	}
	for _, r := range refs {
		fm := r.Frontmatter
		fm.DependsOn = strip(fm.DependsOn, id)
		fm.RequiredBy = strip(fm.RequiredBy, id)
		fm.RelatedTo = strip(fm.RelatedTo, id)
		if err := store.WriteItem(store.Entry{ID: fm.ID, Path: r.Path, Frontmatter: fm}, nil); err != nil {
			return PrintError(env, jf.enabled, 1, err)
		}
	}

	_, _ = sync.Run(root)

	out := deleteResult{ID: id, Path: e.Path, Removed: true, References: refIDs}
	if jf.enabled {
		return PrintJSON(env, out)
	}
	fmt.Fprintf(env.Stdout, "removed %s\n", dir)
	return 0
}

// references reports whether the item links to id in any link field.
func references(fm item.Frontmatter, id string) bool {
	return containsID(fm.DependsOn, id) || containsID(fm.RequiredBy, id) || containsID(fm.RelatedTo, id)
}

// strip removes every occurrence of id from a link list.
func strip(values []string, id string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v != id {
			out = append(out, v)
		}
	}
	return out
}
