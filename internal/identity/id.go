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
	// mostDraws is how many ids Random draws before it gives up: with about a
	// million ids, and a few held, drawing that many held is a draw that
	// isn't random.
	mostDraws = 100
)

// Derive returns the id of the account of the given organization: the
// record's, where it has the organization; else spelled from the
// organization's SHA-256, so it's the same on every Mac, idLength characters
// long, or, where that's held, as clashes says of the record and the
// configured accounts' ids, a character longer, spelled on from the same
// hash, as many times as it takes.
func (r Record) Derive(organization string, configured []string) string {
	if e, ok := r.ByOrganization(organization); ok {
		return e.ID
	}
	return spelledFrom(organization, func(id string) bool { return !r.clashes(id, configured) })
}

// spelledFrom returns the first id spelled from seed's SHA-256, idLength
// characters long, then a character longer each time, spelled on from the
// same hash, that free reports may be given.
func spelledFrom(seed string, free func(id string) bool) string {
	sum := sha256.Sum256([]byte(seed))
	for length := idLength; ; length++ {
		if id := spell(sum[:], length); free(id) {
			return id
		}
	}
}

// Random returns a random id, for an account whose organization can't be
// known: idLength characters, spelled from what rand gives, as Derive spells
// one from a hash, and never one held, as clashes says of the record and the
// configured accounts' ids. It fails where rand does, and where mostDraws
// drawn are each held.
func (r Record) Random(rand io.Reader, configured []string) (string, error) {
	return draw(rand, func(id string) bool { return !r.clashes(id, configured) })
}

// NewPoolID returns the id of a pool being made: random, as no organization
// stands behind a pool, spelled as Random spells an account's, and never one
// held, as clashes says of the record and the ids configured, the config's
// accounts' and pools', nor one of the record's pools'. It fails as Random
// does.
func (r Record) NewPoolID(rand io.Reader, configured []string) (string, error) {
	return draw(rand, func(id string) bool {
		_, pooled := r.PoolByID(id)
		return !pooled && !r.clashes(id, configured)
	})
}

// draw returns an id spelled from what rand gives, idLength characters long,
// the first drawn that free reports may be given, failing where rand does,
// and where mostDraws drawn may each not.
func draw(rand io.Reader, free func(id string) bool) (string, error) {
	drawn := make([]byte, sha256.Size)
	for range mostDraws {
		if _, err := io.ReadFull(rand, drawn); err != nil {
			return "", fmt.Errorf("draw a random id: %w", err)
		}
		if id := spell(drawn, idLength); free(id) {
			return id, nil
		}
	}
	return "", fmt.Errorf("draw a random id: each of %d drawn is held already", mostDraws)
}

// Spells reports whether id is the one the given organization spells at its
// length, in any case, as Derive spells one from its SHA-256: an id derived
// from it, the record holding those shorter, as far as can be told.
func Spells(organization, id string) bool {
	sum := sha256.Sum256([]byte(organization))
	return len(id) >= idLength && strings.EqualFold(id, spell(sum[:], len(id)))
}

// WellFormed reports whether id is one switchboard makes, as wellFormed says.
func WellFormed(id string) bool {
	return wellFormed(id)
}

// clashes reports whether id can't be given to an account: the record holds
// it, as an account's id or one an account had before switchboard made ids,
// or it's one of the configured accounts' ids, or it's no id switchboard
// makes, as the word pin takes for routing isn't.
func (r Record) clashes(id string, configured []string) bool {
	return !wellFormed(id) || containsFold(configured, id) || slices.ContainsFunc(r.Accounts, func(e Entry) bool {
		return strings.EqualFold(e.ID, id) || strings.EqualFold(e.Was, id)
	})
}

// containsFold reports whether ids holds id. Ids are compared ignoring case
// throughout, as macOS ignores it in the token files' names.
func containsFold(ids []string, id string) bool {
	return slices.ContainsFunc(ids, func(held string) bool { return strings.EqualFold(held, id) })
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
