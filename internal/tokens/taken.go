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

// lastYear is the last year a record's time may fall in. Its token's year
// runs on past it, and RFC 3339, which the status document gives times in,
// has four-digit years alone.
const lastYear = 9998

// ErrUnrecorded is what Taken fails with where there's no record of when the
// token was taken.
var ErrUnrecorded = errors.New("no record of when the token was taken")

// record is when switchboard took a token, and the token's SHA-256 hash, in
// hex, never the token, so the record counts for that token alone. Its file
// holds two lines, the last ending in a newline or not:
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

// parseRecord returns the record a record's file holds, reporting false for
// anything else: a time that's zero, or past lastYear, among it.
func parseRecord(data []byte) (record, bool) {
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != 2 {
		return record{}, false
	}
	at, isTaken := strings.CutPrefix(lines[0], "taken ")
	s, isSum := strings.CutPrefix(lines[1], "sha256 ")
	taken, err := time.Parse(time.RFC3339, at)
	if !isTaken || !isSum || err != nil || taken.IsZero() || taken.UTC().Year() > lastYear || !isSum256(s) {
		return record{}, false
	}
	return record{taken: taken.UTC(), sum: s}, true
}

// isSum256 reports whether s is a SHA-256 hash, in hex.
func isSum256(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == sha256.Size
}

// takenPath is where the record of when the account's token was taken is.
func (s Store) takenPath(id string) string {
	return s.Path(id) + takenExt
}

// Taken returns when switchboard took token as the account's, as the record
// it wrote beside the token file says. It fails with ErrUnrecorded where
// there's no record, and saying why, never quoting the record, where it isn't
// the user's alone, isn't one, or is of another token, as of a token file
// replaced by something else. Not knowing never makes the token unusable.
func (s Store) Taken(id string, token Token) (time.Time, error) {
	r, err := s.readTaken(id)
	if err != nil {
		return time.Time{}, err
	}
	if r.sum != sum(token) {
		return time.Time{}, fmt.Errorf("the record %s is of another token", s.takenPath(id))
	}
	return r.taken, nil
}

// readTaken returns the record of when the account's token was taken, once
// it's shown to be a file of the user's that no one else can read or write,
// failing with ErrUnrecorded where there's none.
func (s Store) readTaken(id string) (record, error) {
	path := s.takenPath(id)
	data, err := readChecked(path, "the record of when the token was taken", s.checkRecord(path))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return record{}, ErrUnrecorded
	case err != nil:
		return record{}, err
	}
	r, ok := parseRecord(data)
	if !ok {
		return record{}, fmt.Errorf("the record %s isn't one of when a token was taken", path)
	}
	return r, nil
}

// checkRecord checks the record at path, which the info it's given
// describes, is a file, the user's, that no one else can read or write.
func (s Store) checkRecord(path string) func(fs.FileInfo) error {
	return func(info fs.FileInfo) error {
		mode := info.Mode()
		switch {
		case !mode.IsRegular():
			return fmt.Errorf("the record %s isn't a file", path)
		case !s.owns(info):
			return fmt.Errorf("the record %s is another user's", path)
		case mode.Perm()&shared != 0:
			return fmt.Errorf("other users can %s the record %s (mode %04o)", access(mode.Perm()&shared), path, mode.Perm())
		}
		return nil
	}
}

// writeTaken writes the record of token, taken at taken, as the account's, as
// Write writes its token: through a link, whole or not at all, and private.
// A record of token there already stands, keeping its date. It never writes
// where the token file, written at tokenTarget, is, as through a link.
func (s Store) writeTaken(id string, token Token, taken time.Time, tokenTarget string) error {
	if r, err := s.readTaken(id); err == nil && r.sum == sum(token) {
		return nil
	}
	target, err := atomicfile.Target(s.takenPath(id))
	if err != nil {
		return err
	}
	if sameFile(target, tokenTarget) {
		return fmt.Errorf("the record %s leads to the token file, which it would replace", s.takenPath(id))
	}
	return atomicfile.Write(target, recordOf(token, taken).bytes(), 0o600)
}

// sameFile reports whether the files at a and b are one: false where either
// isn't there.
func sameFile(a, b string) bool {
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	return err == nil && os.SameFile(ai, bi)
}

// forgetTaken removes the record of when the account's token was taken, if
// it has one: of one that's a link, the link alone. One left behind counts
// for nothing but the token it's of, so failing to remove it is only logged.
func (s Store) forgetTaken(id string) {
	if err := os.Remove(s.takenPath(id)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		logger.Debug("couldn't remove the record of when the token was taken", "account", id, "error", err)
	}
}
