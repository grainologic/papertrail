// Package item parses and writes papertrail item files: YAML frontmatter followed by a markdown body that is preserved byte-for-byte. It also generates Crockford base32 item IDs, derives slugs, validates frontmatter, and resolves unique ID prefixes.
package item

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Item types fixed by the application.
const (
	TypeBug       = "bug"
	TypeFeature   = "feature"
	TypeTask      = "task"
	TypeChore     = "chore"
	TypeFeedback  = "feedback"
	TypeResearch  = "research"
	TypeSpike     = "spike"
	TypeDebate    = "debate"
	TypeQuestion  = "question"
	TypeMilestone = "milestone"
)

// Item statuses.
const (
	StatusOpen       = "open"
	StatusInProgress = "in_progress"
	StatusReview     = "review"
	StatusBlocked    = "blocked"
	StatusClosed     = "closed"
)

// Tracking modes.
const (
	ModeAuto = "auto"
	ModeHitl = "hitl"
)

// Frontmatter field keys in the fixed write order.
const (
	KeyID         = "id"
	KeyTitle      = "title"
	KeyType       = "type"
	KeyStatus     = "status"
	KeyPriority   = "priority"
	KeyMode       = "mode"
	KeyCreated    = "created"
	KeyDependsOn  = "dependsOn"
	KeyRequiredBy = "requiredBy"
	KeyRelatedTo  = "relatedTo"
)

var validTypes = map[string]bool{
	TypeBug: true, TypeFeature: true, TypeTask: true, TypeChore: true,
	TypeFeedback: true, TypeResearch: true, TypeSpike: true, TypeDebate: true,
	TypeQuestion: true, TypeMilestone: true,
}

var validStatuses = map[string]bool{
	StatusOpen: true, StatusInProgress: true, StatusReview: true,
	StatusBlocked: true, StatusClosed: true,
}

// Distinct validation failures.
var (
	ErrNoFrontmatter   = errors.New("item: no frontmatter")
	ErrMalformedYAML   = errors.New("item: malformed frontmatter YAML")
	ErrInvalidType     = errors.New("item: invalid type")
	ErrInvalidStatus   = errors.New("item: invalid status")
	ErrInvalidPriority = errors.New("item: invalid priority")
	ErrInvalidMode     = errors.New("item: invalid mode")
	ErrInvalidCreated  = errors.New("item: invalid created")
	ErrInvalidID       = errors.New("item: invalid id")
	ErrInvalidTitle    = errors.New("item: title has no file slug")
)

// Frontmatter is the parsed YAML frontmatter of an item.
type Frontmatter struct {
	ID         string    `yaml:"id"`
	Title      string    `yaml:"title"`
	Type       string    `yaml:"type"`
	Status     string    `yaml:"status"`
	Priority   int       `yaml:"priority"`
	Mode       string    `yaml:"mode"`
	Created    time.Time `yaml:"created"`
	DependsOn  []string  `yaml:"dependsOn"`
	RequiredBy []string  `yaml:"requiredBy"`
	RelatedTo  []string  `yaml:"relatedTo"`
}

// Item is a parsed item file with its body preserved verbatim.
type Item struct {
	Frontmatter Frontmatter
	Body        []byte
}

// Parse splits frontmatter from the body and decodes the YAML. LF and CRLF line endings are both accepted. The body returned is the exact bytes after the closing delimiter.
func Parse(data []byte) (Item, error) {
	rest, err := afterOpenDelim(data)
	if err != nil {
		return Item{}, err
	}
	closing := indexOfClosing(rest)
	if closing < 0 {
		return Item{}, fmt.Errorf("%w: unterminated frontmatter", ErrNoFrontmatter)
	}
	fm := Frontmatter{}
	if err := yaml.Unmarshal(rest[:closing], &fm); err != nil {
		return Item{}, fmt.Errorf("%w: %v", ErrMalformedYAML, err)
	}
	body := rest[closing:]
	// The closing delimiter line ends after its newline; the body starts there.
	if len(body) > 0 && body[0] == '\n' {
		body = body[1:]
	}
	return Item{Frontmatter: fm, Body: body}, nil
}

// afterOpenDelim returns the bytes after the opening frontmatter delimiter, accepting both LF and CRLF line endings; a file that does not start with the delimiter fails with ErrNoFrontmatter.
func afterOpenDelim(data []byte) ([]byte, error) {
	switch {
	case len(data) >= 5 && string(data[:5]) == "---\r\n":
		return data[5:], nil
	case len(data) >= 4 && string(data[:4]) == "---\n":
		return data[4:], nil
	}
	return nil, fmt.Errorf("%w: file does not start with ---", ErrNoFrontmatter)
}

// indexOfClosing returns the byte offset of the closing "---" line within the frontmatter section, or -1 when absent. The offset points at the newline terminating the delimiter line.
func indexOfClosing(rest []byte) int {
	i := 0
	for i < len(rest) {
		j := i
		for j < len(rest) && rest[j] != '\n' {
			j++
		}
		line := strings.TrimSuffix(string(rest[i:j]), "\r")
		if line == "---" && j < len(rest) {
			return j
		}
		if j >= len(rest) {
			return -1
		}
		i = j + 1
	}
	return -1
}

// Validate checks the type, status, priority, mode, created timestamp, and ID format. An empty mode is filled with the default auto.
func (f *Frontmatter) Validate() error {
	if !validTypes[f.Type] {
		return fmt.Errorf("%w: %q", ErrInvalidType, f.Type)
	}
	if !validStatuses[f.Status] {
		return fmt.Errorf("%w: %q", ErrInvalidStatus, f.Status)
	}
	if f.Priority < 0 || f.Priority > 4 {
		return fmt.Errorf("%w: %d", ErrInvalidPriority, f.Priority)
	}
	if f.Mode == "" {
		f.Mode = ModeAuto
	}
	if f.Mode != ModeAuto && f.Mode != ModeHitl {
		return fmt.Errorf("%w: %q", ErrInvalidMode, f.Mode)
	}
	if f.Created.IsZero() {
		return fmt.Errorf("%w: missing", ErrInvalidCreated)
	}
	if f.Created.Location() != time.UTC {
		f.Created = f.Created.UTC()
	}
	if err := CheckID(f.ID); err != nil {
		return err
	}
	if Slug(f.Title) == "" {
		return fmt.Errorf("%w: %q", ErrInvalidTitle, f.Title)
	}
	return nil
}

// Encode renders the frontmatter and body back into item file bytes. The body is emitted exactly as given. A marshal failure of one key is reported as an error instead of silently skipping the key.
func (f *Frontmatter) Encode(body []byte) ([]byte, error) {
	var keys []string
	values := map[string]any{
		KeyID:         f.ID,
		KeyTitle:      f.Title,
		KeyType:       f.Type,
		KeyStatus:     f.Status,
		KeyPriority:   f.Priority,
		KeyMode:       f.Mode,
		KeyCreated:    f.Created.Format(time.RFC3339),
		KeyDependsOn:  f.DependsOn,
		KeyRequiredBy: f.RequiredBy,
		KeyRelatedTo:  f.RelatedTo,
	}
	for _, k := range []string{
		KeyID, KeyTitle, KeyType, KeyStatus, KeyPriority, KeyMode,
		KeyCreated, KeyDependsOn, KeyRequiredBy, KeyRelatedTo,
	} {
		if v, ok := values[k]; ok && !isEmptyValue(v) {
			keys = append(keys, k)
		}
	}
	out := []byte("---\n")
	for _, k := range keys {
		doc, err := yaml.Marshal(map[string]any{k: values[k]})
		if err != nil {
			return nil, fmt.Errorf("item: encode key %s: %w", k, err)
		}
		out = append(out, doc...)
	}
	out = append(out, "---\n"...)
	out = append(out, body...)
	return out, nil
}

func isEmptyValue(v any) bool {
	switch t := v.(type) {
	case string:
		return t == ""
	case int:
		return false
	case time.Time:
		return t.IsZero()
	case []string:
		return len(t) == 0
	default:
		return false
	}
}
