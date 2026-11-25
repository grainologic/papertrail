package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestHelpVariants(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantJSON bool
	}{
		{"double dash", []string{"--help"}, false},
		{"short", []string{"-h"}, false},
		{"plain help", []string{"help"}, false},
		{"help help", []string{"help", "--help"}, false},
		{"help create", []string{"help", "create"}, false},
		{"help json create", []string{"help", "--json", "create"}, true},
		{"create help", []string{"create", "--help"}, false},
		{"create short", []string{"create", "-h"}, false},
		{"help json", []string{"help", "--json"}, true},
		{"create help json", []string{"create", "--help", "--json"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr strings.Builder
			code := RunIn(t.TempDir(), c.args, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("%v: exit %d, stderr %q", c.args, code, stderr.String())
			}
			if stdout.String() == "" {
				t.Fatalf("%v: empty stdout", c.args)
			}
			if c.wantJSON {
				var doc any
				if err := json.Unmarshal([]byte(stdout.String()), &doc); err != nil {
					t.Fatalf("%v: stdout is not JSON: %q", c.args, stdout.String())
				}
			}
		})
	}
}

func TestHelpUnknownCommandExitsTwo(t *testing.T) {
	var stdout, stderr strings.Builder
	code := RunIn(t.TempDir(), []string{"help", "bogus"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Errorf("stderr %q, want unknown-command mark", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout %q, want empty", stdout.String())
	}
}

func TestHelpNotTriggeredOnPositionalDashes(t *testing.T) {
	// "-h" after "--" is positional and must not open help.
	var stdout, stderr strings.Builder
	code := RunIn(t.TempDir(), []string{"create", "--", "--title", "-h"}, &stdout, &stderr)
	if code == 0 {
		t.Errorf("create -- --title -h: exit 0, want a usage error not help")
	}
	if strings.Contains(stdout.String(), "usage:") {
		t.Errorf("create -- --title -h printed help: %q", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	// "--title -h" on update consumes -h as the title value, not help.
	code = RunIn(t.TempDir(), []string{"update", "MYPREFIX", "--title", "-h"}, &stdout, &stderr)
	if code == 0 {
		t.Errorf("update MYPREFIX --title -h: unexpected success")
	}
	if strings.Contains(stdout.String(), "usage:") {
		t.Errorf("update --title -h printed help: %q", stdout.String())
	}
}
