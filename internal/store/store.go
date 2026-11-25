// Package store scans item folders under the configured root.
//
// The item files are the only source of truth; everything here is disposable.
package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"papertrail/internal/item"
)

// cacheName sits in the item root. With the default root .papertrail/ this is exactly the .papertrail/cache.json the spec names; with a custom root the cache travels with the items, which keeps it disposable with them.
const cacheName = "cache.json"

var (
	// ErrNoMDFile and ErrSeveralMDFiles report an item folder that does not hold exactly one .md file.
	ErrNoMDFile      = errors.New("store: item folder has no .md file")
	ErrSeveralMDFile = errors.New("store: item folder has several .md files")
)

// Entry is one scanned item.
type Entry struct {
	ID          string
	Path        string
	Frontmatter item.Frontmatter
	// BodyHead is the first body line, trimmed of CR/LF, captured during the frontmatter read so sync can check the heading without reading the body.
	BodyHead string
}

// Finding reports a problem with one item folder or file. The sync pass decides what each finding means; the scan never fails on them.
type Finding struct {
	Path   string
	Reason string
}

// Result is everything one scan saw.
type Result struct {
	Items    []Entry
	Findings []Finding
}

type cachedEntry struct {
	Size    int64 `json:"size"`
	ModTime int64 `json:"modtime"`
	Entry   Entry `json:"entry"`
}

type cache map[string]cachedEntry

// cachePath is the location of the disposable scan cache for an item root.
func cachePath(root string) string { return filepath.Join(root, cacheName) }

// loadCache returns the parsed cache, or nil when it is missing or corrupt.
func loadCache(root string) cache {
	data, err := os.ReadFile(cachePath(root))
	if err != nil {
		return nil
	}
	var c cache
	if json.Unmarshal(data, &c) != nil {
		return nil
	}
	return c
}

// saveCache writes the cache; a failed write only costs speed, so the error is ignored.
func saveCache(root string, c cache) {
	data, err := json.Marshal(c)
	if err != nil {
		return
	}
	_ = os.WriteFile(cachePath(root), data, 0o644)
}

// Scan walks every item folder directly under root and reads each item file only up to the closing frontmatter delimiter. Results are identical with or without a cache file; a missing or corrupt cache is rebuilt silently.
func Scan(root string) (Result, error) {
	res := Result{}
	dirents, err := os.ReadDir(root)
	if err != nil {
		return res, fmt.Errorf("store: read root %s: %w", root, err)
	}

	// Collect one .md path per item folder, reporting odd folders.
	var paths []string
	for _, d := range dirents {
		if !d.IsDir() {
			continue
		}
		dir := filepath.Join(root, d.Name())
		mds, err := mdFiles(dir)
		switch {
		case err != nil:
			res.Findings = append(res.Findings, Finding{Path: dir, Reason: err.Error()})
		case len(mds) == 0:
			res.Findings = append(res.Findings, Finding{Path: dir, Reason: ErrNoMDFile.Error()})
		case len(mds) > 1:
			res.Findings = append(res.Findings, Finding{
				Path: dir, Reason: fmt.Sprintf("%s: %s", ErrSeveralMDFile, strings.Join(mds, ", ")),
			})
		default:
			paths = append(paths, mds[0])
		}
	}
	sort.Strings(paths)

	c := loadCache(root)
	// scanned carries either the parsed entry or the reason it failed.
	type scanned struct {
		entry  *Entry
		reason string
	}
	results := make([]scanned, len(paths))
	runBounded(len(paths), func(i int) {
		if e, hit := cachedEntryFor(c, paths[i]); hit {
			results[i].entry = e
			return
		}
		fm, head, err := readFrontmatter(paths[i])
		if err != nil {
			results[i].reason = err.Error()
			return
		}
		if err := fm.Validate(); err != nil {
			results[i].reason = err.Error()
			return
		}
		results[i].entry = &Entry{ID: fm.ID, Path: paths[i], Frontmatter: *fm, BodyHead: head}
	})

	for _, r := range results {
		if r.entry != nil {
			res.Items = append(res.Items, *r.entry)
		} else {
			res.Findings = append(res.Findings, Finding{Reason: r.reason})
		}
	}
	sort.Slice(res.Items, func(i, j int) bool { return res.Items[i].Path < res.Items[j].Path })

	// Rebuild the cache from what this scan read, valid entries only. A file that vanished mid-scan is simply left out.
	fresh := cache{}
	for _, e := range res.Items {
		if info, err := os.Stat(e.Path); err == nil {
			fresh[e.Path] = cachedEntry{
				Size:    info.Size(),
				ModTime: info.ModTime().UnixNano(),
				Entry:   e,
			}
		}
	}
	saveCache(root, fresh)
	return res, nil
}

// ReadItem reads one item by full ID straight from <root>/<id>/, using the cache when it matches the file's size and modification time.
func ReadItem(root, id string) (Entry, error) {
	dir := filepath.Join(root, id)
	mds, err := mdFiles(dir)
	if err != nil {
		return Entry{}, err
	}
	if len(mds) == 0 {
		return Entry{}, fmt.Errorf("%w: %s", ErrNoMDFile, dir)
	}
	if len(mds) > 1 {
		return Entry{}, fmt.Errorf("%w: %s: %s", ErrSeveralMDFile, dir, strings.Join(mds, ", "))
	}
	path := mds[0]
	if e, hit := cachedEntryFor(loadCache(root), path); hit {
		return *e, nil
	}
	fm, head, err := readFrontmatter(path)
	if err != nil {
		return Entry{}, err
	}
	if err := fm.Validate(); err != nil {
		return Entry{}, fmt.Errorf("store: %s: %w", path, err)
	}
	return Entry{ID: fm.ID, Path: path, Frontmatter: *fm, BodyHead: head}, nil
}

// mdFiles lists the .md files directly inside dir, sorted.
func mdFiles(dir string) ([]string, error) {
	dirents, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("store: read folder %s: %w", dir, err)
	}
	var mds []string
	for _, d := range dirents {
		if d.IsDir() || !strings.EqualFold(filepath.Ext(d.Name()), ".md") {
			continue
		}
		mds = append(mds, filepath.Join(dir, d.Name()))
	}
	sort.Strings(mds)
	return mds, nil
}

// cachedEntryFor returns the cached entry when the file's size and modification time still match what the cache recorded.
func cachedEntryFor(c cache, path string) (*Entry, bool) {
	if c == nil {
		return nil, false
	}
	ce, ok := c[path]
	if !ok {
		return nil, false
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	if info.Size() != ce.Size || info.ModTime().UnixNano() != ce.ModTime {
		return nil, false
	}
	e := ce.Entry
	return &e, true
}

// readFrontmatter reads only the bytes through the closing "---" line, so bodies never reach memory during a scan.
func readFrontmatter(path string) (*item.Frontmatter, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", fmt.Errorf("store: open %s: %w", path, err)
	}
	defer f.Close()
	r := bufio.NewReader(f)
	var buf []byte
	closed := false
	first := true
	for !closed {
		line, rerr := r.ReadString('\n')
		trimmed := strings.TrimRight(line, "\r\n")
		if first {
			// Reject early so we never buffer a body looking for a delimiter.
			if trimmed != "---" {
				return nil, "", fmt.Errorf("store: %s: %w", path, item.ErrNoFrontmatter)
			}
			first = false
		} else {
			if trimmed == "---" {
				closed = true
				break
			}
			buf = append(buf, line...)
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				return nil, "", fmt.Errorf("store: %s: %w", path, item.ErrNoFrontmatter)
			}
			return nil, "", fmt.Errorf("store: read %s: %w", path, rerr)
		}
	}
	parsed, err := item.Parse(append(append([]byte("---\n"), buf...), "---\n"...))
	if err != nil {
		return nil, "", fmt.Errorf("store: %s: %w", path, err)
	}
	// Capture the first body line while buffered so sync checks the heading without a second file pass.
	head, _ := r.ReadString('\n')
	return &parsed.Frontmatter, strings.TrimRight(head, "\r\n"), nil
}

// runBounded runs fn for i in [0, n) through a worker pool bounded by the CPU count, and joins before returning.
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
	var wg sync.WaitGroup
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
