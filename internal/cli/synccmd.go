package cli

import (
	"fmt"

	"papertrail/internal/graph"
	"papertrail/internal/store"
	"papertrail/internal/sync"
)

const syncUsage = "pt sync"

// syncCmd is the sync command; Run is wired in init so the initializer stays cycle-free.
var syncCmd = Command{
	Name:    "sync",
	Usage:   syncUsage,
	Summary: "Recompute links and report project health",
}

func init() {
	syncCmd.Run = runSync
	Register(syncCmd)
}

// syncResult is the structured sync report: findings (errors and warnings) render as data and never cause a non-zero exit on their own.
type syncResult struct {
	Errors   []string        `json:"errors,omitempty"`
	Warnings []warningResult `json:"warnings,omitempty"`
}

type warningResult struct {
	ID               string `json:"id"`
	Status           string `json:"status"`
	Dependency       string `json:"dependency"`
	DependencyStatus string `json:"dependencyStatus"`
}

func runSync(env Env, args []string) int {
	fs := NewFlagSet(env, syncCmd)
	jf := &JSONFlag{}
	fs.Var(jf, "json", "print the result as JSON")

	if code, done := ParseFlags(env, fs, syncCmd, args); done {
		return code
	}
	if fs.NArg() != 0 {
		return UsageExit(env, jf.enabled, syncUsage)
	}

	root, code, ok := RootFor(env, jf.enabled)
	if !ok {
		return code
	}
	res, err := sync.Run(root)
	// Error findings return a non-nil error too; only an error without findings is an I/O failure. Findings render as the structured report below.
	if err != nil && len(res.Errors) == 0 {
		return PrintError(env, jf.enabled, 1, err)
	}

	out := syncResult{}
	for _, f := range res.Errors {
		out.Errors = append(out.Errors, f.Path+": "+f.Reason)
	}
	return finishSync(env, jf.enabled, root, out)
}

// finishSync renders the sync result after the pass and its warnings are known. It runs its own scan so warnings reflect the healed state.
func finishSync(env Env, asJSON bool, root string, out syncResult) int {
	scanned, err := store.Scan(root)
	if err != nil {
		return PrintError(env, asJSON, 1, err)
	}
	g := graph.Build(frontmatterOf(scanned))
	for _, w := range g.Warnings() {
		out.Warnings = append(out.Warnings, warningResult{
			ID:               w.ID,
			Status:           w.Status,
			Dependency:       w.Dependency,
			DependencyStatus: w.DependencyStatus,
		})
	}
	if asJSON {
		return PrintJSON(env, out)
	}
	for _, e := range out.Errors {
		fmt.Fprintf(env.Stderr, "pt sync: error: %s\n", e)
	}
	for _, w := range out.Warnings {
		fmt.Fprintf(env.Stdout, "warning: %s %s but dependency %s is %s\n",
			w.ID, w.Status, w.Dependency, w.DependencyStatus)
	}
	if len(out.Errors) > 0 {
		fmt.Fprintf(env.Stdout, "sync complete with %d error(s) reported\n", len(out.Errors))
	} else {
		fmt.Fprintln(env.Stdout, "sync ok")
	}
	return 0
}
