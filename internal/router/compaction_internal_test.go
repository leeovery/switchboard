package router

import (
	"cmp"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

// sideSooner is when side's week resets in these tests, but where they say:
// tomorrow, before work's, in five days.
var sideSooner = start.Add(24 * time.Hour)

// compactedToSide is why a session moves to side at its compaction, side's
// week used as readRising has it read: at 0.6% an hour, it reaches its reset
// 64.6% used, 35.4% of it lost.
var compactedToSide = status.Compacted("side", 0.354)

func TestTheFirstRequestAfterACompactionMovesItsSessionWhereAChoiceAfreshWouldSendIt(t *testing.T) {
	// The session is on work, its cache warm, side's week read rising as
	// readRising has it: its quota needs using first, but where its week
	// resets in six days, later than work's; and with work's week 30% used,
	// work's needs using first.
	later := start.Add(6 * 24 * time.Hour)
	tests := []struct {
		name string
		// headers are what the request carries beside its session.
		headers map[string]string
		// model is the request's, Opus where it's "".
		model string
		// pin is the session's own, as it was launched with it.
		pin string
		// sideResets is when side's week resets, and sideFresh has it read
		// first as the request comes, too lately for its rate to be known.
		sideResets time.Time
		sideFresh  bool
		// workUsed is how much of work's week is used, 90% where it's 0.
		workUsed float64
		// wantOn is the account the request goes out on, which the session is
		// left on, and wantReason why; wantStays is the log's line of why the
		// session stays where it is, nil for none.
		wantOn, wantReason string
		wantStays          []string
	}{
		{
			name:       "after its compaction, another account's week resetting sooner with room at risk: moves",
			headers:    map[string]string{claude.CompactedHeader: "auto"},
			sideResets: sideSooner,
			wantOn:     "side", wantReason: compactedToSide,
		},
		{
			name:       "after a compaction by hand, the same",
			headers:    map[string]string{claude.CompactedHeader: "manual"},
			sideResets: sideSooner,
			wantOn:     "side", wantReason: compactedToSide,
		},
		{
			name:       "after its compaction, the account chosen's week resetting later: stays, saying why",
			headers:    map[string]string{claude.CompactedHeader: "auto"},
			sideResets: later,
			wantOn:     "work", wantReason: reasonSticky,
			wantStays: []string{"level=DEBUG", `msg="stays after compaction"`, "session=one", "model=" + opus, "account=work", `why="resets later"`, "chosen=side"},
		},
		{
			name:       "after its compaction, the account chosen's week read too lately for its rate to be known: stays, saying why",
			headers:    map[string]string{claude.CompactedHeader: "auto"},
			sideResets: sideSooner,
			sideFresh:  true,
			wantOn:     "work", wantReason: reasonSticky,
			wantStays: []string{"level=DEBUG", `msg="stays after compaction"`, "account=work", "why=unknown", "chosen=side"},
		},
		{
			name:       "after its compaction, its own account chosen: stays, saying why",
			headers:    map[string]string{claude.CompactedHeader: "auto"},
			sideResets: later,
			workUsed:   0.3,
			wantOn:     "work", wantReason: reasonSticky,
			wantStays: []string{"level=DEBUG", `msg="stays after compaction"`, "account=work", `why="same account"`, "chosen=work"},
		},
		{
			name:       "no compaction: not judged",
			sideResets: sideSooner,
			wantOn:     "work", wantReason: reasonSticky,
		},
		{
			name:       "the compaction itself: not judged",
			headers:    map[string]string{claude.CompactionHeader: "auto"},
			sideResets: sideSooner,
			wantOn:     "work", wantReason: reasonSticky,
		},
		{
			name:       "after its compaction, on a model whose thinking is bound to its account: not judged",
			headers:    map[string]string{claude.CompactedHeader: "auto"},
			model:      sonnet,
			sideResets: sideSooner,
			wantOn:     "work", wantReason: reasonSticky,
		},
		{
			name:       "after its compaction, held by its own pin: not judged",
			headers:    map[string]string{claude.CompactedHeader: "auto", PinHeader: "work"},
			pin:        "work",
			sideResets: sideSooner,
			wantOn:     "work", wantReason: reasonPinned,
		},
		{
			name:       "after its compaction, kept where it went when its own pin yielded: not judged",
			headers:    map[string]string{claude.CompactedHeader: "auto", PinHeader: "side"},
			pin:        "side",
			sideResets: sideSooner,
			wantOn:     "work", wantReason: reasonSticky,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			clock := &testClock{}
			prober := &stubProber{}
			r := newTestRouter(t, clock.read, prober)
			readRising(r, clock, cmp.Or(tt.workUsed, 0.9), tt.sideResets, tt.sideFresh)
			model := cmp.Or(tt.model, opus)
			k := key{session: "one", model: model}
			assign(r.sessions, k, tt.pin, decision{account: "work", reason: reasonNew}, start.Add(-5*time.Minute))
			upstream := scripted(served, served)
			r.proxy.transport = upstream

			routeWith(t, r, model, tt.headers)

			if sent := upstream.sent(); !slices.Equal(sent, []string{tt.wantOn}) {
				t.Errorf("the request went out on %q, want %s alone", sent, tt.wantOn)
			}
			if routed := []string{"msg=routed", "account=" + tt.wantOn, "reason=" + logged(tt.wantReason)}; !log.Has(routed...) {
				t.Errorf("log reads\n%s\nwant a line with %q", log, routed)
			}
			if a := r.sessions.lookup(k).current; a.Account != tt.wantOn {
				t.Errorf("the session is on %s, want %s", a.Account, tt.wantOn)
			}
			if probes := prober.counts(); len(probes) > 0 {
				t.Errorf("probes = %v, want none: every account was read as the request came", probes)
			}
			var wantMoves []eventMove
			if tt.wantOn != "work" {
				wantMoves = []eventMove{{from: "work", to: tt.wantOn, reason: tt.wantReason}}
				moved := []string{"level=INFO", "msg=moved", "session=one", "from=work", "to=" + tt.wantOn, "reason=" + logged(tt.wantReason)}
				if !log.Has(moved...) {
					t.Errorf("log reads\n%s\nwant a line with %q", log, moved)
				}
			}
			if moves := movesOf(r); !slices.Equal(moves, wantMoves) {
				t.Errorf("moves = %+v, want %+v", moves, wantMoves)
			}
			judged := slices.DeleteFunc(log.Lines(), func(line string) bool { return !strings.Contains(line, "after compaction") })
			switch {
			case tt.wantStays == nil && len(judged) > 0:
				t.Errorf("log reads\n%s\nwant no line of a judgement: a move is the routed line's and its own to tell", log)
			case tt.wantStays != nil && (len(judged) != 1 || !log.Has(tt.wantStays...)):
				t.Errorf("log reads\n%s\nwant one line of a judgement, with %q", log, tt.wantStays)
			}
		})
	}
}

func TestACompactionsMoveThatHitsALimitIsReplayedAsAnyRequestIsAndNotJudgedAgain(t *testing.T) {
	// The session is on work, its cache warm; side's week resets sooner, its
	// room at risk, so the session moves there; side answers the request at
	// its limit, and work serves it.
	log := logstest.Capture(t)
	clock := &testClock{}
	r := newTestRouter(t, clock.read, &stubProber{})
	readRising(r, clock, 0.9, sideSooner, false)
	k := key{session: "one", model: opus}
	assign(r.sessions, k, "", decision{account: "work", reason: reasonNew}, start.Add(-5*time.Minute))
	upstream := scripted(served, limitHit)
	r.proxy.transport = upstream

	routeWith(t, r, opus, map[string]string{claude.CompactedHeader: "auto"})

	if sent := upstream.sent(); !slices.Equal(sent, []string{"side", "work"}) {
		t.Errorf("the request went out on %q, want side then work", sent)
	}
	offSide := status.ReasonMovedOff + "side " + status.WhyLimit
	if routed := []string{"msg=routed", "account=work", "reason=" + logged(offSide), "attempts=2"}; !log.Has(routed...) {
		t.Errorf("log reads\n%s\nwant a line with %q", log, routed)
	}
	if a := r.sessions.lookup(k).current; a.Account != "work" || a.Reason != offSide {
		t.Errorf("the session is on %s for %q, want work for %q", a.Account, a.Reason, offSide)
	}
	want := []eventMove{{from: "work", to: "side", reason: compactedToSide}, {from: "side", to: "work", reason: offSide}}
	if moves := movesOf(r); !slices.Equal(moves, want) {
		t.Errorf("moves = %+v, want %+v", moves, want)
	}
	if log.Has("after compaction") {
		t.Errorf("log reads\n%s\nwant no line of a judgement: the replay is moved off side, and the move there was the routed line's to tell", log)
	}
}

func TestARequestThatMovedItsSessionAtItsCompactionAndWentUnansweredPutsItBack(t *testing.T) {
	log := logstest.Capture(t)
	clock := &testClock{}
	r := newTestRouter(t, clock.read, &stubProber{})
	readRising(r, clock, 0.9, sideSooner, false)
	k := key{session: "one", model: opus}
	assign(r.sessions, k, "", decision{account: "work", reason: reasonNew}, start.Add(-5*time.Minute))
	req := Request{ID: "b2c3d4e5", Session: "one", Model: opus, Compacted: true, Client: "work"}
	if got := choose(t.Context(), r, req); got.Account != "side" || got.Reason != compactedToSide || got.From != "work" {
		t.Fatalf("Choose() = %+v, want the session moved from work to side, as %q", got, compactedToSide)
	}

	if back, ok := r.proxy.chooser.Forget(req); back != "work" || !ok {
		t.Fatalf("Forget() = %q, %v, want work, true", back, ok)
	}
	if a := r.sessions.lookup(k).current; a.Account != "work" || a.Reason != reasonNew {
		t.Errorf("the session is on %s for %q, want work for %q, as it was before its request", a.Account, a.Reason, reasonNew)
	}
	if back := []string{"level=INFO", `msg="session back where it was before its request"`, "session=one", "account=work"}; !log.Has(back...) {
		t.Errorf("log reads\n%s\nwant a line with %q", log, back)
	}
}

func TestACompactionsMoveThatAProbesReadingTurnsIntoAStayKeepsTheSessionWhereItIs(t *testing.T) {
	// Side's week, as readRising has it read, last read 20 minutes before the
	// request, its usage stale: rising at 0.3% an hour over the 40 minutes
	// since it was first read, it has 42.7% of its week at risk, and the
	// session moves there, but for the probe a choice afresh waits on. That
	// reads it 95% used, used since at 67.5% an hour: at that rate it runs
	// out before its reset, nothing of it at risk.
	log := logstest.Capture(t)
	clock := &testClock{}
	prober := &stubProber{readings: map[string]quota.Probe{
		workToken: probed(nil, risingSession, quota.Window{Key: "7d", Label: "Week", Utilization: 0.9, ResetsAt: start.Add(5 * 24 * time.Hour)}),
		sideToken: probed(nil, risingSession, quota.Window{Key: "7d", Label: "Week", Utilization: 0.95, ResetsAt: sideSooner}),
	}}
	r := newTestRouter(t, clock.read, prober)
	readRising(r, clock, 0.9, sideSooner, false)
	clock.now = start.Add(20 * time.Minute)
	k := key{session: "one", model: opus}
	assign(r.sessions, k, "", decision{account: "work", reason: reasonNew}, start)

	got := choose(t.Context(), r, Request{ID: "c3d4e5f6", Session: "one", Model: opus, Compacted: true, Client: "work"})

	if want := (Choice{Account: "work", Reason: reasonSticky}); got != want {
		t.Errorf("Choose() = %+v, want %+v", got, want)
	}
	if probes := prober.counts(); probes[sideToken] != 1 {
		t.Errorf("probes = %v, want side probed once", probes)
	}
	if a := r.sessions.lookup(k).current; a.Account != "work" || a.Reason != reasonNew {
		t.Errorf("the session is on %s for %q, want work for %q, as it was", a.Account, a.Reason, reasonNew)
	}
	if moves := movesOf(r); len(moves) > 0 {
		t.Errorf("moves = %+v, want none", moves)
	}
	overturned := []string{"level=DEBUG", `msg="decision overturned"`, "account=work", "account_before=side", "reason=sticky",
		"reason_before=" + logged(status.Compacted("side", 0.427))}
	stays := []string{"level=DEBUG", `msg="stays after compaction"`, "account=work", `why="not at risk"`, "chosen=side"}
	for _, line := range [][]string{overturned, stays} {
		if !log.Has(line...) {
			t.Errorf("log reads\n%s\nwant a line with %q", log, line)
		}
	}
}

// risingSession is the 5-hour window as readRising reads it of each account.
var risingSession = quota.Window{Key: "5h", Label: "Session", Utilization: 0.1, ResetsAt: start.Add(3 * time.Hour)}

// readRising has r read, on clock, work's week used as given, resetting in
// five days, at 20 minutes before start and again at start; and side's,
// resetting at sideResets, rising from 50% to 50.2% between the two, 0.6% an
// hour, but where sideFresh has it read only at start. It leaves the clock at
// start.
func readRising(r *Router, clock *testClock, workUsed float64, sideResets time.Time, sideFresh bool) {
	workWeek := quota.Window{Key: "7d", Label: "Week", Utilization: workUsed, ResetsAt: start.Add(5 * 24 * time.Hour)}
	for i, u := range []float64{0.5, 0.502} {
		clock.now = start.Add(time.Duration(i-1) * 20 * time.Minute)
		r.state.record("work", []quota.Window{risingSession, workWeek}, r.state.mark())
		if i == 0 && sideFresh {
			continue
		}
		sideWeek := quota.Window{Key: "7d", Label: "Week", Utilization: u, ResetsAt: sideResets}
		r.state.record("side", []quota.Window{risingSession, sideWeek}, r.state.mark())
	}
}

// eventMove is a move of session one's requests as the router's events keep
// it: the account they left, the one they went to, and why.
type eventMove struct {
	from, to, reason string
}

// movesOf returns the moves of session one's requests that r keeps among its
// events, the oldest first.
func movesOf(r *Router) []eventMove {
	var moves []eventMove
	for _, e := range r.recent.events() {
		if e.Kind == status.EventMoved && e.Session == "one" {
			moves = append(moves, eventMove{from: e.From, to: e.To, reason: e.Reason})
		}
	}
	slices.Reverse(moves)
	return moves
}

// logged is a value as the log writes it: quoted where it has a space.
func logged(value string) string {
	if strings.Contains(value, " ") {
		return strconv.Quote(value)
	}
	return value
}

// routeWith has the router's proxy route a messages request of model, of the
// session "one", on work's token, carrying headers beside its session.
func routeWith(t *testing.T, r *Router, model string, headers map[string]string) {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"`+model+`","max_tokens":1}`))
	req.Header.Set("Authorization", "Bearer "+workToken)
	req.Header.Set(claude.SessionHeader, "one")
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	r.Proxy().ServeHTTP(httptest.NewRecorder(), req)
}
