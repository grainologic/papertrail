package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// Regression: a usage error under --json must exit 2, not PrintJSON's 0.
func TestUsageExitJSONExitsTwo(t *testing.T) {
	p := newProject(t)
	var stdout, stderr strings.Builder
	code := RunIn(p.dir, []string{"show", "--json"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit %d, want 2", code)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &doc); err != nil {
		t.Fatalf("stdout is not JSON: %q", stdout.String())
	}
	if doc["error"] == "" {
		t.Errorf("no error field: %v", doc)
	}
}
