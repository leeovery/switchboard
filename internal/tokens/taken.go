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
	"github.com/leeovery/switchboard/internal/redact"
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

// Sum is the token's SHA-256 hash, in hex: never the token, but what tells
// it from another, as the record of when it was taken keeps it.
func (t Token) Sum() string {
	s := sha256.Sum256([]byte(t.Reveal()))
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

// unreplaceable is why a file at a record's path can't be gone by, and is
// never replaced nor removed, as it may be a token, perhaps its only copy, as
// one a token file led to that accounts remove leaves: it holds anything
// shaped like a token, as redact recognises one, or is too large to tell it
// holds none. It never says what the file holds.
type unreplaceable struct{ error }

// mostRead is how much of a file at a record's path is read, a record or
// not, to tell whether it holds a token: far more than a record or a token
// file holds.
const mostRead = 1 << 20

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
	taken, ok := r.of(token.Sum())
	if !ok {
		return time.Time{}, fmt.Errorf("the record %s is of other tokens", s.takenPath(id))
	}
	return taken, nil
}

// readTaken returns the record of when the account's tokens were taken, once
// it's shown to be a file of the user's that no one else can read or write,
// failing with ErrUnrecorded where there's none.
func (s Store) readTaken(id string) (record, error) {
	r, _, err := s.readRecord(id, false)
	return r, err
}

// readRecord returns the record of when the account's tokens were taken, as
// readTaken does, but with readable, one others can read, though not write,
// too, which it reports exposed, as a copy restored by a dotfiles step, or by
// a second name, is often left. A file, whoever's it is and whoever can write
// it, is read for what it holds, up to mostRead, as one that holds a token,
// or more than that, is unreplaceable. One that isn't a record, isn't the
// user's alone, or isn't a file, is replaceable; one that can't be read is
// left.
func (s Store) readRecord(id string, readable bool) (r record, exposed bool, err error) {
	path := s.takenPath(id)
	var info fs.FileInfo
	data, err := readChecked(path, "the record of when the token was taken", mostRead, func(i fs.FileInfo) error {
		info = i
		if !i.Mode().IsRegular() {
			return replaceable{fmt.Errorf("the record %s isn't a file", path)}
		}
		return nil
	})
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, false, ErrUnrecorded
	case err != nil:
		return nil, false, unopened(path, err)
	case len(data) > mostRead:
		return nil, false, unreplaceable{fmt.Errorf("the record %s holds over %d bytes, too many to tell it holds no token", path, mostRead)}
	}
	r, ok := parseRecord(data)
	if !ok && redact.HoldsToken(string(data)) {
		return nil, false, unreplaceable{fmt.Errorf("the record %s holds a token, not when tokens were taken", path)}
	}
	if err := s.checkRecord(path, info, readable); err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, replaceable{fmt.Errorf("the record %s isn't one of when tokens were taken", path)}
	}
	return r, info.Mode().Perm()&shared != 0, nil
}

// checkRecord checks the record at path, a file, which info describes, is
// the user's, that no one else can write, nor, but where it's readable, read.
func (s Store) checkRecord(path string, info fs.FileInfo, readable bool) error {
	mode := info.Mode()
	exposed := mode.Perm() & shared
	if readable {
		exposed &= 0o022
	}
	switch {
	case !s.owns(info):
		return replaceable{fmt.Errorf("the record %s is another user's", path)}
	case exposed != 0:
		return replaceable{fmt.Errorf("other users can %s the record %s (mode %04o)", access(mode.Perm()&shared), path, mode.Perm())}
	}
	return nil
}

// unopened is why the record at path couldn't be read, as err says: where
// it isn't a file, as what the path leads to says, it's replaceable. A file
// that can't be read, whoever's it is, as one root wrote, is left as it is,
// as it may hold a token.
func unopened(path string, err error) error {
	if _, ok := errors.AsType[replaceable](err); ok {
		return err
	}
	if info, statErr := os.Stat(path); statErr == nil && !info.Mode().IsRegular() {
		return replaceable{fmt.Errorf("the record %s isn't a file", path)}
	}
	return err
}

// writeTaken records token, taken at taken, as the account's, whole or not
// at all, and private. It keeps the dates of the tokens the record has
// already, as with says, those of one others can read but not write among
// them, which it writes afresh, private, and writes nothing where that
// changes nothing. A record at its own path that it can't go by, having
// looked at it, is replaced; one it couldn't read, or a time a record can't
// hold, as of a clock gone wrong, leaves the record as it is. Through a link,
// it writes only where what the link leads to was read as a record it can
// go by, or one others can only read: anything else there, or nothing, is
// left as it is, as a link to another file of the user's, such as another
// account's token, or one to where a token will go, would otherwise have it
// replaced by a record. Nor, by its own path or through a link, does it write
// over a file that's any account's token or another's record, as kept says.
// The tokens directory's lock must be held, so one record's reading and
// writing waits for another's, as of two writes of a token at once, and
// neither loses the other's.
func (s Store) writeTaken(id string, token Token, taken time.Time) error {
	if !takeable(taken) {
		return fmt.Errorf("a record can't say a token was taken at %s, so it's left as it is", taken.UTC().Format(time.RFC3339))
	}
	path := s.takenPath(id)
	target, linked, err := recordTarget(path)
	if err != nil {
		return err
	}
	s.tightenRecord(path)
	was, exposed, err := s.readRecord(id, true)
	if err := leaveFor(path, linked, err); err != nil {
		return err
	}
	next := was.with(token.Sum(), taken)
	if !exposed && bytes.Equal(next.bytes(), was.bytes()) {
		return nil
	}
	if err := s.kept(id, path, target); err != nil {
		return err
	}
	return atomicfile.Write(target, next.bytes(), 0o600)
}

// kept returns why the record at path, written at target, is left as it is,
// or nil where it's written. It's left where the file at target, however
// it's reached, is any account's token file or another account's record,
// which writing would replace: as where a token file is a link to the
// record's own path, or the record a link to another's. Where nothing's at
// target yet, it's left where another's link leads there, as one leading
// nowhere, to where the record would go, would then lead to the record. The
// account's own record is the entry named for id, whatever its name's case,
// as on a file system that doesn't tell them apart, that's the file at path;
// another account's that's a second name of it is another's.
func (s Store) kept(id, path, target string) error {
	info, err := os.Stat(target)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("the record %s couldn't be looked at, so it's left as it is: %w", path, err)
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return fmt.Errorf("the tokens directory couldn't be read, so the record %s is left as it is: %w", path, err)
	}
	own, _ := os.Lstat(path)
	for _, e := range entries {
		other, record := strings.CutSuffix(e.Name(), takenExt)
		at := filepath.Join(s.dir, e.Name())
		if config.CheckID(other) != nil || record && strings.EqualFold(other, id) && isFile(at, own) || !leadsTo(at, target, info) {
			continue
		}
		what := other + "'s token file"
		if record {
			what = other + "'s record"
		}
		if info == nil {
			return fmt.Errorf("the record %s is where %s leads, so it's left as it is", path, what)
		}
		return fmt.Errorf("the record %s is %s too, so it's left as it is", path, what)
	}
	return nil
}

// isFile reports whether what's at path, not followed where it's a link, is
// the one own describes: false where own is nil.
func isFile(path string, own fs.FileInfo) bool {
	info, err := os.Lstat(path)
	return own != nil && err == nil && os.SameFile(info, own)
}

// leadsTo reports whether at is, or leads to, the file at target, which info
// describes; or, where info is nil, as nothing's at target, whether at is a
// link that leads to target, by the file's name in its directory.
func leadsTo(at, target string, info fs.FileInfo) bool {
	if info != nil {
		o, err := os.Stat(at)
		return err == nil && os.SameFile(info, o)
	}
	l, err := os.Lstat(at)
	if err != nil || l.Mode()&fs.ModeSymlink == 0 {
		return false
	}
	to, err := atomicfile.Target(at)
	return err == nil && sameName(to, target)
}

// sameName reports whether paths a and b name one entry of one directory,
// their directories one by the file, their names alike but for case, as a
// file system may not tell them apart.
func sameName(a, b string) bool {
	if !strings.EqualFold(filepath.Base(a), filepath.Base(b)) {
		return false
	}
	ad, err := os.Stat(filepath.Dir(a))
	if err != nil {
		return false
	}
	bd, err := os.Stat(filepath.Dir(b))
	return err == nil && os.SameFile(ad, bd)
}

// recordTarget returns where the record at path is written: path itself,
// else, where it's a link, where that leads, which it reports linked.
func recordTarget(path string) (target string, linked bool, err error) {
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return path, false, nil
	case err != nil:
		return "", false, err
	case info.Mode()&fs.ModeSymlink == 0:
		return path, false, nil
	}
	target, err = atomicfile.Target(path)
	return target, true, err
}

// leaveFor returns why the record at path, read as err says, is left as it
// is, or nil where it's written: read as one it can go by; or, at its own
// path, not a link, missing, or looked at and found one it can't go by that's
// replaceable.
func leaveFor(path string, linked bool, err error) error {
	_, left := errors.AsType[unreplaceable](err)
	switch {
	case err == nil:
		return nil
	case linked:
		return fmt.Errorf("the record %s is a link to what isn't a record of the user's that others can't write, so it's left as it is: %w", path, err)
	case !mustLeave(err):
		return nil
	case left:
		return fmt.Errorf("%w, so it's left as it is", err)
	}
	return fmt.Errorf("the record couldn't be read, so it's left as it is: %w", err)
}

// mustLeave reports whether a file at a record's own path, read as err says,
// is left as it is, neither written over nor removed: it's unreplaceable, or
// couldn't be read, so it may hold a token.
func mustLeave(err error) bool {
	_, replace := errors.AsType[replaceable](err)
	return err != nil && !replace && !errors.Is(err, ErrUnrecorded)
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
// it has one: of one that's a link, the link alone. A directory in its place
// isn't a record, and is left, empty or not, and so is a file Write would
// leave, as mustLeave says, as it may be a token, with a warning. One left
// behind counts for nothing but the tokens it's of, so failing to remove it
// is only logged.
func (s Store) forgetTaken(id string) {
	path := s.takenPath(id)
	info, err := os.Lstat(path)
	if err != nil || info.IsDir() {
		return
	}
	if info.Mode().IsRegular() {
		if _, _, err := s.readRecord(id, true); mustLeave(err) {
			logger.Warn("left what's where the record of when the token was taken goes", "account", id, "error", err)
			return
		}
	}
	s.removeRecord(id)
}

// removeRecord removes the name at the account's record's own path, never
// what it leads to where it's a link. Failing to is only logged, as a
// warning, as what's left may be a second name of the token removed.
func (s Store) removeRecord(id string) {
	if err := os.Remove(s.takenPath(id)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		logger.Warn("couldn't remove the record of when the token was taken", "account", id, "error", err)
	}
}
