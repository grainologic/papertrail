package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}, {"-v"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr strings.Builder
			code := Run(args, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
			}
			if !strings.Contains(stdout.String(), Version) {
				t.Fatalf("stdout = %q, want it to contain version %q", stdout.String(), Version)
			}
		})
	}
}

func TestVersionJSON(t *testing.T) {
	var stdout, stderr strings.Builder
	code := Run([]string{"version", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(stdout.String()), &out); err != nil {
		t.Fatalf("invalid JSON %q: %v", stdout.String(), err)
	}
	if out["version"] != Version {
		t.Fatalf("version = %q, want %q", out["version"], Version)
	}
}
