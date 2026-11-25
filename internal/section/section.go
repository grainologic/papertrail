// Package section implements the section get and section set commands. A section is the bytes after an exactly matching heading line up to the next heading of the same or higher level, so it includes subheadings.
package section

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"papertrail/internal/cli"
	"papertrail/internal/item"
	"papertrail/internal/store"
	"papertrail/internal/sync"
)

const (
	getUsage = "pt section get <id> <heading> [--json]"
	// setUsage includes --file so --help and usage errors advertise the stdin override.
	setUsage = "pt section set <id> <heading> [--file PATH] [--json]"
)

// getCmd and setCmd wire Run in init so the initializers stay cycle-free.
var getCmd = cli.Command{
	Name:    "section get",
	Usage:   getUsage,
	Summary: "Print the body of one section",
}
var setCmd = cli.Command{
	Name:    "section set",
	Usage:   setUsage,
	Summary: "Replace the body of one section",
	Help:    "The replacement reads from stdin unless --file PATH names a file to read instead; the file is left in place. The heading must match exactly and uniquely.",
}

func init() {
	getCmd.Run = runGet
	setCmd.Run = runSet
	cli.Register(getCmd, setCmd)
}

// findSection returns the byte range of a section body in data: the bytes after the heading line whose trimmed text equals heading exactly, up to the next heading line of the same or higher level. A heading at any level counts for the exact-text match, so repeated text is ambiguous.
func findSection(data []byte, heading string) (start, end int, err error) {
	type atxHeading struct {
		level int
		start int
		end   int
	}
	var hs []atxHeading
	inFence := false
	for i := 0; i < len(data); {
		lineEnd := bytes.IndexByte(data[i:], '\n')
		var lineEndAbs int
		if lineEnd < 0 {
			lineEndAbs = len(data)
		} else {
			lineEndAbs = i + lineEnd
		}
		trimmed := bytes.TrimSpace(data[i:lineEndAbs])
		if bytes.HasPrefix(trimmed, []byte("```")) || bytes.HasPrefix(trimmed, []byte("~~~")) {
			inFence = !inFence
		} else if !inFence {
			if level, ok := headingLevel(data[i:lineEndAbs]); ok {
				hs = append(hs, atxHeading{level: level, start: i, end: lineEndAbs})
			}
		}
		if lineEnd < 0 {
			break
		}
		i = lineEndAbs + 1
	}
	match := -1
	for _, h := range hs {
		text := string(bytes.TrimSpace(data[h.start+h.level : h.end]))
		if text == heading {
			if match >= 0 {
				return 0, 0, fmt.Errorf("section: heading %q matches more than once", heading)
			}
			match = h.start
		}
	}
	if match < 0 {
		return 0, 0, fmt.Errorf("section: no heading %q", heading)
	}
	// start is the byte after the heading line's newline; end is the offset of the next heading of the same or higher level, or the file end.
	var m atxHeading
	for _, h := range hs {
		if h.start == match {
			m = h
			break
		}
	}
	start = m.end + 1
	if start > len(data) {
		start = len(data)
	}
	end = len(data)
	for _, h := range hs {
		if h.start > m.start && h.level <= m.level {
			end = h.start
			break
		}
	}
	return start, end, nil
}

// headingLevel reports the ATX heading level of a heading line: the count of leading '#' characters on a trimmed line.
func headingLevel(line []byte) (int, bool) {
	trimmed := bytes.TrimSpace(line)
	i := 0
	for i < len(trimmed) && trimmed[i] == '#' {
		i++
	}
	if i == 0 || i > len(trimmed) || (i < len(trimmed) && trimmed[i] != ' ' && trimmed[i] != '\t') {
		return 0, false
	}
	if i == len(trimmed) {
		return 0, false
	}
	return i, true
}

func runGet(env cli.Env, args []string) int {
	fs := cli.NewFlagSet(env, getCmd)
	jf := &cli.JSONFlag{}
	fs.Var(jf, "json", "print the result as JSON")
	if code, done := cli.ParseFlags(env, fs, getCmd, args); done {
		return code
	}
	rest := fs.Args()
	if len(rest) < 2 {
		return cli.UsageExit(env, cli.JSONEnabled(fs), getUsage)
	}
	id, heading := rest[0], strings.Join(rest[1:], " ")
	root, code, ok := cli.RootFor(env, cli.JSONEnabled(fs))
	if !ok {
		return code
	}
	_, entry, err := resolveID(root, id)
	if err != nil {
		return cli.PrintError(env, cli.JSONEnabled(fs), 1, err)
	}
	raw, err := os.ReadFile(entry.Path)
	if err != nil {
		return cli.PrintError(env, cli.JSONEnabled(fs), 1, fmt.Errorf("store: read %s: %w", entry.Path, err))
	}
	parsed, err := item.Parse(raw)
	if err != nil {
		return cli.PrintError(env, cli.JSONEnabled(fs), 1, fmt.Errorf("store: parse %s: %w", entry.Path, err))
	}
	bodyStart := len(raw) - len(parsed.Body)
	start, end, err := findSection(parsed.Body, heading)
	if err != nil {
		return cli.PrintError(env, cli.JSONEnabled(fs), 1, err)
	}
	content := string(raw[bodyStart+start : bodyStart+end])
	if cli.JSONEnabled(fs) {
		return cli.PrintJSON(env, map[string]any{"id": entry.Frontmatter.ID, "heading": heading, "content": content})
	}
	fmt.Fprint(env.Stdout, content)
	return 0
}

func runSet(env cli.Env, args []string) int {
	fs := cli.NewFlagSet(env, setCmd)
	jf := &cli.JSONFlag{}
	fs.Var(jf, "json", "print the result as JSON")
	file := fs.String("file", "", "read replacement from this file instead of stdin")
	if code, done := cli.ParseFlags(env, fs, setCmd, args); done {
		return code
	}
	rest := fs.Args()
	if len(rest) < 2 {
		return cli.UsageExit(env, cli.JSONEnabled(fs), setUsage)
	}
	id, heading := rest[0], strings.Join(rest[1:], " ")
	root, code, ok := cli.RootFor(env, cli.JSONEnabled(fs))
	if !ok {
		return code
	}
	_, entry, err := resolveID(root, id)
	if err != nil {
		return cli.PrintError(env, cli.JSONEnabled(fs), 1, err)
	}
	raw, err := os.ReadFile(entry.Path)
	if err != nil {
		return cli.PrintError(env, cli.JSONEnabled(fs), 1, fmt.Errorf("store: read %s: %w", entry.Path, err))
	}
	parsed, err := item.Parse(raw)
	if err != nil {
		return cli.PrintError(env, cli.JSONEnabled(fs), 1, fmt.Errorf("store: parse %s: %w", entry.Path, err))
	}
	bodyStart := len(raw) - len(parsed.Body)
	start, end, err := findSection(parsed.Body, heading)
	if err != nil {
		return cli.PrintError(env, cli.JSONEnabled(fs), 1, err)
	}
	var replacement []byte
	if *file != "" {
		replacement, err = os.ReadFile(*file)
		if err != nil {
			return cli.PrintError(env, cli.JSONEnabled(fs), 1, fmt.Errorf("section: read %s: %w", *file, err))
		}
	} else {
		replacement, err = io.ReadAll(env.Stdin)
		if err != nil {
			return cli.PrintError(env, cli.JSONEnabled(fs), 1, fmt.Errorf("section: read stdin: %w", err))
		}
	}
	start += bodyStart
	end += bodyStart
	updated := append(append(append([]byte{}, raw[:start]...), replacement...), raw[end:]...)
	// WriteRaw replaces only the section's lines; WriteItem would re-encode frontmatter and rewrite bytes outside the section.
	if err := store.WriteRaw(entry.Path, updated); err != nil {
		return cli.PrintError(env, cli.JSONEnabled(fs), 1, err)
	}
	_, _ = sync.Run(root)
	if cli.JSONEnabled(fs) {
		return cli.PrintJSON(env, map[string]any{"id": entry.Frontmatter.ID, "heading": heading, "path": entry.Path})
	}
	return 0
}

// resolveID scans the store and resolves one ID prefix to its entry.
func resolveID(root, prefix string) (store.Result, store.Entry, error) {
	res, err := store.Scan(root)
	if err != nil {
		return store.Result{}, store.Entry{}, err
	}
	known := make([]string, len(res.Items))
	for i, e := range res.Items {
		known[i] = e.Frontmatter.ID
	}
	id, err := item.ResolvePrefix(known, prefix)
	if err != nil {
		return res, store.Entry{}, err
	}
	for _, e := range res.Items {
		if e.Frontmatter.ID == id {
			return res, e, nil
		}
	}
	return res, store.Entry{}, fmt.Errorf("item: unknown id %q", prefix)
}
