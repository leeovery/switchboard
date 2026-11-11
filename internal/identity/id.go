package identity

import (
	"crypto/sha256"
	"fmt"
	"io"
	"math/big"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/config"
)

const (
	// idLength is how long an id is at its shortest.
	idLength = 4
	// letters are what an id starts with, so none reads as a number.
	letters = "abcdefghijklmnopqrstuvwxyz"
	// characters are what the rest of an id is spelled with, in the order a
	// place's value picks them.
	characters = "0123456789" + letters
)

// Derive returns the id of the account of the given organization: spelled
// from the organization's SHA-256, so it's the same on every Mac, idLength
// characters long; or, where the record holds that for another account, a
// character longer, spelled on from the same hash, as many times as it takes.
// One the record holds for the organization itself is its own.
func (r Record) Derive(organization string) string {
	sum := sha256.Sum256([]byte(organization))
	for length := idLength; ; length++ {
		if id := spell(sum[:], length); !r.clashes(id, organization) {
			return id
		}
	}
}

// Random returns a random id, for an account whose organization can't be
// known: idLength characters, spelled from what rand gives, as Derive spells
// one from a hash, and never one the record holds. It fails where rand does.
func (r Record) Random(rand io.Reader) (string, error) {
	drawn := make([]byte, sha256.Size)
	for {
		if _, err := io.ReadFull(rand, drawn); err != nil {
			return "", fmt.Errorf("draw a random id: %w", err)
		}
		if id := spell(drawn, idLength); !r.clashes(id, "") {
			return id, nil
		}
	}
}

// clashes reports whether id can't be the id of the account of the given
// organization, "" for one not known: it's the record's for another account,
// or one an account had before switchboard made ids, or no id switchboard
// makes, as the word pin takes for routing isn't. They're compared ignoring
// case, as macOS ignores it in the token files' names.
func (r Record) clashes(id, organization string) bool {
	if own, ok := r.ByOrganization(organization); ok && own.ID == id {
		return false
	}
	return !wellFormed(id) || slices.ContainsFunc(r.Accounts, func(e Entry) bool {
		return strings.EqualFold(e.ID, id) || strings.EqualFold(e.Was, id)
	})
}

// spell returns the id of the given length that b spells, read as a number,
// a place at a time from its lowest: its first character a letter, the rest
// any of the characters. Past what b holds, the places are zeros.
func spell(b []byte, length int) string {
	n := new(big.Int).SetBytes(b)
	place := new(big.Int)
	id := make([]byte, length)
	for i := range id {
		chars := characters
		if i == 0 {
			chars = letters
		}
		n.DivMod(n, big.NewInt(int64(len(chars))), place)
		id[i] = chars[place.Int64()]
	}
	return string(id)
}

// wellFormed reports whether id is one switchboard makes: at least idLength
// characters, a letter first, and never the word pin takes for routing.
func wellFormed(id string) bool {
	return len(id) >= idLength && strings.IndexByte(letters, id[0]) >= 0 &&
		strings.Trim(id, characters) == "" && !strings.EqualFold(id, config.ReservedID)
}
