package tokens

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/atomicfile"
	"github.com/leeovery/switchboard/internal/logs"
)

var logger = logs.For("tokens")

// takenExt names an account's record of when its token was taken, beside its
// token file: <id>.taken. An account's id can't hold a '.', so no token file
// is named so.
const takenExt = ".taken"

// errMalformed is what reading a record that isn't one fails with.
var errMalformed = errors.New("not a record of when a token was taken")

// record is when switchboard took a token, and the token's SHA-256 hash, in
// hex, never the token, so the record counts for that token alone. Its file
// holds two lines:
//
//	taken 2026-10-27T09:30:00Z
//	sha256 <the token's hash>
type record struct {
	taken time.Time
	sum   string
}

// recordOf is the record of token, taken at taken, in UTC.
func recordOf(token Token, taken time.Time) record {
	return record{taken: taken.UTC(), sum: sum(token)}
}

// sum is the SHA-256 hash of token, in hex.
func sum(token Token) string {
	s := sha256.Sum256([]byte(token.Reveal()))
	return hex.EncodeToString(s[:])
}

// bytes is the record as its file holds it.
func (r record) bytes() []byte {
	return fmt.Appendf(nil, "taken %s\nsha256 %s\n", r.taken.Format(time.RFC3339), r.sum)
}

// parseRecord returns the record a record's file holds, failing with
// errMalformed for anything else.
func parseRecord(data []byte) (record, error) {
	taken, hashed, ok := strings.Cut(strings.TrimSuffix(string(data), "\n"), "\n")
	at, isTaken := strings.CutPrefix(taken, "taken ")
	s, isSum := strings.CutPrefix(hashed, "sha256 ")
	if !ok || !isTaken || !isSum {
		return record{}, errMalformed
	}
	t, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return record{}, fmt.Errorf("%w: %w", errMalformed, err)
	}
	return record{taken: t.UTC(), sum: s}, nil
}

// takenPath is where the record of when the account's token was taken is.
func (s Store) takenPath(id string) string {
	return s.Path(id) + takenExt
}

// Taken returns when switchboard took token as the account's, as the record
// it wrote beside the token file says, reporting false where that isn't
// known: the record is missing, isn't the user's alone, isn't one, or is of
// another token, as of a token file replaced by something else. Not knowing
// never makes the token unusable.
func (s Store) Taken(id string, token Token) (time.Time, bool) {
	r, err := s.readTaken(id)
	switch {
	case errors.Is(err, ErrMissing):
	case err != nil:
		logger.Debug("the record of when the token was taken is unusable", "account", id, "error", err)
	case r.sum != sum(token):
		logger.Debug("the record of when the token was taken is of another token", "account", id)
	default:
		return r.taken, true
	}
	return time.Time{}, false
}

// readTaken returns the record of when the account's token was taken, once
// it's shown to be a file of the user's that no one else can read or write,
// failing as a token file's read does, wrapping ErrMissing where there's
// none, or with errMalformed where it isn't one.
func (s Store) readTaken(id string) (record, error) {
	data, err := s.readPrivate(s.takenPath(id))
	if err != nil {
		return record{}, err
	}
	return parseRecord(data)
}

// writeTaken writes the record of token, taken at taken, as the account's, as
// Write writes its token: through a link, whole or not at all, and private.
func (s Store) writeTaken(id string, token Token, taken time.Time) error {
	target, err := atomicfile.Target(s.takenPath(id))
	if err != nil {
		return err
	}
	return atomicfile.Write(target, recordOf(token, taken).bytes(), 0o600)
}

// forgetTaken removes the record of when the account's token was taken, if
// it has one: of one that's a link, the link alone. One left behind counts
// for nothing but the token it's of, so failing to remove it is only logged.
func (s Store) forgetTaken(id string) {
	if err := os.Remove(s.takenPath(id)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		logger.Debug("couldn't remove the record of when the token was taken", "account", id, "error", err)
	}
}
