package cli

import (
	"fmt"
	"strconv"
	"strings"

	"papertrail/internal/item"
	"papertrail/internal/store"
	"papertrail/internal/sync"
)

const updateUsage = "pt update <id> [--title <title>] [--type <type>] [--status <status>] [--priority 0-4] [--mode auto|hitl]"

// updateCmd is the update command; Run is wired in init so the initializer stays cycle-free.
var updateCmd = Command{
	Name:    "update",
	Usage:   updateUsage,
	Summary: "Update an item's fields",
	Help:    "--type and --status accept the item vocabulary, --priority is 0-4, and --mode is auto or hitl. --title renames the item file.",
}

func init() {
	updateCmd.Run = runUpdate
	Register(updateCmd)
}

// updateResult is the structured result of a successful update.
type updateResult struct {
	ID      string   `json:"id"`
	Path    string   `json:"path"`
	Updated []string `json:"updated"`
}

func runUpdate(env Env, args []string) int {
	fs := NewFlagSet(env, updateCmd)
	jf := &JSONFlag{}
	fs.Var(jf, "json", "print the result as JSON")
	title := fs.String("title", "", "new title (renames the file)")
	typ := fs.String("type", "", "new type")
	status := fs.String("status", "", "new status")
	priority := fs.String("priority", "", "new priority 0-4")
	mode := fs.String("mode", "", "auto or hitl")

	if code, done := ParseFlags(env, fs, updateCmd, args); done {
		return code
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return UsageExit(env, jf.enabled, updateUsage)
	}
	changes := map[string]string{
		"title":    *title,
		"type":     *typ,
		"status":   *status,
		"priority": *priority,
		"mode":     *mode,
	}
	var changed []string
	for _, name := range []string{"title", "type", "status", "priority", "mode"} {
		if changes[name] != "" {
			changed = append(changed, name)
		}
	}
	if len(changed) == 0 {
		return UsageExit(env, jf.enabled, updateUsage)
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
	fm := e.Frontmatter

	if changes["priority"] != "" {
		newPriority, err := strconv.Atoi(changes["priority"])
		if err != nil {
			return PrintError(env, jf.enabled, 1, fmt.Errorf("item: invalid priority %q", changes["priority"]))
		}
		fm.Priority = newPriority
	}
	if changes["title"] != "" {
		fm.Title = changes["title"]
	}
	if changes["type"] != "" {
		fm.Type = changes["type"]
	}
	if changes["status"] != "" {
		fm.Status = changes["status"]
	}
	if changes["mode"] != "" {
		fm.Mode = changes["mode"]
	}
	if err := fm.Validate(); err != nil {
		return PrintError(env, jf.enabled, 1, err)
	}
	if err := store.WriteItem(store.Entry{ID: e.ID, Path: e.Path, Frontmatter: fm}, nil); err != nil {
		return PrintError(env, jf.enabled, 1, err)
	}

	_, _ = sync.Run(root)

	if changes["title"] != "" {
		if renamed, ok := EntryByID(scanOrEmpty(root), id); ok {
			e.Path = renamed.Path
		}
	}

	out := updateResult{ID: id, Path: e.Path, Updated: changed}
	if jf.enabled {
		return PrintJSON(env, out)
	}
	fmt.Fprintf(env.Stdout, "%s updated: %s\n", id, strings.Join(changed, ", "))
	return 0
}

// scanOrEmpty scans root and returns an empty result when the scan cannot run, leaving the pre-rename path reported.
func scanOrEmpty(root string) store.Result {
	res, err := store.Scan(root)
	if err != nil {
		return store.Result{}
	}
	return res
}
