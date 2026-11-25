package cli

import (
	"fmt"

	"papertrail/internal/item"
	"papertrail/internal/store"
	"papertrail/internal/sync"
)

const frontierUsage = "pt frontier"

// frontierCmd is the frontier command; Run is wired in init so the initializer stays cycle-free.
var frontierCmd = Command{
	Name:    "frontier",
	Usage:   frontierUsage,
	Summary: "List blocker-free, in-progress items in priority order",
}

func init() {
	frontierCmd.Run = runFrontier
	Register(frontierCmd)
}

func runFrontier(env Env, args []string) int {
	fs := NewFlagSet(env, frontierCmd)
	jf := &JSONFlag{}
	fs.Var(jf, "json", "print the result as JSON")

	if code, done := ParseFlags(env, fs, frontierCmd, args); done {
		return code
	}
	if fs.NArg() != 0 {
		return UsageExit(env, jf.enabled, frontierUsage)
	}

	root, code, ok := RootFor(env, jf.enabled)
	if !ok {
		return code
	}
	// Sync first so healed links update frontier membership. Error findings still exit 0 with the healthy frontier; only an I/O failure exits 1.
	syncRes, err := sync.Run(root)
	if err != nil && len(syncRes.Errors) == 0 {
		return PrintError(env, jf.enabled, 1, err)
	}

	res, err := store.Scan(root)
	if err != nil {
		return PrintError(env, jf.enabled, 1, err)
	}
	out := make([]itemRow, 0, len(syncRes.Frontier))
	for _, fm := range syncRes.Frontier {
		e, ok := EntryByID(res, fm.ID)
		if !ok {
			continue
		}
		out = append(out, itemRow{
			itemJSON: rowJSON(e.Frontmatter),
			Path:     e.Path,
		})
	}
	if jf.enabled {
		return PrintJSON(env, out)
	}
	for _, r := range out {
		fmt.Fprintf(env.Stdout, "%s  p%d  %-10s %s\n", r.ID, r.Priority, r.Type, r.Title)
	}
	return 0
}

// itemRow extends the shared item row with the file path.
type itemRow struct {
	itemJSON
	Path string `json:"path"`
}

// frontmatterOf extracts the frontmatter values of a scan result.
func frontmatterOf(res store.Result) []item.Frontmatter {
	out := make([]item.Frontmatter, 0, len(res.Items))
	for _, e := range res.Items {
		out = append(out, e.Frontmatter)
	}
	return out
}
