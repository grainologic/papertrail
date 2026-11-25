package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"papertrail/internal/graph"
	"papertrail/internal/item"
	"papertrail/internal/store"
)

// indexRow is one rendered table row.
type indexRow struct {
	fm   item.Frontmatter
	link string
}

// renderIndex writes the item table: unclosed rows first, closed rows last, each group sorted by priority then created.
func renderIndex(root string, res store.Result, excluded map[string]bool, paths map[string]string) string {
	rows := indexRows(root, res, excluded, paths, func(item.Frontmatter) bool { return true })
	return renderTable("# INDEX", rows)
}

// frontierItems returns the unblocked, non-excluded items in index order.
// FRONTIER.md and pt frontier share it so the two never disagree.
func frontierItems(g *graph.Graph, excluded map[string]bool) []item.Frontmatter {
	var out []item.Frontmatter
	for _, id := range g.Frontier() {
		if !excluded[id] {
			out = append(out, g.Items[id])
		}
	}
	sort.Slice(out, func(i, j int) bool { return lessRow(out[i], out[j]) })
	return out
}

// renderFrontier writes the frontier items plus the same columns and ordering.
func renderFrontier(root string, res store.Result, g *graph.Graph, excluded map[string]bool, paths map[string]string) string {
	frontier := map[string]bool{}
	for _, fm := range frontierItems(g, excluded) {
		frontier[fm.ID] = true
	}
	rows := indexRows(root, res, excluded, paths, func(fm item.Frontmatter) bool { return frontier[fm.ID] })
	return renderTable("# FRONTIER", rows)
}

// indexRows builds every table row, filtered by keep, in the stable order the index uses.
func indexRows(root string, res store.Result, excluded map[string]bool, paths map[string]string, keep func(item.Frontmatter) bool) []indexRow {
	var rows []indexRow
	for _, e := range res.Items {
		fm := e.Frontmatter
		if excluded[fm.ID] || !keep(fm) {
			continue
		}
		rel, err := filepath.Rel(root, paths[fm.ID])
		if err != nil {
			rel = e.Path
		}
		rows = append(rows, indexRow{fm: fm, link: filepath.ToSlash(rel)})
	}
	sort.Slice(rows, func(i, j int) bool { return lessRow(rows[i].fm, rows[j].fm) })
	return rows
}

// lessRow orders rows: unclosed before closed, then priority, created, id.
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
	return a.ID < b.ID
}

// renderTable renders a heading and a markdown table for the given rows.
func renderTable(heading string, rows []indexRow) string {
	var b strings.Builder
	b.WriteString(heading)
	b.WriteString("\n\n")
	b.WriteString("| id | title | type | status | priority | mode | dependsOn |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "| [%s](%s) | %s | %s | %s | %d | %s | %s |\n",
			r.fm.ID, r.link, cell(r.fm.Title), cell(r.fm.Type), cell(r.fm.Status),
			r.fm.Priority, cell(r.fm.Mode), cell(strings.Join(r.fm.DependsOn, ", ")))
	}
	return b.String()
}

// renderHealth lists error findings first, then warnings, each with its item path, or one line stating there are none.
func renderHealth(root string, errs []Finding, warnings []Finding) string {
	var b strings.Builder
	b.WriteString("# HEALTH\n\n")
	all := append(append([]Finding{}, errs...), warnings...)
	if len(all) == 0 {
		b.WriteString("No findings.\n")
		return b.String()
	}
	for _, f := range all {
		rel, err := filepath.Rel(root, f.Path)
		if err != nil {
			rel = f.Path
		}
		fmt.Fprintf(&b, "- %s: %s\n", filepath.ToSlash(rel), f.Reason)
	}
	return b.String()
}

// cell escapes a table cell so a pipe in the value cannot break the row.
func cell(s string) string {
	return strings.ReplaceAll(s, "|", `\|`)
}

// writeIfChanged writes content to path only when the file's current bytes differ, so unchanged generated files keep their inode and timestamp.
func writeIfChanged(path, content string) error {
	if old, err := os.ReadFile(path); err == nil && string(old) == content {
		return nil
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
