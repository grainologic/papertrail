package link

import (
	"errors"
	"fmt"

	"papertrail/internal/cli"
	"papertrail/internal/item"
	"papertrail/internal/store"
	"papertrail/internal/sync"
)

const linkUsage = "pt link <id> (--depends-on <id> | --required-by <id> | --related-to <id>)"
const unlinkUsage = "pt unlink <id> (--depends-on <id> | --required-by <id> | --related-to <id>)"

// linkCmd and unlinkCmd wire Run in init so the initializers stay cycle-free.
var linkCmd = cli.Command{
	Name:    "link",
	Usage:   linkUsage,
	Summary: "Add a dependency link between two items",
	Help:    "Exactly one of --depends-on, --required-by, or --related-to links the positional ID to the flag's ID value. Both IDs may be given as unique prefixes.",
}
var unlinkCmd = cli.Command{
	Name:    "unlink",
	Usage:   unlinkUsage,
	Summary: "Remove a dependency link between two items",
	Help:    "Exactly one of --depends-on, --required-by, or --related-to removes the link between the positional ID and the flag's ID value.",
}

func init() {
	linkCmd.Run = runLink
	unlinkCmd.Run = runUnlink
	cli.Register(linkCmd, unlinkCmd)
}

// linkResult is the structured result of a successful link or unlink.
type linkResult struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	Field   string `json:"field"`
	Target  string `json:"target"`
	Removed bool   `json:"removed"`
}

// runLink and runUnlink differ only in removal instead of addition.
func runLink(e cli.Env, args []string) int {
	return runLinkUnlink(e, args, linkCmd, false)
}

func runUnlink(e cli.Env, args []string) int {
	return runLinkUnlink(e, args, unlinkCmd, true)
}

func runLinkUnlink(e cli.Env, args []string, cmd cli.Command, unlink bool) int {
	fs := cli.NewFlagSet(e, cmd)
	jf := &cli.JSONFlag{}
	fs.Var(jf, "json", "print the result as JSON")
	dependsOn := fs.String("depends-on", "", "ID this item depends on")
	requiredBy := fs.String("required-by", "", "ID that depends on this item")
	relatedTo := fs.String("related-to", "", "ID to relate symmetrically")

	if code, done := cli.ParseFlags(e, fs, cmd, args); done {
		return code
	}
	rest := fs.Args()
	flags := []struct {
		key, value string
	}{
		{item.KeyDependsOn, *dependsOn},
		{item.KeyRequiredBy, *requiredBy},
		{item.KeyRelatedTo, *relatedTo},
	}
	// Exactly one link flag with one positional ID is required.
	var chosen, targetArg string
	for _, f := range flags {
		if f.value == "" {
			continue
		}
		if chosen != "" {
			return cli.UsageExit(e, cli.JSONEnabled(fs), cmd.Usage)
		}
		chosen, targetArg = f.key, f.value
	}
	if chosen == "" || len(rest) != 1 {
		return cli.UsageExit(e, cli.JSONEnabled(fs), cmd.Usage)
	}

	root, code, ok := cli.RootFor(e, cli.JSONEnabled(fs))
	if !ok {
		return code
	}
	ids, err := cli.ResolveTargets(root, rest)
	if err != nil {
		return cli.PrintError(e, cli.JSONEnabled(fs), 1, err)
	}
	targets, err := cli.ResolveTargets(root, []string{targetArg})
	if err != nil {
		return cli.PrintError(e, cli.JSONEnabled(fs), 1, err)
	}
	res, err := store.Scan(root)
	if err != nil {
		return cli.PrintError(e, cli.JSONEnabled(fs), 1, err)
	}
	id, target := ids[0], targets[0]

	// Plan every file mutation before writing so a cycle refusal or an unknown ID leaves both files untouched.
	type change struct {
		entry store.Entry
		fm    item.Frontmatter
	}
	var changes []change
	addChange := func(cid string, mutate func(*item.Frontmatter) bool) (int, bool) {
		entry, ok := cli.EntryByID(res, cid)
		if !ok {
			return cli.PrintError(e, cli.JSONEnabled(fs), 1, fmt.Errorf("item: unknown id %q", cid)), false
		}
		fm := entry.Frontmatter
		if !mutate(&fm) {
			return 0, false
		}
		changes = append(changes, change{entry: entry, fm: fm})
		return 0, true
	}

	switch chosen {
	case item.KeyDependsOn:
		if c, ok := addChange(id, func(fm *item.Frontmatter) bool {
			return mutateDepends(fm, target, unlink)
		}); !ok {
			return c
		}
	case item.KeyRequiredBy:
		// The requiredBy flag mirrors the field name, but requiredBy is derived from the authoritative dependsOn, so the edge target -> id is written into the target's dependsOn.
		if c, ok := addChange(target, func(fm *item.Frontmatter) bool {
			return mutateDepends(fm, id, unlink)
		}); !ok {
			return c
		}
	case item.KeyRelatedTo:
		if c, ok := addChange(id, func(fm *item.Frontmatter) bool {
			return mutateRelated(fm, target, unlink)
		}); !ok {
			return c
		}
		if c, ok := addChange(target, func(fm *item.Frontmatter) bool {
			return mutateRelated(fm, id, unlink)
		}); !ok {
			return c
		}
	}

	// Cycle check over the planned state; relatedTo edges cannot cycle.
	if !unlink && chosen != item.KeyRelatedTo {
		planned := make([]item.Frontmatter, 0, len(res.Items))
		for _, e2 := range res.Items {
			planned = append(planned, e2.Frontmatter)
		}
		for _, ch := range changes {
			for i := range planned {
				if planned[i].ID == ch.fm.ID {
					planned[i] = ch.fm
				}
			}
		}
		owner := id
		if chosen == item.KeyRequiredBy {
			owner = target
		}
		if cycled, err := cli.CycleIn(planned, owner); err != nil {
			return cli.PrintError(e, cli.JSONEnabled(fs), 1, err)
		} else if cycled {
			return cli.PrintError(e, cli.JSONEnabled(fs), 1, errors.New("link: link would create a dependency cycle"))
		}
	}

	primary, path := id, ""
	for _, ch := range changes {
		if err := store.WriteItem(store.Entry{ID: ch.entry.ID, Path: ch.entry.Path, Frontmatter: ch.fm}, nil); err != nil {
			return cli.PrintError(e, cli.JSONEnabled(fs), 1, err)
		}
		if ch.entry.ID == id {
			path = ch.entry.Path
		}
	}
	_, _ = sync.Run(root)

	out := linkResult{ID: primary, Path: path, Field: chosen, Target: target, Removed: unlink}
	if cli.JSONEnabled(fs) {
		return cli.PrintJSON(e, out)
	}
	verb := "linked"
	if unlink {
		verb = "unlinked"
	}
	fmt.Fprintf(e.Stdout, "%s %s %s %s\n", primary, verb, chosen, target)
	return 0
}

// mutateDepends adds or removes the dependency edge fm.DependsOn edge, reporting whether the file needs a rewrite. A repeated link is a no-op.
func mutateDepends(fm *item.Frontmatter, edge string, unlink bool) bool {
	if unlink {
		var changed bool
		fm.DependsOn, changed = removeID(fm.DependsOn, edge)
		return changed
	}
	var added bool
	fm.DependsOn, added = addUnique(fm.DependsOn, edge)
	return added
}

// mutateRelated is mutateDepends for the symmetric relatedTo field.
func mutateRelated(fm *item.Frontmatter, other string, unlink bool) bool {
	if unlink {
		var changed bool
		fm.RelatedTo, changed = removeID(fm.RelatedTo, other)
		return changed
	}
	var added bool
	fm.RelatedTo, added = addUnique(fm.RelatedTo, other)
	return added
}
