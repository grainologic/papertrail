package cli

import (
	"fmt"

	"papertrail/internal/item"
	"papertrail/internal/store"
)

const showUsage = "pt show <id>"

// showCmd is the show command; Run is wired in init so the initializer stays cycle-free.
var showCmd = Command{
	Name:    "show",
	Usage:   showUsage,
	Summary: "Show one item and its links",
}

func init() {
	showCmd.Run = runShow
	Register(showCmd)
}

// showResult is the structured result of show: the item and every link with the target's status.
type showResult struct {
	itemJSON
	Path         string       `json:"path"`
	Created      string       `json:"created"`
	Dependencies []Dependency `json:"dependencies"`
}

func runShow(env Env, args []string) int {
	fs := NewFlagSet(env, showCmd)
	jf := &JSONFlag{}
	fs.Var(jf, "json", "print the result as JSON")

	if code, done := ParseFlags(env, fs, showCmd, args); done {
		return code
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return UsageExit(env, jf.enabled, showUsage)
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
	out := showResult{
		itemJSON:     rowJSON(e.Frontmatter),
		Path:         e.Path,
		Created:      e.Frontmatter.Created.Format("2006-01-02T15:04:05Z07:00"),
		Dependencies: dependencies(res, e.Frontmatter),
	}
	if jf.enabled {
		return PrintJSON(env, out)
	}
	fm := e.Frontmatter
	fmt.Fprintf(env.Stdout, "id: %s\ntitle: %s\ntype: %s\nstatus: %s\npriority: %d\nmode: %s\ncreated: %s\npath: %s\n",
		fm.ID, fm.Title, fm.Type, fm.Status, fm.Priority, fm.Mode, out.Created, e.Path)
	for _, d := range out.Dependencies {
		fmt.Fprintf(env.Stdout, "%s %s %s\n", d.Field, d.ID, d.Status)
	}
	return 0
}
