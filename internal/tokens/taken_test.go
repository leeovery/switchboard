package tokens_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/tokens"
)

// workSum is the SHA-256 hash of workToken, in hex, as a record holds it.
var workSum = sumOf(workToken)

// workRecord is the record of workToken, taken at taken, as its file holds it.
var workRecord = "taken 2026-10-27T09:30:00Z\nsha256 " + workSum + "\n"

// recordTaken is a record of workToken, taken at the time given.
func recordTaken(at string) string {
	return "taken " + at + "\nsha256 " + workSum + "\n"
}

func TestWriteRecordsWhenTheTokenWasTaken(t *testing.T) {
	tests := []struct {
		name string
		at   time.Time
	}{
		{name: "in UTC", at: taken},
		{name: "in another zone, to the second", at: taken.Add(400 * time.Millisecond).In(time.FixedZone("east", 3*60*60))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := tokens.NewStore(t.TempDir(), os.Getuid())

			if err := store.Write("work", parse(t, workToken), tt.at); err != nil {
				t.Fatalf("Write() error = %v", err)
			}
			path := store.Path("work") + ".taken"
			checkMode(t, path, 0o600)
			if data, err := os.ReadFile(path); err != nil || string(data) != workRecord {
				t.Errorf("the record holds %q (%v), want %q", data, err, workRecord)
			}
			if at, err := store.Taken("work", parse(t, workToken)); err != nil || !at.Equal(taken) || at.Location() != time.UTC {
				t.Errorf("Taken() = %v, %v; want %v", at, err, taken)
			}
		})
	}
}

// given is a token given at a time, as Write is given it.
type given struct {
	secret string
	at     time.Time
}

func TestATokensDateNeverMoves(t *testing.T) {
	day := func(n int) time.Time { return taken.Add(time.Duration(n) * 24 * time.Hour) }
	tests := []struct {
		name  string
		given []given
		// want is when each token was taken, as the record says once they're
		// given, by the token: zero where it says nothing of one.
		want map[string]time.Time
	}{
		{
			name:  "the same token again, which keeps the date it was first taken",
			given: []given{{workToken, day(0)}, {workToken, day(2)}},
			want:  map[string]time.Time{workToken: day(0)},
		},
		{
			name:  "another token, taken when it's given",
			given: []given{{workToken, day(0)}, {"test-token-renewed", day(2)}},
			want:  map[string]time.Time{workToken: day(0), "test-token-renewed": day(2)},
		},
		{
			name:  "a token back after another, as after one given by mistake",
			given: []given{{workToken, day(0)}, {"test-token-side", day(2)}, {workToken, day(3)}},
			want:  map[string]time.Time{workToken: day(0), "test-token-side": day(2)},
		},
		{
			name:  "the same token given at an earlier time, as after a clock set wrong",
			given: []given{{workToken, day(5)}, {workToken, day(1)}},
			want:  map[string]time.Time{workToken: day(1)},
		},
		{
			name: "more tokens than a record keeps, the one given longest ago forgotten",
			given: []given{
				{"test-token-1", day(1)}, {"test-token-2", day(2)}, {"test-token-3", day(3)}, {"test-token-4", day(4)}, {"test-token-5", day(5)},
				{"test-token-6", day(6)}, {"test-token-7", day(7)}, {"test-token-8", day(8)}, {"test-token-9", day(9)},
			},
			want: map[string]time.Time{
				"test-token-1": {}, "test-token-2": day(2), "test-token-3": day(3), "test-token-4": day(4), "test-token-5": day(5),
				"test-token-6": day(6), "test-token-7": day(7), "test-token-8": day(8), "test-token-9": day(9),
			},
		},
		{
			name: "a token given again kept, as the latest given, while others are forgotten",
			given: []given{
				{"test-token-1", day(1)}, {"test-token-2", day(2)}, {"test-token-3", day(3)}, {"test-token-4", day(4)}, {"test-token-5", day(5)},
				{"test-token-6", day(6)}, {"test-token-7", day(7)}, {"test-token-8", day(8)}, {"test-token-1", day(9)}, {"test-token-9", day(10)},
			},
			want: map[string]time.Time{"test-token-1": day(1), "test-token-2": {}, "test-token-3": day(3), "test-token-9": day(10)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := tokens.NewStore(t.TempDir(), os.Getuid())

			for _, g := range tt.given {
				if err := store.Write("work", parse(t, g.secret), g.at); err != nil {
					t.Fatal(err)
				}
			}
			for secret, want := range tt.want {
				at, err := store.Taken("work", parse(t, secret))
				if !at.Equal(want) || (err == nil) == want.IsZero() {
					t.Errorf("Taken() of %s = %v, %v; want %v", secret, at, err, want)
				}
			}
		})
	}
}

func TestARecordHoldsTheLatestTokenFirst(t *testing.T) {
	store := tokens.NewStore(t.TempDir(), os.Getuid())
	later := taken.Add(48 * time.Hour)
	for _, g := range []given{{workToken, taken}, {"test-token-side", later}, {workToken, later.Add(time.Hour)}} {
		if err := store.Write("work", parse(t, g.secret), g.at); err != nil {
			t.Fatal(err)
		}
	}

	want := workRecord + "taken 2026-10-29T09:30:00Z\nsha256 " + sumOf("test-token-side") + "\n"
	if data, err := os.ReadFile(store.Path("work") + ".taken"); err != nil || string(data) != want {
		t.Errorf("the record holds %q (%v), want %q", data, err, want)
	}
}

func TestARecordRestoredReadableByOthersKeepsItsDates(t *testing.T) {
	later := taken.Add(48 * time.Hour)
	tests := []struct {
		name string
		mode fs.FileMode
		// want is the token's date once it's given again, later.
		want time.Time
	}{
		{name: "readable by others, made private", mode: 0o644, want: taken},
		{name: "writable by others, which can't be gone by", mode: 0o666, want: later},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := tokens.NewStore(t.TempDir(), os.Getuid())
			if err := store.Write("work", parse(t, workToken), taken); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(store.Path("work")+".taken", tt.mode); err != nil {
				t.Fatal(err)
			}

			if err := store.Write("work", parse(t, workToken), later); err != nil {
				t.Fatal(err)
			}
			if at, err := store.Taken("work", parse(t, workToken)); err != nil || !at.Equal(tt.want) {
				t.Errorf("Taken() = %v, %v; want %v", at, err, tt.want)
			}
			checkMode(t, store.Path("work")+".taken", 0o600)
		})
	}
}

func TestTightenMakesTheRecordsPrivate(t *testing.T) {
	tests := []struct {
		name string
		// put puts the record at path, which want is its mode after.
		put  func(t *testing.T, path string)
		want fs.FileMode
	}{
		{name: "a private record", put: recordFile(0o600), want: 0o600},
		{name: "a record others can read, made private", put: recordFile(0o644), want: 0o600},
		{name: "a record its group can read, made private", put: recordFile(0o640), want: 0o600},
		{name: "a record others can write, left as it is", put: recordFile(0o664), want: 0o664},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := t.TempDir()
			store := tokens.NewStore(state, os.Getuid())
			mkdirPrivate(t, store.Dir())
			path := store.Path("work") + ".taken"
			tt.put(t, path)
			// Another account's, alike.
			tt.put(t, store.Path("side")+".taken")

			if _, _, err := store.Tighten(); err != nil {
				t.Fatal(err)
			}
			checkMode(t, path, tt.want)
			checkMode(t, store.Path("side")+".taken", tt.want)
		})
	}
}

func TestTightenLeavesWhereARecordsLinkLeads(t *testing.T) {
	store := tokens.NewStore(t.TempDir(), os.Getuid())
	path := store.Path("work") + ".taken"
	mkdirPrivate(t, filepath.Dir(path))
	linkToRecord(0o644)(t, path)
	target, err := os.Readlink(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := store.Tighten(); err != nil {
		t.Fatal(err)
	}
	checkMode(t, target, 0o644)
}

// recordFile puts a file holding workRecord at path, with mode.
func recordFile(mode fs.FileMode) func(t *testing.T, path string) {
	return func(t *testing.T, path string) {
		t.Helper()
		writeFile(t, path, workRecord, mode)
	}
}

// sumOf is the SHA-256 hash of secret, in hex.
func sumOf(secret string) string {
	s := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(s[:])
}

func TestTakenIsntKnownOfAnotherToken(t *testing.T) {
	tests := []struct {
		name string
		// replace has the account's token file hold the token given, in place
		// of the one written with the record.
		replace func(t *testing.T, store tokens.Store, secret string)
		// want is when the token that replaced it was taken: zero where that
		// isn't known.
		want time.Time
	}{
		{name: "by switchboard, which records it", replace: func(t *testing.T, store tokens.Store, secret string) {
			if err := store.Write("work", parse(t, secret), taken.Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
		}, want: taken.Add(time.Hour)},
		{name: "by something else, which doesn't", replace: func(t *testing.T, store tokens.Store, secret string) {
			writeFile(t, store.Path("work"), secret+"\n", 0o600)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := tokens.NewStore(t.TempDir(), os.Getuid())
			if err := store.Write("work", parse(t, workToken), taken); err != nil {
				t.Fatal(err)
			}

			tt.replace(t, store, "test-token-renewed")
			at, err := store.Taken("work", parse(t, "test-token-renewed"))
			if !at.Equal(tt.want) || (err == nil) == tt.want.IsZero() {
				t.Errorf("Taken() of the token that replaced it = %v, %v; want %v", at, err, tt.want)
			}
		})
	}
}

func TestTakenIsntKnownFromAnUnusableRecord(t *testing.T) {
	const (
		notARecord = "the record %s isn't one of when tokens were taken"
		unrecorded = "no record of when the token was taken"
	)
	tests := []struct {
		name string
		// put puts what's at the record's path, when it's given; otherwise a
		// file holding content, with mode, unless mode is zero.
		put     func(t *testing.T, path string)
		content string
		mode    fs.FileMode
		// otherUser has the record be another user's than the store's.
		otherUser bool
		// wantErr is why Taken doesn't know, %s standing for the record's
		// path: "" where it knows.
		wantErr string
	}{
		{name: "a record of the token", content: workRecord, mode: 0o600},
		{name: "a link to one", put: linkToRecord(0o600)},
		{name: "a record without its last line ending", content: strings.TrimSuffix(workRecord, "\n"), mode: 0o600},
		{name: "a time of the last year it can be", content: recordTaken("9998-12-31T23:59:59Z"), mode: 0o600},
		{name: "missing", wantErr: unrecorded},
		{name: "a link leading nowhere", put: linkToNothing, wantErr: unrecorded},
		{name: "empty", content: "", mode: 0o600, wantErr: notARecord},
		{name: "the time alone", content: "taken 2026-10-27T09:30:00Z\n", mode: 0o600, wantErr: notARecord},
		{name: "the hash alone", content: "sha256 " + workSum + "\n", mode: 0o600, wantErr: notARecord},
		{name: "its lines the other way round", content: "sha256 " + workSum + "\ntaken 2026-10-27T09:30:00Z\n", mode: 0o600, wantErr: notARecord},
		{name: "a line more", content: workRecord + "more\n", mode: 0o600, wantErr: notARecord},
		{name: "a blank line more", content: workRecord + "\n", mode: 0o600, wantErr: notARecord},
		{name: "a time that isn't one", content: recordTaken("last tuesday"), mode: 0o600, wantErr: notARecord},
		{name: "a time without its zone", content: recordTaken("2026-10-27T09:30:00"), mode: 0o600, wantErr: notARecord},
		{name: "the zero time", content: recordTaken("0001-01-01T00:00:00Z"), mode: 0o600, wantErr: notARecord},
		{name: "a time before the first year there is, in UTC", content: recordTaken("0000-01-01T00:00:00+01:00"), mode: 0o600, wantErr: notARecord},
		{name: "a time in the first year there is", content: recordTaken("0001-01-01T00:00:01Z"), mode: 0o600},
		{name: "a hash in uppercase", content: "taken 2026-10-27T09:30:00Z\nsha256 " + strings.ToUpper(workSum) + "\n", mode: 0o600, wantErr: notARecord},
		{name: "the token's date after another's", content: "taken 2026-11-01T00:00:00Z\nsha256 " + sumOf("test-token-side") + "\n" + workRecord, mode: 0o600},
		{name: "a pair cut short", content: workRecord + "taken 2026-11-01T00:00:00Z\n", mode: 0o600, wantErr: notARecord},
		{name: "a pair after the token's that isn't one", content: workRecord + "taken soon\nsha256 " + strings.Repeat("0", 64) + "\n", mode: 0o600, wantErr: notARecord},
		{name: "a token's hash twice", content: workRecord + workRecord, mode: 0o600, wantErr: notARecord},
		{name: "more pairs than a record keeps", content: manyPairs(9), mode: 0o600, wantErr: notARecord},
		{name: "as many pairs as a record keeps", content: manyPairs(7) + workRecord, mode: 0o600},
		{name: "a time in the last year there is", content: recordTaken("9999-01-01T00:00:00Z"), mode: 0o600, wantErr: notARecord},
		{name: "a time in it, in UTC, given in another zone", content: recordTaken("9998-12-31T23:00:00-05:00"), mode: 0o600, wantErr: notARecord},
		{name: "a hash too short to be one", content: "taken 2026-10-27T09:30:00Z\nsha256 " + workSum[:63] + "\n", mode: 0o600, wantErr: notARecord},
		{name: "the token in place of its hash", content: "taken 2026-10-27T09:30:00Z\nsha256 " + workToken + "\n", mode: 0o600, wantErr: notARecord},
		{name: "the hash of another token", content: "taken 2026-10-27T09:30:00Z\nsha256 " + strings.Repeat("0", 64) + "\n", mode: 0o600, wantErr: "the record %s is of other tokens"},
		{name: "readable by others", content: workRecord, mode: 0o644, wantErr: "other users can read the record %s (mode 0644)"},
		{name: "writable by others", content: workRecord, mode: 0o602, wantErr: "other users can write the record %s (mode 0602)"},
		{name: "another user's", content: workRecord, mode: 0o600, otherUser: true, wantErr: "the record %s is another user's"},
		{name: "unreadable", content: workRecord, mode: 0o200, wantErr: "read the record of when the token was taken: open %s: permission denied"},
		{name: "a directory", put: mkdir, wantErr: "the record %s isn't a file"},
		{name: "a link to a record others can read", put: linkToRecord(0o644), wantErr: "other users can read the record %s (mode 0644)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mode != 0 && tt.mode&0o400 == 0 && os.Getuid() == 0 {
				t.Skip("root reads any file")
			}
			uid := os.Getuid()
			if tt.otherUser {
				uid++
			}
			store := tokens.NewStore(t.TempDir(), uid)
			path := store.Path("work") + ".taken"
			mkdirPrivate(t, filepath.Dir(path))
			switch {
			case tt.put != nil:
				tt.put(t, path)
			case tt.mode != 0:
				writeFile(t, path, tt.content, tt.mode)
			}

			at, err := store.Taken("work", parse(t, workToken))
			if tt.wantErr == "" {
				if err != nil || at.IsZero() {
					t.Errorf("Taken() = %v, %v; want it known", at, err)
				}
				return
			}
			if want := strings.ReplaceAll(tt.wantErr, "%s", path); err == nil || err.Error() != want || !at.IsZero() {
				t.Errorf("Taken() = %v, %v; want why it isn't known: %q", at, err, want)
			}
			if unknown := errors.Is(err, tokens.ErrUnrecorded); unknown != (tt.wantErr == unrecorded) {
				t.Errorf("Taken() error is ErrUnrecorded: %v, want %v", unknown, !unknown)
			}
		})
	}
}

func TestAnUnusableRecordLeavesTheTokenUsable(t *testing.T) {
	store := tokens.NewStore(t.TempDir(), os.Getuid())
	if err := store.Write("work", parse(t, workToken), taken); err != nil {
		t.Fatal(err)
	}
	writeFile(t, store.Path("work")+".taken", "taken soon\n", 0o644)

	if token, err := store.Read("work"); err != nil || token.Reveal() != workToken {
		t.Errorf("Read() = %q, %v, want the token, whatever its record", token.Reveal(), err)
	}
}

func TestWriteKeepsTheTokenWhenItCantRecordIt(t *testing.T) {
	tests := []struct {
		name string
		// put puts what keeps the record from being written, given the
		// paths of the token file and the record.
		put func(t *testing.T, token, record string)
	}{
		{name: "a directory where the record goes", put: func(t *testing.T, _, record string) {
			writeFile(t, filepath.Join(record, "kept"), "kept", 0o600)
		}},
		{name: "the record a link to the token file", put: func(t *testing.T, token, record string) {
			writeFile(t, token, "test-token-stale\n", 0o600)
			if err := os.Symlink(token, record); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "the token file a link to the record", put: func(t *testing.T, token, record string) {
			writeFile(t, record, recordTaken("2026-01-01T00:00:00Z"), 0o600)
			if err := os.Symlink(record, token); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			store := tokens.NewStore(t.TempDir(), os.Getuid())
			mkdirPrivate(t, filepath.Dir(store.Path("work")))
			tt.put(t, store.Path("work"), store.Path("work")+".taken")

			if err := store.Write("work", parse(t, workToken), taken); err != nil {
				t.Fatalf("Write() error = %v, want the token written all the same", err)
			}
			if token, err := store.Read("work"); err != nil || token.Reveal() != workToken {
				t.Errorf("Read() = %q, %v, want the token written", token.Reveal(), err)
			}
			if at, err := store.Taken("work", parse(t, workToken)); err == nil {
				t.Errorf("Taken() = %v, want it not known, as the record wasn't written", at)
			}
			if !log.Has("level=WARN", `msg="couldn't record when the token was taken"`, "account=work") {
				t.Errorf("the log reads\n%s\nwant the record's failure", log)
			}
			if strings.Contains(log.String(), workToken) || strings.Contains(log.String(), workSum) {
				t.Errorf("the log shows the token or its hash:\n%s", log)
			}
		})
	}
}

func TestWriteRecordsThroughALink(t *testing.T) {
	store := tokens.NewStore(t.TempDir(), os.Getuid())
	// The record, a link to a file kept elsewhere, as by a dotfiles step.
	target := filepath.Join(t.TempDir(), "secrets", "work.taken")
	writeFile(t, target, "taken soon\n", 0o600)
	mkdirPrivate(t, filepath.Dir(store.Path("work")))
	if err := os.Symlink(target, store.Path("work")+".taken"); err != nil {
		t.Fatal(err)
	}

	if err := store.Write("work", parse(t, workToken), taken); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Readlink(store.Path("work") + ".taken"); err != nil {
		t.Errorf("the record isn't a link still: %v", err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != workRecord {
		t.Errorf("where the link leads holds %q (%v), want %q", data, err, workRecord)
	}
}

func TestRemoveTakesARecordsLinkAlone(t *testing.T) {
	store := tokens.NewStore(t.TempDir(), os.Getuid())
	path := store.Path("work") + ".taken"
	mkdirPrivate(t, filepath.Dir(path))
	linkToRecord(0o600)(t, path)
	target, err := os.Readlink(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.Remove("work"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("once removed, the record: %v, want it gone", err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != workRecord {
		t.Errorf("where the record's link led holds %q (%v), want it as it was", data, err)
	}
}

func TestRemoveKeepsTheRecordOfATokenItCantRemove(t *testing.T) {
	store := tokens.NewStore(t.TempDir(), os.Getuid())
	// A directory, with something in it, where the token file goes.
	writeFile(t, filepath.Join(store.Path("work"), "kept"), "kept", 0o600)
	writeFile(t, store.Path("work")+".taken", workRecord, 0o600)

	if _, err := store.Remove("work"); err == nil {
		t.Fatal("Remove() succeeded, want it to fail removing the token file")
	}
	if data, err := os.ReadFile(store.Path("work") + ".taken"); err != nil || string(data) != workRecord {
		t.Errorf("the record holds %q (%v), want it kept with the token file", data, err)
	}
}

// linkToRecord puts a link at path to a file elsewhere, holding workRecord,
// with mode.
func linkToRecord(mode fs.FileMode) func(t *testing.T, path string) {
	return func(t *testing.T, path string) {
		t.Helper()
		target := filepath.Join(t.TempDir(), "secrets", "work.taken")
		writeFile(t, target, workRecord, mode)
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
	}
}

// manyPairs is a record of n tokens, none of them workToken.
func manyPairs(n int) string {
	var b strings.Builder
	for i := range n {
		b.WriteString("taken 2026-11-01T00:00:00Z\nsha256 " + sumOf(fmt.Sprintf("test-token-%d", i)) + "\n")
	}
	return b.String()
}
