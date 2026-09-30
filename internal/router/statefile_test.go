package router

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens"
	"github.com/leeovery/switchboard/internal/tokens/tokenstest"
)

func TestTheStateFileKeepsSessionsTheirPinsAndTheGlobalPin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	saved := newTestFile(at(start), testAccounts())
	saved.load(path)
	s := saved.sessions
	s.setPin(status.Pin{Accounts: []string{"work", "side"}, Since: start.Add(-time.Hour), Move: true}, false)
	assign(s, key{session: "one", model: opus}, "", decision{account: "work", reason: reasonNew}, start.Add(-2*time.Hour))
	assign(s, key{session: "one", model: opus}, "", decision{account: "work", reason: reasonSticky, sticky: true}, start.Add(-time.Hour))
	assign(s, key{session: "one", model: haiku}, "side", decision{account: "side", reason: reasonPinned}, start.Add(-time.Minute))
	assign(s, key{session: "two", model: opus}, "work", decision{account: "work", reason: reasonPinned}, start.Add(-time.Minute))
	s.pinSession("one", "work")
	s.pinSession("two", "")
	saved.save()

	loaded := newTestFile(at(start), testAccounts())
	loaded.load(path)
	if l := loaded.sessions; !maps.Equal(l.assignments, s.assignments) || !maps.Equal(l.own, s.own) || !reflect.DeepEqual(l.pin, s.pin) {
		t.Errorf("loaded\n%+v, pins %+v, pin %+v\nwant what was saved\n%+v, pins %+v, pin %+v",
			l.assignments, l.own, l.pin, s.assignments, s.own, s.pin)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "version": 1,
  "pin": {
    "accounts": [
      "work",
      "side"
    ],
    "account": "work",
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

func TestTheStateFileKeepsEachAccountsReadings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	clock := &testClock{now: start.Add(-time.Hour)}
	saved := newTestFile(clock.read, testAccounts())
	saved.load(path)
	models := map[string][]string{"5h": {haiku, fable}, "7d": {haiku, fable}, "7d_oi": {fable}}
	saved.state.recordProbe("side", probed(models, session, week, fableWeek), nil, saved.state.mark(), fromProbe)
	clock.now = start
	saved.state.record("work", []quota.Window{session, week}, saved.state.mark())
	saved.state.learn(opus, []quota.Window{session, week})
	saved.save()

	loaded := newTestFile(at(start), testAccounts())
	loaded.load(path)
	for id, u := range saved.state.usage {
		got := loaded.state.usage[id]
		if !maps.Equal(got.windows, u.windows) || !got.updated.Equal(u.updated) {
			t.Errorf("%s loaded as read at %v, %+v; want what was saved: read at %v, %+v", id, got.updated, got.windows, u.updated, u.windows)
		}
	}
	if !reflect.DeepEqual(loaded.state.seen, saved.state.seen) {
		t.Errorf("loaded the windows as seen on %v, want what was saved: %v", loaded.state.seen, saved.state.seen)
	}
	if loaded.state.view(opus, start).applies("7d_oi") {
		t.Error("once loaded, the Fable week counts an Opus request, want it to count Fable's alone, as it was seen to")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "version": 1,
  "sessions": [],
  "tokens": {
    "side": {
      "sha256": "` + sideHash + `"
    },
    "work": {
      "sha256": "` + workHash + `"
    }
  },
  "readings": {
    "side": {
      "read_at": "2026-09-28T12:12:00Z",
      "windows": [
        {
          "key": "5h",
          "label": "Session",
          "utilization": 0.23,
          "resets_at": "2026-09-28T18:10:00Z",
          "status": "allowed"
        },
        {
          "key": "7d",
          "label": "Week",
          "utilization": 0.93,
          "resets_at": "2026-10-02T21:00:00Z",
          "status": "allowed_warning"
        },
        {
          "key": "7d_oi",
          "label": "Fable week",
          "utilization": 0.05,
          "resets_at": "2026-10-04T01:10:00Z",
          "status": "allowed"
        }
      ]
    },
    "work": {
      "read_at": "2026-09-28T13:12:00Z",
      "windows": [
        {
          "key": "5h",
          "label": "Session",
          "utilization": 0.23,
          "resets_at": "2026-09-28T18:10:00Z",
          "status": "allowed"
        },
        {
          "key": "7d",
          "label": "Week",
          "utilization": 0.93,
          "resets_at": "2026-10-02T21:00:00Z",
          "status": "allowed_warning"
        }
      ]
    }
  },
  "window_families": {
    "5h": [
      "fable",
      "haiku",
      "opus"
    ],
    "7d": [
      "fable",
      "haiku",
      "opus"
    ],
    "7d_oi": [
      "fable"
    ]
  }
}
`
	if string(data) != want {
		t.Errorf("state file holds\n%s\nwant\n%s", data, want)
	}
}

func TestWhenAWindowStartedAgainOutlastsARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	clock := &testClock{now: start}
	saved := newTestFile(clock.read, testAccounts())
	saved.load(path)
	saved.state.record("work", []quota.Window{session, week}, saved.state.mark())
	// A reset made by hand drops the week's use, keeping its reset.
	clock.now = start.Add(time.Minute)
	emptied := week
	emptied.Utilization, emptied.Status = 0, quota.StatusAllowed
	saved.state.record("work", []quota.Window{session, emptied}, saved.state.mark())
	saved.save()

	loaded := newTestFile(at(clock.now), testAccounts())
	loaded.load(path)
	want := emptied
	want.RestartedAt = clock.now
	if got := loaded.state.usage["work"].windows["7d"]; got != want {
		t.Errorf("the week loaded as %+v, want %+v: started again as the reset made by hand was read", got, want)
	}
	if got := loaded.state.usage["work"].windows["5h"]; got != session {
		t.Errorf("the session loaded as %+v, want %+v: it never started again", got, session)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"restarted_at": "2026-09-28T13:13:00Z"`) {
		t.Errorf("state file holds\n%s\nwant the week's restart kept with its reading", data)
	}
}

func TestAStateFileFromBeforeWindowsStartedAgainLoadsTheirReadingsAsRunningWhole(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	before := `{
  "version": 1,
  "sessions": [],
  "readings": {
    "work": {
      "read_at": "2026-09-28T13:12:00Z",
      "windows": [
        {"key": "7d", "label": "Week", "utilization": 0.93, "resets_at": "2026-10-02T21:00:00Z", "status": "allowed_warning"}
      ]
    }
  }
}
`
	if err := os.WriteFile(path, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}
	f := newTestFile(at(start), testAccounts())

	f.load(path)
	if got := f.state.usage["work"].windows["7d"]; got != week {
		t.Errorf("the week loaded as %+v, want %+v, running a whole week before its reset", got, week)
	}
}

func TestAStateFileFromBeforeReadingsLoadsWithNone(t *testing.T) {
	log := logstest.Capture(t)
	path := filepath.Join(t.TempDir(), "state.json")
	before := `{
  "version": 1,
  "pin": {"account": "side", "since": "2026-09-28T12:12:00Z", "move": false},
  "sessions": [
    {"session": "one", "model": "claude-opus-5-5", "account": "work", "reason": "new", "assigned_at": "2026-09-28T12:12:00Z", "last_seen": "2026-09-28T13:11:00Z"}
  ],
  "tokens": {"side": {"sha256": "` + sideHash + `"}, "work": {"sha256": "` + workHash + `"}}
}
`
	if err := os.WriteFile(path, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}
	f := newTestFile(at(start), testAccounts())

	f.load(path)
	if got := f.sessions.lookup(key{session: "one", model: opus}); !got.assigned || got.current.Account != "work" || !slices.Equal(got.global.Accounts, []string{"side"}) {
		t.Errorf("loaded %+v, want the session on work, and the pin to side, as the file holds", got)
	}
	for _, id := range []string{"work", "side"} {
		if !f.state.unread(id, start) {
			t.Errorf("%s loaded as read, want nothing read of it: the file holds no readings", id)
		}
	}
	if f.changes.unsaved.Load() {
		t.Error("loading the file left it due to be saved, want it as it was")
	}
	if !log.Has("level=INFO", `msg="loaded state"`, "assignments=1", "pin=side", "readings=0") {
		t.Errorf("log reads\n%s\nwant the state loaded, without readings", log)
	}
}

func TestLoadingThePin(t *testing.T) {
	since := start.Add(-time.Hour)
	tests := []struct {
		name string
		pin  string
		want status.Pin
		// wantDropped are the accounts the pin loses, as they're no longer
		// configured.
		wantDropped []string
	}{
		{
			name: "to one account, as a switchboard from before pins named several wrote it",
			pin:  `{"account": "side", "since": "2026-09-28T12:12:00Z", "move": true}`,
			want: status.Pin{Accounts: []string{"side"}, Since: since, Move: true},
		},
		{
			name: "to several",
			pin:  `{"accounts": ["work", "side"], "account": "work", "since": "2026-09-28T12:12:00Z", "move": false}`,
			want: status.Pin{Accounts: []string{"work", "side"}, Since: since},
		},
		{
			name: "to several, named in the order configured",
			pin:  `{"accounts": ["side", "work"], "account": "side", "since": "2026-09-28T12:12:00Z", "move": false}`,
			want: status.Pin{Accounts: []string{"work", "side"}, Since: since},
		},
		{
			name:        "to several, one no longer configured",
			pin:         `{"accounts": ["retired", "side"], "account": "retired", "since": "2026-09-28T12:12:00Z", "move": true}`,
			want:        status.Pin{Accounts: []string{"side"}, Since: since, Move: true},
			wantDropped: []string{"retired"},
		},
		{
			name:        "to one no longer configured, as a switchboard from before pins named several wrote it",
			pin:         `{"account": "retired", "since": "2026-09-28T12:12:00Z", "move": false}`,
			wantDropped: []string{"retired"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			path := filepath.Join(t.TempDir(), "state.json")
			content := `{"version": 1, "pin": ` + tt.pin + `, "sessions": [], "tokens": {"side": {"sha256": "` + sideHash + `"}, "work": {"sha256": "` + workHash + `"}}}`
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			f := newTestFile(at(start), testAccounts())

			f.load(path)
			if got := f.sessions.globalPin(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("loaded pin %+v, want %+v", got, tt.want)
			}
			for _, id := range tt.wantDropped {
				if want := []string{"level=WARN", `msg="dropped from the pin: the account is no longer configured"`, "account=" + id}; !log.Has(want...) {
					t.Errorf("log reads\n%s\nwant a line with %q", log, want)
				}
			}
			if dropped := len(tt.wantDropped) > 0; f.changes.unsaved.Load() != dropped {
				t.Errorf("due to be saved = %v, want %v: only what's dropped is a change", f.changes.unsaved.Load(), dropped)
			}
		})
	}
}

func TestLoadingForgetsTheReadingsOfAccountsNoLongerConfigured(t *testing.T) {
	log := logstest.Capture(t)
	path := filepath.Join(t.TempDir(), "state.json")
	putState(t, path, savedState{
		Version: stateVersion,
		Tokens:  map[string]savedTokens{"work": {SHA256: workHash}, "side": {SHA256: sideHash}},
		Readings: map[string]savedReading{
			"work":     {ReadAt: start, Windows: []quota.Window{session, week}},
			"personal": {ReadAt: start, Windows: []quota.Window{session, week}},
			"gone":     {ReadAt: start, Windows: []quota.Window{session, week}},
		},
	})
	f := newTestFile(at(start), testAccounts())

	f.load(path)
	if got := slices.Sorted(maps.Keys(f.state.saved().Readings)); !slices.Equal(got, []string{"personal", "work"}) {
		t.Errorf("loaded readings of %q, want work's, and personal's, kept for when its token is back: gone is no longer configured", got)
	}
	if !f.changes.unsaved.Load() {
		t.Error("gone's reading is still in the state file, want it due to be saved")
	}
	if !log.Has("level=INFO", `msg="loaded state"`, "path="+path, "readings=2") {
		t.Errorf("log reads\n%s\nwant the state loaded, with two readings", log)
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
	accounts := testAccounts()
	saving := newTestFile(at(start), accounts)
	saving.load(path)
	work, _ := accounts.byID("work")
	if !work.secret.replace(mustToken(t, renewedToken), start) {
		t.Fatal("replace() = false, want work's token replaced")
	}
	saving.changes.note()
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
			newTestFile(at(start.Add(tt.after)), restarted).load(path)

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
	before := newTestFile(at(start), testAccounts())
	before.load(path)
	before.save()

	later := start.Add(time.Hour)
	restarted := resolve(testConfigured, tokenstest.Files{"work": renewedToken, "side": sideToken}.Read)
	f := newTestFile(at(later), restarted)
	f.load(path)
	for token, want := range map[string]string{renewedToken: "work", workToken: "work", sideToken: "side"} {
		if a, ok := restarted.byToken(token, later); !ok || a.ID != want {
			t.Errorf("byToken() of %s's token = %q, %v, want %s", want, a.ID, ok, want)
		}
	}
	if !f.changes.unsaved.Load() {
		t.Error("work's former token isn't due to be saved")
	}
	if !log.Has("level=INFO", `msg="token replaced while the router was away; the one before still counts as the account's"`, "account=work") {
		t.Errorf("log reads\n%s\nwant work's token noted as replaced", log)
	}
	if strings.Contains(log.String(), "account=side") {
		t.Errorf("log reads\n%s\nwant nothing of side, whose token stands", log)
	}
}

func TestAnAccountsTokensOutlastAGapInItsTokenFileAndARestart(t *testing.T) {
	const again = "test-token-work-again"
	tests := []struct {
		name string
		// back is the token work's file holds once it's back.
		back         string
		wantReplaced bool
	}{
		{name: "back holding the token it held", back: renewedToken},
		{name: "back holding another", back: again, wantReplaced: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			watch := newTestTokenFiles(testTokens, tokenstest.Files{"work": renewedToken, "side": sideToken})
			running := newTestFile(at(start), watch.accounts)
			running.load(path)
			// Work's token is replaced as the router runs, then its file goes.
			watch.look()
			watch.files.set(tokenstest.Files{"side": sideToken})
			watch.look()
			watch.look()
			running.changes.note()
			running.save()

			// The router starts again an hour on, work's file still gone,
			// and it comes back.
			later := start.Add(time.Hour)
			restarted := resolve(testConfigured, watch.files.read)
			newTestFile(at(later), restarted).load(path)
			work, _ := restarted.byID("work")
			if r := work.secret.reload(mustToken(t, tt.back), nil, later); r.replaced != tt.wantReplaced || !r.gained {
				t.Errorf("work, its token file back, has gained its token: %v, and replaced the one it held: %v, want true and %v", r.gained, r.replaced, tt.wantReplaced)
			}
			for _, token := range []string{workToken, renewedToken, tt.back} {
				if a, ok := restarted.byToken(token, later); !ok || a.ID != "work" {
					t.Errorf("a request carrying %s is routed as %q (%v), want work's: it's a token work had this week", token, a.ID, ok)
				}
			}
		})
	}
}

func TestTheTokensOfAnAccountNoLongerConfiguredCountAsThePrimarysForAWeek(t *testing.T) {
	const goneToken, olderToken = "test-token-gone", "test-token-gone-older"
	log := logstest.Capture(t)
	path := filepath.Join(t.TempDir(), "state.json")
	older := formerToken{SHA256: hash(olderToken), ReplacedAt: start.Add(-24 * time.Hour)}
	putState(t, path, savedState{
		Version: stateVersion,
		Tokens: map[string]savedTokens{
			"work": {SHA256: workHash},
			"side": {SHA256: sideHash},
			"gone": {SHA256: hash(goneToken), Former: []formerToken{older}},
		},
	})
	f := newTestFile(at(start), testAccounts())

	f.load(path)
	if !log.Has("level=INFO", `msg="account no longer configured; its tokens count as the primary's for a week"`, "account=gone", "primary=work") {
		t.Errorf("log reads\n%s\nwant gone's tokens noted as work's, the primary's", log)
	}
	if !f.changes.unsaved.Load() {
		t.Fatal("gone's tokens aren't due to be saved as work's")
	}
	f.save()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), goneToken) {
		t.Errorf("state file holds\n%s\nwant no token in it", data)
	}
	fromGone := older
	fromGone.From = "gone"
	want := map[string]savedTokens{
		"work": {SHA256: workHash, Former: []formerToken{fromGone, {SHA256: hash(goneToken), ReplacedAt: start, From: "gone"}}},
		"side": {SHA256: sideHash},
	}
	if held := readState(t, path); !maps.EqualFunc(held.Tokens, want, savedTokens.equal) {
		t.Errorf("state file holds tokens %+v, want %+v", held.Tokens, want)
	}

	// Restarted, gone still not configured.
	tests := []struct {
		name  string
		after time.Duration
		// want are the tokens routed as work's.
		want []string
	}{
		{name: "a moment on", after: time.Minute, want: []string{goneToken, olderToken}},
		{name: "a week after gone's older token was replaced", after: formerFor - 24*time.Hour, want: []string{goneToken}},
		{name: "a week on", after: formerFor},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restarted := testAccounts()
			later := start.Add(tt.after)
			newTestFile(at(later), restarted).load(path)
			for _, token := range []string{goneToken, olderToken} {
				a, ok := restarted.byToken(token, later)
				if got, want := ok && a.ID == "work", slices.Contains(tt.want, token); got != want {
					t.Errorf("%s taken for work's %v on: %v, want %v", token, tt.after, got, want)
				}
			}
		})
	}
}

func TestTheTokensOfAnAccountThatLeftTheConfigOutlastARestartAsThePrimarysAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	running := testAccounts()
	f := newTestFile(at(start), running)
	f.load(path)
	if !running.retire(withoutWork, start) {
		t.Fatal("retire() = false, want work's tokens left to side")
	}
	f.changes.note()
	f.save()
	held := readState(t, path)
	if _, kept := held.Tokens["work"]; kept {
		t.Errorf("state file holds work's tokens as its own, %+v, want them kept as side's alone", held.Tokens["work"])
	}
	if want := []formerToken{{SHA256: workHash, ReplacedAt: start, From: "work"}}; !slices.EqualFunc(held.Tokens["side"].Former, want, formerToken.equal) {
		t.Errorf("state file holds side's former tokens as %+v, want %+v", held.Tokens["side"].Former, want)
	}

	tests := []struct {
		name string
		// configured are the accounts the router is started again with, after.
		configured config.Accounts
		after      time.Duration
		// wantSides is whether work's token counts as side's.
		wantSides bool
	}{
		{name: "work still gone, a moment on", configured: withoutWork, after: time.Minute, wantSides: true},
		{name: "work still gone, a week on", configured: withoutWork, after: formerFor},
		{name: "work configured again", configured: testConfigured, after: time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			later := start.Add(tt.after)
			restarted := resolve(tt.configured, testTokens.Read)
			newTestFile(at(later), restarted).load(path)
			side, _ := restarted.byID("side")
			if got := side.secret.was(workHash, later); got != tt.wantSides {
				t.Errorf("work's token counts as side's: %v, want %v", got, tt.wantSides)
			}
		})
	}
}

func TestLoadingAStateFileThatKnowsTheTokensChangesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	first := newTestFile(at(start), testAccounts())
	first.load(path)
	first.state.record("work", []quota.Window{session, week}, first.state.mark())
	first.save()

	f := newTestFile(at(start.Add(time.Hour)), testAccounts())
	f.load(path)
	if f.changes.unsaved.Load() {
		t.Error("loading the state file left it due to be saved, want it as it was")
	}
}

func TestFormerTokensAreForgottenHourlyAWeekAfterTheyWereReplaced(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state.json")
		accounts := testAccounts()
		f := newTestFile(time.Now, accounts)
		f.load(path)
		work, _ := accounts.byID("work")
		work.secret.replace(mustToken(t, renewedToken), time.Now().Add(-formerFor+30*time.Minute))
		f.changes.note()
		stop := keep(f)
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
	putState(t, path, savedState{
		Version: stateVersion,
		Pin:     status.Pin{Accounts: []string{"retired"}, Since: start.Add(-time.Hour)},
		Sessions: []savedAssignment{
			{Session: "recent", Model: opus, Account: "work", LastSeen: start.Add(-week + time.Second)},
			{Session: "pinned", Model: opus, Account: "work", LastSeen: start},
			{Session: "old", Model: opus, Account: "work", LastSeen: start.Add(-week)},
			{Session: "gone", Model: opus, Account: "retired", LastSeen: start},
			{Model: opus, Account: "work", LastSeen: start},
		},
		SessionPins: map[string]ownPin{
			"pinned": {Account: "side", Since: start},
			"recent": {Account: "retired", Since: start},
			"old":    {Account: "side", Since: start},
		},
	})
	f := newTestFile(at(start), testAccounts())

	f.load(path)
	s := f.sessions
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
	if !s.pin.IsZero() {
		t.Errorf("loaded pin %+v, want none: its account is no longer configured", s.pin)
	}
	for _, want := range [][]string{
		{"level=WARN", `msg="dropped from the pin: the account is no longer configured"`, "account=retired"},
		{"level=WARN", `msg="session's pin dropped: its account is no longer configured"`, "session=recent", "account=retired"},
		{"level=INFO", `msg="loaded state"`, "path=" + path, "assignments=2", "pin=\"\""},
	} {
		if !log.Has(want...) {
			t.Errorf("log reads\n%s\nwant a line with %q", log, want)
		}
	}
	if !f.changes.unsaved.Load() {
		t.Error("what was forgotten is still in the state file, want it due to be saved")
	}
}

func TestLoadingKeepsWhatsOfAnAccountWhoseTokenFileCantBeReadAsTheRouterStarts(t *testing.T) {
	log := logstest.Capture(t)
	path := filepath.Join(t.TempDir(), "state.json")
	pin := status.Pin{Accounts: []string{"work"}, Since: start.Add(-time.Hour)}
	assignments := map[key]assignment{{session: "one", model: opus}: {Account: "work", LastSeen: start}}
	own := map[string]ownPin{"one": {Account: "work", Since: start}}
	putState(t, path, savedState{
		Version:     stateVersion,
		Pin:         pin,
		Sessions:    []savedAssignment{{Session: "one", Model: opus, Account: "work", LastSeen: start}},
		SessionPins: own,
		Tokens:      map[string]savedTokens{"work": {SHA256: workHash}, "side": {SHA256: sideHash}},
	})
	// Work's token file is caught emptied as it's rewritten.
	f := newTestFile(at(start), resolve(testConfigured, tokenstest.Files{"work": "", "side": sideToken}.Read))

	f.load(path)
	if s := f.sessions; !maps.Equal(s.assignments, assignments) || !maps.Equal(s.own, own) || !reflect.DeepEqual(s.pin, pin) {
		t.Errorf("loaded %+v, pins %+v, pin %+v\nwant what was saved\n%+v, pins %+v, pin %+v: work is still configured",
			s.assignments, s.own, s.pin, assignments, own, pin)
	}
	if log.Has("dropped") {
		t.Errorf("log reads\n%s\nwant nothing dropped", log)
	}
	if f.changes.unsaved.Load() {
		t.Error("loading the state file left it due to be saved, want it as it was")
	}
}

func TestAssignmentsKeepTheirTimesInUTC(t *testing.T) {
	local := start.In(time.FixedZone("UTC+1", 60*60))
	path := filepath.Join(t.TempDir(), "state.json")
	putState(t, path, savedState{Version: stateVersion, Sessions: []savedAssignment{
		{Session: "saved", Model: opus, Account: "work", Reason: reasonNew, AssignedAt: local, LastSeen: local},
	}})
	f := newTestFile(at(local), testAccounts())

	f.load(path)
	assign(f.sessions, key{session: "new", model: opus}, "", decision{account: "side", reason: reasonNew}, local)
	for _, id := range []string{"saved", "new"} {
		if got, _ := f.sessions.session(id); len(got.Assignments) != 1 || got.Assignments[0].AssignedAt != start || got.Assignments[0].LastSeen != start {
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
		{name: "readings of another shape", content: `{"version": 1, "sessions": [], "readings": {"work": []}}`, wantError: "cannot unmarshal array"},
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
			f := newTestFile(at(start), testAccounts())

			f.load(path)
			if held := f.snapshot(); len(held.Sessions) > 0 || !held.Pin.IsZero() || len(held.Readings) > 0 {
				t.Errorf("loaded %+v, want nothing", held)
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
	f := newTestFile(at(start), testAccounts())

	f.load(path)
	if len(f.sessions.assignments) > 0 {
		t.Errorf("loaded %+v, want nothing", f.sessions.assignments)
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Errorf("%s: %v, want it left where it is", path, err)
	}
	if !log.Has("level=WARN", `msg="can't read the state file; starting empty"`, "path="+path) {
		t.Errorf("log reads\n%s\nwant the failure to read it", log)
	}
}

func TestTheStateFileIsItsOwnersAlone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte(`{"version": 1, "sessions": []}`), 0o644); err != nil {
		t.Fatal(err)
	}
	f := newTestFile(at(start), testAccounts())
	f.load(path)
	f.sessions.setPin(status.Pin{Accounts: []string{"side"}, Since: start}, false)

	f.save()
	if info, err := os.Stat(path); err != nil || info.Mode() != 0o600 {
		t.Errorf("state file's mode is %v (%v), want -rw-------", info.Mode(), err)
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 1 {
		t.Errorf("directory holds %v (%v), want the state file alone", entries, err)
	}
}

func TestABurstOfChangesIsSavedOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state.json")
		f := newTestFile(time.Now, testAccounts())
		f.load(path)
		writes := countWrites(f)
		stop := keep(f)

		for i := range 100 {
			assign(f.sessions, key{session: fmt.Sprint(i), model: opus}, "", decision{account: "work", reason: reasonNew}, time.Now())
			f.state.record("work", []quota.Window{session, week}, f.state.mark())
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
		if held := readState(t, path); len(held.Sessions) != 100 || len(held.Readings) != 1 {
			t.Errorf("state file holds %d sessions and %d readings, want all 100, and work's", len(held.Sessions), len(held.Readings))
		}
		stop()
		if n := writes.Load(); n != 1 {
			t.Errorf("wrote the state file %d times, want once: nothing changed after", n)
		}
	})
}

func TestAnAssignmentUsedAgainIsSavedOnceAMinute(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state.json")
		f := newTestFile(time.Now, testAccounts())
		f.load(path)
		writes := countWrites(f)
		stop := keep(f)
		k := key{session: "one", model: opus}
		assign(f.sessions, k, "", decision{account: "work", reason: reasonNew}, time.Now())
		time.Sleep(saveAfter)
		synctest.Wait()
		if n := writes.Load(); n != 1 {
			t.Fatalf("wrote the state file %d times, want once, for the new session", n)
		}
		lastSeen := func() time.Time {
			t.Helper()
			held := readState(t, path)
			if len(held.Sessions) != 1 {
				t.Fatalf("state file holds %+v, want the one session", held.Sessions)
			}
			return held.Sessions[0].LastSeen
		}

		stay := decision{account: "work", reason: reasonSticky, sticky: true}
		var used time.Time
		for range 30 {
			time.Sleep(time.Second)
			used = time.Now()
			assign(f.sessions, k, "", stay, used)
		}
		synctest.Wait()
		if n := writes.Load(); n != 1 {
			t.Errorf("wrote the state file %d times as the session was used again, want no more", n)
		}
		time.Sleep(saveRoutineEvery - saveAfter - 30*time.Second)
		synctest.Wait()
		if n, seen := writes.Load(), lastSeen(); n != 2 || !seen.Equal(used) {
			t.Errorf("a minute on, wrote the state file %d times, the session last seen %v, want twice, and %v", n, seen, used)
		}

		used = time.Now()
		assign(f.sessions, k, "", stay, used)
		stop()
		if n, seen := writes.Load(), lastSeen(); n != 3 || !seen.Equal(used) {
			t.Errorf("stopped, wrote the state file %d times, the session last seen %v, want three times, and %v", n, seen, used)
		}
	})
}

func TestReadingsOffAnswersAreSavedOnceAMinute(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state.json")
		f := newTestFile(time.Now, testAccounts())
		f.load(path)
		stop := keep(f)
		defer stop()
		// The tokens' hashes, new to the file, are saved first.
		time.Sleep(saveAfter)
		synctest.Wait()
		writes := countWrites(f)

		for range 30 {
			f.state.record("work", []quota.Window{session, week}, f.state.mark())
			time.Sleep(time.Second)
		}
		synctest.Wait()
		if n := writes.Load(); n != 0 {
			t.Errorf("wrote the state file %d times as readings came in, want none yet", n)
		}
		time.Sleep(saveRoutineEvery - saveAfter - 30*time.Second)
		synctest.Wait()
		if n := writes.Load(); n != 1 || len(readState(t, path).Readings) != 1 {
			t.Errorf("a minute on, wrote the state file %d times, want once, with work's reading", n)
		}
	})
}

func TestStoppingSavesWhatsUnsaved(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state.json")
		f := newTestFile(time.Now, testAccounts())
		f.load(path)
		stop := keep(f)
		f.sessions.setPin(status.Pin{Accounts: []string{"side"}, Since: time.Now()}, false)
		synctest.Wait()

		began := time.Now()
		stop()
		if waited := time.Since(began); waited != 0 {
			t.Errorf("stopping waited %v, want it to save at once", waited)
		}
		if held := readState(t, path); !slices.Equal(held.Pin.Accounts, []string{"side"}) {
			t.Errorf("state file holds pin %+v, want the one just set", held.Pin)
		}
	})
}

func TestSessionsUnusedForAWeekAreForgottenHourlyWithTheirPins(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state.json")
		f := newTestFile(time.Now, testAccounts())
		f.load(path)
		s := f.sessions
		stop := keep(f)
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
		f := newTestFile(time.Now, testAccounts())
		f.load(path)
		var failed atomic.Bool
		f.write = func(path string, data []byte) error {
			if failed.CompareAndSwap(false, true) {
				return errors.New("no space left on device")
			}
			return writeState(path, data)
		}
		stop := keep(f)
		f.sessions.setPin(status.Pin{Accounts: []string{"side"}, Since: time.Now()}, false)

		time.Sleep(saveAfter)
		synctest.Wait()
		if !log.Has("level=WARN", `msg="can't save the state file"`, "path="+path, `error="no space left on device"`) {
			t.Errorf("log reads\n%s\nwant the save's failure", log)
		}
		stop()
		if held := readState(t, path); !slices.Equal(held.Pin.Accounts, []string{"side"}) {
			t.Errorf("state file holds pin %+v, want the one that failed to save the first time", held.Pin)
		}
	})
}

// keep has f keep the state file until the stop it returns is called, which
// waits for it to finish.
func keep(f *stateFile) (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())
	var kept sync.WaitGroup
	kept.Go(func() { f.keep(ctx) })
	return func() {
		cancel()
		kept.Wait()
	}
}

// countWrites counts the writes of f from now on.
func countWrites(f *stateFile) *atomic.Int32 {
	var writes atomic.Int32
	write := f.write
	f.write = func(path string, data []byte) error {
		writes.Add(1)
		return write(path, data)
	}
	return &writes
}

// putState writes saved to the state file at path.
func putState(t *testing.T, path string, saved savedState) {
	t.Helper()
	if err := (&stateFile{path: path, write: writeState}).store(saved); err != nil {
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
