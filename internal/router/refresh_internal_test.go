package router

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

func TestRefreshProbesOnlyTheAccountsOlderThanAsked(t *testing.T) {
	tests := []struct {
		name   string
		maxAge string
		// before readies the router's state, on a clock the test then moves
		// to start.
		before func(r *Router, clock *testClock)
		want   map[string]int
	}{
		{
			name:   "one read too long ago, and one read lately",
			maxAge: "30m",
			before: func(r *Router, clock *testClock) {
				clock.now = start.Add(-40 * time.Minute)
				r.state.record("side", []quota.Window{session, week}, r.state.mark())
				clock.now = start.Add(-10 * time.Minute)
				r.state.record("work", []quota.Window{session, week}, r.state.mark())
			},
			want: map[string]int{sideToken: 1},
		},
		{
			name:   "both read too long ago for a shorter age",
			maxAge: "5m",
			before: func(r *Router, clock *testClock) {
				clock.now = start.Add(-40 * time.Minute)
				r.state.record("side", []quota.Window{session, week}, r.state.mark())
				clock.now = start.Add(-10 * time.Minute)
				r.state.record("work", []quota.Window{session, week}, r.state.mark())
			},
			want: map[string]int{workToken: 1, sideToken: 1},
		},
		{
			name:   "never read",
			maxAge: "30m",
			before: func(*Router, *testClock) {},
			want:   map[string]int{workToken: 1, sideToken: 1},
		},
		{
			name:   "not one whose session has lapsed, however long ago it was read",
			maxAge: "30m",
			before: func(r *Router, clock *testClock) {
				clock.now = start.Add(-40 * time.Minute)
				lapsing := session
				lapsing.ResetsAt = start.Add(-time.Minute)
				r.state.record("work", []quota.Window{lapsing, week}, r.state.mark())
				r.state.record("side", []quota.Window{session, week}, r.state.mark())
			},
			want: map[string]int{sideToken: 1},
		},
		{
			name:   "not one whose probe failed in the last minute",
			maxAge: "30m",
			before: func(r *Router, clock *testClock) {
				clock.now = start.Add(-30 * time.Second)
				r.state.recordProbe("side", quota.Probe{}, errors.New("HTTP 529 · Overloaded"), r.state.mark())
			},
			want: map[string]int{workToken: 1},
		},
		{
			name:   "any read at all, however lately, for no age",
			maxAge: "0s",
			before: func(r *Router, clock *testClock) {
				clock.now = start.Add(-time.Second)
				r.state.record("work", []quota.Window{session, week}, r.state.mark())
				r.state.record("side", []quota.Window{session, week}, r.state.mark())
			},
			want: map[string]int{workToken: 1, sideToken: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			prober := &stubProber{readings: map[string]quota.Probe{
				workToken: probed(nil, session, week),
				sideToken: probed(nil, session, week),
			}}
			r := newTestRouter(t, clock.read, prober)
			tt.before(r, clock)
			clock.now = start

			got := askToRefresh(t.Context(), r, `{"max_age": "`+tt.maxAge+`"}`)
			if got.Code != http.StatusOK {
				t.Fatalf("POST /refresh answered %d %s, want 200", got.Code, got.Body)
			}
			if probes := prober.counts(); !maps.Equal(probes, tt.want) {
				t.Errorf("probes = %v, want %v", probes, tt.want)
			}
		})
	}
}

func TestRefreshProbesAnAccountWhoseSessionHasLapsedWhenItCanTakeNoRequest(t *testing.T) {
	tests := []struct {
		name string
		// spent is set when work's week was read spent.
		spent bool
		// holdBack holds work back, its session lapsed, at the moment it's
		// called, a minute before start.
		holdBack   func(s *state)
		wantProbed bool
	}{
		{
			name:       "a limit reached in its week",
			spent:      true,
			holdBack:   func(s *state) { s.limit("work", []string{"7d"}, start.Add(72*time.Hour)) },
			wantProbed: true,
		},
		{
			name:       "a limit reached in no window named",
			holdBack:   func(s *state) { s.limit("work", nil, start.Add(72*time.Hour)) },
			wantProbed: true,
		},
		{
			name:       "its week read spent, with no limit, as a restart leaves it",
			spent:      true,
			holdBack:   func(*state) {},
			wantProbed: true,
		},
		{
			name:     "a limit that has lifted",
			holdBack: func(s *state) { s.limit("work", []string{"7d"}, start.Add(-time.Second)) },
		},
		{
			name:     "a limit reached in its Fable week, which Opus requests can still go out beside",
			holdBack: func(s *state) { s.limit("work", []string{"7d_oi"}, start.Add(72*time.Hour)) },
		},
		{
			name:     "its token refused",
			holdBack: func(s *state) { s.refuse("work", http.StatusUnauthorized, someRequest) },
		},
		{
			name:     "a model refused",
			holdBack: func(s *state) { s.forbid("work", "opus", http.StatusForbidden, someRequest) },
		},
		{
			name:     "nothing",
			holdBack: func(*state) {},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start.Add(-6 * time.Hour)}
			prober := &stubProber{readings: map[string]quota.Probe{workToken: probed(nil, session, week)}}
			r := newTestRouter(t, clock.read, prober)
			// Read six hours ago, work's session has lapsed since, with
			// nothing read of it; side was read just now.
			lapsing, workWeek := session, week
			lapsing.ResetsAt = start.Add(-time.Hour)
			if tt.spent {
				workWeek.Utilization, workWeek.Status = 1, quota.StatusRejected
			}
			r.state.record("work", []quota.Window{lapsing, workWeek}, r.state.mark())
			clock.now = start.Add(-time.Minute)
			tt.holdBack(r.state)
			r.state.record("side", []quota.Window{session, week}, r.state.mark())
			clock.now = start

			// As the dashboard's r asks: what hasn't been read in a minute.
			askToRefresh(t.Context(), r, `{"max_age": "1m"}`)
			if probed := prober.counts()[workToken] > 0; probed != tt.wantProbed {
				t.Errorf("work probed = %v, want %v", probed, tt.wantProbed)
			}
		})
	}
}

func TestRefreshAnswersWithTheStatusDocumentItLeaves(t *testing.T) {
	prober := &stubProber{readings: map[string]quota.Probe{workToken: probed(nil, session, week)}}
	r := newTestRouter(t, at(start), prober)

	got := askToRefresh(t.Context(), r, `{"max_age": "30m"}`)
	var doc status.Document
	if err := json.NewDecoder(got.Body).Decode(&doc); err != nil {
		t.Fatalf("POST /refresh answered %q: %v", got.Body, err)
	}
	work, _ := doc.Account("work")
	if doc.Source != status.SourceRouter || got.Header().Get("Content-Type") != "application/json" || len(work.Windows) != 2 {
		t.Errorf("POST /refresh answered %+v (%s), want the router's document as JSON, with what work's probe read", doc, got.Header().Get("Content-Type"))
	}
}

func TestRefreshWaitsTenSecondsAtMost(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := logstest.Capture(t)
		prober := &stubProber{gate: make(chan struct{}), readings: map[string]quota.Probe{
			workToken: probed(nil, session, week),
		}}
		r := newTestRouter(t, at(start), prober)
		defer r.probes.stop()

		began := time.Now()
		got := askToRefresh(t.Context(), r, `{"max_age": "30m"}`)
		if waited := time.Since(began); waited != 10*time.Second {
			t.Errorf("POST /refresh waited %v for probes that never end, want 10s", waited)
		}
		if got.Code != http.StatusOK {
			t.Errorf("POST /refresh answered %d, want 200 with the document as it stands", got.Code)
		}
		if !log.Has("level=DEBUG", `msg="stopped waiting for probes to refresh"`, "accounts=work,side", "after=10s") {
			t.Errorf("log reads\n%s\nwant the wait cut short", log)
		}

		close(prober.gate)
		synctest.Wait()
		if work, _ := r.Status().Account("work"); len(work.Windows) != 2 {
			t.Errorf("once the probes end, work = %+v, want what its probe read: they carry on after the answer", work)
		}
	})
}

func TestRefreshStopsWaitingWhenItsClientGoes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newTestRouter(t, at(start), &stubProber{gate: make(chan struct{})})
		defer r.probes.stop()
		ctx, cancel := context.WithCancel(t.Context())
		time.AfterFunc(time.Second, cancel)

		began := time.Now()
		askToRefresh(ctx, r, `{"max_age": "30m"}`)
		if waited := time.Since(began); waited != time.Second {
			t.Errorf("POST /refresh waited %v, want 1s, when its client went", waited)
		}
	})
}

func TestRefreshesAreLogged(t *testing.T) {
	log := logstest.Capture(t)
	r := newTestRouter(t, at(start), &stubProber{})

	askToRefresh(t.Context(), r, `{"max_age": "30m"}`)
	if !log.Has("level=DEBUG", "msg=refreshed", "accounts=work,side", "max_age=30m0s", "duration=") {
		t.Errorf("log reads\n%s\nwant the refresh, and how long it took", log)
	}
}

// askToRefresh has the router's control API answer POST /refresh with body,
// under ctx.
func askToRefresh(ctx context.Context, r *Router, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/refresh", strings.NewReader(body))
	rec := httptest.NewRecorder()
	r.Control().ServeHTTP(rec, req)
	return rec
}
