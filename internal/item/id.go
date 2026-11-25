package item

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
)

// crockfordLower is the lowercase Crockford base32 alphabet: digits and letters without i, l, o, or u.
const crockfordLower = "0123456789abcdefghjkmnpqrstvwxyz"

// idLength is the fixed ID length in characters.
const idLength = 8

// ErrIDCollision is returned when a generated ID's folder already exists for every candidate the generator tried.
var ErrIDCollision = errors.New("item: id collision")

// CheckID verifies the 8 character lowercase Crockford base32 ID format.
func CheckID(id string) error {
	if len(id) != idLength {
		return fmt.Errorf("%w: %q is not %d characters", ErrInvalidID, id, idLength)
	}
	for _, c := range id {
		if !strings.ContainsRune(crockfordLower, c) {
			return fmt.Errorf("%w: %q contains %q", ErrInvalidID, id, string(c))
		}
	}
	return nil
}

// NewID generates a random 8 character lowercase Crockford base32 ID whose folder does not yet exist under rootDir. Collision is checked by folder existence, retrying on the (astronomically unlikely) collision.
func NewID(rootDir string) (string, error) {
	for range 100 {
		id, err := randomID()
		if err != nil {
			return "", err
		}
		_, statErr := os.Stat(filepath.Join(rootDir, id))
		if os.IsNotExist(statErr) {
			return id, nil
		}
		if statErr != nil {
			return "", fmt.Errorf("item: stat %s: %w", id, statErr)
		}
	}
	return "", ErrIDCollision
}

func randomID() (string, error) {
	var sb strings.Builder
	for i := 0; i < idLength; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(crockfordLower))))
		if err != nil {
			return "", fmt.Errorf("item: generate id: %w", err)
		}
		sb.WriteByte(crockfordLower[n.Int64()])
	}
	return sb.String(), nil
}
