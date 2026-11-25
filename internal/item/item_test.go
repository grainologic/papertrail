package item

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sampleFrontmatter() Frontmatter {
	created, _ := time.Parse(time.RFC3339, "2026-11-23T00:00:00Z")
	return Frontmatter{
		ID: "ab12cd34", Title: "Fix parser", Type: TypeBug, Status: StatusOpen,
		Priority: 2, Mode: ModeAuto, Created: created,
	}
}

func TestParseEncodeRoundTripPreservesBody(t *testing.T) {
	body := "# Fix parser\n\nSome text with `code` and trailing spaces.   \nline2\n"
	fm := sampleFrontmatter()
	file, err := fm.Encode([]byte(body))
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	parsed, err := Parse(file)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if string(parsed.Body) != body {
		t.Errorf("body changed:\n%q\nwant\n%q", parsed.Body, body)
	}
	if err := parsed.Frontmatter.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if parsed.Frontmatter.ID != fm.ID || parsed.Frontmatter.Title != fm.Title {
		t.Errorf("frontmatter mismatch: %+v", parsed.Frontmatter)
	}
}

func TestEncodeOmitsZeroCollections(t *testing.T) {
	fm := sampleFrontmatter()
	fm.DependsOn = nil
	enc, err := fm.Encode(nil)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	out := string(enc)
	if strings.Contains(out, "dependsOn") || strings.Contains(out, "requiredBy") || strings.Contains(out, "relatedTo") {
		t.Errorf("empty collections written:\n%s", out)
	}
	if !strings.HasPrefix(out, "---\nid:") {
		t.Errorf("id not first key:\n%s", out)
	}
}

func TestParseStructuralErrors(t *testing.T) {
	cases := map[string]struct {
		file string
		want error
	}{
		"no frontmatter": {"just body\n", ErrNoFrontmatter},
		"unterminated":   {"---\nid: ab12cd34\n", ErrNoFrontmatter},
		"malformed yaml": {"---\nid: [unclosed\n---\n", ErrMalformedYAML},
	}
	for name, tc := range cases {
		if _, err := Parse([]byte(tc.file)); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
}

func TestParseCRLFFile(t *testing.T) {
	fm := sampleFrontmatter()
	body := "# Fix parser\n\nSome text.\n"
	file, err := fm.Encode([]byte(body))
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	crlf := bytes.ReplaceAll(file, []byte("\n"), []byte("\r\n"))
	parsed, err := Parse(crlf)
	if err != nil {
		t.Fatalf("Parse CRLF: %v", err)
	}
	wantBody := strings.ReplaceAll(body, "\n", "\r\n")
	if string(parsed.Body) != wantBody {
		t.Errorf("CRLF body changed:\n%q\nwant\n%q", parsed.Body, wantBody)
	}
	if parsed.Frontmatter.ID != fm.ID || parsed.Frontmatter.Title != fm.Title {
		t.Errorf("CRLF frontmatter mismatch: %+v", parsed.Frontmatter)
	}
}

func TestValidateDistinctErrors(t *testing.T) {
	cases := map[string]func(*Frontmatter) error{
		"invalid type": func(f *Frontmatter) error { f.Type = "nonsense"; return nil },
		"invalid status": func(f *Frontmatter) error {
			f.Type = TypeBug
			f.Status = "done"
			return nil
		},
		"invalid priority": func(f *Frontmatter) error {
			f.Type, f.Status = TypeBug, StatusOpen
			f.Priority = 9
			return nil
		},
		"invalid mode": func(f *Frontmatter) error {
			f.Type, f.Status = TypeBug, StatusOpen
			f.Mode = "maybe"
			return nil
		},
		"invalid id": func(f *Frontmatter) error {
			f.Type, f.Status = TypeBug, StatusOpen
			f.ID = "SHORT"
			return nil
		},
		"invalid created": func(f *Frontmatter) error {
			f.Type, f.Status = TypeBug, StatusOpen
			f.Created = time.Time{}
			return nil
		},
	}
	wants := map[string]error{
		"invalid type": ErrInvalidType, "invalid status": ErrInvalidStatus,
		"invalid priority": ErrInvalidPriority, "invalid mode": ErrInvalidMode,
		"invalid id": ErrInvalidID, "invalid created": ErrInvalidCreated,
	}
	for name, set := range cases {
		fm := sampleFrontmatter()
		if err := set(&fm); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if err := fm.Validate(); !errors.Is(err, wants[name]) {
			t.Errorf("%s: err = %v, want %v", name, err, wants[name])
		}
	}
}

func TestValidateFillsDefaultMode(t *testing.T) {
	fm := sampleFrontmatter()
	fm.Mode = ""
	if err := fm.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if fm.Mode != ModeAuto {
		t.Errorf("Mode = %q, want auto", fm.Mode)
	}
}

func TestCheckIDRejectsBadCharacters(t *testing.T) {
	if err := CheckID("ab12cd3I"); err == nil {
		t.Error("want error for i in id")
	}
	if err := CheckID("ab12cd3"); err == nil {
		t.Error("want error for short id")
	}
}

func TestNewIDSkipsExistingFolders(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "aaaaaaaa"), 0o755); err != nil {
		t.Fatal(err)
	}
	for range 50 {
		id, err := NewID(root)
		if err != nil {
			t.Fatalf("NewID: %v", err)
		}
		if id == "aaaaaaaa" {
			t.Fatal("NewID returned an id whose folder already exists")
		}
		if err := CheckID(id); err != nil {
			t.Fatalf("generated invalid id %q: %v", id, err)
		}
	}
}

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"Fix parser bug!":     "fix-parser-bug",
		"Add --json flag  v2": "add-json-flag-v2",
		"  --trimmed--  ":     "trimmed",
		"a b c d e f g h i j": "a-b-c-d-e-f-g-h",
		"Многословный title":  "многословный-title",
	}
	for title, want := range cases {
		if got := Slug(title); got != want {
			t.Errorf("Slug(%q) = %q, want %q", title, got, want)
		}
	}
}

func TestResolvePrefix(t *testing.T) {
	known := []string{"ab12cd34", "ab12ffff", "zzzzzzzz"}
	id, err := ResolvePrefix(known, "ab12cd")
	if err != nil || id != "ab12cd34" {
		t.Errorf("unique prefix = %q, %v", id, err)
	}
	if _, err := ResolvePrefix(known, "ab12"); !errors.Is(err, ErrAmbiguousPrefix) {
		t.Errorf("err = %v, want ErrAmbiguousPrefix", err)
	}
	if _, err := ResolvePrefix(known, "qq"); !errors.Is(err, ErrUnknownPrefix) {
		t.Errorf("err = %v, want ErrUnknownPrefix", err)
	}
}

func TestResolvePrefixNormalizesLookalikes(t *testing.T) {
	known := []string{"ab12cd34"}
	id, err := ResolvePrefix(known, "ABl2")
	if err != nil || id != "ab12cd34" {
		t.Errorf("prefix with lookalikes = %q, %v", id, err)
	}
}

func TestValidateRejectsEmptySlugTitle(t *testing.T) {
	fm := sampleFrontmatter()
	fm.Title = "!!!"
	if err := fm.Validate(); !errors.Is(err, ErrInvalidTitle) {
		t.Fatalf("Validate(!!!) = %v, want ErrInvalidTitle", err)
	}
}
