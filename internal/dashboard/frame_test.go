package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// claudeLike judges windows as Claude's are judged: the session and the week
// every model shares, the session a request starts and pressure is judged
// by, and the week perishing.
var claudeLike = score.Policy{Shared: []string{"5h", "7d"}, Perishable: "7d", Tiebreak: "5h", Started: "5h", Pressure: "5h"}

// windowOf is a window used so far, resetting a while after now.
func windowOf(key, label string, used float64, resetsIn time.Duration) quota.Window {
	return quota.Window{Key: key, Label: label, Utilization: used, ResetsAt: now.Add(resetsIn).UTC()}
}

func sessionOf(used float64, resetsIn time.Duration) quota.Window {
	return windowOf("5h", "Session", used, resetsIn)
}

func weekOf(used float64, resetsIn time.Duration) quota.Window {
	return windowOf("7d", "Week", used, resetsIn)
}

// readAccount is an account with a usable token, labelled as its id, its
// windows read a minute ago.
func readAccount(id string, windows ...quota.Window) status.Account {
	return status.Account{ID: id, Label: id, TokenSet: true, FetchedAt: now.Add(-time.Minute).UTC(), Windows: windows}
}

// limitedAccount is an account whose session reached its limit, which holds
// until it resets in an hour.
func limitedAccount(id string) status.Account {
	session := sessionOf(1, time.Hour)
	session.Status = quota.StatusRejected
	a := readAccount(id, session, weekOf(0.89, 3*day))
	a.Limit = status.Limit{Windows: []string{"5h"}, Until: session.ResetsAt}
	return a
}

// pressedAccount is an account whose session runs out at its last half
// hour's rate before it resets: under pressure.
func pressedAccount(id string) status.Account {
	a := readAccount(id, sessionOf(0.58, 3*time.Hour), weekOf(0.34, 4*day))
	rate := 0.42 / 1.5
	a.Rates = []status.Rate{{Window: "5h", Rate: rate, Since: now.Add(-score.Recent).UTC()}}
	a.Pressure = status.Pressure{Window: "5h", Rate: rate, Recent: true, Since: now.Add(-score.Recent).UTC(), RunsOut: now.Add(90 * time.Minute).UTC(), Under: true}
	return a
}

// threeRouted is the healthy router's document of three accounts, as the
// frames have them: work under pressure, personal at its limit, and side,
// where new sessions go.
func threeRouted() status.Document {
	return routerDoc("side", 5, pressedAccount("work"), limitedAccount("personal"), readAccount("side", sessionOf(0.12, 4*time.Hour), weekOf(0.33, 5*day)))
}

// routerDoc is the healthy router's document of the accounts, with as many
// sessions, new ones going to best.
func routerDoc(best string, sessions int, accounts ...status.Account) status.Document {
	return status.Document{
		GeneratedAt: now.UTC(), Source: status.SourceRouter, Best: best, Router: status.Health{Healthy: true},
		Sessions: sessions, Accounts: accounts,
	}
}

// probed is a document of the accounts built by probing, the router not
// running.
func probed(accounts ...status.Account) status.Document {
	return status.Document{
		GeneratedAt: now.UTC(), Source: status.SourceProbe, Fallback: status.Fallback{Router: status.RouterNotRunning},
		Accounts: accounts,
	}
}

// frameOf is a frame of a terminal the size given, as text alone, listing a
// and q, and saying it was read 4s ago.
func frameOf(width, height int) Frame {
	return Frame{
		Width: width, Height: height, Views: Views(), View: Accounts, Policy: claudeLike,
		Keys: []Key{{Key: "a", Does: "auto"}, {Key: "q", Does: "quit", Always: true}}, Status: "read 4s ago",
	}
}

func TestAFrameIsTheTerminalsSize(t *testing.T) {
	for _, size := range [][2]int{{160, 40}, {120, 30}, {52, 36}, {80, 6}} {
		f := frameOf(size[0], size[1])
		f.Look = Screen(builtin(t, "nord"))
		rows := f.Draw(threeRouted(), now)
		if len(rows) != size[1] {
			t.Errorf("at %d×%d, drew %d rows, want %d", size[0], size[1], len(rows), size[1])
		}
		for i, row := range rows {
			if got := ansi.StringWidth(row); got != size[0] {
				t.Errorf("at %d×%d, row %d is %d cells wide, want %d", size[0], size[1], i+1, got, size[0])
			}
		}
	}
}

func TestAFrameFitsEveryWidth(t *testing.T) {
	docs := map[string]status.Document{
		"three":       threeRouted(),
		"one":         routerDoc("work", 3, pressedAccount("work")),
		"probed":      probed(readAccount("work", sessionOf(0.2, time.Hour))),
		"nothing yet": {},
	}
	for name, doc := range docs {
		t.Run(name, func(t *testing.T) {
			for width := 1; width <= 200; width++ {
				for _, height := range []int{1, 3, 8, 40} {
					rows := frameOf(width, height).Draw(doc, now)
					if len(rows) != height {
						t.Errorf("at %d×%d, drew %d rows", width, height, len(rows))
					}
					for i, row := range rows {
						if got := ansi.StringWidth(row); got > width {
							t.Errorf("at %d×%d, row %d is %d cells wide:\n%s", width, height, i+1, got, row)
						}
					}
				}
			}
		})
	}
}

func TestAFrameDrawsTheTitleTheHeadingTheViewAndTheFooter(t *testing.T) {
	rows := frameOf(160, 40).Draw(threeRouted(), now)

	if !strings.HasPrefix(rows[0], "  SWITCHBOARD    Accounts ") {
		t.Errorf("row 1 = %q, want the title row", rows[0])
	}
	if !strings.HasPrefix(rows[2], " ROUTER                   NEW SESSIONS GO TO") {
		t.Errorf("row 3 = %q, want the heading's labels", rows[2])
	}
	recent := slices.IndexFunc(rows, func(r string) bool { return strings.HasPrefix(r, " RECENT") })
	if recent < 0 || rows[recent-1] != "" || strings.TrimSpace(rows[recent-2]) == "" {
		t.Errorf("rows are\n%s\nwant RECENT a blank row under the cards", strings.Join(rows, "\n"))
	}
	if got, want := rows[39], " a auto   q quit"; !strings.HasPrefix(got, want) || !strings.HasSuffix(got, "read 4s ago") {
		t.Errorf("the last row = %q, want the footer", got)
	}
}

func TestOneAccountUnder150ColumnsHasCOMINGUPUnderItsCardBeforeRECENT(t *testing.T) {
	doc := routerDoc("work", 3, pressedAccount("work"))
	doc.Events = []status.Event{{ID: 1, At: now.Add(-4 * time.Minute).UTC(), Kind: status.EventRoom, Account: "work"}}
	rows := frameOf(149, 40).Draw(doc, now)

	coming := slices.IndexFunc(rows, func(r string) bool { return strings.HasPrefix(r, " COMING UP") })
	recent := slices.IndexFunc(rows, func(r string) bool { return strings.HasPrefix(r, " RECENT") })
	if coming < 0 || recent < coming {
		t.Fatalf("rows are\n%s\nwant COMING UP, then RECENT", strings.Join(rows, "\n"))
	}
	if got, want := rows[coming], " COMING UP   14:42  work runs out at its pace"; !strings.HasPrefix(got, want) {
		t.Errorf("COMING UP reads %q, want %q, the strips' lines in one column", got, want)
	}
	if got, want := rows[recent], " RECENT      13:08  ● work has room again"; got != want {
		t.Errorf("RECENT reads %q, want %q", got, want)
	}
}

func TestTheLineOverTheFooterSaysWhichWindowsHideFromEveryCard(t *testing.T) {
	doc := threeRouted()
	for i := range doc.Accounts {
		doc.Accounts[i].Windows = append(doc.Accounts[i].Windows, windowOf("7d_oi", "Fable week", 0, 4*day))
	}
	rows := frameOf(160, 40).Draw(doc, now)

	if got, want := rows[38], "Fable wk hidden: unused on every account"; !strings.HasSuffix(got, want) {
		t.Errorf("row 39 = %q, want it to end %q", got, want)
	}
	if strings.Contains(strings.Join(rows[:38], "\n"), "Fable wk ") {
		t.Errorf("rows are\n%s\nwant Fable's week on no card", strings.Join(rows, "\n"))
	}
	if rows := frameOf(160, 40).Draw(threeRouted(), now); strings.TrimSpace(rows[38]) != "" {
		t.Errorf("without a window hidden, row 39 = %q, want it blank", rows[38])
	}
}

func TestAViewNotBuiltDrawsNothing(t *testing.T) {
	f := frameOf(160, 40)
	f.View = "a-later-view"
	rows := f.Draw(threeRouted(), now)

	for i, row := range rows[7:39] {
		if row != "" {
			t.Errorf("row %d = %q, want nothing drawn of a view not built", i+8, row)
		}
	}
}
