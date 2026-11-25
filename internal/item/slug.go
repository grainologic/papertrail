package item

import (
	"strings"
	"unicode"
)

// maxSlugWords bounds the slug so item folder names stay short.
const maxSlugWords = 8

// Slug derives a kebab-case slug from a title: lowercase, non-alphanumeric characters become hyphens, repeated hyphens collapse, leading and trailing hyphens are trimmed, and the slug is cut after maxSlugWords words.
func Slug(title string) string {
	words := strings.FieldsFunc(title, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if len(words) > maxSlugWords {
		words = words[:maxSlugWords]
	}
	for i, w := range words {
		words[i] = strings.ToLower(w)
	}
	return strings.Join(words, "-")
}
