package router

import (
	"cmp"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/claude/claudetest"
	"github.com/leeovery/switchboard/internal/forecast"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

// defaultHold is the hold curve the document gives before a day of answers:
// e^(−h/6h) at 1, 3, 6, 12, 24 and 48 hours.
var defaultHold = []status.HoldPoint{
	{Hours: 1, Share: math.Exp(-1.0 / 6)},
	{Hours: 3, Share: math.Exp(-3.0 / 6)},
	{Hours: 6, Share: math.Exp(-6.0 / 6)},
	{Hours: 12, Share: math.Exp(-12.0 / 6)},
	{Hours: 24, Share: math.Exp(-24.0 / 6)},
	{Hours: 48, Share: math.Exp(-48.0 / 6)},
}

// sameHold reports whether two hold curves match, to within rounding.
func sameHold(got, want []status.HoldPoint) bool {
	return slices.EqualFunc(got, want, func(g, w status.HoldPoint) bool { return near(g.Hours, w.Hours) && near(g.Share, w.Share) })
}

func TestAnAnswerReachesTheHoldCurveByItsTokensAndItsSessionsTimeOnItsAccount(t *testing.T) {
	// answered is what claudetest's answers weigh, their closing usage 3
	// tokens in, 120 out, 512 written to the cache and 40,000 read from it.
	const answered = 3 + 5*120 + 1.25*512 + 0.1*40000
	tests := []struct {
		name string
		// path is the request's, /v1/messages unless it says, of session.
		path, session string
		// work and side are the answers of each, in turn.
		work, side []answer
		// weight is what the curve takes in, from a session on its account
		// for onAccount: none where it takes in nothing.
		weight    float64
		onAccount time.Duration
	}{
		{
			name:    "a message answered whole, its session on work three hours, kept there since",
			session: "one", work: []answer{messageWhole},
			weight: answered, onAccount: 3 * time.Hour,
		},
		{
			name:    "a streamed answer, its session on work three hours",
			session: "one", work: []answer{streamed(time.Second, claudetest.AnswerPieces("Hello")...)},
			weight: answered, onAccount: 3 * time.Hour,
		},
		{
			name:    "an answer on side, its session moved there at work's limit, there no time",
			session: "one", work: []answer{limitHit}, side: []answer{messageWhole},
			weight: answered,
		},
		{
			name: "an answer to a request without a session, chosen afresh",
			work: []answer{messageWhole}, side: []answer{messageWhole},
			weight: answered,
		},
		{name: "an error, with no usage", session: "one", work: []answer{serverError}},
		{name: "a count of tokens, which spends no quota, its answer a message's", path: "/v1/messages/count_tokens", session: "one", work: []answer{messageWhole}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				// Session one came to work three hours before, and stuck
				// there ten minutes before. Side has room too, and its quota
				// needs using sooner.
				r := newTestRouter(t, at(start), &stubProber{})
				r.state.record("work", []quota.Window{session, laterWeek}, r.state.mark())
				r.state.record("side", []quota.Window{session, soonWeek}, r.state.mark())
				k := key{session: "one", model: opus}
				assign(r.sessions, k, "", decision{account: "work", reason: reasonNew}, start.Add(-3*time.Hour))
				assign(r.sessions, k, "", decision{account: "work", reason: reasonSticky, sticky: true}, start.Add(-10*time.Minute))
				r.proxy.transport = &scriptedUpstream{answers: map[string][]answer{workToken: tt.work, sideToken: tt.side}}

				routeAs(t.Context(), r, cmp.Or(tt.path, "/v1/messages"), tt.session, opusAsked)
				if got, want := r.state.hold, (forecast.Hold{}).Answered(tt.weight, tt.onAccount, start); got != want {
					t.Errorf("the hold curve is %+v, want %+v: an answer weighing %v from a session on its account %v", got, want, tt.weight, tt.onAccount)
				}
			})
		})
	}
}

func TestASessionsTimeOnItsAccountRunsFromWhenItCameThere(t *testing.T) {
	// Session one came to work at start; what befell it after, each test
	// has, and its time on each account is read three hours on.
	req := Request{ID: "a1b2c3d4", Session: "one", Model: opus}
	other := Request{ID: "e5f6a7b8", Session: "one", Model: opus}
	toSide := decision{account: "side", reason: "moved: work hit its limit"}
	toWork := decision{account: "work", reason: "moved: side hit its limit"}
	tests := []struct {
		name               string
		since              func(s *sessions)
		wantWork, wantSide time.Duration
	}{
		{
			name: "kept on work, as it stuck an hour on",
			since: func(s *sessions) {
				assignFor(s, req, decision{account: "work", reason: reasonSticky, sticky: true}, start.Add(time.Hour))
			},
			wantWork: 3 * time.Hour,
		},
		{
			name: "kept on work, rescored an hour on",
			since: func(s *sessions) {
				assignFor(s, req, decision{account: "work", reason: "rescored after 1h"}, start.Add(time.Hour))
			},
			wantWork: 3 * time.Hour,
		},
		{
			name: "kept on work, pinned there an hour on",
			since: func(s *sessions) {
				assignFor(s, Request{ID: req.ID, Session: "one", Model: opus, Pin: "work"}, decision{account: "work", reason: reasonPinned}, start.Add(time.Hour))
			},
			wantWork: 3 * time.Hour,
		},
		{
			name:     "moved to side two hours on",
			since:    func(s *sessions) { assignFor(s, req, toSide, start.Add(2*time.Hour)) },
			wantSide: time.Hour,
		},
		{
			name: "moved to side an hour on, and back to work two hours on",
			since: func(s *sessions) {
				assignFor(s, req, toSide, start.Add(time.Hour))
				assignFor(s, req, toWork, start.Add(2*time.Hour))
			},
			wantWork: time.Hour,
		},
		{
			name: "moved to side two hours on by a request taken back",
			since: func(s *sessions) {
				assignFor(s, other, toSide, start.Add(2*time.Hour))
				s.forget(other)
			},
			wantWork: 3 * time.Hour,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRouter(t, at(start), &stubProber{})
			assignFor(r.sessions, req, decision{account: "work", reason: reasonNew}, start)
			tt.since(r.sessions)

			then := start.Add(3 * time.Hour)
			if got := r.scheduler.OnAccount(req, "work", then); got != tt.wantWork {
				t.Errorf("on work for %v, want %v", got, tt.wantWork)
			}
			if got := r.scheduler.OnAccount(req, "side", then); got != tt.wantSide {
				t.Errorf("on side for %v, want %v", got, tt.wantSide)
			}
		})
	}
}

func TestTheDocumentGivesTheHoldCurve(t *testing.T) {
	clock := &testClock{now: start}
	s := newTestState(clock)
	s.record("work", []quota.Window{session, week}, s.mark())
	if got := s.document().Demand.Hold; !reflect.DeepEqual(got, defaultHold) {
		t.Errorf("before any answer, the hold curve is %+v, want the default, %+v", got, defaultHold)
	}

	// An answer from a session new to its account, and 25 hours on, one
	// weighing as much from a session on its account for 30.
	s.answered(quota.Tokens{Input: 1000}, 0, start)
	clock.now = start.Add(25 * time.Hour)
	s.answered(quota.Tokens{Output: 200}, 30*time.Hour, clock.now)
	// The first counts less by 25 hours of a week's time constant.
	held := 1000 / (1000*math.Exp(-25.0/(7*24)) + 1000)
	want := []status.HoldPoint{{Hours: 1, Share: held}, {Hours: 3, Share: held}, {Hours: 6, Share: held}, {Hours: 12, Share: held}, {Hours: 24, Share: held}, {Hours: 48}}
	if got := s.document().Demand.Hold; !sameHold(got, want) {
		t.Errorf("a day of answers on, the hold curve is %+v, want %+v", got, want)
	}
}

func TestAnAnswerIsKeptAsARoutineChange(t *testing.T) {
	f := newTestFile(at(start), testAccounts())
	f.state.answered(quota.Tokens{Input: 1000}, 0, start)
	if !f.changes.unsaved.Load() || len(f.changes.noted) > 0 {
		t.Errorf("an answer left the state file unsaved %v, a write due at once %v; want it unsaved, waiting for the next routine write",
			f.changes.unsaved.Load(), len(f.changes.noted) > 0)
	}
}

func TestTheHoldCurveOutlivesARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	clock := &testClock{now: start}
	saved := newTestFile(clock.read, testAccounts())
	saved.load(path)
	saved.state.record("work", []quota.Window{session, week}, saved.state.mark())
	saved.state.answered(quota.Tokens{Input: 1000}, 0, start)
	clock.now = start.Add(25 * time.Hour)
	saved.state.answered(quota.Tokens{Input: 1000}, 2*time.Hour, clock.now)
	saved.save()
	want := saved.state.document().Demand.Hold
	if sameHold(want, defaultHold) {
		t.Fatalf("the hold curve is the default, %+v, before the restart: want a day of answers in it", want)
	}

	loaded := newTestFile(clock.read, testAccounts())
	loaded.load(path)
	if got := loaded.state.document().Demand.Hold; !sameHold(got, want) {
		t.Errorf("after a restart, the hold curve is %+v, want it as it was, %+v", got, want)
	}
}

func TestAStateFileWithoutAHoldCurveOrWithAnOddOneLoadsWholeWithTheDefault(t *testing.T) {
	tests := []struct {
		name string
		// hold is the file's hold field, with its comma, or none.
		hold string
	}{
		{name: "none, as from before it was kept"},
		{name: "null", hold: `"hold": null,`},
		{name: "a string", hold: `"hold": "odd",`},
		{name: "a number", hold: `"hold": 7,`},
		{name: "a list", hold: `"hold": [1, 2],`},
		{name: "too few bins", hold: `"hold": {"weights": [1, 2], "weighed_at": "2026-09-28T13:00:00Z", "first_answer": "2026-09-27T13:00:00Z"},`},
		{name: "a time that isn't one", hold: `"hold": {"weights": [1, 0, 0, 0, 0, 0, 0], "weighed_at": "soon", "first_answer": "2026-09-27T13:00:00Z"},`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			file := `{
  "version": 1,
  "sessions": [
    {"session": "one", "model": "claude-opus-5-5", "account": "work", "reason": "new", "assigned_at": "2026-09-28T12:12:00Z", "last_seen": "2026-09-28T13:11:00Z"}
  ],
  ` + tt.hold + `
  "readings": {
    "work": {
      "read_at": "2026-09-28T13:00:00Z",
      "windows": [
        {"key": "7d", "label": "Week", "utilization": 0.93, "resets_at": "2026-10-02T21:00:00Z", "status": "allowed_warning"}
      ]
    }
  }
}
`
			if err := os.WriteFile(path, []byte(file), 0o600); err != nil {
				t.Fatal(err)
			}
			f := newTestFile(at(start.Add(48*time.Hour)), testAccounts())

			f.load(path)
			if got := f.sessions.lookup(key{session: "one", model: opus}); !got.assigned || got.current.Account != "work" {
				t.Errorf("the session loaded as %+v, want it on work, as the file holds", got)
			}
			if got := f.state.usage["work"].windows["7d"]; got != week {
				t.Errorf("work's week loaded as %+v, want %+v, as the file holds", got, week)
			}
			if got := f.state.document().Demand.Hold; !reflect.DeepEqual(got, defaultHold) {
				t.Errorf("the hold curve loaded as %+v, want the default, %+v", got, defaultHold)
			}
			if aside, _ := filepath.Glob(path + ".corrupt-*"); len(aside) > 0 {
				t.Errorf("the state file was set aside as %v, want it loaded", aside)
			}
		})
	}
}
