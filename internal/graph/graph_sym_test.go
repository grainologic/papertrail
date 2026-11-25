package graph_test

import (
	"reflect"
	"testing"

	"papertrail/internal/graph"
	"papertrail/internal/item"
)

// TestSymmetrizeAsymmetricOneWay covers A=[B], B=[]: B gains A.
func TestSymmetrizeAsymmetricOneWay(t *testing.T) {
	a := fm("aaaaaaaa", item.StatusOpen, nil, []string{"bbbbbbbb"})
	b := fm("bbbbbbbb", item.StatusOpen, nil, nil)
	g := graph.Build([]item.Frontmatter{a, b})

	gotA := g.Symmetrize(a)
	if !reflect.DeepEqual(gotA.RelatedTo, []string{"bbbbbbbb"}) {
		t.Fatalf("a relatedTo = %v, want [bbbbbbbb]", gotA.RelatedTo)
	}
	gotB := g.Symmetrize(b)
	if !reflect.DeepEqual(gotB.RelatedTo, []string{"aaaaaaaa"}) {
		t.Fatalf("b relatedTo = %v, want [aaaaaaaa]", gotB.RelatedTo)
	}
}

// TestSymmetrizeRelatedToNotTransitive covers A=[B], B=[C]: A does NOT gain C (blocker-1 regression). relatedTo mirrors the reverse declaration only.
func TestSymmetrizeRelatedToNotTransitive(t *testing.T) {
	a := fm("aaaaaaaa", item.StatusOpen, nil, []string{"bbbbbbbb"})
	b := fm("bbbbbbbb", item.StatusOpen, nil, []string{"cccccccc"})
	c := fm("cccccccc", item.StatusOpen, nil, nil)
	g := graph.Build([]item.Frontmatter{a, b, c})

	gotA := g.Symmetrize(a)
	if !reflect.DeepEqual(gotA.RelatedTo, []string{"bbbbbbbb"}) {
		t.Fatalf("a relatedTo = %v, want [bbbbbbbb]", gotA.RelatedTo)
	}
}
