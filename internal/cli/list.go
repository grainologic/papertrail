package cli

import (
	"fmt"

	"papertrail/internal/store"
)

const listUsage = "pt list [--type <type>] [--status <status>]"

// listCmd is the list command; Run is wired in init so the initializer stays cycle-free.
var listCmd = Command{
	Name:    "list",
	Usage:   listUsage,
	Summary: "List items, filtered by type and status when given",
	Help:    "--type accepts one of bug, chore, debate, feature, feedback, milestone, question, research, spike, task. --status accepts one of open, in_progress, review, blocked, closed.",
}

func init() {
	listCmd.Run = runList
	Register(listCmd)
}

func runList(env Env, args []string) int {
	fs := NewFlagSet(env, listCmd)
	jf := &JSONFlag{}
	fs.Var(jf, "json", "print the result as JSON")
	typ := fs.String("type", "", "filter by item type")
	status := fs.String("status", "", "filter by status")

	if code, done := ParseFlags(env, fs, listCmd, args); done {
		return code
	}
	if *typ != "" && !validTypes[*typ] {
		return PrintError(env, jf.enabled, 1, fmt.Errorf("item: invalid type %q", *typ))
	}
	if *status != "" && !validStatuses[*status] {
		return PrintError(env, jf.enabled, 1, fmt.Errorf("item: invalid status %q", *status))
	}

	root, code, ok := RootFor(env, jf.enabled)
	if !ok {
		return code
	}
	res, err := store.Scan(root)
	if err != nil {
		return PrintError(env, jf.enabled, 1, err)
	}
	var rows []store.Entry
	for _, e := range res.Items {
		if *typ != "" && e.Frontmatter.Type != *typ {
			continue
		}
		if *status != "" && e.Frontmatter.Status != *status {
			continue
		}
		rows = append(rows, e)
	}
	sortItems(rows)
	if jf.enabled {
		out := make([]itemJSON, len(rows))
		for i, e := range rows {
			out[i] = rowJSON(e.Frontmatter)
		}
		return PrintJSON(env, out)
	}
	for _, e := range rows {
		fm := e.Frontmatter
		fmt.Fprintf(env.Stdout, "%s  p%d  %-10s %-12s %s\n", fm.ID, fm.Priority, fm.Type, fm.Status, fm.Title)
	}
	return 0
}
