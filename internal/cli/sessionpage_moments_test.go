package cli_test

import (
	"log/slog"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// At each hard moment of the sessions, sessions <id> prints what the views
// say of the session's page, read as the verb reads it, the router listing
// what the moment lists, and no stream: what it's doing, when it started and
// how it has run since, its time on each account, and how long each turn
// took.
func TestSessionsPrintsWhatItsPageSaysAtEachHardMoment(t *testing.T) {
	for _, hm := range viewstest.SessionMoments() {
		t.Run(hm.Name, func(t *testing.T) {
			days := make(map[string][]ledger.Line)
			for _, l := range hm.Lines {
				date := l.At.Local().Format(time.DateOnly)
				days[date] = append(days[date], l)
			}
			deps := sessionsDepsOf(t, days)
			deps.Now = func() time.Time { return hm.Now }
			if !hm.Unlisted {
				serveSessions(t, deps, hm.Sessions)
			}
			l := ledger.NewFollower(stateDir(t, deps), deps.Now, ledger.Caps{}, slog.New(slog.DiscardHandler))
			for _, id := range slices.Sorted(maps.Keys(hm.Page)) {
				got := run(t, deps, "sessions", id, "--pretty")
				for _, want := range pageWords(l, hm, id) {
					if !strings.Contains(got.stdout, want) {
						t.Errorf("switchboard sessions %s prints\n%s\nwant it to say %q", id[:4], got.stdout, want)
					}
				}
			}
		})
	}
}

// pageWords are what the session with the given id's page says at the
// moment hm, read from l as sessions reads it: what it's doing, by Said;
// when it started and how it has run, by StartedSaid and RanSaid; its time
// on each account, by Stays; and how long each turn took, by Took.
func pageWords(l views.Ledger, hm viewstest.SessionMoment, id string) []string {
	running := hm.Sessions
	if hm.Unlisted {
		running = nil
	}
	page, ok := views.SessionPageOf(id, views.PageSources{
		Routed: views.Routed(running, id), Unlisted: hm.Unlisted, Read: views.ReadSessions(l, running, hm.Now, id), Prices: ledger.Pricing, Now: hm.Now,
	})
	if !ok {
		return []string{"no page of " + status.ShortID(id)}
	}
	words := []string{"\n" + page.Activity.Said(hm.Now)}
	if !page.Started.IsZero() {
		words = append(words, "started "+page.StartedSaid(hm.Now)+" · "+page.RanSaid(hm.Now))
	}
	for _, s := range page.Stays(hm.Now) {
		words = append(words, s.Account+"  "+s.Said)
	}
	for _, t := range page.Turns {
		took, going := t.Took(hm.Now)
		if going {
			took += "…"
		}
		words = append(words, " "+took+" ")
	}
	return words
}
