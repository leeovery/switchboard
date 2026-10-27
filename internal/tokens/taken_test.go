package tokens_test

import (
	"crypto/sha256"
	"encoding/hex"
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
var workSum = func() string {
	s := sha256.Sum256([]byte(workToken))
	return hex.EncodeToString(s[:])
}()

// workRecord is the record of workToken, taken at taken, as its file holds it.
var workRecord = "taken 2026-10-27T09:30:00Z\nsha256 " + workSum + "\n"

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
			if at, ok := store.Taken("work", parse(t, workToken)); !ok || !at.Equal(taken) || at.Location() != time.UTC {
				t.Errorf("Taken() = %v, %v; want %v", at, ok, taken)
			}
		})
	}
}

func TestTakenIsTheTokensLatestWrite(t *testing.T) {
	store := tokens.NewStore(t.TempDir(), os.Getuid())
	later := taken.Add(48 * time.Hour)
	if err := store.Write("work", parse(t, workToken), taken); err != nil {
		t.Fatal(err)
	}

	if err := store.Write("work", parse(t, workToken), later); err != nil {
		t.Fatal(err)
	}
	if at, ok := store.Taken("work", parse(t, workToken)); !ok || !at.Equal(later) {
		t.Errorf("Taken() = %v, %v; want %v, when the token was written again", at, ok, later)
	}
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
			at, ok := store.Taken("work", parse(t, "test-token-renewed"))
			if !at.Equal(tt.want) || ok == tt.want.IsZero() {
				t.Errorf("Taken() of the token that replaced it = %v, %v; want %v", at, ok, tt.want)
			}
		})
	}
}

func TestTakenIsntKnownFromAnUnusableRecord(t *testing.T) {
	tests := []struct {
		name string
		// put puts what's at the record's path, when it's given; otherwise a
		// file holding content, with mode, unless mode is zero.
		put     func(t *testing.T, path string)
		content string
		mode    fs.FileMode
		// otherUser has the record be another user's than the store's.
		otherUser bool
		want      bool
	}{
		{name: "a record of the token", content: workRecord, mode: 0o600, want: true},
		{name: "a link to one", put: linkToRecord(0o600), want: true},
		{name: "a record without its line ending", content: strings.TrimSuffix(workRecord, "\n"), mode: 0o600, want: true},
		{name: "missing"},
		{name: "empty", content: "", mode: 0o600},
		{name: "the time alone", content: "taken 2026-10-27T09:30:00Z\n", mode: 0o600},
		{name: "the hash alone", content: "sha256 " + workSum + "\n", mode: 0o600},
		{name: "its lines the other way round", content: "sha256 " + workSum + "\ntaken 2026-10-27T09:30:00Z\n", mode: 0o600},
		{name: "a line more", content: workRecord + "more\n", mode: 0o600},
		{name: "a time that isn't one", content: "taken last tuesday\nsha256 " + workSum + "\n", mode: 0o600},
		{name: "a time without its zone", content: "taken 2026-10-27T09:30:00\nsha256 " + workSum + "\n", mode: 0o600},
		{name: "the hash of another token", content: "taken 2026-10-27T09:30:00Z\nsha256 " + strings.Repeat("0", 64) + "\n", mode: 0o600},
		{name: "the token in place of its hash", content: "taken 2026-10-27T09:30:00Z\nsha256 " + workToken + "\n", mode: 0o600},
		{name: "readable by others", content: workRecord, mode: 0o644},
		{name: "writable by others", content: workRecord, mode: 0o602},
		{name: "another user's", content: workRecord, mode: 0o600, otherUser: true},
		{name: "unreadable", content: workRecord, mode: 0o200},
		{name: "a directory", put: mkdir},
		{name: "a link to a record others can read", put: linkToRecord(0o644)},
		{name: "a link leading nowhere", put: linkToNothing},
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

			at, ok := store.Taken("work", parse(t, workToken))
			if ok != tt.want || (tt.want && !at.Equal(taken)) || (!tt.want && !at.IsZero()) {
				t.Errorf("Taken() = %v, %v; want it known: %v", at, ok, tt.want)
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

func TestTakenLogsNeitherTheTokenNorItsHash(t *testing.T) {
	log := logstest.Capture(t)
	store := tokens.NewStore(t.TempDir(), os.Getuid())
	if err := store.Write("work", parse(t, workToken), taken); err != nil {
		t.Fatal(err)
	}

	if _, ok := store.Taken("work", parse(t, "test-token-renewed")); ok {
		t.Fatal("Taken() of another token is known, want it not")
	}
	writeFile(t, store.Path("work")+".taken", "taken soon\nsha256 "+workSum+"\n", 0o600)
	if _, ok := store.Taken("work", parse(t, workToken)); ok {
		t.Fatal("Taken() of a malformed record is known, want it not")
	}
	if !log.Has("level=DEBUG", `msg="the record of when the token was taken is of another token"`, "account=work") ||
		!log.Has("level=DEBUG", `msg="the record of when the token was taken is unusable"`, "account=work") {
		t.Errorf("the log reads\n%s\nwant why each record didn't count, at debug", log)
	}
	for _, secret := range []string{workToken, "test-token-renewed", workSum} {
		if strings.Contains(log.String(), secret) {
			t.Errorf("the log shows %q:\n%s", secret, log)
		}
	}
}

func TestWriteKeepsTheTokenWhenItCantRecordIt(t *testing.T) {
	log := logstest.Capture(t)
	store := tokens.NewStore(t.TempDir(), os.Getuid())
	// A directory, with something in it, where the record goes.
	writeFile(t, filepath.Join(store.Path("work")+".taken", "kept"), "kept", 0o600)

	if err := store.Write("work", parse(t, workToken), taken); err != nil {
		t.Fatalf("Write() error = %v, want the token written all the same", err)
	}
	if token, err := store.Read("work"); err != nil || token.Reveal() != workToken {
		t.Errorf("Read() = %q, %v, want the token written", token.Reveal(), err)
	}
	if _, ok := store.Taken("work", parse(t, workToken)); ok {
		t.Error("Taken() is known, want it not, as the record wasn't written")
	}
	if !log.Has("level=WARN", `msg="couldn't record when the token was taken"`, "account=work") {
		t.Errorf("the log reads\n%s\nwant the record's failure", log)
	}
	if strings.Contains(log.String(), workToken) || strings.Contains(log.String(), workSum) {
		t.Errorf("the log shows the token or its hash:\n%s", log)
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
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Errorf("once removed, the record: %v, want it gone", err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != workRecord {
		t.Errorf("where the record's link led holds %q (%v), want it as it was", data, err)
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
