package capture

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// usageWords are how a world's document reads at the frames' moment, as the
// usage block draws it: each account's windows, their use and even pace in
// whole percent, then where they run out, or their allowance; the pool's
// windows, their bars' use and the room left; and what's coming up.
func usageWords(doc status.Document, now time.Time) []string {
	var words []string
	for _, a := range doc.Accounts {
		for _, w := range a.Windows {
			line := fmt.Sprintf("%s %s %d%%", a.ID, w.Key, percent(w.Utilization))
			if w.Pace != nil {
				line += fmt.Sprintf(" pace %d%%", percent(*w.Pace))
			}
			if out, ok := doc.RunsOut(a, w, now); ok {
				line += " out " + out.At.In(now.Location()).Format("Mon 15:04")
			} else if w.Allowance.Per != "" {
				line += fmt.Sprintf(" %d%%/%s", percent(w.Allowance.Share), w.Allowance.Per)
			}
			words = append(words, line)
		}
	}
	for _, p := range doc.Pool.Windows {
		words = append(words, fmt.Sprintf("pool %s %d%% %.1f left", p.Key, percent(p.Used), p.Room))
	}
	for _, u := range doc.ComingUp {
		line := strings.TrimSpace(fmt.Sprintf("%s %s %s %s", u.At.In(now.Location()).Format("Mon 15:04"), u.Account, u.Kind, u.Window))
		if u.Cap {
			line += " at its cap"
		}
		words = append(words, line)
	}
	return words
}

func TestTheWorldsDocumentsWorkOutAsTheFramesDrawThem(t *testing.T) {
	worlds := worldsNow()
	tests := []struct {
		name  string
		world accountWorld
		best  string
		// words are those of usageWords the frames draw as the real code
		// works them out, the rest left out, coming up in the order given.
		words, comingUp []string
	}{
		{
			name: "auto, as usage and the Overview draw it", world: worlds.auto(),
			words: []string{
				"work 5h 95% pace 51%", "work 7d 34% pace 53% 19%/day", "work 7d_oi 12% pace 53% 25%/day",
				"personal 5h 39% pace 64% 34%/hour", "personal 7d 66% pace 65% out Sat 06:00", "personal 7d_oi 0% pace 65% 40%/day",
				"side 5h 13% pace 17% 21%/hour", "side 7d_oi 4% pace 46% 25%/day",
				"pool 5h 51% 1.5 left", "pool 7d 61% 1.2 left", "pool 7d_oi 7% 2.8 left",
			},
			comingUp: []string{
				"Thu 16:30 personal reset 5h", "Thu 17:10 work back at its cap", "Thu 18:50 side reset 5h",
				"Sat 06:00 personal runs_out 7d", "Sun 02:00 personal reset 7d", "Sun 21:00 work reset 7d", "Mon 10:00 side reset 7d",
			},
		},
		{
			name: "pinned to personal since 14:02", world: worlds.pinned(), best: "personal",
			words: []string{"pool 5h 39% 0.6 left", "pool 7d 66% 0.3 left", "pool 7d_oi 0% 1.0 left"},
		},
		{
			name: "routing's limit, personal at its limit till 15:54", world: worlds.limit(), best: "side",
			words:    []string{"personal 5h 100% pace 76%", "pool 5h 71% 0.9 left", "pool 7d_oi 7% 2.8 left"},
			comingUp: []string{"Thu 15:54 personal back", "Thu 17:10 work back at its cap", "Thu 18:50 side reset 5h"},
		},
		{
			name: "the bad hour, pinned to personal and side, work refused", world: worlds.badHour(), best: "side",
			words:    []string{"pool 5h 56% 0.9 left", "pool 7d 72% 0.6 left", "pool 7d_oi 2% 2.0 left"},
			comingUp: []string{"Thu 15:54 personal back", "Thu 18:50 side reset 5h"},
		},
		{
			name: "ten accounts, lab under pressure and team at its limit", world: worlds.many(10),
			words: []string{
				"client 5h 52%", "client 7d 47%", "spare 5h 4%", "spare 7d 5%", "lab 5h 71% pace 56% out Thu 16:05", "lab 7d 58%",
				"team 5h 100%", "team 7d 83%", "extra 5h 22%", "extra 7d 12%", "ops 5h 6%", "ops 7d 29%", "studio 5h 33%", "studio 7d 41%",
				"pool 5h 44% 5.6 left",
			},
			comingUp: []string{
				"Thu 15:30 team back", "Thu 16:05 lab runs_out 5h", "Thu 16:20 studio reset 5h", "Thu 16:30 personal reset 5h",
				"Thu 16:55 lab reset 5h", "Thu 17:10 work back at its cap", "Thu 17:40 client reset 5h", "Thu 17:55 extra reset 5h",
				"Thu 18:50 side reset 5h", "Thu 19:10 ops reset 5h", "Thu 19:35 spare reset 5h",
			},
		},
		{
			name: "work alone, under pressure", world: worlds.one(), best: "work",
			words: []string{"work 5h 58% pace 51% out Thu 16:05", "work 7d 34% pace 53% 19%/day", "work 7d_oi 0% pace 53% 29%/day"},
			comingUp: []string{
				"Thu 16:05 work runs_out 5h at its cap", "Thu 17:10 work reset 5h", "Sun 21:00 work reset 7d",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := tt.world.document(worlds.now)
			words := usageWords(doc, worlds.now)
			for _, want := range tt.words {
				if !slices.ContainsFunc(words, func(w string) bool { return w == want || strings.HasPrefix(w, want+" ") }) {
					t.Errorf("the document reads\n%s\nwith no %q", strings.Join(words, "\n"), want)
				}
			}
			if !inOrder(words, tt.comingUp) {
				t.Errorf("the document reads\n%s\nwant coming up, in turn, %q", strings.Join(words, "\n"), tt.comingUp)
			}
			if tt.best != "" && doc.Best != tt.best {
				t.Errorf("new sessions go to %q, want %q", doc.Best, tt.best)
			}
		})
	}
}

// inOrder reports whether every one of want is among got, in want's order.
func inOrder(got, want []string) bool {
	i := 0
	for _, g := range got {
		if i < len(want) && g == want[i] {
			i++
		}
	}
	return i == len(want)
}

func TestTheWorldsRouterStandsAsTheFramesDrawIt(t *testing.T) {
	worlds := worldsNow()
	at := func(hour, minute int) time.Time { return dayAt(worlds.now, today, hour, minute, 0).UTC() }
	tests := []struct {
		name     string
		world    accountWorld
		sessions int
		check    func(t *testing.T, doc status.Document)
	}{
		{name: "auto", world: worlds.auto(), sessions: 6},
		{name: "pinned", world: worlds.pinned(), sessions: 6, check: func(t *testing.T, doc status.Document) {
			if !slices.Equal(doc.Pin.Accounts, []string{"personal"}) || !doc.Pin.Since.Equal(at(14, 2)) {
				t.Errorf("the pin is %+v, want personal since 14:02", doc.Pin)
			}
		}},
		{name: "the bad hour", world: worlds.badHour(), sessions: 6, check: func(t *testing.T, doc status.Document) {
			work, _ := doc.Account("work")
			if work.Refused.Status != 401 || !work.FetchedAt.Equal(at(14, 20)) {
				t.Errorf("work is refused with %d, read at %s, want 401, read at 14:20", work.Refused.Status, work.FetchedAt)
			}
			if personal, _ := doc.Account("personal"); !personal.Limit.Until.Equal(at(15, 54)) {
				t.Errorf("personal's limit holds till %s, want 15:54", personal.Limit.Until)
			}
		}},
		{name: "ten accounts", world: worlds.many(10), sessions: 14, check: func(t *testing.T, doc status.Document) {
			if team, _ := doc.Account("team"); !team.Limit.Until.Equal(at(15, 30)) {
				t.Errorf("team's limit holds till %s, want 15:30", team.Limit.Until)
			}
			if lab, _ := doc.Account("lab"); !lab.Pressure.Under {
				t.Errorf("lab's pressure is %+v, want it under pressure", lab.Pressure)
			}
		}},
		{name: "work alone", world: worlds.one(), sessions: 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := tt.world.document(worlds.now)
			if doc.Sessions != tt.sessions {
				t.Errorf("the router has sent %d sessions in the last hour, want %d", doc.Sessions, tt.sessions)
			}
			if !doc.GeneratedAt.Equal(worlds.now.Add(-readAgo).UTC()) {
				t.Errorf("the document was read at %s, want %s before the moment", doc.GeneratedAt, readAgo)
			}
			if tt.check != nil {
				tt.check(t, doc)
			}
		})
	}
}

func TestTheWorldsAccountsAreTheSpecsInConfigOrder(t *testing.T) {
	want := []string{
		"work reserve 0.05 max20x", "personal primary max20x", "side max20x", "client max20x", "spare max5x",
		"lab max20x", "team max20x", "extra max5x", "ops max5x", "studio max20x",
	}
	for i, a := range tenAccounts {
		got := a.ID
		if a.Primary {
			got += " primary"
		}
		if a.Reserve > 0 {
			got += fmt.Sprintf(" reserve %.2f", a.Reserve)
		}
		if got += " " + a.Plan; got != want[i] {
			t.Errorf("account %d is %q, want %q", i+1, got, want[i])
		}
	}
	if one := firstAccounts(1); !one[0].Primary {
		t.Error("work alone isn't the primary, as the config makes the first account")
	}
}
