package router

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/status"
)

// anyAccount says requests can go out on every account.
func anyAccount(string) bool { return true }

func TestTheStateFileKeepsSessionsAndThePin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	saved := newSessions(at(start))
	saved.load(path, anyAccount)
	saved.setPin(status.Pin{Account: "side", Since: start.Add(-time.Hour), Move: true})
	saved.remember(key{session: "one", model: opus}, "", decision{account: "work", reason: reasonNew}, start.Add(-2*time.Hour))
	saved.remember(key{session: "one", model: opus}, "", decision{account: "work", reason: reasonSticky, sticky: true}, start.Add(-time.Hour))
	saved.remember(key{session: "one", model: haiku}, "side", decision{account: "side", reason: reasonPinned}, start.Add(-time.Minute))
	saved.save()

	loaded := newSessions(at(start))
	loaded.load(path, anyAccount)
	if !maps.Equal(loaded.assignments, saved.assignments) || loaded.pin != saved.pin {
		t.Errorf("loaded\n%+v, pin %+v\nwant what was saved\n%+v, pin %+v", loaded.assignments, loaded.pin, saved.assignments, saved.pin)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "version": 1,
  "pin": {
    "account": "side",
    "since": "2026-09-28T12:12:00Z",
    "move": true
  },
  "sessions": [
    {
      "session": "one",
      "model": "claude-haiku-4-5-20251001",
      "account": "side",
      "pin": "side",
      "reason": "pinned",
      "assigned_at": "2026-09-28T13:11:00Z",
      "last_seen": "2026-09-28T13:11:00Z"
    },
    {
      "session": "one",
      "model": "claude-opus-5-5",
      "account": "work",
      "reason": "new",
      "assigned_at": "2026-09-28T11:12:00Z",
      "last_seen": "2026-09-28T12:12:00Z"
    }
  ]
}
`
	if string(data) != want {
		t.Errorf("state file holds\n%s\nwant\n%s", data, want)
	}
}

func TestLoadingForgetsWhatCantBeUsed(t *testing.T) {
	log := logstest.Capture(t)
	path := filepath.Join(t.TempDir(), "state.json")
	week := 7 * 24 * time.Hour
	writeState(t, path, savedState{
		Version: stateVersion,
		Pin:     status.Pin{Account: "personal", Since: start.Add(-time.Hour)},
		Sessions: []savedAssignment{
			{Session: "recent", Model: opus, Account: "work", LastSeen: start.Add(-week + time.Second)},
			{Session: "old", Model: opus, Account: "work", LastSeen: start.Add(-week)},
			{Session: "gone", Model: opus, Account: "personal", LastSeen: start},
			{Model: opus, Account: "work", LastSeen: start},
		},
	})
	s := newSessions(at(start))

	s.load(path, testAccounts().canSend)
	want := map[key]assignment{{session: "recent", model: opus}: {Account: "work", LastSeen: start.Add(-week + time.Second)}}
	if !maps.Equal(s.assignments, want) {
		t.Errorf("loaded %+v, want %+v alone", s.assignments, want)
	}
	if s.pin != (status.Pin{}) {
		t.Errorf("loaded pin %+v, want none: nothing can go out on its account", s.pin)
	}
	for _, want := range [][]string{
		{"level=WARN", `msg="pin dropped: nothing can go out on its account"`, "account=personal"},
		{"level=INFO", `msg="loaded state"`, "path=" + path, "assignments=1", "pin=\"\""},
	} {
		if !log.Has(want...) {
			t.Errorf("log reads\n%s\nwant a line with %q", log, want)
		}
	}
	if !s.unsaved {
		t.Error("what was forgotten is still in the state file, want it due to be saved")
	}
}

func TestAssignmentsKeepTheirTimesInUTC(t *testing.T) {
	local := start.In(time.FixedZone("UTC+1", 60*60))
	path := filepath.Join(t.TempDir(), "state.json")
	writeState(t, path, savedState{Version: stateVersion, Sessions: []savedAssignment{
		{Session: "saved", Model: opus, Account: "work", Reason: reasonNew, AssignedAt: local, LastSeen: local},
	}})
	s := newSessions(at(local))

	s.load(path, anyAccount)
	s.remember(key{session: "new", model: opus}, "", decision{account: "side", reason: reasonNew}, local)
	for _, id := range []string{"saved", "new"} {
		if got := s.of(id); len(got) != 1 || got[0].AssignedAt != start || got[0].LastSeen != start {
			t.Errorf("session %s is assigned %+v, want its times in UTC", id, got)
		}
	}
}

func TestLoadingSetsACorruptStateFileAside(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantError string
	}{
		{name: "not JSON", content: "{\"version\": 1, \"sessi", wantError: "unexpected end of JSON input"},
		{name: "JSON of another shape", content: `[{"version": 1}]`, wantError: "cannot unmarshal array"},
		{name: "a version unknown", content: `{"version": 2, "sessions": []}`, wantError: "version 2, where 1 is known"},
		{name: "no version", content: `{"sessions": []}`, wantError: "version 0, where 1 is known"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			dir := t.TempDir()
			path := filepath.Join(dir, "state.json")
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
			s := newSessions(at(start))

			s.load(path, anyAccount)
			if len(s.assignments) > 0 || s.pin != (status.Pin{}) {
				t.Errorf("loaded %+v, pin %+v, want nothing", s.assignments, s.pin)
			}
			aside := filepath.Join(dir, fmt.Sprintf("state.json.corrupt-%d", start.Unix()))
			if data, err := os.ReadFile(aside); err != nil || string(data) != tt.content {
				t.Errorf("%s holds %q (%v), want the corrupt file", aside, data, err)
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("state file: %v, want it gone", err)
			}
			want := []string{"level=WARN", `msg="state file corrupt; set aside, starting empty"`, "aside=" + aside, tt.wantError}
			if !log.Has(want...) {
				t.Errorf("log reads\n%s\nwant a line with %q", log, want)
			}
		})
	}
}

func TestLoadingAStateFileThatCantBeRead(t *testing.T) {
	log := logstest.Capture(t)
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	s := newSessions(at(start))

	s.load(path, anyAccount)
	if len(s.assignments) > 0 {
		t.Errorf("loaded %+v, want nothing", s.assignments)
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Errorf("%s: %v, want it left where it is", path, err)
	}
	if !log.Has("level=WARN", `msg="can't read the state file; starting empty"`, "path="+path) {
		t.Errorf("log reads\n%s\nwant the failure to read it", log)
	}
}

func TestABurstOfChangesIsSavedOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state.json")
		s := newSessions(time.Now)
		s.load(path, anyAccount)
		writes := countWrites(s)
		stop := keep(s)

		for i := range 100 {
			s.remember(key{session: fmt.Sprint(i), model: opus}, "", decision{account: "work", reason: reasonNew}, time.Now())
		}
		synctest.Wait()
		if n := writes.Load(); n != 0 {
			t.Errorf("wrote the state file %d times as the changes came, want none until a second has passed", n)
		}
		time.Sleep(saveAfter)
		synctest.Wait()
		if n := writes.Load(); n != 1 {
			t.Errorf("wrote the state file %d times, want once", n)
		}
		if held := readState(t, path); len(held.Sessions) != 100 {
			t.Errorf("state file holds %d sessions, want all 100", len(held.Sessions))
		}
		stop()
		if n := writes.Load(); n != 1 {
			t.Errorf("wrote the state file %d times, want once: nothing changed after", n)
		}
	})
}

func TestStoppingSavesWhatsUnsaved(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state.json")
		s := newSessions(time.Now)
		s.load(path, anyAccount)
		stop := keep(s)
		s.setPin(status.Pin{Account: "side", Since: time.Now()})
		synctest.Wait()

		began := time.Now()
		stop()
		if waited := time.Since(began); waited != 0 {
			t.Errorf("stopping waited %v, want it to save at once", waited)
		}
		if held := readState(t, path); held.Pin.Account != "side" {
			t.Errorf("state file holds pin %+v, want the one just set", held.Pin)
		}
	})
}

func TestSessionsUnusedForAWeekAreForgottenHourly(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state.json")
		s := newSessions(time.Now)
		s.load(path, anyAccount)
		stop := keep(s)
		defer stop()
		s.remember(key{session: "fading", model: opus}, "", decision{account: "work", reason: reasonNew}, time.Now().Add(-7*24*time.Hour+30*time.Minute))
		s.remember(key{session: "recent", model: opus}, "", decision{account: "work", reason: reasonNew}, time.Now())

		time.Sleep(pruneEvery - time.Nanosecond)
		synctest.Wait()
		if len(s.of("fading")) == 0 {
			t.Fatal("forgot a session before the hour was up")
		}
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		if len(s.of("fading")) > 0 || len(s.of("recent")) == 0 {
			t.Errorf("an hour on, sessions are %+v, want the one unused for a week forgotten, and the other kept", s.assignments)
		}
		time.Sleep(saveAfter)
		synctest.Wait()
		if held := readState(t, path); len(held.Sessions) != 1 || held.Sessions[0].Session != "recent" {
			t.Errorf("state file holds %+v, want the recent session alone", held.Sessions)
		}
	})
}

func TestASaveThatFailsIsTriedAgain(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := logstest.Capture(t)
		path := filepath.Join(t.TempDir(), "state.json")
		s := newSessions(time.Now)
		s.load(path, anyAccount)
		var failed atomic.Bool
		s.file.write = func(path string, data []byte) error {
			if failed.CompareAndSwap(false, true) {
				return errors.New("no space left on device")
			}
			return writeAtomic(path, data)
		}
		stop := keep(s)
		s.setPin(status.Pin{Account: "side", Since: time.Now()})

		time.Sleep(saveAfter)
		synctest.Wait()
		if !log.Has("level=WARN", `msg="can't save the state file"`, "path="+path, `error="no space left on device"`) {
			t.Errorf("log reads\n%s\nwant the save's failure", log)
		}
		stop()
		if held := readState(t, path); held.Pin.Account != "side" {
			t.Errorf("state file holds pin %+v, want the one that failed to save the first time", held.Pin)
		}
	})
}

func TestSessionsAreSafeForConcurrentUse(t *testing.T) {
	s := newSessions(at(start))
	var wg sync.WaitGroup
	for i := range 8 {
		k := key{session: fmt.Sprint(i % 2), model: opus}
		wg.Go(func() { s.remember(k, "", decision{account: "work", reason: reasonNew}, start) })
		wg.Go(func() { _, _, _ = s.lookup(k) })
		wg.Go(func() { s.setPin(status.Pin{Account: "side", Since: start}) })
		wg.Go(func() { _ = s.unpin() })
		wg.Go(func() { _ = s.of(k.session) })
		wg.Go(func() { _, _ = s.active(start) })
		wg.Go(func() { s.prune(start) })
	}
	wg.Wait()
}

func TestActiveCountsEachSessionOnceByAccountAndOnceInAll(t *testing.T) {
	s := newSessions(at(start))
	remember := func(session, model, account string, lastSeen time.Time) {
		s.remember(key{session: session, model: model}, "", decision{account: account, reason: reasonNew}, lastSeen)
	}
	remember("one", opus, "work", start)
	remember("one", haiku, "work", start.Add(-time.Hour))
	remember("two", opus, "work", start.Add(-time.Minute))
	remember("two", haiku, "side", start.Add(-time.Minute))
	remember("three", opus, "side", start.Add(-time.Hour-time.Second))

	want := map[string]int{"work": 2, "side": 1}
	byAccount, all := s.active(start)
	if !maps.Equal(byAccount, want) {
		t.Errorf("active() by account = %v, want %v: sessions used within the hour, each once an account", byAccount, want)
	}
	if all != 2 {
		t.Errorf("active() in all = %d, want 2: two's models went to two accounts, and it counts once", all)
	}
}

func TestOfListsASessionsAssignmentsTheOneUsedLastFirst(t *testing.T) {
	s := newSessions(at(start))
	s.remember(key{session: "one", model: haiku}, "", decision{account: "side", reason: reasonNew}, start.Add(-time.Hour))
	s.remember(key{session: "one", model: opus}, "work", decision{account: "work", reason: reasonPinned}, start)
	s.remember(key{session: "two", model: opus}, "", decision{account: "side", reason: reasonNew}, start)

	want := []Assignment{
		{Model: opus, Account: "work", Pinned: true, Reason: "pinned", AssignedAt: start, LastSeen: start},
		{Model: haiku, Account: "side", Reason: "new", AssignedAt: start.Add(-time.Hour), LastSeen: start.Add(-time.Hour)},
	}
	if got := s.of("one"); !reflect.DeepEqual(got, want) {
		t.Errorf("of() =\n%+v\nwant\n%+v", got, want)
	}
	if got := s.of("nope"); got != nil {
		t.Errorf("of() a session never seen = %+v, want none", got)
	}
}

// keep has s keep its state file until the stop it returns is called, which
// waits for it to finish.
func keep(s *sessions) (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())
	var kept sync.WaitGroup
	kept.Go(func() { s.keep(ctx) })
	return func() {
		cancel()
		kept.Wait()
	}
}

// countWrites counts the writes of s's state file from now on.
func countWrites(s *sessions) *atomic.Int32 {
	var writes atomic.Int32
	write := s.file.write
	s.file.write = func(path string, data []byte) error {
		writes.Add(1)
		return write(path, data)
	}
	return &writes
}

// writeState writes saved to the state file at path.
func writeState(t *testing.T, path string, saved savedState) {
	t.Helper()
	if err := (&stateFile{path: path, write: writeAtomic}).save(saved); err != nil {
		t.Fatal(err)
	}
}

// readState reads the state file at path.
func readState(t *testing.T, path string) savedState {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := parseState(data)
	if err != nil {
		t.Fatal(err)
	}
	return saved
}
