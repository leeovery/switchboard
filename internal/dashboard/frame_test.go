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
// sessions, new ones going to best, with what's worked out of it, as the
// dashboard is given it.
func routerDoc(best string, sessions int, accounts ...status.Account) status.Document {
	return status.Document{
		GeneratedAt: now.UTC(), Source: status.SourceRouter, Best: best, Router: status.Health{Healthy: true},
		Sessions: sessions, Accounts: accounts,
	}.WorkedOut(claudeLike)
}

// probed is a document of the accounts built by probing, the router not
// running, with what's worked out of it, as the dashboard is given it.
func probed(accounts ...status.Account) status.Document {
	return status.Document{
		GeneratedAt: now.UTC(), Source: status.SourceProbe, Fallback: status.Fallback{Router: status.RouterNotRunning},
		Accounts: accounts,
	}.WorkedOut(claudeLike)
}

// frameOf is a frame of a terminal the size given, as text alone, showing
// the Overview, its key line listing tab, ? and q.
func frameOf(width, height int) Frame {
	return Frame{
		Width: width, Height: height, View: Overview, Policy: claudeLike,
		KeyLine: KeyLine{Groups: [][]Key{{{Key: "tab", Does: "views"}}, {{Key: "?", Does: "help"}, {Key: "q", Does: "quit"}}}, Lead: 3},
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
		for _, view := range Views() {
			t.Run(name+", "+view.Title(), func(t *testing.T) {
				for width := 1; width <= 200; width++ {
					for _, height := range []int{1, 3, 8, 40} {
						f := frameOf(width, height)
						f.View = view
						rows := f.Draw(doc, now)
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
}

func TestThePageDrawsEachPartOnItsRow(t *testing.T) {
	tests := []struct {
		name  string
		width int
		view  View
		// tabs, status and heading are the rows the tabs, the status line
		// and the heading row are on, heading -1 where there's none; top is
		// the row the view starts at, and first what it draws there.
		tabs, status, heading, top int
		first                      string
	}{
		{name: "the Overview, whose sections head themselves", width: 160, view: Overview, status: 2, heading: -1, top: 4, first: "USAGE"},
		{name: "Sessions", width: 160, view: Sessions, status: 2, heading: 4, top: 6, first: "  CALLS"},
		{name: "Runway", width: 160, view: Runway, status: 2, heading: 4, top: 6, first: textsAt(160, map[int]string{27: "now"})},
		{name: "the Overview on a phone", width: 52, view: Overview, tabs: 1, status: 3, heading: -1, top: 5, first: "USAGE"},
		{name: "Sessions on a phone", width: 52, view: Sessions, tabs: 1, status: 3, heading: 5, top: 7, first: " ┌─ 1 · WORK "},
		{name: "a phone of 99 columns", width: 99, view: Sessions, tabs: 1, status: 3, heading: 5, top: 7, first: "  CALLS"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(tt.width, 40)
			f.View = tt.view
			rows := textOf(f.Draw(runwayThree(), now))

			if !strings.HasPrefix(rows[0], " SWITCHBOARD ") || !strings.Contains(rows[tt.tabs], " Overview ") {
				t.Errorf("rows 1 and %d read\n%s\n%s\nwant the title row and its tabs", tt.tabs+1, rows[0], rows[tt.tabs])
			}
			if !strings.HasPrefix(rows[tt.status], "● healthy") {
				t.Errorf("row %d reads %q, want the status line", tt.status+1, rows[tt.status])
			}
			if tt.heading >= 0 && !strings.HasPrefix(rows[tt.heading], strings.ToUpper(tt.view.Title())+"  ─") {
				t.Errorf("row %d reads %q, want the heading row", tt.heading+1, rows[tt.heading])
			}
			if !strings.HasPrefix(rows[tt.top], tt.first) {
				t.Errorf("row %d reads %q, want the view's first row, %q", tt.top+1, rows[tt.top], tt.first)
			}
			for y := tt.tabs + 1; y < tt.top; y++ {
				if y != tt.status && y != tt.heading && rows[y] != "" {
					t.Errorf("row %d reads %q, want it blank", y+1, rows[y])
				}
			}
			if got, want := strings.TrimSpace(rows[39]), "tab views   ? help  q quit"; got != want {
				t.Errorf("the last row reads %q, want the key line, %q", got, want)
			}
		})
	}
}

func TestATerminalTooShortForTheViewDrawsTheTitleRowTheStatusLineAndTheKeyLineAlone(t *testing.T) {
	tests := []struct {
		name  string
		width int
		view  View
		// least is the fewest rows the view is drawn in: the page's fixed
		// rows and three of the view's.
		least int
	}{
		{name: "the Overview", width: 160, view: Overview, least: 9},
		{name: "a tab with a heading row", width: 160, view: Sessions, least: 11},
		{name: "the Overview on a phone", width: 52, view: Overview, least: 10},
		{name: "a phone's tab with a heading row", width: 52, view: Runway, least: 12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(tt.width, tt.least-1)
			f.View = tt.view
			for y, row := range textOf(f.Draw(threeRouted(), now)) {
				switch {
				case y < f.titleRows():
					if row == "" {
						t.Errorf("a row short, row %d is blank, want the title row", y+1)
					}
				case y == f.statusRow():
					if !strings.HasPrefix(row, "● healthy") {
						t.Errorf("a row short, row %d reads %q, want the status line", y+1, row)
					}
				case y == f.Height-1:
					if !strings.HasSuffix(row, "q quit") {
						t.Errorf("a row short, the last row reads %q, want the key line", row)
					}
				case row != "":
					t.Errorf("a row short, row %d reads %q, want it blank: no heading row and no view", y+1, row)
				}
			}
			if got := f.Scrolling(threeRouted(), now); got != (Scrolling{}) {
				t.Errorf("a row short, the view scrolls %+v, want it not to, as it isn't drawn", got)
			}
			f.Height = tt.least
			if rows := textOf(f.Draw(threeRouted(), now)); rows[f.top()] == "" {
				t.Errorf("at %d rows, drew\n%s\nwant the view from row %d", tt.least, strings.Join(rows, "\n"), f.top()+1)
			}
		})
	}
}

func TestATerminalOfAFewRowsDrawsWhatFits(t *testing.T) {
	tests := []struct {
		width, height int
		want          []string
	}{
		{width: 160, height: 1, want: []string{"title"}},
		{width: 160, height: 2, want: []string{"title", "keys"}},
		{width: 160, height: 3, want: []string{"title", "", "keys"}},
		{width: 160, height: 4, want: []string{"title", "", "status", "keys"}},
		{width: 52, height: 2, want: []string{"title", "tabs"}},
		{width: 52, height: 3, want: []string{"title", "tabs", "keys"}},
		{width: 52, height: 4, want: []string{"title", "tabs", "", "keys"}},
		{width: 52, height: 5, want: []string{"title", "tabs", "", "status", "keys"}},
	}
	for _, tt := range tests {
		rows := textOf(frameOf(tt.width, tt.height).Draw(threeRouted(), now))
		got := make([]string, len(rows))
		for i, row := range rows {
			switch {
			case strings.HasPrefix(row, " SWITCHBOARD"):
				got[i] = "title"
			case strings.HasPrefix(row, " Overview"):
				got[i] = "tabs"
			case strings.HasPrefix(row, "● healthy"):
				got[i] = "status"
			case strings.HasSuffix(row, "q quit"):
				got[i] = "keys"
			default:
				got[i] = row
			}
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("at %d×%d, drew %q, want %q", tt.width, tt.height, got, tt.want)
		}
	}
}

func TestATabNotBuiltDrawsItsHeadingRowAloneAndExplainsNoGlyphs(t *testing.T) {
	for _, v := range []View{Accounts, Log, "a-later-view"} {
		f := frameOf(160, 40)
		f.View = v
		rows := f.Draw(threeRouted(), now)

		name := strings.ToUpper(v.Title())
		if got, want := rows[4], name+"  "+rule(158-len(name)); got != want {
			t.Errorf("on %s, row 5 reads %q, want its heading row, %q", v, got, want)
		}
		for i, row := range rows[5:39] {
			if row != "" {
				t.Errorf("on %s, row %d = %q, want nothing drawn of a tab not built", v, i+6, row)
			}
		}
		if glyphs := f.glyphs(); glyphs != nil {
			t.Errorf("on %s, the glyphs explained are %v, want none", v, glyphs)
		}
	}
}

// twoWithRoom is the healthy router's document of work and side, both with
// room, read a minute before threeRouted: another document, whose pool isn't
// threeRouted's.
func twoWithRoom() status.Document {
	doc := routerDoc("work", 2, readAccount("work", sessionOf(0.5, 2*time.Hour), weekOf(0.2, 3*day)), readAccount("side", sessionOf(0.1, 4*time.Hour), weekOf(0.4, 5*day)))
	doc.GeneratedAt = now.Add(-time.Minute).UTC()
	return doc
}

func TestAFrameJudgedJudgesAnotherDocumentsPoolAfresh(t *testing.T) {
	other := twoWithRoom()
	want := other.PoolAt(now, claudeLike)
	if first := threeRouted().PoolAt(now, claudeLike); slices.Equal(first.Accounts, want.Accounts) {
		t.Fatalf("both documents pool %v, want pools that differ", want.Accounts)
	}
	if got := frameOf(160, 40).Judged(threeRouted(), now).pool(other, now); !slices.Equal(got.Accounts, want.Accounts) {
		t.Errorf("judged of one document, the pool of another at the same moment is of %v, want its own, of %v", got.Accounts, want.Accounts)
	}
}
