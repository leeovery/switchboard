package watch

import (
	"maps"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/views"
)

// The sessions of the pools' projects: Default's in api twenty days back and
// in web today; Acme's in api five days back and today; and one in api
// yesterday, routed in Default, then resumed in Acme, as once its
// directory's fence changes.
const (
	defaultEarly = "e1000000-0000-4000-8000-000000000001"
	acmeEarly    = "e2000000-0000-4000-8000-000000000002"
	splitSession = "e3000000-0000-4000-8000-000000000003"
	acmeToday    = "e4000000-0000-4000-8000-000000000004"
	defaultWeb   = "e5000000-0000-4000-8000-000000000005"
)

// inPool are projectAsks' requests, on the account given, routed in the pool
// with the given id.
func inPool(lines []ledger.Held, account, pool string) []ledger.Held {
	for i := range lines {
		lines[i].Account, lines[i].Pool = account, pool
	}
	return lines
}

// pooledProjects are the lines of the pools' projects, as their sessions
// have them, a minute apart from each start.
var pooledProjects = slices.Concat(
	inPool(projectAsks(defaultEarly, "/code/api", start.Add(-20*day), 1), "work", "b2n8"),
	inPool(projectAsks(acmeEarly, "/code/api", start.Add(-5*day), 2), "k3f9", "q7mt"),
	inPool(projectAsks(splitSession, "/code/api", start.Add(-day), 1), "work", "b2n8"),
	inPool(projectAsks(splitSession, "/code/api", start.Add(-day+time.Hour), 2), "k3f9", "q7mt"),
	inPool(projectAsks(acmeToday, "/code/api", start.Add(-2*time.Hour), 1), "k3f9", "q7mt"),
	inPool(projectAsks(defaultWeb, "/code/web", start.Add(-time.Hour), 1), "work", "b2n8"),
)

// projectRequests are the requests of each project Projects lists, by its
// path.
func projectRequests(h *harness) map[string]int {
	requests := make(map[string]int)
	for _, r := range h.model.projects.shown.Page.Rows {
		requests[r.Path] = r.Requests
	}
	return requests
}

// headsBuilt fails the test where the router built no head of each day the
// given number of days before now's, whose sessions are read from it.
func headsBuilt(t *testing.T, h *harness, state string, back ...int) {
	t.Helper()
	for _, b := range back {
		date := dayfile.DateOf(dayfile.DayStart(h.clock.Now(), -b))
		if found, _ := filepath.Glob(filepath.Join(ledger.Dir(state), "logday-"+date+"*")); len(found) == 0 {
			t.Fatalf("no head of %s was built", date)
		}
	}
}

// onAccounts are the ids of the accounts a project's page names, sorted.
func onAccounts(on []views.AccountShare) []string {
	ids := make([]string, len(on))
	for i, a := range on {
		ids[i] = a.Account
	}
	slices.Sort(ids)
	return ids
}

// periodRequests are the requests of each session the rows given row, by its
// id.
func periodRequests(sessions []views.PeriodSession) map[string]int {
	requests := make(map[string]int)
	for _, s := range sessions {
		requests[s.Session] = s.Requests
	}
	return requests
}

// Projects, a project's page and its SESSIONS, and the List read the
// sessions' days of the pool shown's work alone, over 30 days, as its read
// reads them, each day's head built as the router builds it: a project
// worked on in two pools shows each pool's work alone, the session split
// between them its requests in the pool alone, read from its day's lines
// where its head holds it whole; and the days are read, and a project's
// WORTH BY charted, from where the pool's work begins, never before, Acme's
// five days back; and a project's page names the accounts of the pool's work
// alone. Every pool's read holds every pool's work, from where the ledger
// begins.
func TestProjectsAndTheListReadThePoolShownsWorkAlone(t *testing.T) {
	tests := []struct {
		name     string
		read     views.PoolRead
		projects map[string]int
		api      map[string]int
		on       []string
		began    time.Duration
	}{
		{
			name: "Acme's", read: acmeRead, projects: map[string]int{"/code/api": 5},
			api: map[string]int{acmeEarly: 2, splitSession: 2, acmeToday: 1}, on: []string{"k3f9"}, began: 5 * day,
		},
		{
			name: "Default's", read: firstRead, projects: map[string]int{"/code/api": 2, "/code/web": 1},
			api: map[string]int{defaultEarly: 1, splitSession: 1}, on: []string{"work"}, began: 20 * day,
		},
		{
			name: "every pool's", read: views.PoolRead{}, projects: map[string]int{"/code/api": 7, "/code/web": 1},
			api: map[string]int{defaultEarly: 1, acmeEarly: 2, splitSession: 3, acmeToday: 1}, on: []string{"k3f9", "work"}, began: 20 * day,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := t.TempDir()
			writeLines(t, state, pooledProjects...)
			h := newHarness(t, probedWithoutTheRouter())
			digestedAsTheRouter(t, h, state, "b2n8")
			headsBuilt(t, h, state, 1, 5, 20)
			h.model.cfg.Ledger = ledger.NewFollower(state, h.clock.Now, ledger.Caps{Shared: policy.Shared}, views.DigestVersion, logs.For("watch"))
			h.model.cfg.PoolRead, h.model.view, h.model.period = tt.read, dashboard.Projects, views.PeriodThirtyDays
			h.start()

			if got := projectRequests(h); !maps.Equal(got, tt.projects) {
				t.Errorf("Projects lists %v, want %v, the pool's requests alone", got, tt.projects)
			}
			if page := h.model.projects.shown.Project; page == nil || page.Path != "/code/api" {
				t.Fatalf("Projects chose %+v, want api's page", page)
			}
			if got := periodRequests(h.model.projects.shown.Project.Sessions); !maps.Equal(got, tt.api) {
				t.Errorf("api's SESSIONS lists %v, want %v, the pool's requests alone", got, tt.api)
			}
			if got := onAccounts(h.model.projects.shown.Project.OnAccounts); !slices.Equal(got, tt.on) {
				t.Errorf("api's page names the accounts %v, want %v, the pool's work's alone", got, tt.on)
			}
			if want := dayfile.DayStart(past(-tt.began).Local(), 0); !h.model.sessionDays.from.Equal(want) {
				t.Errorf("the sessions' days are read from %v, want %v, where the pool's work begins", h.model.sessionDays.from, want)
			}
			if got, want := h.model.projects.shown.Project.WorthBy[0].Buckets[0].Start, dayfile.CivilOf(past(-tt.began)); !got.Equal(want) {
				t.Errorf("api's WORTH BY begins %v, want %v, where the pool's work begins", got, want)
			}

			h.model.view, h.model.sessionsTab.shown.Shown = dashboard.Sessions, dashboard.SessionsList
			h.deliver(h.update(nudgeMsg{})...)
			want := maps.Clone(tt.api)
			if tt.projects["/code/web"] > 0 {
				want[defaultWeb] = 1
			}
			log := h.model.sessionsTab.shown.Log
			if log == nil {
				t.Fatal("the List built no log")
			}
			if got := periodRequests(log.Sessions); !maps.Equal(got, want) {
				t.Errorf("the List lists %v, want %v, the pool's requests alone", got, want)
			}
		})
	}
}

// A P turn on Projects, and on the List, reads the sessions' days afresh in
// the pool turned to, from where its work begins, keeping the tab, the List
// shown and the dial's period: their rows, a project's page and the days
// held are the pool turned to's, none of the pool before.
func TestAPTurnReadsProjectsAndTheListAfreshInThePoolTurnedTo(t *testing.T) {
	state := t.TempDir()
	writeLines(t, state, pooledProjects...)
	h := newHarness(t, probedWithoutTheRouter())
	digestedAsTheRouter(t, h, state, "b2n8")
	pools := newPoolTurns(t, testPools, slices.Concat(defaultAccounts, acmeAccounts), map[string]Source{"b2n8": h.source, "q7mt": h.source})
	h.model.cfg = pools.opened(h.model.cfg, "b2n8")
	followedOn(h, state)
	h.model.view, h.model.period = dashboard.Projects, views.PeriodThirtyDays
	h.start()
	if got, want := projectRequests(h), map[string]int{"/code/api": 2, "/code/web": 1}; !maps.Equal(got, want) {
		t.Fatalf("on Default, Projects lists %v, want %v", got, want)
	}

	h.deliver(h.press("P")...)
	if h.model.cfg.Pool.ID != "q7mt" || h.model.view != dashboard.Projects || h.model.period != views.PeriodThirtyDays {
		t.Fatalf("P turned to %q, showing %v over %v; want Acme, Projects over 30 days", h.model.cfg.Pool.ID, h.model.view, h.model.period)
	}
	if got, want := projectRequests(h), map[string]int{"/code/api": 5}; !maps.Equal(got, want) {
		t.Errorf("turned to Acme, Projects lists %v, want %v, Acme's alone", got, want)
	}
	if page := h.model.projects.shown.Project; page == nil || !maps.Equal(periodRequests(page.Sessions), map[string]int{acmeEarly: 2, splitSession: 2, acmeToday: 1}) {
		t.Errorf("turned to Acme, api's page is %+v, want Acme's sessions alone", page)
	}
	if want := dayfile.DayStart(past(-5*day).Local(), 0); !h.model.sessionDays.from.Equal(want) {
		t.Errorf("turned to Acme, the sessions' days are read from %v, want %v, where Acme's work begins", h.model.sessionDays.from, want)
	}

	h.model.view, h.model.sessionsTab.shown.Shown = dashboard.Sessions, dashboard.SessionsList
	h.deliver(h.update(nudgeMsg{})...)
	h.deliver(h.press("P")...)
	if h.model.cfg.Pool.ID != "b2n8" || h.model.sessionsTab.shown.Shown != dashboard.SessionsList {
		t.Fatalf("P turned to %q, Sessions showing %v; want Default, the List", h.model.cfg.Pool.ID, h.model.sessionsTab.shown.Shown)
	}
	log := h.model.sessionsTab.shown.Log
	if log == nil {
		t.Fatal("turned back to Default, the List built no log")
	}
	if got, want := periodRequests(log.Sessions), map[string]int{defaultEarly: 1, splitSession: 1, defaultWeb: 1}; !maps.Equal(got, want) {
		t.Errorf("turned back to Default, the List lists %v, want %v, Default's alone", got, want)
	}
}
