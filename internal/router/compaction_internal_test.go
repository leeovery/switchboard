package router

import (
	"cmp"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
)

func TestTheFirstRequestAfterACompactionIsJudgedForAMoveThatIsOnlyLogged(t *testing.T) {
	// The session is on work, whose week is 90% used and resets in five days,
	// and was last seen five minutes ago, its cache warm. Side's week resets
	// tomorrow, and rises from 50% to 50.2% over the 20 minutes before the
	// request, 0.6% an hour: at that rate it reaches its reset 64.6% used,
	// 35.4% of it lost. Side's quota needs using first either way, but its
	// week resetting in six days resets later than work's; and with work's
	// week 30% used, work's needs using first.
	sooner, later := start.Add(24*time.Hour), start.Add(6*24*time.Hour)
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
		// wantReason is why the request goes out on work, and want the log's
		// line of the judgement, nil for none.
		wantReason string
		want       []string
	}{
		{
			name:       "after its compaction, another account's week resetting sooner with room at risk: would move",
			headers:    map[string]string{claude.CompactedHeader: "auto"},
			sideResets: sooner,
			wantReason: reasonSticky,
			want: []string{"level=INFO", `msg="would move after compaction"`, "session=one", "model=" + opus, "account=work", "to=side",
				`rate="0.6% an hour"`, "resets=" + sooner.Local().Format("2006-01-02T15:04:05.000-07:00"), "left=35.4%"},
		},
		{
			name:       "after a compaction by hand, the same",
			headers:    map[string]string{claude.CompactedHeader: "manual"},
			sideResets: sooner,
			wantReason: reasonSticky,
			want:       []string{"level=INFO", `msg="would move after compaction"`, "account=work", "to=side"},
		},
		{
			name:       "after its compaction, the account chosen's week resetting later: stays, saying why",
			headers:    map[string]string{claude.CompactedHeader: "auto"},
			sideResets: later,
			wantReason: reasonSticky,
			want:       []string{"level=DEBUG", `msg="stays after compaction"`, "session=one", "account=work", `why="resets later"`, "chosen=side"},
		},
		{
			name:       "after its compaction, the account chosen's week read too lately for its rate to be known: stays, saying why",
			headers:    map[string]string{claude.CompactedHeader: "auto"},
			sideResets: sooner,
			sideFresh:  true,
			wantReason: reasonSticky,
			want:       []string{"level=DEBUG", `msg="stays after compaction"`, "account=work", "why=unknown", "chosen=side"},
		},
		{
			name:       "after its compaction, its own account chosen: stays, saying why",
			headers:    map[string]string{claude.CompactedHeader: "auto"},
			sideResets: later,
			workUsed:   0.3,
			wantReason: reasonSticky,
			want:       []string{"level=DEBUG", `msg="stays after compaction"`, "account=work", `why="same account"`, "chosen=work"},
		},
		{
			name:       "no compaction: not judged",
			sideResets: sooner,
			wantReason: reasonSticky,
		},
		{
			name:       "the compaction itself: not judged",
			headers:    map[string]string{claude.CompactionHeader: "auto"},
			sideResets: sooner,
			wantReason: reasonSticky,
		},
		{
			name:       "after its compaction, on a model whose thinking is bound to its account: not judged",
			headers:    map[string]string{claude.CompactedHeader: "auto"},
			model:      sonnet,
			sideResets: sooner,
			wantReason: reasonSticky,
		},
		{
			name:       "after its compaction, held by its own pin: not judged",
			headers:    map[string]string{claude.CompactedHeader: "auto", PinHeader: "work"},
			pin:        "work",
			sideResets: sooner,
			wantReason: reasonPinned,
		},
		{
			name:       "after its compaction, kept where it went when its own pin yielded: not judged",
			headers:    map[string]string{claude.CompactedHeader: "auto", PinHeader: "side"},
			pin:        "side",
			sideResets: sooner,
			wantReason: reasonSticky,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			clock := &testClock{now: start.Add(-20 * time.Minute)}
			prober := &stubProber{}
			r := newTestRouter(t, clock.read, prober)
			fiveHours := quota.Window{Key: "5h", Label: "Session", Utilization: 0.1, ResetsAt: start.Add(3 * time.Hour)}
			workWeek := quota.Window{Key: "7d", Label: "Week", Utilization: cmp.Or(tt.workUsed, 0.9), ResetsAt: start.Add(5 * 24 * time.Hour)}
			for i, u := range []float64{0.5, 0.502} {
				clock.now = start.Add(time.Duration(i-1) * 20 * time.Minute)
				r.state.record("work", []quota.Window{fiveHours, workWeek}, r.state.mark())
				if i == 0 && tt.sideFresh {
					continue
				}
				sideWeek := quota.Window{Key: "7d", Label: "Week", Utilization: u, ResetsAt: tt.sideResets}
				r.state.record("side", []quota.Window{fiveHours, sideWeek}, r.state.mark())
			}
			model := cmp.Or(tt.model, opus)
			k := key{session: "one", model: model}
			assign(r.sessions, k, tt.pin, decision{account: "work", reason: reasonNew}, start.Add(-5*time.Minute))
			upstream := scripted(served, served)
			r.proxy.transport = upstream

			routeWith(t, r, model, tt.headers)

			if sent := upstream.sent(); !slices.Equal(sent, []string{"work"}) {
				t.Errorf("the request went out on %q, want work alone: the router only watches", sent)
			}
			if routed := []string{"msg=routed", "account=work", "reason=" + tt.wantReason}; !log.Has(routed...) {
				t.Errorf("log reads\n%s\nwant a line with %q", log, routed)
			}
			if a := r.sessions.lookup(k).current; a.Account != "work" {
				t.Errorf("the session is on %s, want work, where it was", a.Account)
			}
			if probes := prober.counts(); len(probes) > 0 {
				t.Errorf("probes = %v, want none: judging the move probes nothing", probes)
			}
			judged := slices.DeleteFunc(log.Lines(), func(line string) bool { return !strings.Contains(line, "after compaction") })
			switch {
			case tt.want == nil && len(judged) > 0:
				t.Errorf("log reads\n%s\nwant no judgement of a move", log)
			case tt.want != nil && (len(judged) != 1 || !log.Has(tt.want...)):
				t.Errorf("log reads\n%s\nwant one judgement of a move, a line with %q", log, tt.want)
			}
		})
	}
}

func TestAJudgedRequestReplayedAtALimitIsJudgedOnceAndRoutedAsWithoutTheJudgement(t *testing.T) {
	// The session is on work, its cache warm, and side's week resets sooner,
	// read rising over the 20 minutes before the request; work answers the
	// request at its limit, and side serves it.
	type routing struct {
		sent   []string
		routed string
		on     string
		judged int
	}
	route := func(t *testing.T, headers map[string]string) routing {
		log := logstest.Capture(t)
		clock := &testClock{now: start.Add(-20 * time.Minute)}
		r := newTestRouter(t, clock.read, &stubProber{})
		for _, u := range []float64{0.5, 0.502} {
			r.state.record("work", []quota.Window{session, laterWeek}, r.state.mark())
			r.state.record("side", []quota.Window{session, {Key: "7d", Label: "Week", Utilization: u, ResetsAt: soonWeek.ResetsAt}}, r.state.mark())
			clock.now = start
		}
		k := key{session: "one", model: opus}
		assign(r.sessions, k, "", decision{account: "work", reason: reasonNew}, start.Add(-5*time.Minute))
		upstream := scripted(limitHit, served)
		r.proxy.transport = upstream

		routeWith(t, r, opus, headers)
		routed := slices.DeleteFunc(log.Lines(), func(line string) bool { return !strings.Contains(line, "msg=routed") })
		judged := slices.DeleteFunc(log.Lines(), func(line string) bool { return !strings.Contains(line, "after compaction") })
		if len(routed) != 1 {
			t.Fatalf("log reads\n%s\nwant one routed line", log)
		}
		return routing{sent: upstream.sent(), routed: routedPart(routed[0]), on: r.sessions.lookup(k).current.Account, judged: len(judged)}
	}
	without := route(t, nil)
	with := route(t, map[string]string{claude.CompactedHeader: "auto"})

	if !slices.Equal(without.sent, []string{"work", "side"}) {
		t.Fatalf("the request went out on %q, want work then side", without.sent)
	}
	if with.judged != 1 || without.judged != 0 {
		t.Errorf("judged %d times after a compaction and %d without one, want once and never: the replay chooses afresh", with.judged, without.judged)
	}
	if !slices.Equal(with.sent, without.sent) || with.routed != without.routed || with.on != without.on {
		t.Errorf("after a compaction the request went out on %q, routed as %q, the session left on %s; want %q, %q and %s, as without one",
			with.sent, with.routed, with.on, without.sent, without.routed, without.on)
	}
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

// routedPart is a routed line from its session on, but for how long the
// request took, which differs run to run.
func routedPart(line string) string {
	fields := strings.Fields(line[strings.Index(line, " session="):])
	return strings.Join(slices.DeleteFunc(fields, func(f string) bool { return strings.HasPrefix(f, "duration=") }), " ")
}
