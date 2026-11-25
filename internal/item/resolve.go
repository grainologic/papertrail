package item

import (
	"errors"
	"fmt"
	"strings"
)

// ErrUnknownPrefix is returned when no known ID starts with the prefix.
var ErrUnknownPrefix = errors.New("item: unknown id prefix")

// ErrAmbiguousPrefix is returned when several known IDs start with the prefix.
var ErrAmbiguousPrefix = errors.New("item: ambiguous id prefix")

// ResolvePrefix resolves a unique ID prefix against the known full IDs.
// Crockford lookalikes (i, l, o) in the prefix map to 1, 1, 0.
func ResolvePrefix(knownIDs []string, prefix string) (string, error) {
	normalized := NormalizePrefix(prefix)
	if normalized == "" {
		return "", fmt.Errorf("%w: empty", ErrUnknownPrefix)
	}
	var matches []string
	for _, id := range knownIDs {
		if strings.HasPrefix(id, normalized) {
			matches = append(matches, id)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("%w: %q", ErrUnknownPrefix, prefix)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("%w: %q matches %s", ErrAmbiguousPrefix, prefix, strings.Join(matches, ", "))
	}
}

// NormalizePrefix lowercases a prefix and maps Crockford lookalike characters.
func NormalizePrefix(prefix string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(prefix) {
		switch r {
		case 'i', 'l':
			sb.WriteByte('1')
		case 'o':
			sb.WriteByte('0')
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}
