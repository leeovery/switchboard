package router

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens"
	"github.com/leeovery/switchboard/internal/tokens/tokenstest"
)

func TestTheStateFileKeepsSessionsTheirPinsAndTheGlobalPin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	saved := newSessions(at(start))
	saved.load(path, testAccounts())
	saved.setPin(status.Pin{Account: "side", Since: start.Add(-time.Hour), Move: true}, false)
	assign(saved, key{session: "one", model: opus}, "", decision{account: "work", reason: reasonNew}, start.Add(-2*time.Hour))
	assign(saved, key{session: "one", model: opus}, "", decision{account: "work", reason: reasonSticky, sticky: true}, start.Add(-time.Hour))
	assign(saved, key{session: "one", model: haiku}, "side", decision{account: "side", reason: reasonPinned}, start.Add(-time.Minute))
	assign(saved, key{session: "two", model: opus}, "work", decision{account: "work", reason: reasonPinned}, start.Add(-time.Minute))
	saved.pinSession("one", "work")
	saved.pinSession("two", "")
	saved.save()

	loaded := newSessions(at(start))
	loaded.load(path, testAccounts())
	if !maps.Equal(loaded.assignments, saved.assignments) || !maps.Equal(loaded.own, saved.own) || loaded.pin != saved.pin {
		t.Errorf("loaded\n%+v, pins %+v, pin %+v\nwant what was saved\n%+v, pins %+v, pin %+v",
			loaded.assignments, loaded.own, loaded.pin, saved.assignments, saved.own, saved.pin)
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
    },
    {
      "session": "two",
      "model": "claude-opus-5-5",
      "account": "work",
      "pin": "work",
      "reason": "pinned",
      "assigned_at": "2026-09-28T13:11:00Z",
      "last_seen": "2026-09-28T13:11:00Z"
    }
  ],
  "session_pins": {
    "one": {
      "account": "work",
      "since": "2026-09-28T13:12:00Z"
    },
    "two": {
      "account": "",
      "since": "2026-09-28T13:12:00Z"
    }
  },
  "tokens": {
    "side": {
      "sha256": "` + sideHash + `"
    },
    "work": {
      "sha256": "` + workHash + `"
    }
  }
}
`
	if string(data) != want {
		t.Errorf("state file holds\n%s\nwant\n%s", data, want)
	}
}

// The SHA-256 hashes of the tests' tokens, in hex.
const (
	workHash    = "cf5a3479fe31250e84a05b39f73fdc23bc42e2b218305ca4267d1dd51f2fc4ee"
	sideHash    = "bb48055ab8f9aa9ce2af6e2ea63f848a615f9bdf804f9ad23ee263a480a2461b"
	renewedHash = "e92da524a8f250cefed3757cbc69aa1f819a7b03f4b1c208094351ddf7b49927"
)

// renewedToken is work's token once it's replaced.
const renewedToken = "test-token-work-renewed"

func TestTheStateFileKeepsTheAccountsFormerTokensAsHashesForAWeek(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	saving := newSessions(at(start))
	accounts := testAccounts()
	saving.load(path, accounts)
	work, _ := accounts.byID("work")
	if !work.secret.replace(mustToken(t, renewedToken), start) {
		t.Fatal("replace() = false, want work's token replaced")
	}
	saving.tokensChanged()
	saving.save()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{workToken, sideToken, renewedToken} {
		if strings.Contains(string(data), token) {
			t.Errorf("state file holds\n%s\nwant no token in it", data)
		}
	}
	want := map[string]savedTokens{
		"work": {SHA256: renewedHash, Former: []formerToken{{SHA256: workHash, ReplacedAt: start}}},
		"side": {SHA256: sideHash},
	}
	if held := readState(t, path); !maps.EqualFunc(held.Tokens, want, savedTokens.equal) {
		t.Errorf("state file holds tokens %+v, want %+v", held.Tokens, want)
	}

	// Restarted, reading work's renewed token from its file.
	tests := []struct {
		name  string
		after time.Duration
		want  bool
	}{
		{name: "a moment on", after: time.Minute, want: true},
		{name: "just short of a week on", after: formerFor - time.Second, want: true},
		{name: "a week on", after: formerFor, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restarted := resolve(testConfigured, tokenstest.Files{"work": renewedToken, "side": sideToken}.Read)
			loaded := newSessions(at(start.Add(tt.after)))
			loaded.load(path, restarted)

			a, ok := restarted.byToken(workToken, start.Add(tt.after))
			if got := ok && a.ID == "work"; got != tt.want {
				t.Errorf("work's former token taken for work's %v on: %v, want %v", tt.after, got, tt.want)
			}
		})
	}
}

func TestATokenReplacedWhileTheRouterWasAwayStillCountsAsItsAccounts(t *testing.T) {
	log := logstest.Capture(t)
	path := filepath.Join(t.TempDir(), "state.json")
	before := newSessions(at(start))
	before.load(path, testAccounts())
	before.save()

	later := start.Add(time.Hour)
	restarted := resolve(testConfigured, tokenstest.Files{"work": renewedToken, "side": sideToken}.Read)
	s := newSessions(at(later))
	s.load(path, restarted)
	for token, want := range map[string]string{renewedToken: "work", workToken: "work", sideToken: "side"} {
		if a, ok := restarted.byToken(token, later); !ok || a.ID != want {
			t.Errorf("byToken() of %s's token = %q, %v, want %s", want, a.ID, ok, want)
		}
	}
	if !s.unsaved {
		t.Error("work's former token isn't due to be saved")
	}
	if !log.Has("level=INFO", `msg="token replaced while the router was away; the one before still counts as the account's"`, "account=work") {
		t.Errorf("log reads\n%s\nwant work's token noted as replaced", log)
	}
	if strings.Contains(log.String(), "account=side") {
		t.Errorf("log reads\n%s\nwant nothing of side, whose token stands", log)
	}
}

func TestLoadingAStateFileThatKnowsTheTokensChangesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	first := newSessions(at(start))
	first.load(path, testAccounts())
	first.save()

	s := newSessions(at(start.Add(time.Hour)))
	s.load(path, testAccounts())
	if s.unsaved {
		t.Error("loading the state file left it due to be saved, want it as it was")
	}
}

func TestFormerTokensAreForgottenHourlyAWeekAfterTheyWereReplaced(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state.json")
		s := newSessions(time.Now)
		accounts := testAccounts()
		s.load(path, accounts)
		work, _ := accounts.byID("work")
		work.secret.replace(mustToken(t, renewedToken), time.Now().Add(-formerFor+30*time.Minute))
		s.tokensChanged()
		stop := keep(s)
		defer stop()

		time.Sleep(saveAfter)
		synctest.Wait()
		if held := readState(t, path); len(held.Tokens["work"].Former) != 1 {
			t.Fatalf("state file holds work's tokens as %+v, want its former token kept while it counts", held.Tokens["work"])
		}
		time.Sleep(pruneEvery)
		synctest.Wait()
		if held := readState(t, path); len(held.Tokens["work"].Former) > 0 {
			t.Errorf("an hour on, the state file holds work's tokens as %+v, want its former token forgotten", held.Tokens["work"])
		}
	})
}

// mustToken is secret as a token.
func mustToken(t *testing.T, secret string) tokens.Token {
	t.Helper()
	token, err := tokens.Parse(secret)
	if err != nil {
		t.Fatal(err)
	}
	return token
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
			{Session: "pinned", Model: opus, Account: "work", LastSeen: start},
			{Session: "old", Model: opus, Account: "work", LastSeen: start.Add(-week)},
			{Session: "gone", Model: opus, Account: "personal", LastSeen: start},
			{Model: opus, Account: "work", LastSeen: start},
		},
		SessionPins: map[string]ownPin{
			"pinned": {Account: "side", Since: start},
			"recent": {Account: "personal", Since: start},
			"old":    {Account: "side", Since: start},
		},
	})
	s := newSessions(at(start))

	s.load(path, testAccounts())
	want := map[key]assignment{
		{session: "recent", model: opus}: {Account: "work", LastSeen: start.Add(-week + time.Second)},
		{session: "pinned", model: opus}: {Account: "work", LastSeen: start},
	}
	if !maps.Equal(s.assignments, want) {
		t.Errorf("loaded %+v, want %+v alone", s.assignments, want)
	}
	if want := map[string]ownPin{"pinned": {Account: "side", Since: start}}; !maps.Equal(s.own, want) {
		t.Errorf("loaded sessions' pins %+v, want %+v alone: the others' sessions or accounts are gone", s.own, want)
	}
	if s.pin != (status.Pin{}) {
		t.Errorf("loaded pin %+v, want none: nothing can go out on its account", s.pin)
	}
	for _, want := range [][]string{
		{"level=WARN", `msg="pin dropped: nothing can go out on its account"`, "account=personal"},
		{"level=WARN", `msg="session's pin dropped: nothing can go out on its account"`, "session=recent", "account=personal"},
		{"level=INFO", `msg="loaded state"`, "path=" + path, "assignments=2", "pin=\"\""},
	} {
		if !log.Has(want...) {
			t.Errorf("log reads\n%s\nwant a line with %q", log, want)
		}
	}
	if !s.unsaved {
		t.Error("what was forgotten is still in the state file, want it due to be saved")
	}
}

func TestRememberNotesARequestOnTheAssignmentItsChoiceFoundAlone(t *testing.T) {
	s := newSessions(at(start))
	k := key{session: "one", model: opus}
	if _, noted := s.remember(k, assignment{}, "", decision{account: "work", reason: reasonNew}, start); !noted {
		t.Fatal("remember() didn't note a new session's first request")
	}
	onWork := s.lookup(k).current

	found, noted := s.remember(k, assignment{}, "", decision{account: "side", reason: reasonNew}, start.Add(time.Second))
	if noted || found != onWork {
		t.Errorf("remember() of another request that found the session new = %+v, noted %v, want work's assignment found, and the request not noted", found, noted)
	}
	stay := decision{account: "work", reason: reasonSticky, sticky: true}
	for _, at := range []time.Time{start.Add(time.Minute), start.Add(2 * time.Minute)} {
		if _, noted := s.remember(k, onWork, "", stay, at); !noted {
			t.Errorf("remember() of a request that stayed, at %v, didn't note it: staying leaves the assignment it found", at)
		}
	}
	if got := s.lookup(k).current; !got.same(onWork) || got.LastSeen != start.Add(2*time.Minute) {
		t.Errorf("the session is assigned %+v, want work's assignment, last seen when it last stayed", got)
	}
}

func TestAssignmentsKeepTheirTimesInUTC(t *testing.T) {
	local := start.In(time.FixedZone("UTC+1", 60*60))
	path := filepath.Join(t.TempDir(), "state.json")
	writeState(t, path, savedState{Version: stateVersion, Sessions: []savedAssignment{
		{Session: "saved", Model: opus, Account: "work", Reason: reasonNew, AssignedAt: local, LastSeen: local},
	}})
	s := newSessions(at(local))

	s.load(path, testAccounts())
	assign(s, key{session: "new", model: opus}, "", decision{account: "side", reason: reasonNew}, local)
	for _, id := range []string{"saved", "new"} {
		if got, _ := s.session(id); len(got.Assignments) != 1 || got.Assignments[0].AssignedAt != start || got.Assignments[0].LastSeen != start {
			t.Errorf("session %s is %+v, want its times in UTC", id, got)
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

			s.load(path, testAccounts())
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

	s.load(path, testAccounts())
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
		s.load(path, testAccounts())
		writes := countWrites(s)
		stop := keep(s)

		for i := range 100 {
			assign(s, key{session: fmt.Sprint(i), model: opus}, "", decision{account: "work", reason: reasonNew}, time.Now())
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
		s.load(path, testAccounts())
		stop := keep(s)
		s.setPin(status.Pin{Account: "side", Since: time.Now()}, false)
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

func TestSessionsUnusedForAWeekAreForgottenHourlyWithTheirPins(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state.json")
		s := newSessions(time.Now)
		s.load(path, testAccounts())
		stop := keep(s)
		defer stop()
		assign(s, key{session: "fading", model: opus}, "", decision{account: "work", reason: reasonNew}, time.Now().Add(-7*24*time.Hour+30*time.Minute))
		assign(s, key{session: "recent", model: opus}, "", decision{account: "work", reason: reasonNew}, time.Now())
		for _, id := range []string{"fading", "recent"} {
			s.pinSession(id, "side")
		}

		time.Sleep(pruneEvery - time.Nanosecond)
		synctest.Wait()
		if _, seen := s.session("fading"); !seen {
			t.Fatal("forgot a session before the hour was up")
		}
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		_, fading := s.session("fading")
		_, recent := s.session("recent")
		if fading || !recent {
			t.Errorf("an hour on, sessions are %+v, want the one unused for a week forgotten, and the other kept", s.assignments)
		}
		time.Sleep(saveAfter)
		synctest.Wait()
		held := readState(t, path)
		if len(held.Sessions) != 1 || held.Sessions[0].Session != "recent" {
			t.Errorf("state file holds %+v, want the recent session alone", held.Sessions)
		}
		if _, kept := held.SessionPins["recent"]; len(held.SessionPins) != 1 || !kept {
			t.Errorf("state file holds sessions' pins %+v, want the recent session's alone", held.SessionPins)
		}
	})
}

func TestASaveThatFailsIsTriedAgain(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := logstest.Capture(t)
		path := filepath.Join(t.TempDir(), "state.json")
		s := newSessions(time.Now)
		s.load(path, testAccounts())
		var failed atomic.Bool
		s.file.write = func(path string, data []byte) error {
			if failed.CompareAndSwap(false, true) {
				return errors.New("no space left on device")
			}
			return writeAtomic(path, data)
		}
		stop := keep(s)
		s.setPin(status.Pin{Account: "side", Since: time.Now()}, false)

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
		wg.Go(func() { assign(s, k, "", decision{account: "work", reason: reasonNew}, start) })
		wg.Go(func() { _ = s.lookup(k) })
		wg.Go(func() { s.setPin(status.Pin{Account: "side", Since: start}, i%3 == 0) })
		wg.Go(func() { _, _ = s.unpin(i%3 == 1) })
		wg.Go(func() { s.pinSession(k.session, "side") })
		wg.Go(func() { _, _ = s.session(k.session) })
		wg.Go(func() { _ = s.running(start) })
		wg.Go(func() { _, _ = s.active(start) })
		wg.Go(func() { s.prune(start) })
	}
	wg.Wait()
}

func TestActiveCountsEachSessionOnceByAccountAndOnceInAll(t *testing.T) {
	s := newSessions(at(start))
	remember := func(session, model, account string, lastSeen time.Time) {
		assign(s, key{session: session, model: model}, "", decision{account: account, reason: reasonNew}, lastSeen)
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

func TestSessionReportsItsPinAndItsAssignmentsTheOneUsedLastFirst(t *testing.T) {
	s := newSessions(at(start))
	assign(s, key{session: "one", model: haiku}, "", decision{account: "side", reason: reasonNew}, start.Add(-time.Hour))
	assign(s, key{session: "one", model: opus}, "work", decision{account: "work", reason: reasonPinned}, start)
	assign(s, key{session: "two", model: opus}, "", decision{account: "side", reason: reasonNew}, start)

	want := status.Session{
		ID:  "one",
		Pin: "work",
		Assignments: []status.Assignment{
			{Model: opus, Account: "work", Pinned: true, Reason: "pinned", AssignedAt: start, LastSeen: start},
			{Model: haiku, Account: "side", Reason: "new", AssignedAt: start.Add(-time.Hour), LastSeen: start.Add(-time.Hour)},
		},
	}
	if got, seen := s.session("one"); !seen || !reflect.DeepEqual(got, want) {
		t.Errorf("session() =\n%+v, %v\nwant\n%+v", got, seen, want)
	}
	if got, seen := s.session("nope"); seen {
		t.Errorf("session() of a session never seen = %+v, want none", got)
	}
}

func TestASessionsOwnPin(t *testing.T) {
	tests := []struct {
		name string
		// launched are the pins the session's requests of Opus, and of Haiku
		// a minute later, carried.
		launched [2]string
		// given is the pin the session is given while it runs, "" to clear
		// it, and nil for none.
		given *string
		want  string
	}{
		{name: "none"},
		{name: "the one it was launched with", launched: [2]string{"work", "work"}, want: "work"},
		{name: "the one its last request carried, launched again", launched: [2]string{"work", "side"}, want: "side"},
		{name: "one given while it runs, over the one it was launched with", launched: [2]string{"work", "work"}, given: new("side"), want: "side"},
		{name: "one given while it runs, launched without", given: new("side"), want: "side"},
		{name: "none, once cleared, the one it was launched with included", launched: [2]string{"work", "work"}, given: new("")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSessions(at(start))
			assign(s, key{session: "one", model: opus}, tt.launched[0], decision{account: "work", reason: reasonNew}, start.Add(-time.Minute))
			assign(s, key{session: "one", model: haiku}, tt.launched[1], decision{account: "work", reason: reasonNew}, start)
			if tt.given != nil && !s.pinSession("one", *tt.given) {
				t.Fatal("pinSession() = false, want the session found")
			}

			if got, _ := s.session("one"); got.Pin != tt.want {
				t.Errorf("session's pin = %q, want %q", got.Pin, tt.want)
			}
		})
	}
}

func TestASessionNeverSeenIsGivenNoPin(t *testing.T) {
	s := newSessions(at(start))
	if s.pinSession("nope", "side") {
		t.Error("pinSession() of a session never seen = true, want false")
	}
	if len(s.own) > 0 || s.unsaved {
		t.Errorf("sessions' pins = %+v, unsaved %v, want none, and nothing to save", s.own, s.unsaved)
	}
}

func TestRunningListsTheSessionsRoutedInTheLastHourTheOneSeenLastFirst(t *testing.T) {
	s := newSessions(at(start))
	remember := func(session, model, account string, lastSeen time.Time) {
		assign(s, key{session: session, model: model}, "", decision{account: account, reason: reasonNew}, lastSeen)
	}
	remember("earlier", opus, "work", start.Add(-30*time.Minute))
	remember("latest", opus, "side", start.Add(-time.Minute))
	remember("latest", haiku, "work", start.Add(-3*time.Hour))
	remember("b-tied", opus, "work", start.Add(-45*time.Minute))
	remember("a-tied", opus, "side", start.Add(-45*time.Minute))
	remember("an-hour-ago", opus, "work", start.Add(-time.Hour))
	remember("idle", opus, "work", start.Add(-time.Hour-time.Second))
	s.pinSession("earlier", "side")

	got := s.running(start)
	ids := make([]string, len(got))
	for i, session := range got {
		ids[i] = session.ID
	}
	if want := []string{"latest", "earlier", "a-tied", "b-tied", "an-hour-ago"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("running() lists %q, want %q: those routed in the last hour, the one seen last first", ids, want)
	}
	if want, _ := s.session("latest"); !reflect.DeepEqual(got[0], want) {
		t.Errorf("running() lists\n%+v\nwant the session as session() gives it, every model included\n%+v", got[0], want)
	}
	if got[1].Pin != "side" {
		t.Errorf("running() lists %+v, want its own pin with it", got[1])
	}
	if got := newSessions(at(start)).running(start); got == nil || len(got) > 0 {
		t.Errorf("running() with no sessions = %#v, want an empty list", got)
	}
}

func TestForceClearsEverySessionsOwnPin(t *testing.T) {
	later := start.Add(time.Minute)
	tests := []struct {
		name  string
		force func(s *sessions) int
		// wantPin is the global pin once forced.
		wantPin status.Pin
	}{
		{
			name:    "setting the global pin",
			force:   func(s *sessions) int { return s.setPin(status.Pin{Account: "side", Since: later, Move: true}, true) },
			wantPin: status.Pin{Account: "side", Since: later, Move: true},
		},
		{
			name: "clearing the global pin",
			force: func(s *sessions) int {
				_, cleared := s.unpin(true)
				return cleared
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			s := newSessions(clock.read)
			s.setPin(status.Pin{Account: "work", Since: start.Add(-time.Hour)}, false)
			assign(s, key{session: "launched", model: opus}, "work", decision{account: "work", reason: reasonPinned}, start)
			assign(s, key{session: "given", model: opus}, "", decision{account: "work", reason: reasonNew}, start)
			assign(s, key{session: "unpinned", model: opus}, "", decision{account: "work", reason: reasonNew}, start)
			s.pinSession("given", "side")
			clock.now = later
			s.unsaved = false

			if cleared := tt.force(s); cleared != 2 {
				t.Errorf("cleared %d sessions' pins, want 2: the one launched with a pin, and the one given one", cleared)
			}
			want := map[string]ownPin{"launched": {Since: later}, "given": {Since: later}}
			if !maps.Equal(s.own, want) {
				t.Errorf("sessions' pins = %+v, want %+v: each cleared, the unpinned session's left alone", s.own, want)
			}
			if s.pin != tt.wantPin || !s.unsaved {
				t.Errorf("global pin = %+v, unsaved %v, want %+v, due to be saved", s.pin, s.unsaved, tt.wantPin)
			}
			for _, id := range []string{"launched", "given"} {
				if got := s.lookup(key{session: id, model: opus}); !got.given || got.own.Account != "" {
					t.Errorf("session %s's own pin = %+v, want one given as cleared, which passes over the one it was launched with", id, got)
				}
			}
		})
	}
}

func TestUnpinningWithoutAPinOrForceChangesNothing(t *testing.T) {
	s := newSessions(at(start))
	assign(s, key{session: "launched", model: opus}, "work", decision{account: "work", reason: reasonPinned}, start)
	s.unsaved = false

	if was, cleared := s.unpin(false); was != (status.Pin{}) || cleared != 0 || len(s.own) > 0 || s.unsaved {
		t.Errorf("unpin() = %+v, %d, sessions' pins %+v, unsaved %v, want nothing changed", was, cleared, s.own, s.unsaved)
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
