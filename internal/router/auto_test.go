package router_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

// assigned names a session's assignment for one model.
type assigned struct {
	session, model string
}

// Auto, as the router names it for each of a session's models, is where the
// session's next request of the model goes once its own pin is cleared, as
// picking auto on its routing card clears it: the router's own rule for a
// session of no pin of its own, which keeps it where it runs while that
// account has room and a move would cost it something, the global pin
// counting. Each moment ends with the router reading every account afresh,
// as the dashboard has it do every interval, so the next requests' choices
// know what the listing knew.
func TestAutoIsWhereASessionsNextRequestGoesOnceItsPinIsCleared(t *testing.T) {
	// Of two weeks used alike, the one resetting sooner needs using first.
	sooner, later := weekOf(0.5, 24*time.Hour), weekOf(0.5, 5*24*time.Hour)
	allButSpent := weekOf(0.99, 24*time.Hour)
	tests := []struct {
		name      string
		configure []func(*router.Config)
		// arrange routes the moment's sessions, leaving the accounts to be
		// read as they're to stand.
		arrange func(t *testing.T, r *routed, client *router.Client)
		// launched is the pin each session was launched with, by its id,
		// which its requests go on carrying.
		launched map[string]string
		want     map[assigned]string
	}{
		{
			name: "a warm cache, so it stays though another needs using first",
			arrange: func(t *testing.T, r *routed, _ *router.Client) {
				r.readsAs(workToken, session, later)
				r.readsAs(sideToken, session, allButSpent)
				r.ask(t, "one", opus, "")
				r.readsAs(sideToken, session, sooner)
			},
			want: map[assigned]string{{"one", opus}: "work"},
		},
		{
			name: "a cold cache, its model's thinking bound to its account, so it stays",
			arrange: func(t *testing.T, r *routed, _ *router.Client) {
				r.readsAs(workToken, session, later)
				r.readsAs(sideToken, session, allButSpent)
				r.ask(t, "one", sonnet, "")
				r.readsAs(sideToken, session, sooner)
				r.clock.advance(2 * time.Hour)
			},
			want: map[assigned]string{{"one", sonnet}: "work"},
		},
		{
			name: "a cold cache, free to move, so it's chosen afresh",
			arrange: func(t *testing.T, r *routed, _ *router.Client) {
				r.readsAs(workToken, session, later)
				r.readsAs(sideToken, session, allButSpent)
				r.ask(t, "one", opus, "")
				r.readsAs(sideToken, session, sooner)
				r.clock.advance(2 * time.Hour)
			},
			want: map[assigned]string{{"one", opus}: "side"},
		},
		{
			name: "no room on its account, so it moves",
			arrange: func(t *testing.T, r *routed, _ *router.Client) {
				r.readsAs(workToken, session, sooner)
				r.readsAs(sideToken, session, later)
				r.ask(t, "one", opus, "")
				r.readsAs(workToken, sessionSpent, sooner)
			},
			want: map[assigned]string{{"one", opus}: "side"},
		},
		{
			name: "a global pin moving running sessions, set once it ran",
			arrange: func(t *testing.T, r *routed, client *router.Client) {
				r.readsAs(workToken, session, sooner)
				r.readsAs(sideToken, session, later)
				r.ask(t, "one", opus, "")
				r.clock.advance(time.Minute)
				pinning(t, client, router.PinRequest{Accounts: []string{"side"}, Move: true})
			},
			want: map[assigned]string{{"one", opus}: "side"},
		},
		{
			name:      "a global pin naming the account at its cap it runs on, which the pin spends, so it stays",
			configure: []func(*router.Config){workPrimary},
			arrange: func(t *testing.T, r *routed, client *router.Client) {
				r.readsAs(workToken, session, sooner)
				r.readsAs(sideToken, session, later)
				r.ask(t, "one", opus, "")
				r.readsAs(workToken, sessionAt(0.95), sooner)
				r.readsAs(sideToken, session, weekOf(0.5, 12*time.Hour))
				pinning(t, client, router.PinRequest{Accounts: []string{"work", "side"}})
			},
			want: map[assigned]string{{"one", opus}: "work"},
		},
		{
			name: "a global pin moving running sessions, set before they started",
			arrange: func(t *testing.T, r *routed, client *router.Client) {
				r.readsAs(workToken, session, sooner)
				r.readsAs(sideToken, session, later)
				pinning(t, client, router.PinRequest{Accounts: []string{"side"}, Move: true})
				r.clock.advance(time.Minute)
				r.ask(t, "launched", opus, "work")
				r.ask(t, "after", opus, "")
			},
			launched: map[string]string{"launched": "work"},
			want:     map[assigned]string{{"launched", opus}: "work", {"after", opus}: "side"},
		},
		{
			name: "sessions with pins of their own, which auto passes over",
			arrange: func(t *testing.T, r *routed, client *router.Client) {
				r.readsAs(workToken, session, sooner)
				r.readsAs(sideToken, session, later)
				r.ask(t, "launched", opus, "side")
				r.clock.advance(2 * time.Hour)
				r.ask(t, "given", opus, "")
				if _, err := client.PinSession(t.Context(), "given", "side", ""); err != nil {
					t.Fatalf("PinSession() error = %v", err)
				}
			},
			launched: map[string]string{"launched": "side"},
			want:     map[assigned]string{{"launched", opus}: "work", {"given", opus}: "work"},
		},
		{
			name: "two models on two accounts",
			arrange: func(t *testing.T, r *routed, client *router.Client) {
				r.readsAs(workToken, session, sooner)
				r.readsAs(sideToken, session, later)
				r.ask(t, "one", opus, "")
				r.readsAs(sideToken, session, weekOf(0.5, 12*time.Hour))
				readAfresh(t, r, client)
				r.ask(t, "one", haiku, "")
			},
			want: map[assigned]string{{"one", opus}: "work", {"one", haiku}: "side"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRouted(t, tt.configure...)
			client := router.NewClient(serveControl(t, r.rt))
			tt.arrange(t, r, client)
			readAfresh(t, r, client)

			named := autosNamed(t, client, sessionsOf(tt.want))
			if !maps.Equal(named, tt.want) {
				t.Errorf("auto names %v, want %v", named, tt.want)
			}
			for _, id := range sessionsOf(tt.want) {
				if _, err := client.UnpinSession(t.Context(), id, router.ByDashboard); err != nil {
					t.Fatalf("UnpinSession(%s) error = %v", id, err)
				}
				for _, model := range modelsOf(tt.want, id) {
					if sent, auto := r.ask(t, id, model, tt.launched[id]), named[assigned{id, model}]; sent != auto {
						t.Errorf("session %s's next %s request, its pin cleared, went to %s, where auto named %q", id, model, sent, auto)
					}
				}
			}
		})
	}
}

// Where no account has room for a session's next request, the router would
// probe them again before it sent it on any, and then only for the upstream
// to refuse it, or on none: it can't say where auto sends it, and leaves it
// out.
func TestAutoIsLeftOutWhereNoAccountHasRoom(t *testing.T) {
	tests := []struct {
		name string
		// reserve is the share each account keeps back, and read the session
		// each reads, once the session is routed.
		reserve float64
		read    quota.Window
	}{
		{name: "every account at its limit", read: sessionSpent},
		{name: "every account held back by its reserve alone", reserve: 0.1, read: sessionAt(0.95)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRouted(t, func(cfg *router.Config) {
				for i := range cfg.Accounts {
					cfg.Accounts[i].Reserve = tt.reserve
				}
			})
			client := router.NewClient(serveControl(t, r.rt))
			r.readsAs(workToken, session, weekOf(0.5, 24*time.Hour))
			r.readsAs(sideToken, session, weekOf(0.5, 5*24*time.Hour))
			r.ask(t, "one", opus, "")
			r.readsAs(workToken, tt.read, weekOf(0.5, 24*time.Hour))
			r.readsAs(sideToken, tt.read, weekOf(0.5, 5*24*time.Hour))
			readAfresh(t, r, client)

			if named := autosNamed(t, client, []string{"one"}); named[assigned{"one", opus}] != "" {
				t.Errorf("auto names %v, want none for the session's opus", named)
			}
			rec := httptest.NewRecorder()
			r.rt.Control().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/sessions", nil))
			var listed []struct {
				Assignments []map[string]json.RawMessage `json:"assignments"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil || len(listed) != 1 || len(listed[0].Assignments) != 1 {
				t.Fatalf("GET /sessions answered %s (%v), want the session, with its one assignment", rec.Body, err)
			}
			if auto, ok := listed[0].Assignments[0]["auto"]; ok {
				t.Errorf("GET /sessions gives the session's assignment auto %s, want it left out", auto)
			}
		})
	}
}

func TestTheSessionsAreListedWithAutoSafelyBesideRequests(t *testing.T) {
	r := newRouted(t)
	client := router.NewClient(serveControl(t, r.rt))
	r.readsAs(workToken, session, weekOf(0.5, 24*time.Hour))
	r.readsAs(sideToken, session, weekOf(0.5, 5*24*time.Hour))

	var wg sync.WaitGroup
	for i := range 16 {
		id, model := fmt.Sprint("session-", i%4), []string{opus, haiku, sonnet}[i%3]
		wg.Go(func() {
			if got := asking(r.proxy, id, messageFor(model)); got != http.StatusOK {
				t.Errorf("session %s's %s request was answered %d, want %d", id, model, got, http.StatusOK)
			}
		})
		wg.Go(func() {
			listed, err := client.Sessions(t.Context())
			if err != nil {
				t.Errorf("Sessions() error = %v", err)
			}
			for _, s := range listed {
				checkAutos(t, "Sessions()", s)
			}
		})
		wg.Go(func() {
			s, err := client.Session(t.Context(), id)
			switch {
			case errors.Is(err, router.ErrUnknownSession):
			case err != nil:
				t.Errorf("Session(%s) error = %v", id, err)
			default:
				checkAutos(t, "Session()", s)
			}
		})
		wg.Go(func() {
			if _, err := client.Pin(t.Context(), router.PinRequest{Accounts: []string{"side"}, Move: i%2 == 0}); err != nil {
				t.Errorf("Pin() error = %v", err)
			}
			if _, err := client.Unpin(t.Context(), false, ""); err != nil {
				t.Errorf("Unpin() error = %v", err)
			}
		})
		wg.Go(func() {
			if _, err := client.PinSession(t.Context(), id, "work", ""); err != nil && !errors.Is(err, router.ErrUnknownSession) {
				t.Errorf("PinSession(%s) error = %v", id, err)
			}
		})
	}
	wg.Wait()
}

// readAfresh has the router read every account afresh a minute on, as the
// dashboard has it do every interval: a minute after the probes before it,
// as the router probes an account no sooner.
func readAfresh(t *testing.T, r *routed, client *router.Client) {
	t.Helper()
	r.clock.advance(time.Minute)
	if _, err := client.Refresh(t.Context(), 0); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
}

// autosNamed returns the account auto sends each assignment of the sessions
// with the ids given to, as GET /sessions/{id} names it, "" where it names
// none, and fails the test where GET /sessions, listing one of them, names it
// otherwise.
func autosNamed(t *testing.T, client *router.Client, ids []string) map[assigned]string {
	t.Helper()
	listed, err := client.Sessions(t.Context())
	if err != nil {
		t.Fatalf("Sessions() error = %v", err)
	}
	named := make(map[assigned]string)
	for _, id := range ids {
		s, err := client.Session(t.Context(), id)
		if err != nil {
			t.Fatalf("Session(%s) error = %v", id, err)
		}
		for _, a := range s.Assignments {
			named[assigned{id, a.Model}] = a.Auto
		}
	}
	for _, s := range listed {
		for _, a := range s.Assignments {
			if alone := named[assigned{s.ID, a.Model}]; a.Auto != alone {
				t.Errorf("Sessions() names auto %q for session %s's %s, and Session() %q: want them alike", a.Auto, s.ID, a.Model, alone)
			}
		}
	}
	return named
}

// sessionsOf returns the ids of the sessions of the assignments given, in
// order.
func sessionsOf(of map[assigned]string) []string {
	var ids []string
	for a := range of {
		if !slices.Contains(ids, a.session) {
			ids = append(ids, a.session)
		}
	}
	slices.Sort(ids)
	return ids
}

// modelsOf returns the models of the assignments given of the session with
// the given id, in order.
func modelsOf(of map[assigned]string, id string) []string {
	var models []string
	for a := range of {
		if a.session == id {
			models = append(models, a.model)
		}
	}
	slices.Sort(models)
	return models
}

// checkAutos fails the test where what, giving s, names auto any account but
// one a request can go out on, or none.
func checkAutos(t *testing.T, what string, s status.Session) {
	t.Helper()
	for _, a := range s.Assignments {
		if !slices.Contains([]string{"", "work", "side"}, a.Auto) {
			t.Errorf("%s names auto %q for session %s's %s, want work, side or none", what, a.Auto, s.ID, a.Model)
		}
	}
}
