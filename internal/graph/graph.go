// Package graph builds the dependency graph over parsed item frontmatter: requiredBy derived from authoritative dependsOn, symmetric relatedTo, cycle and dangling-link detection, frontier membership, stale-status warnings, and insert rewiring. All functions are pure over item values.
package graph

import (
	"errors"
	"fmt"
	"sort"

	"papertrail/internal/item"
)

// Distinct graph failures.
var (
	ErrUnknownItem   = errors.New("graph: unknown item")
	ErrInvalidInsert = errors.New("graph: invalid insert")
)

// LinkField names the frontmatter field a link was declared in.
type LinkField string

const (
	FieldDependsOn  LinkField = "dependsOn"
	FieldRequiredBy LinkField = "requiredBy"
	FieldRelatedTo  LinkField = "relatedTo"
)

// Link is a declared edge to an ID that has no item.
type Link struct {
	From  string
	To    string
	Field LinkField
}

// Warning is a stale-status finding: the item's status implies work on its dependencies is done, but at least one dependency is unclosed.
type Warning struct {
	ID               string
	Status           string
	Dependency       string
	DependencyStatus string
}

// Graph is the dependency graph over items keyed by ID.
type Graph struct {
	Items map[string]item.Frontmatter
}

// Build indexes items by ID and derives requiredBy from dependsOn.
func Build(items []item.Frontmatter) *Graph {
	g := &Graph{Items: make(map[string]item.Frontmatter, len(items))}
	for _, fm := range items {
		g.Items[fm.ID] = fm
	}
	return g
}

// RequiredBy returns the sorted IDs that depend on id, derived from the dependsOn of every item.
func (g *Graph) RequiredBy(id string) []string {
	var req []string
	for _, dep := range sortedIDs(g.Items) {
		fm := g.Items[dep]
		if contains(fm.DependsOn, id) {
			req = append(req, dep)
		}
	}
	return req
}

// Symmetrize returns the item frontmatter with requiredBy rewritten from dependsOn and relatedTo made symmetric with the other side's relatedTo. Dependencies pointing at missing items are preserved; detection is the caller's job.
func (g *Graph) Symmetrize(fm item.Frontmatter) item.Frontmatter {
	fm.RequiredBy = g.RequiredBy(fm.ID)
	// relatedTo is the reverse declaration, not a transitive union: X is related to every ID that declares X in its relatedTo, and nothing more.
	related := append([]string{}, fm.RelatedTo...)
	for _, otherID := range sortedIDs(g.Items) {
		other := g.Items[otherID]
		if contains(other.RelatedTo, fm.ID) {
			related = append(related, otherID)
		}
	}
	fm.RelatedTo = uniqueSorted(related)
	fm.RelatedTo = removeValue(fm.RelatedTo, fm.ID)
	return fm
}

// DanglingLinks returns every declared link whose target ID has no item.
// Both directions of relatedTo are reported, mirroring the declarations.
func (g *Graph) DanglingLinks() []Link {
	var links []Link
	for _, id := range sortedIDs(g.Items) {
		fm := g.Items[id]
		for _, dep := range fm.DependsOn {
			if _, ok := g.Items[dep]; !ok {
				links = append(links, Link{From: id, To: dep, Field: FieldDependsOn})
			}
		}
		for _, rel := range fm.RelatedTo {
			if _, ok := g.Items[rel]; !ok {
				links = append(links, Link{From: id, To: rel, Field: FieldRelatedTo})
			}
		}
	}
	return links
}

// Cycles returns the dependency cycles as ID paths in deterministic order, each cycle reported once. A path follows dependency edges and closes at the repeated ID.
func (g *Graph) Cycles() [][]string {
	const (
		white = 0
		grey  = 1
		black = 2
	)

	color := make(map[string]int, len(g.Items))
	seen := map[string]bool{}
	var cycles [][]string
	var path []string

	var visit func(id string)
	visit = func(id string) {
		color[id] = grey
		path = append(path, id)
		for _, dep := range sortedDeps(g.Items[id].DependsOn) {
			if _, ok := g.Items[dep]; !ok {
				continue
			}
			switch color[dep] {
			case white:
				visit(dep)
			case grey:
				start := 0
				for i, p := range path {
					if p == dep {
						start = i
						break
					}
				}
				cycle := append(append([]string{}, path[start:]...), dep)
				key := canonicalCycle(cycle)
				if !seen[key] {
					seen[key] = true
					cycles = append(cycles, cycle)
				}
			}
		}
		path = path[:len(path)-1]
		color[id] = black
	}
	for _, id := range sortedIDs(g.Items) {
		if color[id] == white {
			visit(id)
		}
	}
	sort.Slice(cycles, func(i, j int) bool { return canonicalCycle(cycles[i]) < canonicalCycle(cycles[j]) })
	return cycles
}

// Frontier returns the sorted IDs whose status is open or in_progress and whose every dependency exists and is closed.
func (g *Graph) Frontier() []string {
	var ids []string
	for _, id := range sortedIDs(g.Items) {
		fm := g.Items[id]
		if fm.Status != item.StatusOpen && fm.Status != item.StatusInProgress {
			continue
		}
		if g.allClosed(fm) {
			ids = append(ids, id)
		}
	}
	return ids
}

// Warnings returns one warning per in_progress, review, or closed item that still has an unclosed dependency.
func (g *Graph) Warnings() []Warning {
	stale := map[string]bool{
		item.StatusInProgress: true,
		item.StatusReview:     true,
		item.StatusClosed:     true,
	}
	var warnings []Warning
	for _, id := range sortedIDs(g.Items) {
		fm := g.Items[id]
		if !stale[fm.Status] {
			continue
		}
		for _, dep := range sortedDeps(fm.DependsOn) {
			// Dangling dependencies are reported as errors, not warnings.
			depFM, ok := g.Items[dep]
			if !ok || depFM.Status == item.StatusClosed {
				continue
			}
			warnings = append(warnings, Warning{
				ID:               id,
				Status:           fm.Status,
				Dependency:       dep,
				DependencyStatus: depFM.Status,
			})
		}
	}
	return warnings
}

func (g *Graph) allClosed(fm item.Frontmatter) bool {
	for _, dep := range fm.DependsOn {
		depFM, ok := g.Items[dep]
		if !ok || depFM.Status != item.StatusClosed {
			return false
		}
	}
	return true
}

// ApplyInsert rewires items to place newID between a and b: newID depends on a, b depends on newID, and b's direct dependency on a is dropped. Items with no change are returned unmodified. a and b must be distinct existing items; newID must not collide with one.
func ApplyInsert(items []item.Frontmatter, a, b, newID string) ([]item.Frontmatter, error) {
	if a == b {
		return nil, fmt.Errorf("%w: A and B are the same item %q", ErrInvalidInsert, a)
	}
	index := map[string]int{}
	for i, fm := range items {
		index[fm.ID] = i
	}
	for _, id := range []string{a, b} {
		if _, ok := index[id]; !ok {
			return nil, fmt.Errorf("%w: %q", ErrUnknownItem, id)
		}
	}
	out := make([]item.Frontmatter, len(items))
	copy(out, items)
	bi := index[b]

	bfm := out[bi]
	dep := removeValue(bfm.DependsOn, a)
	bfm.DependsOn = uniqueSorted(append(dep, newID))
	out[bi] = bfm

	var nfm item.Frontmatter
	if ni, ok := index[newID]; ok {
		nfm = out[ni]
	} else {
		nfm = item.Frontmatter{ID: newID}
	}
	nfm.DependsOn = uniqueSorted(append(append([]string{}, nfm.DependsOn...), a))
	if ni, ok := index[newID]; ok {
		out[ni] = nfm
	} else {
		out = append(out, nfm)
	}
	return out, nil
}

func sortedIDs(items map[string]item.Frontmatter) []string {
	ids := make([]string, 0, len(items))
	for id := range items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func sortedDeps(deps []string) []string {
	return uniqueSorted(deps)
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func removeValue(values []string, drop string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v != drop {
			out = append(out, v)
		}
	}
	return out
}

// canonicalCycle rotates a cycle path so it starts at its smallest ID, for dedup and ordering.
func canonicalCycle(cycle []string) string {
	min := 0
	for i, id := range cycle {
		if id < cycle[min] {
			min = i
		}
	}
	rotated := append(append([]string{}, cycle[min:]...), cycle[:min]...)
	return fmt.Sprint(rotated)
}
