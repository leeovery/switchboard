package views_test

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// itemsSaid says a section's items, "name: value" each.
func itemsSaid(items []views.DetailItem) []string {
	said := make([]string, len(items))
	for i, item := range items {
		said[i] = item.Name + ": " + item.Value.String()
	}
	return said
}

// detailsSaid says what an event's details card holds, as a moment wants
// it, its times as the card shows them beside the event's.
func detailsSaid(d views.EventDetails) viewstest.Details {
	said := viewstest.Details{Headline: d.Headline.String(), Happened: itemsSaid(d.Happened)}
	at := d.Row.At
	if d.KnownFrom != nil {
		said.KnownFrom = d.KnownFrom.Request
	}
	if th := d.Then; th != nil {
		said.Then = append(said.Then, "account: "+th.Account)
		for _, w := range th.Windows {
			said.Then = append(said.Then, w.Name+": "+prose.Percent(w.Use))
		}
		if !th.Resets.IsZero() {
			said.Then = append(said.Then, "resets: "+prose.Dated(at, th.Resets))
		}
		said.Then = append(said.Then, "sessions: "+strconv.Itoa(th.Sessions))
	}
	if f := d.Followed; f != nil {
		for _, m := range f.Moves {
			said.Followed = append(said.Followed, status.ShownID(m.Session)+": to "+m.To+" "+costSaid(movedCost(m.Move)))
		}
		if !f.Till.IsZero() {
			said.Followed = append(said.Followed, "new sessions: "+strings.TrimSpace(f.Family+" till "+prose.Dated(at, f.Till)))
		}
		if f.InAll != nil {
			said.Followed = append(said.Followed, "in all: "+f.InAll.Cents())
		}
		if m := f.Move; m != nil {
			said.Followed = append(said.Followed, "request: "+m.Request, "cached again: "+strconv.Itoa(m.Written), "the move: "+costSaid(m.Cost))
		}
	}
	return said
}

// movedCost is what a move came to: none where no line tells of it.
func movedCost(m *views.EventMove) *ledger.Picodollars {
	if m == nil {
		return nil
	}
	return m.Cost
}

// At each hard moment that wants an event's details, its card holds them:
// its headline; WHAT HAPPENED's items, `known from` the request its answer
// first told of what the event tells; the account it befell as it stood
// then, its windows as the last answer on it read them, when what it tells
// of resets and its sessions in the hour before; and WHAT FOLLOWED, the
// moves it counted, each as its row says, where new sessions go, and what
// the moves cost in all.
func TestAnEventsDetailsAtEachHardMoment(t *testing.T) {
	for _, hm := range viewstest.EventMoments() {
		if len(hm.Details) == 0 {
			continue
		}
		t.Run(hm.Name, func(t *testing.T) {
			table := eventsOf(hm, eventsRead(hm, filedIn(t, hm)))
			read := views.ReadLog(ledgerHolding(hm.Now, hm.Lines...), hm.Lines)
			for key, want := range hm.Details {
				d, ok := table.Details(views.EventKey{Run: time.Unix(0, key.Run), ID: key.ID}, read)
				if !ok {
					t.Fatalf("there are no details of event %d", key.ID)
				}
				if got := detailsSaid(d); !sameDetails(got, want) {
					t.Errorf("event %d's details are\n%s\nwant\n%s", key.ID, detailsText(got), detailsText(want))
				}
			}
		})
	}
}

// sameDetails reports whether two details say the same.
func sameDetails(a, b viewstest.Details) bool {
	return a.Headline == b.Headline && slices.Equal(a.Happened, b.Happened) && a.KnownFrom == b.KnownFrom &&
		slices.Equal(a.Then, b.Then) && slices.Equal(a.Followed, b.Followed)
}

// detailsText says details a section a line, for a failure to show.
func detailsText(d viewstest.Details) string {
	return strings.Join([]string{
		"headline " + d.Headline, "happened " + strings.Join(d.Happened, " | "), "known from " + d.KnownFrom,
		"then " + strings.Join(d.Then, " | "), "followed " + strings.Join(d.Followed, " | "),
	}, "\n")
}

// A limit's card follows the moves its own run counted: a router restarted
// gives its ids again, so the first run's move, counted in its limit with
// the same id, is none of the second's.
func TestALimitsCardFollowsTheMovesOfItsOwnRun(t *testing.T) {
	first, second := october(7, 8, 0), october(7, 12, 0)
	limit := func(at time.Time) status.Event {
		return status.Event{ID: 1, At: at, Kind: status.EventLimit, Account: "work", Count: 1, To: "side"}
	}
	move := func(at time.Time, session string) status.Event {
		return status.Event{ID: 2, At: at, Kind: status.EventMoved, Session: session, From: "work", To: "side", ForcedBy: 1}
	}
	read := views.ReadEvents(nil, now, now).
		Look(liveAt([]status.Event{move(october(7, 11, 1), older), limit(october(7, 11, 0))}, now), first).
		Look(liveAt([]status.Event{move(october(7, 12, 31), mover), limit(october(7, 12, 30))}, now), second)
	table := views.LogEvents(views.LogEventSources{From: now, Events: read, Now: now})
	d, _ := table.Details(views.EventKey{Run: second, ID: 1}, views.LogRead{})
	if d.Followed == nil || len(d.Followed.Moves) != 1 || d.Followed.Moves[0].Session != mover {
		t.Errorf("the second run's limit's card follows %+v, want its own run's move of %s alone", d.Followed, mover)
	}
}

// Each kind's card says what happened as its headline, then the items the
// kind carries, worded as the Events table words them.
func TestEachKindsDetailsSayWhatHappened(t *testing.T) {
	at := october(7, 13, 0)
	tests := []struct {
		name     string
		event    status.Event
		headline string
		happened []string
	}{
		{name: "a session started", event: status.Event{Kind: status.EventStarted, Account: "side", Session: older, Model: opus, Reason: status.ReasonNew},
			headline: "0a1b started on side", happened: []string{"session: 0a1b", "model: opus-5-5"}},
		{name: "a session started passing an account over", event: status.Event{Kind: status.EventStarted, Account: "side", Session: older, Model: opus, Reason: status.PassingOver(status.ReasonNew, "personal")},
			headline: "0a1b started on side", happened: []string{"session: 0a1b", "model: opus-5-5", "why: personal came under pressure"}},
		{name: "a session moved", event: status.Event{Kind: status.EventMoved, Session: older, Model: opus, From: "work", To: "side", Reason: "moved: work is at its reserve"},
			headline: "work reached its cap", happened: []string{"session: 0a1b", "model: opus-5-5", "from: work", "to: side"}},
		{name: "a session moved for no reason given", event: status.Event{Kind: status.EventMoved, Session: older, From: "work", To: "side"},
			headline: "0a1b moved", happened: []string{"session: 0a1b", "from: work", "to: side"}},
		{name: "an account under pressure", event: status.Event{Kind: status.EventPressure, Account: "personal", Windows: []string{"5h"}, Until: october(7, 16, 5), Since: october(7, 12, 0)},
			headline: "personal came under pressure", happened: []string{"window: its 5-hour window", "runs out: 16:05", "rate since: 12:00"}},
		{name: "a cap", event: status.Event{Kind: status.EventCap, Account: "work", Windows: []string{"5h"}, Reserve: 0.05, Until: october(7, 17, 10)},
			headline: "work reached its cap", happened: []string{"its cap: 95% of its 5-hour window"}},
		{name: "a cap in two windows", event: status.Event{Kind: status.EventCap, Account: "work", Windows: []string{"5h", "7d"}, Reserve: 0.2},
			headline: "work reached its cap", happened: []string{"its cap: 80% of its 5-hour and week windows"}},
		{name: "a limit lifting the next day", event: status.Event{Kind: status.EventLimit, Account: "personal", Windows: []string{"5h"}, Until: october(8, 1, 15)},
			headline: "personal reached its limit", happened: []string{"its limit: its 5-hour window", "till: Thu 01:15"}},
		{name: "a token refused", event: status.Event{Kind: status.EventRefused, Account: "work", Status: 401, Until: october(7, 14, 0)},
			headline: "work's token refused", happened: []string{"answer: 401", "till: 14:00"}},
		{name: "a family refused alone", event: status.Event{Kind: status.EventRefused, Account: "work", Status: 403, Family: "opus", Until: october(7, 14, 0)},
			headline: "work's opus requests refused", happened: []string{"answer: 403", "family: opus", "till: 14:00"}},
		{name: "a refusal of no answer given", event: status.Event{Kind: status.EventRefused, Account: "work", Until: october(7, 14, 0)},
			headline: "work's token refused", happened: []string{"till: 14:00"}},
		{name: "a prime", event: status.Event{Kind: status.EventPrimed, Account: "side", Windows: []string{"5h"}, Until: october(7, 18, 0)},
			headline: "side primed", happened: []string{"window: its 5-hour window"}},
		{name: "room again", event: status.Event{Kind: status.EventRoom, Account: "work", Windows: []string{"5h"}},
			headline: "work open again", happened: []string{"window: its 5-hour window"}},
		{name: "the global pin of three, moving running sessions, with force", event: status.Event{Kind: status.EventPin, Account: "work", Accounts: []string{"work", "side", "personal"}, Move: true, Force: true, By: "cli"},
			headline: "pin set by hand", happened: []string{"accounts: work, side, personal", "goes: new and running sessions", "own pins: every session's cleared", "set from: the command line"}},
		{name: "a session's own pin", event: status.Event{Kind: status.EventPin, Account: "side", Session: older, By: "dashboard"},
			headline: "0a1b pinned to side", happened: []string{"session: 0a1b", "accounts: side", "set from: the dashboard"}},
		{name: "the global pin cleared", event: status.Event{Kind: status.EventAuto, Accounts: []string{"personal"}, By: "dashboard"},
			headline: "new sessions back to the router's choice", happened: []string{"from: personal", "set from: the dashboard"}},
		{name: "a session's own pin cleared", event: status.Event{Kind: status.EventAuto, Account: "side", Session: older},
			headline: "0a1b back to the router's choice", happened: []string{"session: 0a1b", "from: side"}},
		{name: "a restart falling due", event: status.Event{Kind: status.EventRestart, Reason: status.RestartForUpgrade},
			headline: "restart due", happened: []string{"why: switchboard upgraded"}},
		{name: "the router unhealthy", event: status.Event{Kind: status.EventHealth, Reason: "most requests failing"},
			headline: "the router turned unhealthy", happened: []string{"why: most requests failing"}},
		{name: "the router well again", event: status.Event{Kind: status.EventHealth},
			headline: "the router is healthy again"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.event.ID, tt.event.At = 1, at
			run := october(7, 8, 0)
			read := views.ReadEvents(nil, at, at).Look(liveAt([]status.Event{tt.event}, at), run)
			table := views.LogEvents(views.LogEventSources{From: at, Events: read, Now: at})
			d, ok := table.Details(views.EventKey{Run: run, ID: 1}, views.LogRead{})
			if !ok {
				t.Fatal("there are no details of the event")
			}
			want := append([]string{"event: " + tt.headline}, tt.happened...)
			if got := itemsSaid(d.Happened); d.Headline.String() != tt.headline || !slices.Equal(got, want) {
				t.Errorf("the card's headline is %q, its items %q; want %q, %q", d.Headline, got, tt.headline, want)
			}
		})
	}
}
