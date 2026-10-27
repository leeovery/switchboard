package tokens_test

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/tokens"
)

func TestRemoveLeavesTheRecordOnlyWhereTheTokenFileLinksToIt(t *testing.T) {
	tests := []struct {
		name string
		// put puts the token file at token and the record at record, beside
		// it, and returns the file elsewhere either leads to, if any.
		put func(t *testing.T, token, record string) string
		// wantRecord is whether the record's path is there still.
		wantRecord bool
	}{
		{name: "the token file a link to the record", wantRecord: true, put: func(t *testing.T, token, record string) string {
			writeFile(t, record, workToken+"\n", 0o600)
			symlink(t, record, token)
			return ""
		}},
		{name: "the record a link to the token file", put: func(t *testing.T, token, record string) string {
			writeFile(t, token, workToken+"\n", 0o600)
			symlink(t, token, record)
			return ""
		}},
		{name: "both links to one file elsewhere", put: func(t *testing.T, token, record string) string {
			elsewhere := filepath.Join(t.TempDir(), "secrets", "work")
			writeFile(t, elsewhere, workToken+"\n", 0o600)
			symlink(t, elsewhere, token)
			symlink(t, elsewhere, record)
			return elsewhere
		}},
		{name: "the token file a link to a file elsewhere the record is a second name of", put: func(t *testing.T, token, record string) string {
			elsewhere := filepath.Join(t.TempDir(), "secrets", "work")
			writeFile(t, elsewhere, workToken+"\n", 0o600)
			symlink(t, elsewhere, token)
			if err := os.Link(elsewhere, record); err != nil {
				t.Fatal(err)
			}
			return elsewhere
		}},
		{name: "the token file a link to a link to the record", wantRecord: true, put: func(t *testing.T, token, record string) string {
			writeFile(t, record, workToken+"\n", 0o600)
			between := filepath.Join(t.TempDir(), "work")
			symlink(t, record, between)
			symlink(t, between, token)
			return ""
		}},
		{name: "the token file a link to the record, by a name spelled in other cases", wantRecord: true, put: func(t *testing.T, token, record string) string {
			writeFile(t, record, workToken+"\n", 0o600)
			upper := filepath.Join(filepath.Dir(record), strings.ToUpper(filepath.Base(record)))
			if _, err := os.Stat(upper); err != nil {
				t.Skipf("names here are told apart by case: %v", err)
			}
			symlink(t, upper, token)
			return ""
		}},
		{name: "the token file a relative link to the record", wantRecord: true, put: func(t *testing.T, token, record string) string {
			writeFile(t, record, workToken+"\n", 0o600)
			symlink(t, filepath.Base(record), token)
			return ""
		}},
		{name: "the token file a link to the record through a link to its directory", wantRecord: true, put: func(t *testing.T, token, record string) string {
			writeFile(t, record, workToken+"\n", 0o600)
			dir := filepath.Join(t.TempDir(), "tokens")
			symlink(t, filepath.Dir(record), dir)
			symlink(t, filepath.Join(dir, filepath.Base(record)), token)
			return ""
		}},
		{name: "both names of one file", put: func(t *testing.T, token, record string) string {
			writeFile(t, token, workToken+"\n", 0o600)
			if err := os.Link(token, record); err != nil {
				t.Fatal(err)
			}
			return ""
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := tokens.NewStore(t.TempDir(), os.Getuid())
			record := store.Path("work") + ".taken"
			mkdirPrivate(t, store.Dir())
			elsewhere := tt.put(t, store.Path("work"), record)

			if _, err := store.Remove("work"); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Lstat(record); errors.Is(err, fs.ErrNotExist) == tt.wantRecord {
				t.Errorf("once removed, the record: %v, want it there: %v", err, tt.wantRecord)
			}
			if tt.wantRecord {
				if data, err := os.ReadFile(record); err != nil || string(data) != workToken+"\n" {
					t.Errorf("where the token file led holds %q (%v), want it as it was", data, err)
				}
			}
			if elsewhere == "" {
				return
			}
			if data, err := os.ReadFile(elsewhere); err != nil || string(data) != workToken+"\n" {
				t.Errorf("the file elsewhere holds %q (%v), want it as it was", data, err)
			}
			if err := store.Write("work", parse(t, workToken), taken); err != nil {
				t.Fatal(err)
			}
			if data, err := os.ReadFile(elsewhere); err != nil || string(data) != workToken+"\n" {
				t.Errorf("once the token's written again, the file elsewhere holds %q (%v), want it as it was", data, err)
			}
		})
	}
}

func TestTightenTouchesNothingButTheAccountsRecords(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	store := tokens.NewStore(state, os.Getuid())
	mkdirPrivate(t, store.Dir())
	// Each name ending in .taken, as no id does, and what reading it as one
	// would have led to, outside the tokens directory.
	odd := []string{".taken", "..taken", "...taken", "work.old.taken"}
	outside := []string{store.Dir() + ".taken", state + ".taken"}
	for _, name := range odd {
		writeFile(t, filepath.Join(store.Dir(), name), workRecord, 0o644)
	}
	for _, path := range outside {
		writeFile(t, path, workRecord, 0o644)
	}
	// A record by a second name of a file elsewhere.
	elsewhere := filepath.Join(t.TempDir(), "kept")
	writeFile(t, elsewhere, workRecord, 0o644)
	if err := os.Link(elsewhere, store.Path("side")+".taken"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, store.Path("work")+".taken", workRecord, 0o644)

	if _, _, err := store.Tighten(); err != nil {
		t.Fatal(err)
	}
	checkMode(t, store.Path("work")+".taken", 0o600)
	for _, path := range append(outside, elsewhere) {
		checkMode(t, path, 0o644)
	}
	for _, name := range odd {
		checkMode(t, filepath.Join(store.Dir(), name), 0o644)
	}
}

func TestTightenPassesOverAnotherUsersRecord(t *testing.T) {
	state := t.TempDir()
	writeFile(t, filepath.Join(state, "tokens", "work.taken"), workRecord, 0o644)

	if _, _, err := tokens.NewStore(state, os.Getuid()+1).Tighten(); err != nil {
		t.Fatal(err)
	}
	checkMode(t, filepath.Join(state, "tokens", "work.taken"), 0o644)
}

func TestWriteLeavesTheRecordForATimeItCantHold(t *testing.T) {
	tests := []struct {
		name string
		at   time.Time
	}{
		{name: "the zero time", at: time.Time{}},
		{name: "a time past the years a record holds", at: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)},
		{name: "a time before them", at: time.Date(0, 6, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			store := tokens.NewStore(t.TempDir(), os.Getuid())
			if err := store.Write("work", parse(t, workToken), taken); err != nil {
				t.Fatal(err)
			}

			if err := store.Write("work", parse(t, "test-token-renewed"), tt.at); err != nil {
				t.Fatalf("Write() error = %v, want the token written all the same", err)
			}
			if token, err := store.Read("work"); err != nil || token.Reveal() != "test-token-renewed" {
				t.Errorf("Read() = %q, %v, want the token written", token.Reveal(), err)
			}
			if data, err := os.ReadFile(store.Path("work") + ".taken"); err != nil || string(data) != workRecord {
				t.Errorf("the record holds %q (%v), want it as it was", data, err)
			}
			if !log.Has("level=WARN", `msg="couldn't record when the token was taken"`, "account=work") {
				t.Errorf("the log reads\n%s\nwant why the record wasn't written", log)
			}
		})
	}
}

func TestWriteLeavesARecordItCantRead(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root reads any file")
	}
	log := logstest.Capture(t)
	store := tokens.NewStore(t.TempDir(), os.Getuid())
	writeFile(t, store.Path("work")+".taken", workRecord, 0o200)

	if err := store.Write("work", parse(t, "test-token-renewed"), taken); err != nil {
		t.Fatalf("Write() error = %v, want the token written all the same", err)
	}
	if err := os.Chmod(store.Path("work")+".taken", 0o600); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(store.Path("work") + ".taken"); err != nil || string(data) != workRecord {
		t.Errorf("the record holds %q (%v), want it as it was", data, err)
	}
	if !log.Has("level=WARN", `msg="couldn't record when the token was taken"`, "account=work") {
		t.Errorf("the log reads\n%s\nwant why the record wasn't written", log)
	}
}

func TestWriteChangesNothingWhereTheRecordSaysSoAlready(t *testing.T) {
	store := tokens.NewStore(t.TempDir(), os.Getuid())
	path := store.Path("work") + ".taken"
	writeFile(t, path, workRecord, 0o400)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Write("work", parse(t, workToken), taken.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || after.Mode().Perm() != 0o400 || !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("the record is %v, %v, want the file it was, as it was, mode 0400", after.Mode(), after.ModTime())
	}
}

func TestWritesAtOnceKeepEachTokensDate(t *testing.T) {
	store := tokens.NewStore(t.TempDir(), os.Getuid())
	if err := store.Write("work", parse(t, workToken), taken); err != nil {
		t.Fatal(err)
	}
	secrets := make([]string, 7)
	for i := range secrets {
		secrets[i] = fmt.Sprintf("test-token-%d", i)
	}

	var wg sync.WaitGroup
	for _, secret := range secrets {
		token := parse(t, secret)
		wg.Go(func() {
			if err := store.Write("work", token, taken.Add(time.Hour)); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	for _, secret := range append(secrets, workToken) {
		if _, err := store.Taken("work", parse(t, secret)); err != nil {
			t.Errorf("Taken() of %s: %v, want its date kept", secret, err)
		}
	}
	if data, err := os.ReadFile(store.Path("work") + ".taken"); err != nil || strings.Count(string(data), "\n") != 16 {
		t.Errorf("the record holds %q (%v), want 8 pairs of lines", data, err)
	}
}

func TestARecordByALinkReadableByOthersKeepsItsDates(t *testing.T) {
	sidePair := "taken 2026-10-28T09:30:00Z\nsha256 " + sumOf("test-token-side") + "\n"
	links := []struct {
		name string
		// link names the file elsewhere at path, as its record, and check
		// checks what's at each once the token is written: elsewhere, which
		// held was, and the record, which should hold want.
		link  func(t *testing.T, elsewhere, path string)
		check func(t *testing.T, elsewhere, path, was, want string)
	}{
		{
			name: "a link to a copy",
			link: func(t *testing.T, elsewhere, path string) { symlink(t, elsewhere, path) },
			check: func(t *testing.T, elsewhere, path, _, want string) {
				checkLink(t, path, elsewhere)
				checkFile(t, elsewhere, want, 0o600)
			},
		},
		{
			name: "a second name of a copy",
			link: func(t *testing.T, elsewhere, path string) {
				if err := os.Link(elsewhere, path); err != nil {
					t.Fatal(err)
				}
			},
			check: func(t *testing.T, elsewhere, path, was, want string) {
				checkFile(t, elsewhere, was, 0o644)
				if info, err := os.Lstat(path); err != nil || !info.Mode().IsRegular() {
					t.Errorf("the record is %v (%v), want a file of its own", info, err)
				}
				checkFile(t, path, want, 0o600)
			},
		},
	}
	restored := []struct {
		name string
		// was is the copy, and want the record once the token's written.
		was, want string
	}{
		{name: "the token after another", was: sidePair + workRecord, want: workRecord + sidePair},
		{name: "the token first already", was: workRecord + sidePair, want: workRecord + sidePair},
	}
	for _, l := range links {
		for _, r := range restored {
			t.Run(l.name+", "+r.name, func(t *testing.T) {
				store := tokens.NewStore(t.TempDir(), os.Getuid())
				path := store.Path("work") + ".taken"
				mkdirPrivate(t, store.Dir())
				// Restored as a dotfiles checkout leaves it, readable by others.
				elsewhere := filepath.Join(t.TempDir(), "dotfiles", "work.taken")
				writeFile(t, elsewhere, r.was, 0o644)
				l.link(t, elsewhere, path)

				if err := store.Write("work", parse(t, workToken), taken.Add(30*24*time.Hour)); err != nil {
					t.Fatal(err)
				}
				for secret, want := range map[string]time.Time{workToken: taken, "test-token-side": taken.Add(24 * time.Hour)} {
					if at, err := store.Taken("work", parse(t, secret)); err != nil || !at.Equal(want) {
						t.Errorf("Taken() of %s = %v, %v; want %v, as the copy had it", secret, at, err, want)
					}
				}
				l.check(t, elsewhere, path, r.was, r.want)
			})
		}
	}
}

func TestWriteLeavesWhatALinkLeadsToThatIsntARecordToGoBy(t *testing.T) {
	tests := []struct {
		name string
		// put puts the record at record, a link, and returns where it leads,
		// with what's there and its mode, a mode of 0 where nothing is, and
		// holds "" where what's there isn't a file.
		put func(t *testing.T, store tokens.Store, record string) (target, holds string, mode fs.FileMode)
		// another is set where the store's user is another than the one
		// whose files these are.
		another bool
	}{
		{name: "another account's token", put: func(t *testing.T, store tokens.Store, record string) (string, string, fs.FileMode) {
			writeFile(t, store.Path("personal"), "test-token-personal\n", 0o600)
			symlink(t, store.Path("personal"), record)
			return store.Path("personal"), "test-token-personal\n", 0o600
		}},
		{name: "a file of the user's that isn't a record", put: func(t *testing.T, _ tokens.Store, record string) (string, string, fs.FileMode) {
			target := filepath.Join(t.TempDir(), "state.json")
			writeFile(t, target, "{}\n", 0o600)
			symlink(t, target, record)
			return target, "{}\n", 0o600
		}},
		{name: "nothing", put: func(t *testing.T, _ tokens.Store, record string) (string, string, fs.FileMode) {
			target := filepath.Join(t.TempDir(), "work.taken")
			symlink(t, target, record)
			return target, "", 0
		}},
		{name: "a record others can write", put: func(t *testing.T, _ tokens.Store, record string) (string, string, fs.FileMode) {
			target := filepath.Join(t.TempDir(), "work.taken")
			writeFile(t, target, workRecord, 0o666)
			symlink(t, target, record)
			return target, workRecord, 0o666
		}},
		{name: "another user's record", another: true, put: func(t *testing.T, _ tokens.Store, record string) (string, string, fs.FileMode) {
			target := filepath.Join(t.TempDir(), "work.taken")
			writeFile(t, target, workRecord, 0o600)
			symlink(t, target, record)
			return target, workRecord, 0o600
		}},
		{name: "a socket", put: func(t *testing.T, _ tokens.Store, record string) (string, string, fs.FileMode) {
			// Not in t.TempDir(), whose path is too long for a socket's.
			dir, err := os.MkdirTemp("", "sock")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(dir) })
			target := filepath.Join(dir, "work")
			l, err := net.Listen("unix", target)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = l.Close() })
			symlink(t, target, record)
			info, err := os.Lstat(target)
			if err != nil {
				t.Fatal(err)
			}
			return target, "", info.Mode()
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			uid := os.Getuid()
			if tt.another {
				uid++
			}
			store := tokens.NewStore(t.TempDir(), uid)
			mkdirPrivate(t, store.Dir())
			record := store.Path("work") + ".taken"
			target, holds, mode := tt.put(t, store, record)

			if err := store.Write("work", parse(t, "test-token-renewed"), taken); err != nil {
				t.Fatalf("Write() error = %v, want the token written all the same", err)
			}
			if data, err := os.ReadFile(store.Path("work")); err != nil || string(data) != "test-token-renewed\n" {
				t.Errorf("the token file holds %q (%v), want the token", data, err)
			}
			checkLink(t, record, target)
			switch info, err := os.Lstat(target); {
			case mode == 0:
				if !errors.Is(err, fs.ErrNotExist) {
					t.Errorf("where the link leads: %v, %v; want nothing there still", info, err)
				}
			case err != nil || info.Mode() != mode:
				t.Errorf("where the link leads is %v (%v), want %v, as it was", info, err, mode)
			case holds != "":
				checkFile(t, target, holds, mode)
			}
			if !log.Has("level=WARN", `msg="couldn't record when the token was taken"`, "account=work") {
				t.Errorf("the log reads\n%s\nwant why the record wasn't written", log)
			}
			if strings.Contains(log.String(), "test-token-renewed") || strings.Contains(log.String(), sumOf("test-token-renewed")) {
				t.Errorf("the log shows the token or its hash:\n%s", log)
			}
		})
	}
}

func TestWriteNeverWritesARecordOverAnotherAccountsFile(t *testing.T) {
	personalRecord := "taken 2026-10-01T00:00:00Z\nsha256 " + sumOf("test-token-personal") + "\n"
	tests := []struct {
		name string
		// put puts personal's files, and work's record at record, given the
		// path of personal's token file, and returns personal's record's
		// path, where it has one, and what the file it leads to holds.
		put func(t *testing.T, personal, record string) (personalRecord, holds string)
	}{
		{name: "another account's token file a link to the record's own path", put: func(t *testing.T, personal, record string) (string, string) {
			writeFile(t, record, "test-token-personal\n", 0o600)
			symlink(t, record, personal)
			return "", ""
		}},
		{name: "the record a second name of another account's token file", put: func(t *testing.T, personal, record string) (string, string) {
			writeFile(t, personal, "test-token-personal\n", 0o600)
			if err := os.Link(personal, record); err != nil {
				t.Fatal(err)
			}
			return "", ""
		}},
		{name: "the record a link to another account's record", put: func(t *testing.T, personal, record string) (string, string) {
			writeFile(t, personal, "test-token-personal\n", 0o600)
			writeFile(t, personal+".taken", personalRecord, 0o600)
			symlink(t, personal+".taken", record)
			return personal + ".taken", personalRecord
		}},
		{name: "another account's record a link to the record's own path", put: func(t *testing.T, personal, record string) (string, string) {
			writeFile(t, personal, "test-token-personal\n", 0o600)
			writeFile(t, record, personalRecord, 0o600)
			symlink(t, record, personal+".taken")
			return personal + ".taken", personalRecord
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			store := tokens.NewStore(t.TempDir(), os.Getuid())
			mkdirPrivate(t, store.Dir())
			record := store.Path("work") + ".taken"
			otherRecord, holds := tt.put(t, store.Path("personal"), record)

			if err := store.Write("work", parse(t, workToken), taken); err != nil {
				t.Fatalf("Write() error = %v, want the token written all the same", err)
			}
			if token, err := store.Read("work"); err != nil || token.Reveal() != workToken {
				t.Errorf("Read() of work = %q, %v, want the token written", token.Reveal(), err)
			}
			if token, err := store.Read("personal"); err != nil || token.Reveal() != "test-token-personal" {
				t.Errorf("Read() of personal = %q, %v, want its token as it was", token.Reveal(), err)
			}
			if otherRecord != "" {
				if data, err := os.ReadFile(otherRecord); err != nil || string(data) != holds {
					t.Errorf("personal's record holds %q (%v), want it as it was, %q", data, err, holds)
				}
			}
			if at, err := store.Taken("work", parse(t, workToken)); err == nil {
				t.Errorf("Taken() = %v, want it not known, as the record wasn't written", at)
			}
			if !log.Has("level=WARN", `msg="couldn't record when the token was taken"`, "account=work", "personal") {
				t.Errorf("the log reads\n%s\nwant why the record wasn't written, naming personal", log)
			}
			if strings.Contains(log.String(), workToken) || strings.Contains(log.String(), "test-token-personal") {
				t.Errorf("the log shows a token:\n%s", log)
			}
		})
	}
}

func TestRemoveLeavesADirectoryWhereTheRecordGoes(t *testing.T) {
	for _, kept := range []bool{false, true} {
		t.Run(fmt.Sprintf("holding something: %v", kept), func(t *testing.T) {
			store := tokens.NewStore(t.TempDir(), os.Getuid())
			record := store.Path("work") + ".taken"
			mkdirPrivate(t, record)
			if kept {
				writeFile(t, filepath.Join(record, "kept"), "kept", 0o600)
			}
			writeFile(t, store.Path("work"), workToken+"\n", 0o600)

			if _, err := store.Remove("work"); err != nil {
				t.Fatal(err)
			}
			if info, err := os.Lstat(record); err != nil || !info.IsDir() {
				t.Errorf("once removed, where the record goes is %v (%v), want the directory left", info, err)
			}
		})
	}
}

func TestWriteReplacesARecordOfAnotherUsersItCantOpen(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root reads any file")
	}
	// The store's user is another than the one the record is made by, as of
	// one root wrote, which the user can't open.
	store := tokens.NewStore(t.TempDir(), os.Getuid()+1)
	writeFile(t, store.Path("work")+".taken", workRecord, 0o200)

	if err := store.Write("work", parse(t, "test-token-renewed"), taken); err != nil {
		t.Fatal(err)
	}
	want := "taken 2026-10-27T09:30:00Z\nsha256 " + sumOf("test-token-renewed") + "\n"
	if data, err := os.ReadFile(store.Path("work") + ".taken"); err != nil || string(data) != want {
		t.Errorf("the record holds %q (%v), want it written afresh, %q", data, err, want)
	}
}

// checkFile checks the file at path holds want, with mode.
func checkFile(t *testing.T, path, want string, mode fs.FileMode) {
	t.Helper()
	if data, err := os.ReadFile(path); err != nil || string(data) != want {
		t.Errorf("%s holds %q (%v), want %q", path, data, err, want)
	}
	checkMode(t, path, mode)
}

// symlink puts a link at path to target.
func symlink(t *testing.T, target, path string) {
	t.Helper()
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}
