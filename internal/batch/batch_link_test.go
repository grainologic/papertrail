package batch_test

import (
	"testing"

	"papertrail/internal/item"
)

// TestBatchTwoEntriesLinkSameExistingItem guards the blocker-2 regression: two batch entries both linking the same existing item must each land an edge on it, not have the second overwrite the first.
func TestBatchTwoEntriesLinkSameExistingItem(t *testing.T) {
	p := newProject(t)
	existingID := p.existing(t)

	payload := `[
		{"key": "a", "type": "task", "title": "One", "relatedTo": ["` + existingID + `"]},
		{"key": "b", "type": "bug", "title": "Two", "relatedTo": ["` + existingID + `"]}
	]`
	code, _, stderr := p.run(t, payload)
	if code != 0 {
		t.Fatalf("batch: exit %d, stderr %q", code, stderr)
	}

	var ids []string
	for _, title := range []string{"One", "Two"} {
		parsed, err := item.Parse(mustRead(t, firstMD(t, p.root, title)))
		if err != nil {
			t.Fatalf("parse %s: %v", title, err)
		}
		ids = append(ids, parsed.Frontmatter.ID)
	}

	existing := frontmatterOf(t, p.root, existingID)
	if len(existing.RelatedTo) != 2 {
		t.Fatalf("existing relatedTo = %v, want both entry ids %v", existing.RelatedTo, ids)
	}
	for _, id := range ids {
		if !contains(existing.RelatedTo, id) {
			t.Fatalf("existing relatedTo %v missing entry %s", existing.RelatedTo, id)
		}
	}
}
