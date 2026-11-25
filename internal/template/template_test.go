package template_test

import (
	"errors"
	"strings"
	"testing"

	"papertrail/internal/item"
	"papertrail/internal/template"
)

func TestBodyRendersEveryType(t *testing.T) {
	types := []string{
		item.TypeBug, item.TypeFeature, item.TypeTask, item.TypeChore,
		item.TypeFeedback, item.TypeResearch, item.TypeSpike, item.TypeDebate,
		item.TypeQuestion, item.TypeMilestone,
	}
	for _, typ := range types {
		t.Run(typ, func(t *testing.T) {
			body, err := template.Body(typ, "My Title")
			if err != nil {
				t.Fatalf("Body(%q): %v", typ, err)
			}
			if !strings.HasPrefix(body, "# My Title\n\n") {
				t.Errorf("body does not start with the title heading:\n%q", body)
			}
			if !strings.Contains(body, "\n## Notes\n") {
				t.Errorf("body %q is missing the ## Notes heading", body)
			}
		})
	}
}

func TestBodyDebateHeadingsUnique(t *testing.T) {
	body, err := template.Body(item.TypeDebate, "d")
	if err != nil {
		t.Fatalf("Body: %v", err)
	}
	seen := map[string]int{}
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "#") {
			seen[line]++
		}
	}
	for line, n := range seen {
		if n > 1 {
			t.Errorf("heading %q appears %d times", line, n)
		}
	}
	if len(seen) != 11 {
		t.Errorf("debate body has %d headings, want 11", len(seen))
	}
}

func TestBodyTaskChoreMatch(t *testing.T) {
	task, err := template.Body(item.TypeTask, "t")
	if err != nil {
		t.Fatalf("Body(task): %v", err)
	}
	chore, err := template.Body(item.TypeChore, "t")
	if err != nil {
		t.Fatalf("Body(chore): %v", err)
	}
	if task != chore {
		t.Errorf("task and chore bodies differ")
	}
}

func TestBodyUnknownType(t *testing.T) {
	if _, err := template.Body("nope", "t"); !errors.Is(err, template.ErrUnknownType) {
		t.Errorf("want ErrUnknownType, got %v", err)
	}
}
