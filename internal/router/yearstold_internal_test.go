package router

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/tokens/tokenstest"
)

// yearZone is the zone the router's clock is read in as it tells of tokens'
// years: five hours behind UTC, so the bubble's clock begins at 19:00 on
// Friday 31 December 1999 there, its days turning five hours after UTC's.
var yearZone = time.FixedZone("UTC-5", -5*60*60)

// localOn is the hour given, in yearZone, days after the bubble's clock
// begins there.
func localOn(days, hour int) time.Time {
	return time.Date(1999, time.December, 31+days, hour, 0, 0, 0, yearZone)
}

// takenEnding is when a token whose year is up at until was taken.
func takenEnding(until time.Time) time.Time {
	return until.Add(-claude.SetupTokenLifetime)
}

// yearSaid is the notification of 2's token's year, as it's worded after the
// account.
func yearSaid(ends string) string {
	return "2 · two: token " + ends + " — switchboard setup makes a new one"
}

// newYearNotifying is newNotifying on the bubble's clock read in yearZone,
// 2's token's year up at until, as its record says.
func newYearNotifying(t *testing.T, settings config.Notifications, until time.Time) *notifying {
	t.Helper()
	as := resolve(numbered, numberedTokens.Read, recordsOf(map[string]time.Time{numberedTokens["2"]: takenEnding(until)}))
	return newNotifyingIn(t, settings, as, yearZone, newYearsTold(func() {}))
}

func TestATokensYearIsToldOfOnceAMonthAheadThenOnceAWeekAhead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// 31 days left as the router starts, counted in its zone: 30 in UTC,
		// which would tell of it at once.
		h := newYearNotifying(t, config.Notifications{Limits: true, Room: true, Warning: 0.9, Moves: true}, localOn(31, 1))
		h.start()
		h.expect()

		h.after(5*time.Hour - time.Second)
		h.expect()
		h.after(time.Second)
		month := yearSaid("expires 31 Jan 2000 · 30 days left")
		h.expect(month)

		h.after(23*day - time.Second)
		h.expect(month)
		h.after(time.Second)
		week := yearSaid("expires 31 Jan 2000 · 7 days left")
		h.expect(month, week)

		h.after(10 * day)
		h.expect(month, week)
	})
}

func TestAsTheRouterStartsTheNearestWarningATokensYearIsWithinIsToldOfAlone(t *testing.T) {
	tests := []struct {
		name  string
		until time.Time
		want  []string
	}{
		{name: "31 days left", until: localOn(31, 1)},
		{name: "30 days left", until: localOn(30, 1), want: []string{yearSaid("expires 30 Jan 2000 · 30 days left")}},
		{name: "8 days left", until: localOn(8, 1), want: []string{yearSaid("expires 8 Jan 2000 · 8 days left")}},
		{name: "7 days left", until: localOn(7, 1), want: []string{yearSaid("expires 7 Jan 2000 · 7 days left")}},
		{name: "its last day", until: localOn(0, 23), want: []string{yearSaid("expires 31 Dec 1999 · last day")}},
		// Up at 02:00 on 31 December in UTC.
		{name: "past its year, its date in the router's zone", until: localOn(-1, 21), want: []string{yearSaid("expired 30 Dec 1999")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := newYearNotifying(t, config.Notifications{Limits: true, Room: true, Warning: 0.9}, tt.until)
				h.start()
				h.expect(tt.want...)

				// The rest of the day the router started on, there.
				h.after(5*time.Hour - time.Second)
				h.expect(tt.want...)
			})
		})
	}
}

func TestATokensYearIsToldOfWithEveryNotificationOff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newYearNotifying(t, config.Notifications{}, localOn(30, 1))
		h.start()
		h.expect(yearSaid("expires 30 Jan 2000 · 30 days left"))
	})
}

func TestNoQuietMinuteHoldsATokensYearBackButItStartsOne(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := logstest.Capture(t)
		h := newYearNotifying(t, config.Notifications{Moves: true}, localOn(31, 1))
		h.start()
		moved := func(session string) Moved {
			return Moved{Session: session, Model: opus, From: "2", To: "1", Reason: "moved by pin"}
		}

		h.after(5*time.Hour - 10*time.Second)
		h.hear(moved("0b5c6f2e"))
		h.after(10 * time.Second)
		// A minute after the first move, but not after the token's year.
		h.after(50 * time.Second)
		h.hear(moved("18bb978f"))
		h.expect("session 0b5c6f2e moved from 2 · two to 1 · one (moved by pin)", yearSaid("expires 31 Jan 2000 · 30 days left"))
		want := []string{"level=DEBUG", `msg="notification dropped: too soon after the last about the account"`, "account=2", `news="session 18bb978f moved from 2 to 1 (moved by pin)"`}
		if !log.Has(want...) {
			t.Errorf("log reads\n%s\nwant the second move dropped, in the quiet minute the token's year started: a line with %q", log, want)
		}
	})
}

func TestATokensYearThatFailsToPostCountsAsToldOf(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := logstest.Capture(t)
		h := newYearNotifying(t, config.Notifications{}, localOn(30, 1))
		h.notifier.fail(errors.New("post notification: exit status 1"))
		h.start()
		h.after(time.Hour)

		h.expect(yearSaid("expires 30 Jan 2000 · 30 days left"))
		want := []string{"level=WARN", `msg="notification failed" component=router`, "account=2", `news="token expires 30 Jan 2000 · 30 days left"`}
		if !log.Has(want...) {
			t.Errorf("log reads\n%s\nwant a line with %q", log, want)
		}
	})
}

func TestAWarningToldOfATokenCountsForEveryFartherOne(t *testing.T) {
	told := toldYear{SHA256: workHash, Within: 7}
	tests := []struct {
		name string
		sum  string
		days int
		want bool
	}{
		{name: "the warning told", sum: workHash, days: 7, want: true},
		{name: "a farther one, as a clock set back may ask", sum: workHash, days: 30, want: true},
		{name: "a nearer one", sum: workHash, days: 3},
		{name: "of another token", sum: sideHash, days: 30},
	}
	for _, tt := range tests {
		if got := told.covers(tt.sum, tt.days); got != tt.want {
			t.Errorf("%s: covers(%d) = %v, want %v", tt.name, tt.days, got, tt.want)
		}
	}
}

// newKeptNotifying is newNotifying on the bubble's clock read in yearZone,
// over the numbered accounts with the tokens in files, each taken as records
// says, by the token, keeping what's been told of their years in the state
// file at path, which it loads, as the router does as it starts.
func newKeptNotifying(t *testing.T, path string, files tokenstest.Files, records map[string]time.Time) (*notifying, *stateFile) {
	t.Helper()
	changes := newChanges()
	years := newYearsTold(changes.note)
	as := resolve(numbered, files.Read, recordsOf(records))
	h := newNotifyingIn(t, config.Notifications{}, as, yearZone, years)
	f := newStateFile(h.n.now, changes, newSessions(h.n.now, changes.note, changes.routine), as, h.state, years)
	f.load(path)
	return h, f
}

func TestWhatsToldOfATokensYearOutlastsARestartAndANewTokenStartsAfresh(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := logstest.Capture(t)
		path := filepath.Join(t.TempDir(), stateFileName)
		renewed := tokenstest.Files{"1": "test-token-1", "2": "test-token-2-renewed", "3": "test-token-3"}
		records := map[string]time.Time{
			numberedTokens["2"]: takenEnding(localOn(25, 1)),
			renewed["2"]:        takenEnding(localOn(45, 1)),
		}
		var posted []string
		// run starts the router on the tokens in files, and stops it.
		run := func(files tokenstest.Files) []string {
			h, f := newKeptNotifying(t, path, files, records)
			h.start()
			h.stop()
			f.save()
			got := h.notifier.posted()
			posted = append(posted, got...)
			return got
		}
		steps := []struct {
			name  string
			files tokenstest.Files
			// after is how long after the last step the router starts.
			after time.Duration
			want  []string
		}{
			{name: "within 30 days", files: numberedTokens, want: []string{yearSaid("expires 25 Jan 2000 · 25 days left")}},
			{name: "within 30 days, told", files: numberedTokens},
			{name: "within 7 days", files: numberedTokens, after: 19 * day, want: []string{yearSaid("expires 25 Jan 2000 · 6 days left")}},
			{name: "within 7 days, told", files: numberedTokens},
			{name: "a new token, within 30 days", files: renewed, want: []string{yearSaid("expires 14 Feb 2000 · 26 days left")}},
			{name: "a new token, told", files: renewed},
		}
		for _, step := range steps {
			time.Sleep(step.after)
			if got := run(step.files); !slices.Equal(got, step.want) {
				t.Errorf("%s: posted %q, want %q", step.name, got, step.want)
			}
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := readState(t, path).TokenYears, map[string]toldYear{"2": {SHA256: hash(renewed["2"]), Within: 30}}; !maps.Equal(got, want) {
			t.Errorf("state file holds what's told as %+v, want %+v: the new token's, by its hash", got, want)
		}
		for _, token := range []string{numberedTokens["2"], renewed["2"]} {
			if strings.Contains(string(data), token) {
				t.Errorf("state file holds\n%s\nwant no token in it", data)
			}
			for _, secret := range []string{token, hash(token)} {
				if strings.Contains(log.String(), secret) {
					t.Errorf("log reads\n%s\nwant neither a token nor its hash in it", log)
				}
				if slices.ContainsFunc(posted, func(p string) bool { return strings.Contains(p, secret) }) {
					t.Errorf("posted %q, want neither a token nor its hash in any", posted)
				}
			}
		}
	})
}

func TestTheStateFileKeepsWhatsToldOfTheTokensYearsOfTheAccountsConfigured(t *testing.T) {
	path := filepath.Join(t.TempDir(), stateFileName)
	putState(t, path, savedState{
		Version: stateVersion,
		Tokens:  map[string]savedTokens{"work": {SHA256: workHash}, "side": {SHA256: sideHash}},
		TokenYears: map[string]toldYear{
			"work": {SHA256: workHash, Within: 30},
			"gone": {SHA256: renewedHash, Within: 7},
		},
	})
	f := newTestFile(at(start), testAccounts())

	f.load(path)
	want := map[string]toldYear{"work": {SHA256: workHash, Within: 30}}
	if got := f.told.saved(); !maps.Equal(got, want) {
		t.Errorf("loaded what's told as %+v, want %+v: gone is no longer configured", got, want)
	}
	if !f.changes.unsaved.Load() {
		t.Error("what's told of gone is still in the state file, want it due to be saved")
	}
	f.save()
	if got := readState(t, path).TokenYears; !maps.Equal(got, want) {
		t.Errorf("state file holds what's told as %+v, want %+v", got, want)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var older olderState
	if err := json.Unmarshal(data, &older); err != nil || older.Version != 1 {
		t.Errorf("a router from before reads the state file as version %d (%v), want version 1, which it knows", older.Version, err)
	}
}

func TestAStateFileFromBeforeTokensYearsWereToldOfHasNoneTold(t *testing.T) {
	path := filepath.Join(t.TempDir(), stateFileName)
	putState(t, path, savedState{
		Version: stateVersion,
		Tokens:  map[string]savedTokens{"work": {SHA256: workHash}, "side": {SHA256: sideHash}},
	})
	f := newTestFile(at(start), testAccounts())

	f.load(path)
	if got := f.told.saved(); len(got) != 0 {
		t.Errorf("loaded what's told as %+v, want nothing told", got)
	}
	if f.changes.unsaved.Load() {
		t.Error("loading the state file left it due to be saved, want it as it was")
	}
}

func TestAStateFileFromANewerRouterLoadsWhatsToldOfTokensYears(t *testing.T) {
	path := filepath.Join(t.TempDir(), stateFileName)
	data := `{
  "version": 1,
  "sessions": [],
  "tokens": {"work": {"sha256": "` + workHash + `"}},
  "token_years": {"work": {"sha256": "` + workHash + `", "within": 7, "told_at": "2026-09-28T13:12:00Z"}},
  "reminders": {"work": 3}
}
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	f := newTestFile(at(start), testAccounts())

	f.load(path)
	want := map[string]toldYear{"work": {SHA256: workHash, Within: 7}}
	if got := f.told.saved(); !maps.Equal(got, want) {
		t.Errorf("loaded what's told as %+v, want %+v", got, want)
	}
}
