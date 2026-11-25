// Package template provides the embedded body templates for item creation.
package template

import (
	"embed"
	"errors"
	"fmt"

	"papertrail/internal/item"
)

//go:embed templates/*.md
var files embed.FS

// ErrUnknownType is returned when no template exists for the type.
var ErrUnknownType = errors.New("template: unknown type")

var byType = map[string]string{
	item.TypeBug:       "templates/bug.md",
	item.TypeFeature:   "templates/feature.md",
	item.TypeTask:      "templates/task.md",
	item.TypeChore:     "templates/chore.md",
	item.TypeFeedback:  "templates/feedback.md",
	item.TypeResearch:  "templates/research.md",
	item.TypeSpike:     "templates/spike.md",
	item.TypeDebate:    "templates/debate.md",
	item.TypeQuestion:  "templates/question.md",
	item.TypeMilestone: "templates/milestone.md",
}

// Body renders the initial body for an item of the given type: the title as the level 1 heading, an empty overview, the type's headings, then ## Notes. The template files are the source of the headings; editing one changes the output without a Go code change.
func Body(typ, title string) (string, error) {
	name, ok := byType[typ]
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrUnknownType, typ)
	}
	raw, err := files.ReadFile(name)
	if err != nil {
		return "", fmt.Errorf("template: read %s: %w", name, err)
	}
	return fmt.Sprintf("# %s\n\n%s", title, raw), nil
}
