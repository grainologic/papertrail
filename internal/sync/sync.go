// Package sync runs the sync pass: healing item files from the link graph, then regenerating INDEX.md, FRONTIER.md, and HEALTH.md at the item root.
package sync

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	syncstd "sync"

	"papertrail/internal/graph"
	"papertrail/internal/item"
	"papertrail/internal/store"
)

const (
	indexFile    = "INDEX.md"
	frontierFile = "FRONTIER.md"
	healthFile   = "HEALTH.md"
)

// Finding is one error or warning the sync pass produced, holding the item path that owns it and the reason.
type Finding struct {
	Path   string
	Reason string
}

// Result is the sync pass finding summary; errors are returned alongside the non-nil error, warnings never fail the pass.
type Result struct {
	Errors   []Finding
	Warnings []Finding
	// Frontier holds the unblocked, non-excluded items in index order (priority then created), shared by FRONTIER.md and pt frontier.
	Frontier []item.Frontmatter
}

// Run performs the sync pass over the item root: heals requiredBy and relatedTo from the authoritative graph, renames stale slugs, rewrites the level-1 heading from the frontmatter title, excludes erroring items from the index, and regenerates the three index files only when their content changes. Error findings make the returned error non-nil; stale-status warnings are recorded in HEALTH.md but do not.
func Run(root string) (Result, error) {
	res, err := store.Scan(root)
	if err != nil {
		return Result{}, fmt.Errorf("sync: %w", err)
	}

	g := graph.Build(frontmatters(res.Items))

	idPath := make(map[string]string, len(res.Items))
	for _, e := range res.Items {
		idPath[e.Frontmatter.ID] = e.Path
	}

	excluded := map[string]bool{}
	var errs []Finding
	errs = append(errs, folderFindings(res)...)
	errs = append(errs, itemFindings(root, res)...)

	for _, e := range res.Items {
		folder := filepath.Base(filepath.Dir(e.Path))
		if folder != e.Frontmatter.ID {
			excluded[e.Frontmatter.ID] = true
			errs = append(errs, Finding{
				Path:   e.Path,
				Reason: fmt.Sprintf("id %s does not match folder name %s", e.Frontmatter.ID, folder),
			})
		}
	}
	for _, cycle := range g.Cycles() {
		for _, id := range cycle {
			excluded[id] = true
		}
		errs = append(errs, Finding{
			Path:   idPath[cycle[0]],
			Reason: "dependency cycle: " + joinIds(cycle),
		})
	}
	for _, l := range g.DanglingLinks() {
		excluded[l.From] = true
		errs = append(errs, Finding{
			Path:   idPath[l.From],
			Reason: fmt.Sprintf("link to missing id %s (%s)", l.To, l.Field),
		})
	}

	type healedPath struct {
		id   string
		path string
		err  error
	}
	healedPaths := make([]healedPath, len(res.Items))
	runBounded(len(res.Items), func(i int) {
		id := res.Items[i].Frontmatter.ID
		if excluded[id] {
			healedPaths[i] = healedPath{id: id, path: res.Items[i].Path}
			return
		}
		newPath, err := healItem(g, res.Items[i])
		healedPaths[i] = healedPath{id: id, path: newPath, err: err}
	})
	paths := make(map[string]string, len(res.Items))
	for _, h := range healedPaths {
		if h.err != nil {
			return Result{}, h.err
		}
		paths[h.id] = h.path
	}

	var warnings []Finding
	for _, w := range g.Warnings() {
		warnings = append(warnings, Finding{
			Path:   paths[w.ID],
			Reason: fmt.Sprintf("%s but dependency %s is %s", w.Status, w.Dependency, w.DependencyStatus),
		})
	}

	frontier := frontierItems(g, excluded)

	if err := writeIfChanged(filepath.Join(root, indexFile), renderIndex(root, res, excluded, paths)); err != nil {
		return Result{}, err
	}
	if err := writeIfChanged(filepath.Join(root, frontierFile), renderFrontier(root, res, g, excluded, paths)); err != nil {
		return Result{}, err
	}
	if err := writeIfChanged(filepath.Join(root, healthFile), renderHealth(root, errs, warnings)); err != nil {
		return Result{}, err
	}

	if len(errs) > 0 {
		return Result{Errors: errs, Warnings: warnings, Frontier: frontier}, fmt.Errorf("sync: %d error finding(s)", len(errs))
	}
	return Result{Warnings: warnings, Frontier: frontier}, nil
}

// frontmatters extracts the scanned frontmatter values for the graph.
func frontmatters(items []store.Entry) []item.Frontmatter {
	out := make([]item.Frontmatter, len(items))
	for i, e := range items {
		out[i] = e.Frontmatter
	}
	return out
}

// folderFindings keeps the store findings that already carry a folder path:
// item folders with no .md file or with several.
func folderFindings(res store.Result) []Finding {
	var out []Finding
	for _, f := range res.Findings {
		if f.Path != "" {
			out = append(out, Finding{Path: f.Path, Reason: f.Reason})
		}
	}
	return out
}

// itemFindings attaches an item path to the store findings that carry none: folders with exactly one md file whose frontmatter failed to parse or to validate. Reasons are re-derived by parsing the file itself.
func itemFindings(root string, res store.Result) []Finding {
	valid := map[string]bool{}
	for _, e := range res.Items {
		valid[filepath.Dir(e.Path)] = true
	}
	reported := map[string]bool{}
	for _, f := range res.Findings {
		if f.Path != "" {
			reported[f.Path] = true
		}
	}
	var out []Finding
	dirents, err := os.ReadDir(root)
	if err != nil {
		return out
	}
	for _, d := range dirents {
		if !d.IsDir() {
			continue
		}
		dir := filepath.Join(root, d.Name())
		if valid[dir] || reported[dir] {
			continue
		}
		mds, err := mdFiles(dir)
		if err != nil {
			out = append(out, Finding{Path: dir, Reason: err.Error()})
			continue
		}
		if len(mds) != 1 {
			continue
		}
		data, err := os.ReadFile(mds[0])
		if err != nil {
			out = append(out, Finding{Path: mds[0], Reason: err.Error()})
			continue
		}
		parsed, err := item.Parse(data)
		if err != nil {
			out = append(out, Finding{Path: mds[0], Reason: err.Error()})
			continue
		}
		if err := parsed.Frontmatter.Validate(); err != nil {
			out = append(out, Finding{Path: mds[0], Reason: err.Error()})
		}
	}
	return out
}

// mdFiles lists the .md files directly inside dir, sorted.
func mdFiles(dir string) ([]string, error) {
	dirents, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("sync: read folder %s: %w", dir, err)
	}
	var mds []string
	for _, d := range dirents {
		if d.IsDir() || !strings.EqualFold(filepath.Ext(d.Name()), ".md") {
			continue
		}
		mds = append(mds, filepath.Join(dir, d.Name()))
	}
	return mds, nil
}

// healItem rewrites one item file: the graph-derived requiredBy and symmetric relatedTo, the level-1 heading matching the title, and a file name matching the derived slug. The file is written only when one of those changed; otherwise its bytes and name are left intact. The body is read only when a rewrite is actually due, so a pristine item costs one frontmatter scan and no extra file read. Returns the final path of the item file.
func healItem(g *graph.Graph, e store.Entry) (string, error) {
	old := e.Frontmatter
	healed := g.Symmetrize(old)

	reportRemoved(old, healed)

	newPath := e.Path
	if slug := item.Slug(healed.Title); slug != "" && slug+".md" != filepath.Base(e.Path) {
		newPath = filepath.Join(filepath.Dir(e.Path), slug+".md")
	}

	linksChanged := !sameSet(old.RequiredBy, healed.RequiredBy) ||
		!sameSet(old.RelatedTo, healed.RelatedTo)
	headingChanged := e.BodyHead != "# "+healed.Title
	// Skip the body read when no rewrite can follow; the heading check uses the first body line captured during scan.
	if !linksChanged && newPath == e.Path && !headingChanged {
		return e.Path, nil
	}

	raw, err := os.ReadFile(e.Path)
	if err != nil {
		return e.Path, fmt.Errorf("sync: read %s: %w", e.Path, err)
	}
	parsed, err := item.Parse(raw)
	if err != nil {
		return e.Path, fmt.Errorf("sync: parse %s: %w", e.Path, err)
	}
	body, headingChanged := fixHeading(parsed.Body, healed.Title)

	switch {
	case linksChanged || headingChanged:
		if err := store.WriteItem(store.Entry{ID: e.ID, Path: newPath, Frontmatter: healed}, body); err != nil {
			return newPath, fmt.Errorf("sync: write %s: %w", newPath, err)
		}
		if newPath != e.Path {
			if err := os.Remove(e.Path); err != nil {
				return newPath, fmt.Errorf("sync: remove %s: %w", e.Path, err)
			}
		}
	case newPath != e.Path:
		if err := os.Rename(e.Path, newPath); err != nil {
			return newPath, fmt.Errorf("sync: rename %s: %w", e.Path, err)
		}
	}
	return newPath, nil
}

// runBounded runs fn(i) for i in [0, n) across at most 8 workers, joined before it returns.
func runBounded(n int, fn func(int)) {
	if n == 0 {
		return
	}
	workers := runtime.NumCPU()
	if workers > 8 {
		workers = 8
	}
	if workers > n {
		workers = n
	}
	var wg syncstd.WaitGroup
	next := make(chan int)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				fn(i)
			}
		}()
	}
	for i := range n {
		next <- i
	}
	close(next)
	wg.Wait()
}

// stderrMu serializes heal reports; healItem runs inside the worker pool.
var stderrMu syncstd.Mutex

// reportRemoved prints a line per link that drops out of a healed field.
func reportRemoved(old, healed item.Frontmatter) {
	stderrMu.Lock()
	defer stderrMu.Unlock()
	for _, to := range old.RequiredBy {
		if !contains(healed.RequiredBy, to) {
			fmt.Fprintf(os.Stderr, "sync: removed requiredBy link %s from %s\n", to, old.ID)
		}
	}
	for _, to := range old.RelatedTo {
		if !contains(healed.RelatedTo, to) {
			fmt.Fprintf(os.Stderr, "sync: removed relatedTo link %s from %s\n", to, old.ID)
		}
	}
}

// fixHeading rewrites the first level-1 line of the body to the title. A body that does not start with a level-1 heading is returned unchanged.
func fixHeading(body []byte, title string) ([]byte, bool) {
	if len(body) == 0 {
		return body, false
	}
	want := "# " + title
	eol := bytes.IndexByte(body, '\n')
	line, rest := body, []byte(nil)
	if eol >= 0 {
		line, rest = body[:eol], body[eol:]
	}
	// Compare without CR so a CRLF heading is not rewritten to LF.
	trimmed := bytes.TrimRight(line, "\r")
	if !bytes.HasPrefix(trimmed, []byte("# ")) || string(trimmed) == want {
		return body, false
	}
	out := make([]byte, 0, len(want)+len(line)-len(trimmed)+len(rest))
	out = append(out, want...)
	out = append(out, line[len(trimmed):]...)
	out = append(out, rest...)
	return out, true
}

// sameSet reports whether two link lists hold the same ids.
func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for _, id := range a {
		if !contains(b, id) {
			return false
		}
	}
	return true
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func joinIds(ids []string) string {
	out := ""
	for i, id := range ids {
		if i > 0 {
			out += " -> "
		}
		out += id
	}
	return out
}
