package cli

import (
	"strings"

	"papertrail/internal/config"
	"papertrail/internal/graph"
	"papertrail/internal/item"
	"papertrail/internal/store"
)

// RootFor locates the item root from the command's starting directory and reports the error exit when the configuration is missing.
func RootFor(env Env, asJSON bool) (string, int, bool) {
	cfg, err := config.Find(env.Dir)
	if err != nil {
		return "", PrintError(env, asJSON, 1, err), false
	}
	return cfg.Root, 0, true
}

// ResolveTargets resolves the ID prefixes in targets against the items under root, returning the full IDs in target order.
func ResolveTargets(root string, targets []string) ([]string, error) {
	res, err := store.Scan(root)
	if err != nil {
		return nil, err
	}
	known := make([]string, len(res.Items))
	for i, e := range res.Items {
		known[i] = e.Frontmatter.ID
	}
	out := make([]string, len(targets))
	for i, t := range targets {
		id, err := item.ResolvePrefix(known, t)
		if err != nil {
			return nil, err
		}
		out[i] = id
	}
	return out, nil
}

// EntryByID returns the scanned entry with the given full ID.
func EntryByID(res store.Result, id string) (store.Entry, bool) {
	for _, e := range res.Items {
		if e.Frontmatter.ID == id {
			return e, true
		}
	}
	return store.Entry{}, false
}

// CycleIn reports whether any dependency cycle contains id.
func CycleIn(items []item.Frontmatter, id string) (bool, error) {
	cycles := graph.Build(items).Cycles()
	for _, cycle := range cycles {
		for _, c := range cycle {
			if c == id {
				return true, nil
			}
		}
	}
	return false, nil
}

// statusOrMissing resolves an ID's status, "missing" when unknown.
func statusOrMissing(res store.Result, id string) string {
	if e, ok := EntryByID(res, id); ok {
		return e.Frontmatter.Status
	}
	return "missing"
}

// Dependency is one link of an item with the target's current status.
type Dependency struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Field  string `json:"field"`
}

// dependencies lists every link of an item with the target's status.
func dependencies(res store.Result, fm item.Frontmatter) []Dependency {
	var out []Dependency
	for _, field := range []struct {
		name string
		ids  []string
	}{
		{"dependsOn", fm.DependsOn},
		{"requiredBy", fm.RequiredBy},
		{"relatedTo", fm.RelatedTo},
	} {
		for _, id := range field.ids {
			out = append(out, Dependency{
				ID:     id,
				Status: statusOrMissing(res, id),
				Field:  field.name,
			})
		}
	}
	return out
}

// itemJSON is the shared JSON shape of an item row.
type itemJSON struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Type     string `json:"type"`
	Status   string `json:"status"`
	Priority int    `json:"priority"`
	Mode     string `json:"mode"`
}

func rowJSON(fm item.Frontmatter) itemJSON {
	return itemJSON{
		ID:       fm.ID,
		Title:    fm.Title,
		Type:     fm.Type,
		Status:   fm.Status,
		Priority: fm.Priority,
		Mode:     fm.Mode,
	}
}

// sortItems orders items for list and frontier: priority ascending, then created timestamp.
func sortItems(entries []store.Entry) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0; j-- {
			a, b := entries[j-1], entries[j]
			if lessRow(a.Frontmatter, b.Frontmatter) {
				break
			}
			entries[j-1], entries[j] = entries[j], entries[j-1]
		}
	}
}

func lessRow(a, b item.Frontmatter) bool {
	aClosed := a.Status == item.StatusClosed
	bClosed := b.Status == item.StatusClosed
	if aClosed != bClosed {
		return !aClosed
	}
	if a.Priority != b.Priority {
		return a.Priority < b.Priority
	}
	if !a.Created.Equal(b.Created) {
		return a.Created.Before(b.Created)
	}
	return strings.Compare(a.ID, b.ID) < 0
}

// idsOf returns every scanned item's full ID, for prefix resolution.
func idsOf(res store.Result) []string {
	out := make([]string, 0, len(res.Items))
	for _, e := range res.Items {
		out = append(out, e.Frontmatter.ID)
	}
	return out
}

// validTypes and validStatuses mirror the fixed item vocabulary for flag validation; the item package keeps the canonical sets unexported.
var validTypes = map[string]bool{
	item.TypeBug: true, item.TypeFeature: true, item.TypeTask: true, item.TypeChore: true,
	item.TypeFeedback: true, item.TypeResearch: true, item.TypeSpike: true, item.TypeDebate: true,
	item.TypeQuestion: true, item.TypeMilestone: true,
}

var validStatuses = map[string]bool{
	item.StatusOpen: true, item.StatusInProgress: true, item.StatusReview: true,
	item.StatusBlocked: true, item.StatusClosed: true,
}
