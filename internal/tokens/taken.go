package tokens

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/atomicfile"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs"
)

var logger = logs.For("tokens")

// takenExt names an account's record of when its tokens were taken, beside
// its token file: <id>.taken. An account's id can't hold a '.', so no token
// file is named so.
const takenExt = ".taken"

// The years a record's time may fall in, in UTC. Its token's year runs on
// past the last, and RFC 3339, which the status document gives times in, has
// four-digit years alone.
const (
	firstYear = 1
	lastYear  = 9998
)

// mostTaken is how many tokens' dates a record keeps.
const mostTaken = 8

// ErrUnrecorded is what Taken fails with where there's no record of when the
// account's tokens were taken.
var ErrUnrecorded = errors.New("no record of when the token was taken")

// taking is when switchboard first took a token, and the token's SHA-256
// hash, in hex, never the token.
type taking struct {
	taken time.Time
	sum   string
}

// record is when switchboard took each of an account's latest tokens, the
// latest given first, so a token given again, as after another by mistake,
// keeps its date. Its file holds two lines a token, the last ending in a
// newline or not:
//
//	taken 2026-10-27T09:30:00Z
//	sha256 <the token's hash>
type record []taking

// sum is the SHA-256 hash of token, in hex.
func sum(token Token) string {
	s := sha256.Sum256([]byte(token.Reveal()))
	return hex.EncodeToString(s[:])
}

// of returns when the token whose hash is sum was taken, reporting false
// where the record doesn't say.
func (r record) of(sum string) (time.Time, bool) {
	i := slices.IndexFunc(r, func(t taking) bool { return t.sum == sum })
	if i < 0 {
		return time.Time{}, false
	}
	return r[i].taken, true
}

// with is the record once the token whose hash is sum is given at taken:
// first, dated the earlier of taken and any date it has already, which a
// clock set wrong when it was first taken can't have moved on; the rest
// after it, as they were, up to mostTaken in all.
func (r record) with(sum string, taken time.Time) record {
	if had, ok := r.of(sum); ok && had.Before(taken) {
		taken = had
	}
	others := slices.DeleteFunc(slices.Clone(r), func(t taking) bool { return t.sum == sum })
	next := append(record{{taken: taken.UTC(), sum: sum}}, others...)
	return next[:min(len(next), mostTaken)]
}

// bytes is the record as its file holds it.
func (r record) bytes() []byte {
	var b []byte
	for _, t := range r {
		b = fmt.Appendf(b, "taken %s\nsha256 %s\n", t.taken.Format(time.RFC3339), t.sum)
	}
	return b
}

// parseRecord returns the record a record's file holds, reporting false for
// anything else: whole pairs of lines, from one to mostTaken of them, each a
// time in the years a record's may be and a hash in lowercase hex, no hash
// twice.
func parseRecord(data []byte) (record, bool) {
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines)%2 != 0 || len(lines) > 2*mostTaken {
		return nil, false
	}
	var r record
	for i := 0; i < len(lines); i += 2 {
		t, ok := parseTaking(lines[i], lines[i+1])
		if _, twice := r.of(t.sum); !ok || twice {
			return nil, false
		}
		r = append(r, t)
	}
	return r, true
}

// parseTaking returns the taking a record's pair of lines, taken and hashed,
// give, reporting false where they don't.
func parseTaking(taken, hashed string) (taking, bool) {
	at, isTaken := strings.CutPrefix(taken, "taken ")
	s, isSum := strings.CutPrefix(hashed, "sha256 ")
	t, err := time.Parse(time.RFC3339, at)
	if !isTaken || !isSum || err != nil || !takeable(t) || !isSum256(s) {
		return taking{}, false
	}
	return taking{taken: t.UTC(), sum: s}, true
}

// takeable reports whether a record can say a token was taken at t: it isn't
// the zero time, and falls in the years a record's may, in UTC.
func takeable(t time.Time) bool {
	year := t.UTC().Year()
	return !t.IsZero() && year >= firstYear && year <= lastYear
}

// replaceable is why a record can't be gone by, having been looked at, which
// writing the account's token again replaces: it isn't a record, or isn't the
// user's alone. A record that couldn't be read at all is left as it is.
type replaceable struct{ error }

func (r replaceable) Unwrap() error { return r.error }

// isSum256 reports whether s is a SHA-256 hash, in lowercase hex.
func isSum256(s string) bool {
	return len(s) == 2*sha256.Size && !strings.ContainsFunc(s, func(r rune) bool {
		return (r < '0' || r > '9') && (r < 'a' || r > 'f')
	})
}

// takenPath is where the record of when the account's tokens were taken is.
func (s Store) takenPath(id string) string {
	return s.Path(id) + takenExt
}

// Taken returns when switchboard first took token as the account's, as the
// record it keeps beside the token file says. It fails with ErrUnrecorded
// where there's no record, and saying why, never quoting the record, where it
// isn't the user's alone, isn't one, or has no date of token, as of a token
// file replaced by something else. Not knowing never makes the token
// unusable.
func (s Store) Taken(id string, token Token) (time.Time, error) {
	r, err := s.readTaken(id)
	if err != nil {
		return time.Time{}, err
	}
	taken, ok := r.of(sum(token))
	if !ok {
		return time.Time{}, fmt.Errorf("the record %s is of other tokens", s.takenPath(id))
	}
	return taken, nil
}

// readTaken returns the record of when the account's tokens were taken, once
// it's shown to be a file of the user's that no one else can read or write,
// failing with ErrUnrecorded where there's none.
func (s Store) readTaken(id string) (record, error) {
	path := s.takenPath(id)
	data, err := readChecked(path, "the record of when the token was taken", s.checkRecord(path))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, ErrUnrecorded
	case err != nil:
		return nil, err
	}
	r, ok := parseRecord(data)
	if !ok {
		return nil, replaceable{fmt.Errorf("the record %s isn't one of when tokens were taken", path)}
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
			return replaceable{fmt.Errorf("the record %s isn't a file", path)}
		case !s.owns(info):
			return replaceable{fmt.Errorf("the record %s is another user's", path)}
		case mode.Perm()&shared != 0:
			return replaceable{fmt.Errorf("other users can %s the record %s (mode %04o)", access(mode.Perm()&shared), path, mode.Perm())}
		}
		return nil
	}
}

// writeTaken records token, taken at taken, as the account's, as Write writes
// its token: through a link, whole or not at all, and private. It keeps the
// dates of the tokens the record has already, as with says, and writes
// nothing where that changes nothing. A record it can't go by, having looked
// at it, is replaced; one it couldn't read, or a time a record can't hold, as
// of a clock gone wrong, leaves the record as it is. It never writes where
// the token file, written at tokenTarget, is, as through a link. One record's
// reading and writing waits for another's, as of two writes of a token at
// once, so neither loses the other's.
func (s Store) writeTaken(id string, token Token, taken time.Time, tokenTarget string) error {
	if !takeable(taken) {
		return fmt.Errorf("a record can't say a token was taken at %s, so it's left as it is", taken.UTC().Format(time.RFC3339))
	}
	defer lockDir(s.dir)()
	path := s.takenPath(id)
	s.tightenRecord(path)
	was, err := s.readTaken(id)
	if _, ok := errors.AsType[replaceable](err); err != nil && !ok && !errors.Is(err, ErrUnrecorded) {
		return fmt.Errorf("the record couldn't be read, so it's left as it is: %w", err)
	}
	next := was.with(sum(token), taken)
	if bytes.Equal(next.bytes(), was.bytes()) {
		return nil
	}
	target, err := atomicfile.Target(path)
	if err != nil {
		return err
	}
	if sameFile(target, tokenTarget) {
		return fmt.Errorf("the record %s leads to the token file, which it would replace", path)
	}
	return atomicfile.Write(target, next.bytes(), 0o600)
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

// tightenRecords makes each record in the tokens directory private, as
// tightenRecord does: each named for an account, by an id config allows.
func (s Store) tightenRecords() {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		logger.Debug("couldn't look for records of when tokens were taken", "error", err)
		return
	}
	for _, e := range entries {
		if id, ok := strings.CutSuffix(e.Name(), takenExt); ok && config.CheckID(id) == nil {
			s.tightenRecord(filepath.Join(s.dir, e.Name()))
		}
	}
}

// tightenRecord makes the record at path the user's alone, as one restored
// from a copy is often left readable by others: a file of the user's, by
// that one name, not a link, that others can read but not write, which
// nothing but the user can have changed. One others can write stays as it
// is, and counts for nothing. What it checks and changes is the file it has
// open, whatever takes its place at path meanwhile.
func (s Store) tightenRecord(path string) {
	f, err := os.OpenFile(path, tightenFlags, 0)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || !s.owns(info) {
		return
	}
	perm := info.Mode().Perm()
	if n, ok := links(info); !ok || n != 1 || perm&shared == 0 || perm&0o022 != 0 {
		return
	}
	if err := f.Chmod(perm &^ shared); err != nil {
		logger.Debug("couldn't make the record of when the token was taken private", "path", path, "error", err)
		return
	}
	logger.Info("made the record of when the token was taken private", "path", path, "was", fmt.Sprintf("%04o", perm))
}

// forgetTaken removes the record of when the account's tokens were taken, if
// it has one: of one that's a link, the link alone. One left behind counts
// for nothing but the tokens it's of, so failing to remove it is only logged.
func (s Store) forgetTaken(id string) {
	if err := os.Remove(s.takenPath(id)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		logger.Debug("couldn't remove the record of when the token was taken", "account", id, "error", err)
	}
}
