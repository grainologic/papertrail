package graph_test

import (
	"errors"
	"reflect"
	"testing"

	"papertrail/internal/graph"
	"papertrail/internal/item"
)

func fm(id, status string, dependsOn, relatedTo []string) item.Frontmatter {
	return item.Frontmatter{
		ID: id, Title: id, Type: item.TypeTask, Status: status,
		Priority: 2, Mode: item.ModeAuto, DependsOn: dependsOn, RelatedTo: relatedTo,
	}
}

func TestRequiredByDerivedFromDependsOn(t *testing.T) {
	g := graph.Build([]item.Frontmatter{
		fm("aaaaaaaa", item.StatusOpen, []string{"cccccccc"}, nil),
		fm("bbbbbbbb", item.StatusOpen, []string{"cccccccc"}, nil),
		fm("cccccccc", item.StatusOpen, nil, nil),
	})
	if got := g.RequiredBy("cccccccc"); !reflect.DeepEqual(got, []string{"aaaaaaaa", "bbbbbbbb"}) {
		t.Fatalf("RequiredBy = %v", got)
	}
	if got := g.RequiredBy("aaaaaaaa"); got != nil {
		t.Fatalf("RequiredBy of leaf = %v, want nil", got)
	}
}

func TestSymmetrizeMakesRelatedToSymmetric(t *testing.T) {
	a := fm("aaaaaaaa", item.StatusOpen, nil, []string{"bbbbbbbb"})
	b := fm("bbbbbbbb", item.StatusOpen, nil, []string{"aaaaaaaa"})
	g := graph.Build([]item.Frontmatter{a, b})

	gotA := g.Symmetrize(a)
	if !reflect.DeepEqual(gotA.RelatedTo, []string{"bbbbbbbb"}) {
		t.Fatalf("a relatedTo = %v", gotA.RelatedTo)
	}
	gotB := g.Symmetrize(b)
	if !reflect.DeepEqual(gotB.RelatedTo, []string{"aaaaaaaa"}) {
		t.Fatalf("b relatedTo = %v, want [aaaaaaaa]", gotB.RelatedTo)
	}
	if gotB.RequiredBy != nil {
		t.Fatalf("b requiredBy = %v, want nil", gotB.RequiredBy)
	}
}

func TestSymmetrizeDerivedRequiredBy(t *testing.T) {
	a := fm("aaaaaaaa", item.StatusOpen, []string{"bbbbbbbb"}, nil)
	b := fm("bbbbbbbb", item.StatusOpen, nil, []string{"zzzzzzzz"})
	g := graph.Build([]item.Frontmatter{a, b})
	gotB := g.Symmetrize(b)
	if !reflect.DeepEqual(gotB.RequiredBy, []string{"aaaaaaaa"}) {
		t.Fatalf("requiredBy = %v", gotB.RequiredBy)
	}
	// relatedTo pointing at a missing item survives Symmetrize; DanglingLinks is the detector.
	if !reflect.DeepEqual(gotB.RelatedTo, []string{"zzzzzzzz"}) {
		t.Fatalf("relatedTo = %v", gotB.RelatedTo)
	}
}

func TestCycles(t *testing.T) {
	t.Run("detects a cycle", func(t *testing.T) {
		g := graph.Build([]item.Frontmatter{
			fm("aaaaaaaa", item.StatusOpen, []string{"bbbbbbbb"}, nil),
			fm("bbbbbbbb", item.StatusOpen, []string{"cccccccc"}, nil),
			fm("cccccccc", item.StatusOpen, []string{"aaaaaaaa"}, nil),
			fm("dddddddd", item.StatusOpen, []string{"aaaaaaaa"}, nil),
		})
		cycles := g.Cycles()
		want := [][]string{{"aaaaaaaa", "bbbbbbbb", "cccccccc", "aaaaaaaa"}}
		if !reflect.DeepEqual(cycles, want) {
			t.Fatalf("cycles = %v, want %v", cycles, want)
		}
	})
	t.Run("acyclic graph has none", func(t *testing.T) {
		g := graph.Build([]item.Frontmatter{
			fm("aaaaaaaa", item.StatusOpen, []string{"bbbbbbbb"}, nil),
			fm("bbbbbbbb", item.StatusOpen, nil, nil),
		})
		if cycles := g.Cycles(); len(cycles) != 0 {
			t.Fatalf("cycles = %v", cycles)
		}
	})
	t.Run("dangling edges are not cycles", func(t *testing.T) {
		g := graph.Build([]item.Frontmatter{
			fm("aaaaaaaa", item.StatusOpen, []string{"zzzzzzzz"}, nil),
		})
		if cycles := g.Cycles(); len(cycles) != 0 {
			t.Fatalf("cycles = %v", cycles)
		}
	})
}

func TestDanglingLinks(t *testing.T) {
	g := graph.Build([]item.Frontmatter{
		fm("aaaaaaaa", item.StatusOpen, []string{"zzzzzzzz"}, []string{"yyyyyyyy"}),
		fm("bbbbbbbb", item.StatusOpen, nil, []string{"aaaaaaaa"}),
	})
	want := []graph.Link{
		{From: "aaaaaaaa", To: "zzzzzzzz", Field: graph.FieldDependsOn},
		{From: "aaaaaaaa", To: "yyyyyyyy", Field: graph.FieldRelatedTo},
	}
	if got := g.DanglingLinks(); !reflect.DeepEqual(got, want) {
		t.Fatalf("dangling = %v, want %v", got, want)
	}
}

func TestFrontier(t *testing.T) {
	closed := fm("cccccccc", item.StatusClosed, nil, nil)
	open := fm("aaaaaaaa", item.StatusOpen, []string{"cccccccc"}, nil)
	cases := []struct {
		name string
		fm   item.Frontmatter
		want bool
	}{
		{"open with closed deps", open, true},
		{"in_progress with closed deps", fm("dddddddd", item.StatusInProgress, []string{"cccccccc"}, nil), true},
		{"review is excluded", fm("eeeeeeee", item.StatusReview, []string{"cccccccc"}, nil), false},
		{"closed is excluded", fm("ffffffff", item.StatusClosed, []string{"cccccccc"}, nil), false},
		{"blocked is excluded", fm("gggggggg", item.StatusBlocked, []string{"cccccccc"}, nil), false},
		{"open with unclosed dep", fm("hhhhhhhh", item.StatusOpen, []string{"aaaaaaaa"}, nil), false},
		{"open with dangling dep", fm("iiiiiiii", item.StatusOpen, []string{"zzzzzzzz"}, nil), false},
		{"open with no deps", fm("jjjjjjjj", item.StatusOpen, nil, nil), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := graph.Build(append([]item.Frontmatter{closed, open}, tc.fm))
			found := false
			for _, id := range g.Frontier() {
				if id == tc.fm.ID {
					found = true
				}
			}
			if found != tc.want {
				t.Fatalf("frontier membership = %v, want %v; frontier = %v", found, tc.want, g.Frontier())
			}
		})
	}
}

func TestWarnings(t *testing.T) {
	closed := fm("cccccccc", item.StatusClosed, nil, nil)
	openDep := fm("aaaaaaaa", item.StatusOpen, nil, nil)
	g := graph.Build([]item.Frontmatter{
		closed, openDep,
		fm("dddddddd", item.StatusInProgress, []string{"aaaaaaaa"}, nil),
		fm("eeeeeeee", item.StatusReview, []string{"aaaaaaaa"}, nil),
		fm("ffffffff", item.StatusClosed, []string{"aaaaaaaa"}, nil),
		fm("gggggggg", item.StatusClosed, []string{"cccccccc"}, nil),
	})
	want := []graph.Warning{
		{ID: "dddddddd", Status: item.StatusInProgress, Dependency: "aaaaaaaa", DependencyStatus: item.StatusOpen},
		{ID: "eeeeeeee", Status: item.StatusReview, Dependency: "aaaaaaaa", DependencyStatus: item.StatusOpen},
		{ID: "ffffffff", Status: item.StatusClosed, Dependency: "aaaaaaaa", DependencyStatus: item.StatusOpen},
	}
	if got := g.Warnings(); !reflect.DeepEqual(got, want) {
		t.Fatalf("warnings = %v, want %v", got, want)
	}
}

func TestApplyInsert(t *testing.T) {
	a := fm("aaaaaaaa", item.StatusOpen, nil, nil)
	b := fm("bbbbbbbb", item.StatusOpen, []string{"aaaaaaaa"}, nil)
	items := []item.Frontmatter{a, b}

	t.Run("rewires between A and B", func(t *testing.T) {
		out, err := graph.ApplyInsert(items, "aaaaaaaa", "bbbbbbbb", "cccccccc")
		if err != nil {
			t.Fatal(err)
		}
		var gotN, gotB item.Frontmatter
		for _, got := range out {
			switch got.ID {
			case "aaaaaaaa":
				if !reflect.DeepEqual(got.DependsOn, a.DependsOn) {
					t.Fatalf("A changed: %v", got)
				}
			case "bbbbbbbb":
				gotB = got
			case "cccccccc":
				gotN = got
			}
		}
		if !reflect.DeepEqual(gotB.DependsOn, []string{"cccccccc"}) {
			t.Fatalf("B dependsOn = %v, want [cccccccc]", gotB.DependsOn)
		}
		if !reflect.DeepEqual(gotN.DependsOn, []string{"aaaaaaaa"}) {
			t.Fatalf("N dependsOn = %v, want [aaaaaaaa]", gotN.DependsOn)
		}
	})
	t.Run("input items are not mutated", func(t *testing.T) {
		if !reflect.DeepEqual(b.DependsOn, []string{"aaaaaaaa"}) {
			t.Fatalf("input mutated: %v", b)
		}
	})
	t.Run("same A and B rejected", func(t *testing.T) {
		_, err := graph.ApplyInsert(items, "aaaaaaaa", "aaaaaaaa", "cccccccc")
		if !errors.Is(err, graph.ErrInvalidInsert) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("unknown A rejected", func(t *testing.T) {
		_, err := graph.ApplyInsert(items, "zzzzzzzz", "bbbbbbbb", "cccccccc")
		if !errors.Is(err, graph.ErrUnknownItem) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("unknown B rejected", func(t *testing.T) {
		_, err := graph.ApplyInsert(items, "aaaaaaaa", "zzzzzzzz", "cccccccc")
		if !errors.Is(err, graph.ErrUnknownItem) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("existing N gains dependency", func(t *testing.T) {
		n := fm("cccccccc", item.StatusOpen, nil, nil)
		out, err := graph.ApplyInsert(append(items, n), "aaaaaaaa", "bbbbbbbb", "cccccccc")
		if err != nil {
			t.Fatal(err)
		}
		for _, got := range out {
			if got.ID == "cccccccc" && !reflect.DeepEqual(got.DependsOn, []string{"aaaaaaaa"}) {
				t.Fatalf("N dependsOn = %v", got.DependsOn)
			}
		}
	})
}
