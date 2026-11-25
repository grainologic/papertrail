package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFrontierExcludesErrorItems guards the CLI/file consistency fix: an item excluded by an error finding (here an id/folder mismatch) must appear in neither pt frontier output nor FRONTIER.md.
func TestFrontierExcludesErrorItems(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr strings.Builder
	if code := RunIn(dir, []string{"init"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init: exit %d, stderr %q", code, stderr.String())
	}
	stdout.Reset()
	if code := RunIn(dir, []string{"create", "task", "Solo", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("create: exit %d, stderr %q", code, stderr.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &created); err != nil {
		t.Fatalf("create json: %v", err)
	}
	root := filepath.Join(dir, ".papertrail")
	if err := os.Rename(filepath.Join(root, created.ID), filepath.Join(root, "wrongname")); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if code := RunIn(dir, []string{"frontier", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("frontier: exit %d, stderr %q", code, stderr.String())
	}
	if strings.Contains(stdout.String(), created.ID) {
		t.Fatalf("pt frontier includes excluded item: %s", stdout.String())
	}
	data, err := os.ReadFile(filepath.Join(root, "FRONTIER.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), created.ID) {
		t.Fatalf("FRONTIER.md includes excluded item:\n%s", data)
	}
}
