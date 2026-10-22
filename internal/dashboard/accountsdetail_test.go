package dashboard

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// framedAccounts are the accounts Detail's frames configure: work, personal
// and side.
var framedAccounts = config.Accounts{{ID: "work"}, {ID: "personal"}, {ID: "side"}}

// framedLayout is Detail's wide layout as its frames draw it, with its list.
func framedLayout() detailLayout {
	return laidIn(detailFrame(Look{}, framedAccounts, "", "", nil, nil), usageWorld(extraOff))
}

// detailFramed is a row of Detail's frames, as compareFramed has it, with
// its six weeks' days, as levels, a digit a day, a week a word, and those to
// come "-", and the days it hit a 5-hour limit, "x", and its six weeks'
// peaks, as whole percentages.
type detailFramed struct {
	compareFramed
	days, limits string
	peaks        []int
}

// row is the row as the views give it.
func (f detailFramed) row() views.AccountsRow {
	row := f.compareFramed.row()
	levels, limits := strings.ReplaceAll(f.days, " ", ""), strings.ReplaceAll(f.limits, " ", "")
	for i, level := range levels {
		row.Days = append(row.Days, views.StripDay{Level: max(int(level-'0'), 0), Limit: limits[i] == 'x'})
	}
	for i, p := range f.peaks {
		row.Peaks = append(row.Peaks, views.WeekPeak{Share: new(float64(p) / 100), Open: i == len(f.peaks)-1})
	}
	return row
}

// detailPage is Detail's page of its frames, this week, of the rows given,
// the last all accounts', using each of the versions Compare's frames list.
func detailPage(t *testing.T, rows ...detailFramed) *views.AccountsPage {
	page := &views.AccountsPage{
		Period: views.AccountsThisWeek, Recorded: true, ToCome: 3, Versions: comparedVersions(t),
		Weeks: []string{"2026-08-24", "2026-08-31", "2026-09-07", "2026-09-14", "2026-09-21", "2026-09-28"},
	}
	for _, f := range rows[:len(rows)-1] {
		page.Rows = append(page.Rows, f.row())
	}
	page.All = rows[len(rows)-1].row()
	return page
}

// detailsPage is the page of Detail's frames of three accounts.
func detailsPage(t *testing.T) *views.AccountsPage {
	return detailPage(t, detailWork, detailPersonal, detailSide, detailAll)
}

// alonePage is the page of Detail's frame of one account: work's row, its
// shares as the frame has them, using no Fable, and all accounts'.
func alonePage(t *testing.T) *views.AccountsPage {
	work := detailWork
	work.shares = []int{60, 7, 28, 3, -1, 2}
	return detailPage(t, work, detailAll)
}

// limitedOnce is the row, its 5-hour limit hit once.
func limitedOnce(f compareFramed) compareFramed {
	f.limits = map[string]int{"5h": 1}
	return f
}

// The rows of Detail's frames, of Thursday 1 October, Compare's with their
// last six weeks.
var (
	detailWork = detailFramed{
		compareFramed: compareWork,
		days:          "3443221 3444211 3344211 3234211 4334311 2222---", limits: "xxxxx.. xxxxx.. xxxxx.. xxx.x.. xxxxx.. .x.x...",
		peaks: []int{100, 94, 100, 81, 100, 34},
	}
	detailPersonal = detailFramed{
		compareFramed: limitedOnce(comparePersonal),
		days:          "3223211 4324311 4422311 4433411 3442321 2432---", limits: "....... .x..x.. ..x.... x.x.x.. x..x... .......",
		peaks: []int{58, 77, 69, 90, 85, 66},
	}
	detailSide = detailFramed{
		compareFramed: compareSide,
		days:          "4323311 2222211 4433311 3442211 3443221 3444---", limits: ".x..... ....... ..x.... ....... ...x... .x.....",
		peaks: []int{41, 30, 52, 47, 60, 78},
	}
	detailAll = detailFramed{
		compareFramed: compareAll,
		days:          "2333211 2333211 4444211 4433211 4444221 2332---", limits: "xxxxx.. xxxxx.. xxxxx.. xxx.x.. xxxxx.. .x.x...",
		peaks: []int{66, 67, 74, 73, 82, 59},
	}
)

// happened gives the document the events given.
func happened(events ...status.Event) func(*status.Document) {
	return func(d *status.Document) { d.Events = events }
}

// aloneDoc is the world of Detail's frame of one account: work, where new
// sessions go, under pressure, its 5-hour window running out at 16:05 at its
// pace; and what befell it, its pressure at 14:38, 7f3a starting at 14:12
// and its prime at 12:10.
func aloneDoc() status.Document {
	work := usageAccount("work", 0.58, routedClock(1, 17, 10), 0.34, 0, routedClock(4, 21, 0))
	work.Reserve = 0.05
	runsOut, since := routedClock(1, 16, 5), routedAt.Add(-30*time.Minute)
	rate := (0.95 - 0.58) / runsOut.Sub(routedAt).Hours()
	work.Rates = []status.Rate{{Window: "5h", Rate: rate, Since: since}}
	work.Pressure = status.Pressure{Window: "5h", Rate: rate, Recent: true, Since: since, RunsOut: runsOut, Under: true}
	work.Extra = quota.ExtraUsage{Status: quota.StatusRejected}
	return status.Document{
		GeneratedAt: routedAt, Source: status.SourceRouter, Best: "work", Router: status.Health{Healthy: true}, Sessions: 3,
		Accounts: []status.Account{work},
		Events: framedEvents(
			status.Event{At: routedClock(1, 14, 38), Kind: status.EventPressure, Account: "work", Windows: []string{"5h"}, Until: runsOut, Since: since},
			startedAt(14, 12, "7f3a5c90", "work", status.ReasonNew),
			primeAt(12, 10, "work"),
		),
	}.WorkedOut(claudeLike)
}

// detailFrame is a frame of Detail at routedAt, 160 by 37, in the look
// given, every model's own window shown, as w shows them, of the accounts
// configured, the one chosen, the session selected,
// and the page given, the router listing the sessions given, the request
// stream telling what it told in the Overview's world.
func detailFrame(look Look, configured config.Accounts, chosen, selected string, page *views.AccountsPage, sessions []status.Session) Frame {
	_, _, stream := overviewWorld()
	return Frame{
		Width: 160, Height: 37, Look: look, View: Accounts, Policy: claudeLike, Models: true, Sessions: sessions, Stream: stream,
		Accounts: AccountsShown{Shown: AccountsDetail, Accounts: configured, Chosen: chosen, Selected: selected, InSessions: selected != "", Page: page},
	}
}

// rowWith is the first of the rows holding the text given, -1 where none
// does.
func rowWith(rows []string, text string) int {
	return slices.IndexFunc(rows, func(r string) bool { return strings.Contains(r, text) })
}

// sixOnSide are the Overview's sessions, and two more on side, idle, six
// running there in all.
func sixOnSide() []status.Session {
	sessions := overviewSessions()
	for _, id := range []string{"e04f2a71-6b3c-4d85-9e10-a2b3c4d5e6f7", "f19b3c82-7c4d-4e96-8f21-b3c4d5e6f7a8"} {
		seat := status.Assignment{Model: opus, Family: "opus", Account: "side", Reason: status.ReasonNew, AssignedAt: routedClock(1, 14, 0), LastSeen: routedAt.Add(-10 * time.Minute)}
		sessions = append(sessions, status.Session{ID: id, Assignments: []status.Assignment{seat}})
	}
	return sessions
}

// SESSIONS shows as many sessions as its four rows hold: past four, they
// scroll with the one selected, no further than it takes to show it, the
// fourth row saying how many more are below while any are; ▸ marks the one
// selected; and enter opens it, its line held where it is with four.
func TestDetailsSESSIONSScrollWithTheSessionSelected(t *testing.T) {
	doc := usageWorld(extraOff)
	running := views.RunningOn(detailFrame(Look{}, framedAccounts, "side", "", nil, sixOnSide()).running(doc, routedAt), "side")
	if len(running) != 6 {
		t.Fatalf("%d sessions run on side, want 6", len(running))
	}
	tests := []struct {
		selected, first, shown int
		more                   string
	}{
		{selected: 0, first: 0, shown: 3, more: "+3 more"},
		{selected: 2, first: 0, shown: 3, more: "+3 more"},
		{selected: 3, first: 1, shown: 3, more: "+2 more"},
		{selected: 4, first: 2, shown: 4},
		{selected: 5, first: 2, shown: 4},
	}
	for _, tt := range tests {
		picked := running[tt.selected].Session
		t.Run(status.ShownID(picked), func(t *testing.T) {
			rows := textOf(detailFrame(Look{}, framedAccounts, "side", picked, nil, sixOnSide()).Draw(doc, routedAt))
			heading := rowWith(rows, "SESSIONS")
			for i := range sessionRows {
				row := []rune(rows[heading+1+i])
				switch {
				case i < tt.shown:
					if id := status.ShownID(running[tt.first+i].Session); !strings.Contains(string(row), " "+id+"  ") {
						t.Errorf("SESSIONS' row %d reads %q, want %s", i+1, string(row), id)
					}
					if selected := tt.first+i == tt.selected; (row[framedLayout().panel] == '▸') != selected {
						t.Errorf("SESSIONS' row %d reads %q, want ▸ %t", i+1, string(row), selected)
					}
				case !strings.HasPrefix(string(row[framedLayout().panel+enterIn:]), tt.more):
					t.Errorf("SESSIONS' row %d reads %q, want %q from column %d", i+1, string(row), tt.more, framedLayout().panel+enterIn)
				}
			}
			if want := "enter opens " + status.ShownID(picked) + "'s page"; !strings.Contains(rows[heading+enterUnder], want) {
				t.Errorf("row %d under SESSIONS reads %q, want %q", enterUnder, rows[heading+enterUnder], want)
			}
		})
	}
}

// Beside SESSIONS, LATELY's lines start latelyBeside cells into the panel,
// under its heading, on SESSIONS' row; with no sessions, LATELY's lines start
// at cx, on its heading's row. Either way, their words are cut short of the
// frame's edge alone.
func TestDetailsLATELYBesideSESSIONSCutsItsWords(t *testing.T) {
	long := strings.Repeat("w", 45)
	doc := usageWorld(extraOff, happened(onAuto()...), func(d *status.Document) { d.Accounts[0].Label = long })
	_, sessions, _ := overviewWorld()
	l := framedLayout()
	tests := []struct {
		name     string
		sessions []status.Session
		x, under int
		words    int
	}{
		{name: "beside SESSIONS", sessions: sessions, x: l.panel + latelyBeside, under: 1, words: 160 - margin - l.panel - latelyBeside - ansi.StringWidth("14:31  ▸ ")},
		{name: "alone", x: l.cx(), words: 160 - margin - l.cx() - ansi.StringWidth("14:31  ▸ ")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := textOf(detailFrame(Look{}, framedAccounts, "side", "", nil, tt.sessions).Draw(doc, routedAt))
			heading := rowWith(rows, "LATELY")
			if got := strings.Index(rows[heading], "LATELY"); tt.under > 0 && len([]rune(rows[heading][:got])) != tt.x {
				t.Errorf("LATELY's heading is at column %d, want %d", got, tt.x)
			}
			second := []rune(rows[heading+tt.under+1])
			lead := []rune("14:31  ▸ ")
			if got := string(second[tt.x : tt.x+len(lead)]); got != string(lead) {
				t.Fatalf("LATELY's second line reads %q from column %d, want it to start %q", string(second[tt.x:]), tt.x, string(lead))
			}
			words := string(second[tt.x+len(lead):])
			if got := ansi.StringWidth(words); got != tt.words || !strings.HasSuffix(words, "…") {
				t.Errorf("LATELY's second line's words, %q, are %d cells, want them cut to %d", words, got, tt.words)
			}
		})
	}
}

// One session on the account is always selected: the first, where none is,
// or the one selected no longer runs there.
func TestDetailsSESSIONSAlwaysHasOneSelected(t *testing.T) {
	_, sessions, _ := overviewWorld()
	doc := usageWorld(extraOff)
	for _, selected := range []string{"", idC61B[:4] + "-gone", id41E0} {
		t.Run(selected, func(t *testing.T) {
			f := detailFrame(Look{}, framedAccounts, "side", selected, nil, sessions)
			first := views.RunningOn(f.running(doc, routedAt), "side")[0].Session
			rows := textOf(f.Draw(doc, routedAt))
			heading := rowWith(rows, "SESSIONS")
			if row := []rune(rows[heading+1]); row[framedLayout().panel] != '▸' || !strings.Contains(string(row), status.ShownID(first)) {
				t.Errorf("SESSIONS' first row reads %q, want ▸ before %s", string(row), status.ShownID(first))
			}
			if want := "enter opens " + status.ShownID(first) + "'s page"; !strings.Contains(rows[heading+enterUnder], want) {
				t.Errorf("SESSIONS' enter line reads %q, want %q", rows[heading+enterUnder], want)
			}
		})
	}
}

// Detail shows where the focus is: the chosen row is lit on bg.selection in
// the part that holds it alone, the list or SESSIONS, the other's chosen row
// bold, unlit, its colours kept; and SESSIONS is headed in bold
// text.primary while it holds it, else text.subtle. With one account, which
// has no list, SESSIONS holds it.
func TestDetailLightsTheChosenRowWhereTheFocusIs(t *testing.T) {
	_, sessions, _ := overviewWorld()
	doc := usageWorld(extraOff)
	selection := hue{token: theme.BgSelection}
	tests := []struct {
		name       string
		configured config.Accounts
		selected   string
		inSessions bool
	}{
		{name: "the list holding the focus", configured: framedAccounts},
		{name: "SESSIONS holding it, → having taken it there", configured: framedAccounts, selected: idD28C, inSessions: true},
		{name: "one account, SESSIONS holding it", configured: config.Accounts{{ID: "side"}}, inSessions: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := detailFrame(Look{}, tt.configured, "side", tt.selected, nil, sessions)
			c := newCanvas(f.Width, f.Height)
			f.accounts(c, doc, routedAt, f.top())
			rows, l := textOf(c.rows(Look{})), laidIn(f, doc)
			var lit []int
			for y := range f.Height {
				for x := range f.Width {
					if c.at(x, y).ink.on == selection {
						lit = append(lit, y)
						break
					}
				}
			}
			heading, chosen := rowWith(rows, "SESSIONS"), rowWith(rows, "▸ ●")
			want := []int{chosen}
			if !tt.inSessions {
				want = []int{f.top() + 3*listApart}
			}
			if !slices.Equal(lit, want) {
				t.Errorf("rows %v are lit, want %v alone, the chosen row of the part holding the focus", lit, want)
			}
			if session := c.at(l.panel+2, chosen); !tt.inSessions && !session.ink.bold {
				t.Errorf("the session selected, its focus left, reads %+v, want it bold", *session)
			}
			if side := c.at(listAt, f.top()+3*listApart); l.list && tt.inSessions && !side.ink.bold {
				t.Errorf("side's row in the list, its focus left, reads %+v, want it bold", *side)
			}
			if l.list {
				mark, want := c.at(listAt-1, f.top()+3*listApart), " "
				if tt.inSessions {
					want = "▸"
				}
				if mark.glyph != want || tt.inSessions && (mark.ink.token != keyInk.token || !mark.ink.bold) {
					t.Errorf("before side's row in the list is %+v, want %q, ▸ bold accent.key alone while its focus is left", *mark, want)
				}
			}
			head, wantHead := c.at(l.panel, heading).ink, dimInk
			if tt.inSessions {
				wantHead = titleInk
			}
			if head != wantHead {
				t.Errorf("SESSIONS is headed in %+v, want %+v", head, wantHead)
			}
		})
	}
}

// tenAccounts is the auto world of ten accounts, each read, the third where
// new sessions go.
func tenAccounts() (status.Document, config.Accounts) {
	ids := []string{"work", "personal", "side", "lab", "client", "oss", "team", "home", "study", "spare"}
	var accounts []status.Account
	var configured config.Accounts
	for i, id := range ids {
		accounts = append(accounts, readAccount(id, sessionOf(float64(i)/20, 2*time.Hour), weekOf(0.3, 3*quota.Day)))
		configured = append(configured, config.Account{ID: id})
	}
	return routerDoc("side", 0, accounts...), configured
}

// Detail's list holds all accounts at its top, and past the rows it has,
// scrolls its accounts with the one chosen, no further than it takes to
// show it, ↑↓ choose three rows under the last shown; all accounts chosen,
// it stands where it was.
func TestDetailsListScrollsWithTheChoice(t *testing.T) {
	doc, configured := tenAccounts()
	tests := []struct {
		name, chosen string
		scroll, want int
	}{
		{name: "an account shown, as scrolled", chosen: "client", scroll: 2, want: 2},
		{name: "an account below, scrolled to the foot", chosen: "home", scroll: 0, want: 2},
		{name: "the last, as far as it goes", chosen: "spare", scroll: 0, want: 4},
		{name: "an account above, scrolled to the top", chosen: "personal", scroll: 3, want: 1},
		{name: "all accounts, where it was", chosen: "", scroll: 3, want: 3},
		{name: "an account the config doesn't hold, where it was", chosen: "gone", scroll: 3, want: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Width: 160, Height: 24, View: Accounts, Policy: claudeLike, Accounts: AccountsShown{Shown: AccountsDetail, Accounts: configured, Chosen: tt.chosen, ListScroll: tt.scroll}}
			if got := f.DetailListScroll(tt.chosen); got != tt.want {
				t.Errorf("DetailListScroll(%q) = %d, want %d", tt.chosen, got, tt.want)
			}
			rows := textOf(f.Draw(doc, now))
			top, page := f.top(), listScrolling(len(configured), f.viewRows()).Page
			if page != 6 {
				t.Fatalf("the list shows %d accounts, want 6", page)
			}
			if got := listCells(rows[top]); got != "0 all accounts" {
				t.Errorf("the list's top row reads %q, want all accounts", got)
			}
			for i := range page {
				a := tt.want + i
				if got, want := listCells(rows[top+(i+1)*listApart]), fmt.Sprintf("%d %s", a+1, configured[a].ID); !strings.HasPrefix(got, want) {
					t.Errorf("the list's row %d reads %q, want %q", i+1, got, want)
				}
			}
			if got := listCells(rows[top+page*listApart+chooseUnder]); got != "↑↓ choose" {
				t.Errorf("%d rows under the last account, the list reads %q, want ↑↓ choose", chooseUnder, got)
			}
		})
	}
}

// Detail's list holds the configured accounts, one the document lacks yet,
// as one the config has just gained, named as the config names it, ○ not
// read; chosen, its head says it's not read, and USAGE draws its windows
// empty; and all accounts' head counts it, not read.
func TestDetailListsAnAccountTheDocumentLacksAsNotRead(t *testing.T) {
	doc := usageWorld(extraOff)
	configured := append(slices.Clone(framedAccounts), config.Account{ID: "added", Label: "lab"})
	f := detailFrame(Look{}, configured, "added", "", nil, nil)
	rows := textOf(f.Draw(doc, routedAt))
	top := f.top()
	if got, want := listCells(rows[top+4*listApart]), "4 lab         ○"; got != want {
		t.Errorf("the list's fourth row reads %q, want %q", got, want)
	}
	if got, want := panelCells(rows[top]), "4 lab   ○ not read"; got != want {
		t.Errorf("the head reads %q, want %q", got, want)
	}
	d := f.detailDrawn(doc, routedAt, laidIn(f, doc))
	for _, w := range d.windows() {
		if g := d.gauge(w.key); !g.bar.empty {
			t.Errorf("USAGE draws its %s %+v, want it empty", w.key, g.bar)
		}
	}
	targets := d.listTargets()
	if last := targets[len(targets)-1]; last.Account != "added" || last.Y != top+4*listApart {
		t.Errorf("the list's last target is %+v, want added's, on its row", last)
	}
	all := textOf(detailFrame(Look{}, configured, "", "", nil, nil).Draw(doc, routedAt))
	if got, want := panelCells(all[top]), "0 all accounts   2 open · 1 at cap · 1 not read"; got != want {
		t.Errorf("all accounts' head reads %q, want %q, added counted not read", got, want)
	}
}

// listCells are the list's cells of a row, trimmed.
func listCells(row string) string {
	cells := []rune(row)
	return strings.TrimSpace(string(cells[:min(len(cells), framedLayout().ruleAt())]))
}

// Detail's panel scrolls by rows past the view's foot, the list held beside
// it: all accounts on its top row, and its │ down the rows.
func TestDetailsPanelScrollsTheListHeld(t *testing.T) {
	doc := usageWorld(extraOff, happened(onAuto()...))
	_, sessions, _ := overviewWorld()
	page := detailsPage(t)
	f := detailFrame(Look{}, framedAccounts, "side", "", page, sessions)
	f.Height = 29
	scrolling := f.Scrolling(doc, routedAt)
	whole := detailFrame(Look{}, framedAccounts, "side", "", page, sessions)
	panel := textOf(whole.Draw(doc, routedAt))
	if want := rowWith(panel, "enter opens") - whole.top() + 1 - f.viewRows(); scrolling.Most != want {
		t.Fatalf("the panel scrolls %d rows, want %d, to its enter line", scrolling.Most, want)
	}
	f.Scroll = scrolling.Most
	rows := textOf(f.Draw(doc, routedAt))
	top := f.top()
	if got := listCells(rows[top]); got != "0 all accounts" {
		t.Errorf("scrolled, the list's top row reads %q, want all accounts", got)
	}
	for y := top; y < top+f.viewRows(); y++ {
		if cells := []rune(rows[y]); len(cells) <= framedLayout().ruleAt() || cells[framedLayout().ruleAt()] != '│' {
			t.Errorf("scrolled, row %d reads %q, want │ at %d", y, rows[y], framedLayout().ruleAt())
		}
		if got, want := panelCells(rows[y]), panelCells(panel[y+scrolling.Most]); got != want {
			t.Errorf("scrolled %d, the panel's row %d reads %q, want %q", scrolling.Most, y, got, want)
		}
	}
	x, y, width, shown := f.scrolls(doc, routedAt)
	if x != framedLayout().panel || y != top || width != f.Width-framedLayout().panel || shown != f.viewRows() {
		t.Errorf("scrolls() = %d, %d, %d, %d, want the panel's columns, from %d, and the view's rows", x, y, width, shown, framedLayout().panel)
	}
}

// While Detail's panel scrolls, the row over the key line says, from the
// panel's column, how many of its rows are out of view, above and below, and
// how to scroll to them, as Compare's line says it, a heading the page
// leaves out at its foot, with its first row, counted below; where it all
// shows, it's blank. With one account, there's no list, and it says so from
// column 0.
func TestDetailSaysWhatOfItsPanelIsOutOfView(t *testing.T) {
	doc := usageWorld(extraOff, happened(onAuto()...))
	_, sessions, _ := overviewWorld()
	f := detailFrame(Look{}, framedAccounts, "side", "", detailsPage(t), sessions)
	f.Height = 29
	most := f.Scrolling(doc, routedAt).Most
	if most < 2 {
		t.Fatalf("at 29 rows, the panel scrolls %d rows, want it to scroll", most)
	}
	down := f
	down.Scroll = 1
	left := f.viewRows() - down.Scrolling(doc, routedAt).Page
	tests := []struct {
		name   string
		scroll int
		want   string
	}{
		{name: "at its top", want: fmt.Sprintf("▼ %d more below · j/k", most)},
		{name: "a row down", scroll: 1, want: fmt.Sprintf("▲ 1 more above · ▼ %d more below · j/k", most-1+left)},
		{name: "at its foot", scroll: most, want: fmt.Sprintf("▲ %d more above · j/k", most)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f.Scroll = tt.scroll
			c := f.At(doc, routedAt).page(doc, routedAt)
			rows := textOf(c.rows(Look{}))
			y := f.Height - underView
			if got := panelCells(rows[y]); got != tt.want {
				t.Errorf("the row over the key line reads %q in the panel, want %q", got, tt.want)
			}
			if got := listCells(rows[y]); got != "" {
				t.Errorf("the row over the key line reads %q under the list, want it blank, the list all shown", got)
			}
			if got := c.at(framedLayout().panel, y).ink; got != keyMarkInk {
				t.Errorf("the line's mark is in %+v, want %+v, as Compare's", got, keyMarkInk)
			}
		})
	}

	t.Run("all shown", func(t *testing.T) {
		whole := detailFrame(Look{}, framedAccounts, "side", "", detailsPage(t), sessions)
		if got := textOf(whole.Draw(doc, routedAt))[whole.Height-underView]; got != "" {
			t.Errorf("at 37 rows, the row over the key line reads %q, want it blank", got)
		}
	})

	t.Run("one account", func(t *testing.T) {
		alone := detailFrame(Look{}, framedAccounts[:1], "", "", alonePage(t), nil)
		alone.Height = 29
		most := alone.Scrolling(aloneDoc(), routedAt).Most
		if got, want := textOf(alone.Draw(aloneDoc(), routedAt))[alone.Height-underView], fmt.Sprintf("▼ %d more below · j/k", most); got != want {
			t.Errorf("the row over the key line reads %q, want %q from column 0", got, want)
		}
	})
}

// While Detail's list scrolls with the choice, the row over the key line
// says under it how many accounts are out of view, above and below, as
// briefly as its cells need: none where they all show.
func TestDetailSaysWhatOfItsListIsOutOfView(t *testing.T) {
	doc, configured := tenAccounts()
	tests := []struct {
		name, chosen string
		scroll       int
		want         string
	}{
		{name: "at its top", chosen: "work", want: "▼ 4 more accounts"},
		{name: "part way", chosen: "client", scroll: 2, want: "▲ 2 more · ▼ 2 more"},
		{name: "at its foot", chosen: "spare", scroll: 4, want: "▲ 4 more accounts"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Width: 160, Height: 24, View: Accounts, Policy: claudeLike, Accounts: AccountsShown{Shown: AccountsDetail, Accounts: configured, Chosen: tt.chosen, ListScroll: tt.scroll}}
			if got := listCells(textOf(f.Draw(doc, now))[f.Height-underView]); got != tt.want {
				t.Errorf("under the list, the row over the key line reads %q, want %q", got, tt.want)
			}
		})
	}
}

// With one account and sessions running on it, there's no list: SESSIONS
// stands at the panel's left, column 0, on LATELY's row, LATELY beside it
// latelyBeside columns in; the session selected is lit from a cell before
// its ▸, which the frame's edge cuts to column 0, to sessionLit cells on; and
// the enter line is two cells in, enterUnder rows under SESSIONS.
func TestDetailOfOneAccountWithSessionsDrawsThemFromTheFramesLeft(t *testing.T) {
	var sessions []status.Session
	for i, id := range []string{"e04f2a71-6b3c-4d85-9e10-a2b3c4d5e6f7", "f19b3c82-7c4d-4e96-8f21-b3c4d5e6f7a8"} {
		seat := status.Assignment{Model: opus, Family: "opus", Account: "work", Reason: status.ReasonNew, AssignedAt: routedClock(1, 14, 0), LastSeen: routedAt.Add(-time.Duration(i+1) * time.Minute)}
		sessions = append(sessions, status.Session{ID: id, Assignments: []status.Assignment{seat}})
	}
	doc := aloneDoc()
	f := detailFrame(Look{}, framedAccounts[:1], "", "", alonePage(t), sessions)
	c := f.At(doc, routedAt).page(doc, routedAt)
	rows := textOf(c.rows(Look{}))
	y := rowWith(rows, "SESSIONS")
	if y < 0 {
		t.Fatalf("no SESSIONS drawn, want it, sessions running on work:\n%s", strings.Join(rows, "\n"))
	}
	if !strings.HasPrefix(rows[y], "SESSIONS  now") || columnOf(rows[y], "LATELY") != latelyBeside {
		t.Errorf("SESSIONS' row reads %q, want SESSIONS from column 0, LATELY from %d", rows[y], latelyBeside)
	}
	if !strings.HasPrefix(rows[y+1], "▸ ") {
		t.Errorf("the first session's row reads %q, want ▸ at column 0, it selected", rows[y+1])
	}
	for x := range sessionLit {
		if lit := c.at(x, y+1).ink.on.token == theme.BgSelection; lit != (x < sessionLit-1) {
			t.Errorf("column %d of the session selected is lit %v, want columns 0 to %d lit", x, lit, sessionLit-2)
		}
	}
	if columnOf(rows[y+enterUnder], "enter opens") != enterIn {
		t.Errorf("%d rows under SESSIONS reads %q, want enter opens from column %d", enterUnder, rows[y+enterUnder], enterIn)
	}
}

// Detail's USAGE cues a window's words as they turn to out ~, as the
// Overview's do, while the frame's TurnedOut says, and no other window's.
func TestDetailsUSAGECuesAWindowsWordsAsTheyTurnOut(t *testing.T) {
	doc := threeRouted()
	f := detailFrame(Screen(builtin(t, "nord")), framedAccounts, "work", "", nil, nil)
	f.TurnedOut = map[Ref]float64{{Account: "work", Window: "5h"}: 0.25}
	c := f.At(doc, now).page(doc, now)
	rows := textOf(c.rows(Look{}))
	cue := hue{token: theme.BgAttention, fade: 0.25}
	for _, tt := range []struct {
		window string
		cued   bool
	}{{window: "5-hour", cued: true}, {window: "Week"}} {
		y := rowWith(rows, tt.window)
		x := columnOf(rows[y], "out ~")
		if tt.cued != (x >= 0) {
			t.Fatalf("%s's row reads %q, want out ~ in it %v", tt.window, rows[y], tt.cued)
		}
		x = max(x, columnOf(rows[y], "%")+3)
		if got := c.at(x, y).ink.on; (got == cue) != tt.cued {
			t.Errorf("%s's words, at column %d, are on %+v, want cued %v", tt.window, x, got, tt.cued)
		}
	}
}

// panelCells are the panel's cells of a row, trimmed.
func panelCells(row string) string {
	cells := []rune(row)
	return strings.TrimRight(string(cells[min(len(cells), framedLayout().panel):]), " ")
}

// A click on a row of Detail's list chooses its account, all accounts' by
// "", across the cells it's lit across; on a session in SESSIONS, selects
// it, across its row as lit, where the panel's scroll shows it.
func TestDetailsTargetsAreItsListsRowsAndItsSessions(t *testing.T) {
	doc := usageWorld(extraOff)
	_, sessions, _ := overviewWorld()
	page := detailsPage(t)
	f := detailFrame(Look{}, framedAccounts, "side", "", page, sessions)
	running := views.RunningOn(f.running(doc, routedAt), "side")
	top := f.top()
	listed := []Target{{X: 0, Y: top, Width: framedLayout().lit(), On: OnAccount}}
	for i, a := range framedAccounts {
		listed = append(listed, Target{X: 0, Y: top + (i+1)*listApart, Width: framedLayout().lit(), On: OnAccount, Account: a.ID})
	}
	sessionsAt := top + rowWith(textOf(f.Draw(doc, routedAt))[top:], "SESSIONS") + 1
	tests := []struct {
		name   string
		scroll int
		want   []Target
	}{
		{name: "unscrolled", want: slices.Concat(listed, sessionsFrom(running, sessionsAt, 0))},
		{name: "scrolled three rows", scroll: 3, want: slices.Concat(listed, sessionsFrom(running, sessionsAt-3, 0))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := detailFrame(Look{}, framedAccounts, "side", "", page, sessions)
			f.Height, f.Scroll = 29, tt.scroll
			bottom := top + f.viewRows()
			want := slices.DeleteFunc(slices.Clone(tt.want), func(target Target) bool { return target.Y >= bottom })
			if got := f.accountsTargets(doc, routedAt); !slices.Equal(got, want) {
				t.Errorf("targets\n%+v,\nwant\n%+v", got, want)
			}
		})
	}
}

// sessionsFrom are the targets of SESSIONS' rows of the sessions given, from
// row y.
func sessionsFrom(running []views.Running, y, first int) []Target {
	var targets []Target
	for i, r := range running[first:min(len(running), first+sessionRows)] {
		targets = append(targets, Target{X: framedLayout().panel - 1, Y: y + i, Width: sessionLit, On: OnSession, Session: r.Session})
	}
	return targets
}

// Probing, nothing runs on an account as the router would say, nor tells
// what befell it: Detail draws no SESSIONS, and LATELY says why it's quiet.
func TestDetailWhileProbingHasNoSESSIONSAndAQuietLATELY(t *testing.T) {
	hm := hardMoment(t, "probed, of several accounts")
	configured := config.Accounts{{ID: hm.Doc.Accounts[0].ID}, {ID: hm.Doc.Accounts[1].ID}}
	f := Frame{
		Width: 160, Height: 37, View: Accounts, Policy: claudeLike, Sessions: overviewSessions(),
		Accounts: AccountsShown{Shown: AccountsDetail, Accounts: configured, Chosen: configured[0].ID},
	}
	rows := textOf(f.Draw(hm.Doc, hm.Now))
	if y := rowWith(rows, "SESSIONS"); y >= 0 {
		t.Errorf("row %d reads %q, want no SESSIONS while probing", y, rows[y])
	}
	y := rowWith(rows, "LATELY")
	if y < 0 {
		t.Fatal("no LATELY drawn")
	}
	if want := f.quiet(f.Moment(hm.Doc, hm.Now)).plain(); !strings.HasSuffix(rows[y], "LATELY      "+want) {
		t.Errorf("LATELY reads %q, want %q", rows[y], want)
	}
}

// Until the period's page is read, Detail draws the moment's parts alone:
// its head, USAGE and LATELY, where each stands once it's read. Read on a
// fresh install, or a week's first morning, recording nothing, each of the
// period's figures in THIS WEEK is a dash; its plan, from the config, and
// extra use, live, say what they are.
func TestDetailDrawsWhatItHasOfThePeriod(t *testing.T) {
	doc := usageWorld(extraOff, happened(onAuto()...))
	fresh := detailsPage(t)
	fresh.Recorded = false
	read := textOf(detailFrame(Look{}, framedAccounts, "work", "", detailsPage(t), nil).Draw(doc, routedAt))
	unread := textOf(detailFrame(Look{}, framedAccounts, "work", "", nil, nil).Draw(doc, routedAt))
	onFresh := textOf(detailFrame(Look{}, framedAccounts, "work", "", fresh, nil).Draw(doc, routedAt))
	for _, part := range []string{"THIS WEEK", "BY MODEL", "the last six weeks", "BY DAY", "WEEKS"} {
		if y := rowWith(unread, part); y >= 0 {
			t.Errorf("unread, row %d reads %q, want no %s", y, unread[y], part)
		}
	}
	for _, part := range []string{"USAGE", "LATELY"} {
		if rowWith(unread, part) != rowWith(read, part) {
			t.Errorf("unread, %s is on row %d, want %d, where it stands once read", part, rowWith(unread, part), rowWith(read, part))
		}
	}
	for label, want := range map[string]string{"requests": "—", "limits": "—", "plan": "Max 20x", "worth": "—", "extra use": "off"} {
		y := rowWith(onFresh, padded(label, thisWeekLabels))
		if y < 0 {
			t.Fatalf("no %s in THIS WEEK", label)
		}
		thisWeek := []rune(onFresh[y])[framedLayout().thisWeekAt()+thisWeekLabels:]
		if got := strings.TrimSpace(string(thisWeek)); got != want {
			t.Errorf("with nothing recorded, THIS WEEK's %s reads %q, want %q", label, got, want)
		}
	}
}

// THIS WEEK's title names the period.
func TestDetailsTHISWEEKNamesThePeriod(t *testing.T) {
	doc := usageWorld(extraOff)
	for period, want := range map[views.AccountsPeriod]string{
		views.AccountsThisWeek: "THIS WEEK", views.AccountsToday: "TODAY", views.AccountsThirtyDays: "LAST 30 DAYS", views.AccountsTwelveWeeks: "LAST 12 WEEKS",
	} {
		page := detailsPage(t)
		page.Period = period
		rows := textOf(detailFrame(Look{}, framedAccounts, "work", "", page, nil).Draw(doc, routedAt))
		if y := rowWith(rows, "USAGE"); !strings.HasSuffix(rows[y], want) {
			t.Errorf("%s: USAGE's row reads %q, want THIS WEEK's title %q", period.Name(), rows[y], want)
		}
	}
}

// Detail draws wide where flex finds its wide form's leasts fit the frame:
// the list's name column at its least, 14 cells whatever its names; the
// headings' column; and the weeks at the fewest cells that end where USAGE's
// row at its leasts and THIS WEEK beside it reach. With up to nine accounts
// THIS WEEK is 23 cells and that's 68 cells of weeks, four at a gap of 4, so
// from 105 columns; with ten, "on for 9 of 10" makes it 24, four weeks no
// longer reach, five at a gap of 2 take 78, and it's from 115; and a window
// named past 8 cells, as "Sonnet wk", whole, a cell more of USAGE's row,
// takes it to 115 too. Narrower, as the phone's body draws it. Drawn wide,
// its widest row ends at the weeks' edge.
func TestDetailDrawsWideFromWhereItsLeastsFit(t *testing.T) {
	long := config.Accounts{{ID: "work", Label: "work-team-plan"}, {ID: "side", Label: "side-hustle-consulting"}}
	ten := make(config.Accounts, 10)
	for i := range ten {
		ten[i] = config.Account{ID: fmt.Sprintf("account-%d", i)}
	}
	tests := []struct {
		name       string
		doc        status.Document
		configured config.Accounts
		from       int
	}{
		{name: "with the list", doc: usageWorld(extraOff, happened(onAuto()...)), configured: framedAccounts, from: 105},
		{name: "with the list of long names", doc: usageWorld(extraOff), configured: long, from: 105},
		{name: "with ten accounts", doc: usageWorld(extraOff), configured: ten, from: 115},
		{name: "with a Sonnet week", doc: usageWorld(extraOff, sonnetWeek), configured: framedAccounts, from: 115},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := detailFrame(Look{}, tt.configured, "work", "", detailsPage(t), nil)
			f.Width = tt.from
			l, wide := laidOf(f, tt.doc)
			if !wide {
				t.Fatalf("at %d columns, its wide form's leasts don't fit", tt.from)
			}
			drawn := f.detailDrawn(tt.doc, routedAt, l)
			widest := 0
			for _, row := range textOf(drawn.panel().rows(Look{})) {
				widest = max(widest, ansi.StringWidth(row))
			}
			if widest > l.edge() {
				t.Errorf("its widest row ends at %d, past the weeks' edge, %d", widest, l.edge())
			}
			if got, want := accountsDrawn(f, tt.doc), formDrawn(f, accountsForm{draw: drawn.draw}); !slices.Equal(got, want) {
				t.Errorf("at %d columns, its wide form isn't drawn", f.Width)
			}
			f.Width--
			if _, wide := laidOf(f, tt.doc); wide {
				t.Fatalf("at %d columns, its wide form's leasts fit", f.Width)
			}
			if got, want := accountsDrawn(f, tt.doc), formDrawn(f, f.detailPhoneBody(tt.doc, routedAt)); !slices.Equal(got, want) {
				t.Errorf("at %d columns, the phone's body isn't drawn", f.Width)
			}
		})
	}
}

// With one account, which has no list, the wide form's leasts fit from 81
// columns, under the phone's chrome as under the wide, and Detail is drawn
// wide from there, stacked narrower: the chrome's breakpoint chooses nothing
// of it.
func TestOneAccountsDetailIsWideFromWhereItsLeastsFit(t *testing.T) {
	doc := aloneDoc()
	f := detailFrame(Look{}, framedAccounts[:1], "work", "", alonePage(t), nil)
	for _, tt := range []struct {
		width int
		wide  bool
	}{{100, true}, {99, true}, {81, true}, {80, false}} {
		f.Width = tt.width
		l, _ := laidOf(f, doc)
		want := formDrawn(f, f.detailPhoneBody(doc, routedAt))
		if tt.wide {
			want = formDrawn(f, accountsForm{draw: f.detailDrawn(doc, routedAt, l).draw})
		}
		if got := accountsDrawn(f, doc); !slices.Equal(got, want) {
			t.Errorf("at %d columns, drawn wide %v isn't what's drawn", tt.width, tt.wide)
		}
	}
}

// withoutFable takes Fable's week from every account, so the moment lists
// it for none.
func withoutFable(d *status.Document) {
	for i, a := range d.Accounts {
		d.Accounts[i].Windows = slices.DeleteFunc(slices.Clone(a.Windows), func(w quota.Window) bool { return w.Key == "7d_oi" })
	}
}

// sonnetWeek gives work a Sonnet week window, named past 8 cells.
func sonnetWeek(d *status.Document) {
	w := d.Accounts[0].Windows[len(d.Accounts[0].Windows)-1]
	w.Key, w.Label = "7d_sonnet", "Sonnet week"
	d.Accounts[0].Windows = append(slices.Clone(d.Accounts[0].Windows), w)
}

// Wide, Detail's weeks are as many as fit from cx to the frame's edge, six
// at most, their gap 4, then 3, then 2, before a week leaves; past six at a
// gap of 4, their days' cells grow, a whole cell at a time. The rule names
// the weeks shown and ends at their edge, as BY DAY's last week does, and
// THIS WEEK ends there too, 23 cells with three accounts, its figures cut
// there. USAGE's bar takes what's left before THIS WEEK, its words at their
// widest a cell short of it, at least 16 and at most 40.
func TestDetailsWideFormFitsItsWeeksToTheWidth(t *testing.T) {
	doc := usageWorld(extraOff, happened(onAuto()...))
	tests := []struct {
		width          int
		rule           string
		weeks, gap, dw int
		edge, bar      int
		first          string
	}{
		{width: 105, rule: "the last four weeks", weeks: 4, gap: 4, dw: 2, edge: 104, bar: 16, first: "7 Sep"},
		{width: 108, rule: "the last four weeks", weeks: 4, gap: 4, dw: 2, edge: 104, bar: 16, first: "7 Sep"},
		{width: 120, rule: "the last five weeks", weeks: 5, gap: 3, dw: 2, edge: 118, bar: 30, first: "31 Aug"},
		{width: 138, rule: "the last six weeks", weeks: 6, gap: 3, dw: 2, edge: 135, bar: 40, first: "24 Aug"},
		{width: 141, rule: "the last six weeks", weeks: 6, gap: 4, dw: 2, edge: 140, bar: 40, first: "24 Aug"},
		{width: 160, rule: "the last six weeks", weeks: 6, gap: 4, dw: 2, edge: 140, bar: 40, first: "24 Aug"},
		{width: 220, rule: "the last six weeks", weeks: 6, gap: 4, dw: 3, edge: 182, bar: 40, first: "24 Aug"},
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.width), func(t *testing.T) {
			f := detailFrame(Look{}, framedAccounts, "work", "", detailsPage(t), nil)
			f.Width, f.Height = tt.width, 44
			l := laidIn(f, doc)
			if l.weeks != tt.weeks || l.gap != tt.gap || l.dw != tt.dw || l.edge() != tt.edge {
				t.Fatalf("its weeks are %d, %d apart, their days %d cells, ending at %d; want %d, %d apart, %d cells, ending at %d", l.weeks, l.gap, l.dw, l.edge(), tt.weeks, tt.gap, tt.dw, tt.edge)
			}
			rows := accountsDrawn(f, doc)
			rule := rows[rowWith(rows, "┄┄ ")]
			if columnOf(rule, "┄┄ "+tt.rule+" ┄") != l.panel || ansi.StringWidth(rule) != tt.edge {
				t.Errorf("the rule reads %q, want ┄┄ %s from column %d to the weeks' edge, %d", rule, tt.rule, l.panel, tt.edge)
			}
			heads := rows[rowWith(rows, "BY DAY")+1]
			if columnOf(heads, tt.first) != l.cx() || columnOf(heads, "28 Sep") != l.cx()+(tt.weeks-1)*l.step() {
				t.Errorf("BY DAY's weeks are headed %q, want %s at %d and 28 Sep a step of %d apart after", heads, tt.first, l.cx(), l.step())
			}
			if at := columnOf(rows[rowWith(rows, "THIS WEEK")], "THIS WEEK"); at != tt.edge-23 {
				t.Errorf("THIS WEEK is at %d, want it 23 cells before the weeks' edge, %d", at, tt.edge)
			}
			for _, label := range []string{"requests", "limits", "plan", "worth", "extra use"} {
				if row := rows[rowWith(rows, padded(label, thisWeekLabels))]; ansi.StringWidth(row) > tt.edge {
					t.Errorf("THIS WEEK's %s reads %q, past the weeks' edge, %d", label, row, tt.edge)
				}
			}
			week := rows[rowWith(rows, "Week ")]
			if bar := columnOf(week, "34%") - l.panel - 9 - 2; bar != tt.bar {
				t.Errorf("USAGE's week reads %q, its bar %d cells, want %d", week, bar, tt.bar)
			}
		})
	}
}

// Wide at any width, Detail's labels and words beside its bars and its weeks
// read whole: USAGE's windows' names and words, THIS WEEK's items, WEEKS'
// peaks; and the verdict whole, or where it doesn't fit, its shorter words
// on its one row.
func TestDetailsWideFormKeepsItsWordsWhole(t *testing.T) {
	doc := usageWorld(extraOff, happened(onAuto()...))
	whole := []string{
		"Week ", "34%  19%/day till Sun 21:00", "5-hour ", "95%  at cap · back 17:10",
		"requests  1,204", "limits    5-hour ×2", "plan      Max 20x", "worth     $184  4.0×", "extra use off",
	}
	const (
		full  = "hits its limit most weeks   limit hit 3 of the last 7 weeks · 9% left at a reset, on average"
		short = "hits its limit most weeks   limit hit 3 of 7 weeks · 9% left at a reset"
	)
	for _, width := range []int{105, 108, 120, 138, 141, 160, 220} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			f := detailFrame(Look{}, framedAccounts, "work", "", detailsPage(t), nil)
			f.Width, f.Height = width, 44
			rows := accountsDrawn(f, doc)
			for _, words := range whole {
				if rowWith(rows, words) < 0 {
					t.Errorf("no row reads %q whole", words)
				}
			}
			l := laidIn(f, doc)
			peaks := rows[rowWith(rows, "WEEKS")]
			for i, peak := range []string{"100", "94", "100", "81", "100", "34"}[sixWeeks-l.weeks:] {
				if end := l.cx() + i*l.step() + l.weekBar() + 1 + len(peak); !strings.HasSuffix(strings.TrimRight(string([]rune(peaks)[:end]), " "), " "+peak) {
					t.Errorf("WEEKS reads %q, want %s whole, ending at %d", peaks, peak, end)
				}
			}
			want := full
			if l.panel+ansi.StringWidth(full) > f.edge() {
				want = short
			}
			if verdict := rows[rowWith(rows, "hits its limit")]; columnOf(verdict, want) != l.panel {
				t.Errorf("the verdict reads %q, want %q from column %d", verdict, want, l.panel)
			}
		})
	}
}

// THIS WEEK is as wide as its labels and the widest bounded part of its
// figures, at least 20 cells: a limit hit of any window listed, by its short
// name, counted as often as its window can be hit in twelve weeks, "Fable wk
// ×12", "5h ×403"; and all accounts' extra use on for one fewer than every
// account. Alike whichever account is chosen, or all of them.
func TestDetailsTHISWEEKIsAsWideAsItsWidestBoundedPart(t *testing.T) {
	tests := []struct {
		name     string
		doc      status.Document
		accounts int
		cells    int
	}{
		{name: "one account, its windows' limits within 10", doc: usageWorld(withoutFable), accounts: 1, cells: 20},
		{name: "one account, Fable wk ×12", doc: usageWorld(), accounts: 1, cells: 22},
		{name: "three accounts, on for 2 of 3", doc: usageWorld(withoutFable), accounts: 3, cells: 23},
		{name: "ten accounts, on for 9 of 10", doc: usageWorld(withoutFable), accounts: 10, cells: 24},
		{name: "a Sonnet week, Sonnet wk ×12", doc: usageWorld(sonnetWeek), accounts: 1, cells: 23},
	}
	for _, tt := range tests {
		configured := make(config.Accounts, tt.accounts)
		for i := range configured {
			configured[i] = config.Account{ID: fmt.Sprintf("account-%d", i)}
		}
		for _, chosen := range []string{"", "account-0"} {
			if l := laidIn(detailFrame(Look{}, configured, chosen, "", nil, nil), tt.doc); l.thisWeek != tt.cells {
				t.Errorf("%s, %q chosen: THIS WEEK is %d cells, want %d", tt.name, chosen, l.thisWeek, tt.cells)
			}
		}
	}
}

// THIS WEEK never cuts a bounded part of a figure, nor drops one: where an
// item's figure doesn't fit its row, its parts go over the rows they need,
// each in full where it fits, else short, and BY MODEL moves down with them.
// Its width is the same at every width, so are its rows.
func TestDetailsTHISWEEKReadsEveryBoundedPartWhole(t *testing.T) {
	doc := usageWorld(extraOff)
	limited := func(limits map[string]int) *views.AccountsPage {
		work := detailWork
		work.accountsFramed.limits = limits
		return detailPage(t, work, detailPersonal, detailSide, detailAll)
	}
	unknown := detailsPage(t)
	unknown.All.Plan.Plans = views.PlansKnown{Known: 2, Of: 3}
	tests := []struct {
		name, chosen, label string
		unlisted            bool
		page                *views.AccountsPage
		rows                []string
	}{
		{name: "limits of two windows", chosen: "work", label: "limits", page: limited(map[string]int{"5h": 2, "7d": 1}), rows: []string{"week ×1", "5-hour ×2"}},
		{name: "a limit of a window the moment doesn't list", chosen: "work", label: "limits", unlisted: true,
			page: limited(map[string]int{"5h": 2, "7d_oi": 1}), rows: []string{"5-hour ×2", "Fable week ×1"}},
		{name: "limits of three windows, a model's among them", chosen: "work", label: "limits",
			page: limited(map[string]int{"5h": 2, "7d": 1, "7d_oi": 1}), rows: []string{"week ×1", "5-hour ×2", "Fable week ×1"}},
		{name: "every account's worth over the plans known", label: "worth", page: unknown, rows: []string{"$635  4.6×", "2 of 3 plans"}},
	}
	for _, width := range []int{105, 108, 138, 160, 220} {
		for _, tt := range tests {
			t.Run(fmt.Sprintf("%s at %d", tt.name, width), func(t *testing.T) {
				doc := doc
				if tt.unlisted {
					doc = usageWorld(extraOff, withoutFable)
				}
				f := detailFrame(Look{}, framedAccounts, tt.chosen, "", tt.page, nil)
				f.Width, f.Height = width, 44
				l := laidIn(f, doc)
				rows := accountsDrawn(f, doc)
				title, x := rowWith(rows, "THIS WEEK"), l.thisWeekAt()+thisWeekLabels
				first := rowWith(rows, padded(tt.label, thisWeekLabels))
				for i, want := range tt.rows {
					row := []rune(rows[first+i])
					if got := strings.TrimRight(string(row[x:min(len(row), l.edge())]), " "); got != want || len(row) > l.edge() {
						t.Errorf("%s's row %d reads %q, want %q, within the weeks' edge, %d", tt.label, i+1, got, want, l.edge())
					}
				}
				items := 5 + len(tt.rows) - 1
				if got, want := rowWith(rows, "BY MODEL"), title+items+byModelGap; got != want {
					t.Errorf("BY MODEL is on row %d, want %d, %d under THIS WEEK's %d rows", got, want, byModelGap, items)
				}
			})
		}
	}
}

// THIS WEEK's width bounds the limits the page records as well as the
// windows the moment lists: one account's limit of a window it no longer
// lists, Fable's week twelve times, reads whole, "Fable wk ×12", where the
// listed windows alone would bound it at 10 cells.
func TestDetailsTHISWEEKBoundsTheLimitsThePageRecords(t *testing.T) {
	doc := usageWorld(extraOff, withoutFable)
	work := detailWork
	work.accountsFramed.limits = map[string]int{"7d_oi": 12}
	f := detailFrame(Look{}, framedAccounts[:1], "work", "", detailPage(t, work, detailAll), nil)
	const want = "Fable wk ×12"
	for width := 100; width <= 230; width++ {
		f.Width, f.Height = width, 44
		l := laidIn(f, doc)
		rows := accountsDrawn(f, doc)
		row := []rune(rows[rowWith(rows, padded("limits", thisWeekLabels))])
		x := l.thisWeekAt() + thisWeekLabels
		if got := strings.TrimRight(string(row[x:min(len(row), l.edge())]), " "); got != want || len(row) > l.edge() {
			t.Errorf("at %d, limits reads %q, want %q, within the weeks' edge, %d", width, got, want, l.edge())
		}
	}
}

// The verdict fits to the weeks' edge, never the frame's, so it reads alike
// whatever the list's names: in full where that fits from the panel's left
// to the weeks' edge, else its shorter words, as at 117 and 118 columns,
// where the weeks take 78 cells, and in full at 119, where they take 82.
func TestDetailsVerdictFitsToTheWeeksEdge(t *testing.T) {
	doc := usageWorld(extraOff)
	long := config.Accounts{{ID: "work", Label: "work-team-plan"}, {ID: "personal"}, {ID: "side", Label: "side-hustle-consulting"}}
	for _, tt := range []struct {
		width int
		want  string
	}{
		{117, "hits its limit most weeks   limit hit 3 of 7 weeks · 9% left at a reset"},
		{118, "hits its limit most weeks   limit hit 3 of 7 weeks · 9% left at a reset"},
		{119, "hits its limit most weeks   limit hit 3 of the last 7 weeks · 9% left at a reset, on average"},
	} {
		for _, configured := range []config.Accounts{framedAccounts, long} {
			f := detailFrame(Look{}, configured, "work", "", detailsPage(t), nil)
			f.Width, f.Height = tt.width, 44
			l := laidIn(f, doc)
			rows := accountsDrawn(f, doc)
			if verdict := rows[rowWith(rows, "hits its limit")]; columnOf(verdict, tt.want) != l.panel || ansi.StringWidth(verdict) > l.edge() {
				t.Errorf("at %d, %d names: the verdict reads %q, want %q from column %d to the weeks' edge, %d", tt.width, l.names, verdict, tt.want, l.panel, l.edge())
			}
		}
	}
}

// Detail's list fits its name column to the names by the rule USAGE fits
// its own by, nameColumn: the widest, number, label and ◆, and a gap, at
// least 14 cells and at most 24, its dots, its light, its │ and the panel
// moving over with it. A long label is cut with … to what the panel leaves
// the list, as the list gives way first.
func TestDetailsListFitsItsNameColumnToTheNames(t *testing.T) {
	const long, longest = "personal-account", "personal-account-of-many-letters"
	tests := []struct {
		name         string
		width        int
		label        string
		names        int
		listed, head string
	}{
		{name: "the frames' names", width: 160, names: 14, listed: "2 personal ◆", head: "2 personal ◆"},
		{name: "a long name, room to spare", width: 200, label: long, names: 22, listed: "2 personal-account ◆", head: "2 personal-account ◆"},
		{name: "a long name, 4 cells past the panel's natural", width: 145, label: long, names: 18, listed: "2 personal-ac… ◆", head: "2 personal-account ◆"},
		{name: "a long name, the panel at its natural", width: 141, label: long, names: 14, listed: "2 persona… ◆", head: "2 personal-account ◆"},
		{name: "a long name, the cells the weeks leave", width: 138, label: long, names: 16, listed: "2 personal-… ◆", head: "2 personal-account ◆"},
		{name: "a name past the most", width: 200, label: longest, names: 24, listed: "2 personal-account-… ◆", head: "2 " + longest + " ◆"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configured := slices.Clone(framedAccounts)
			configured[1].Label, configured[1].Primary = tt.label, true
			doc := usageWorld(extraOff, func(d *status.Document) { d.Accounts[1].Label = tt.label })
			f := detailFrame(Look{}, configured, "personal", "", detailsPage(t), nil)
			f.Width = tt.width
			l, wide := laidOf(f, doc)
			if l.names != tt.names || !wide {
				t.Fatalf("its list's name column is %d cells, its wide form fitting %v, want %d cells, wide", l.names, wide, tt.names)
			}
			if l.panel != l.dotAt()+panelPastDot {
				t.Errorf("the panel is at %d, want it %d past the name column's dots, at %d", l.panel, panelPastDot, l.dotAt())
			}
			rows := accountsDrawn(f, doc)
			personal := []rune(rows[f.top()+2*listApart])
			if got := strings.TrimSpace(string(personal[:l.dotAt()])); got != tt.listed {
				t.Errorf("personal's row in the list reads %q, want %q", got, tt.listed)
			}
			if got := string(personal[l.ruleAt()]); got != "│" {
				t.Errorf("personal's row reads %q at column %d, want the list's │", got, l.ruleAt())
			}
			if got := string([]rune(rows[f.top()])[l.panel:]); !strings.HasPrefix(got, tt.head) {
				t.Errorf("the panel's head reads %q from column %d, want it to start %q", got, l.panel, tt.head)
			}
		})
	}
}

// The list gives way first (the owner's call, 15 October 2026): the panel
// takes its natural, six weeks at a gap of 4, before the names take more than
// 14 cells; the names narrow to 14 before the panel narrows; and past the
// panel's natural the names take up to 24 before the days grow. The names
// take what the weeks leave too. So the panel, its weeks, their gap and days,
// is laid alike whatever the labels, the list's width alone differing.
func TestDetailsListGivesWayBeforeThePanel(t *testing.T) {
	long := config.Accounts{
		{ID: "work", Label: "work-team-plan"}, {ID: "personal", Label: "personal-research", Primary: true},
		{ID: "side", Label: "side-hustle-consulting"},
	}
	tests := []struct {
		width               int
		short, long         int
		weeks, gap, dw, end int
	}{
		{width: 105, short: 14, long: 14, weeks: 4, gap: 4, dw: 2, end: 68},
		{width: 108, short: 14, long: 17, weeks: 4, gap: 4, dw: 2, end: 68},
		{width: 120, short: 14, long: 15, weeks: 5, gap: 3, dw: 2, end: 82},
		{width: 138, short: 14, long: 16, weeks: 6, gap: 3, dw: 2, end: 99},
		{width: 141, short: 14, long: 14, weeks: 6, gap: 4, dw: 2, end: 104},
		{width: 151, short: 14, long: 24, weeks: 6, gap: 4, dw: 2, end: 104},
		{width: 160, short: 14, long: 24, weeks: 6, gap: 4, dw: 2, end: 104},
		{width: 220, short: 14, long: 24, weeks: 6, gap: 4, dw: 3, end: 146},
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.width), func(t *testing.T) {
			for _, world := range []struct {
				name       string
				configured config.Accounts
				names      int
			}{{"8-cell names", framedAccounts, tt.short}, {"long names", long, tt.long}} {
				f := detailFrame(Look{}, world.configured, "work", "", nil, nil)
				f.Width = tt.width
				l, wide := laidOf(f, usageWorld(extraOff))
				if !wide || l.names != world.names {
					t.Errorf("%s: the list's names are %d cells, wide %v, want %d, wide", world.name, l.names, wide, world.names)
				}
				if l.weeks != tt.weeks || l.gap != tt.gap || l.dw != tt.dw || l.weeksCells() != tt.end || l.edge() > f.edge() {
					t.Errorf("%s: its weeks are %d, %d apart, their days %d cells, %d cells in all to %d; want %d, %d apart, %d cells, %d cells, within %d",
						world.name, l.weeks, l.gap, l.dw, l.weeksCells(), l.edge(), tt.weeks, tt.gap, tt.dw, tt.end, f.edge())
				}
			}
		})
	}
}

// accountsDrawn are the rows Accounts draws of doc at routedAt in the frame
// given, from the view's top, as text alone.
func accountsDrawn(f Frame, doc status.Document) []string {
	c := newCanvas(f.Width, f.Height)
	f.accounts(c, doc, routedAt, f.top())
	return textOf(c.rows(Look{}))
}

// formDrawn are the rows the form given draws in the frame given, from the
// view's top, as text alone.
func formDrawn(f Frame, form accountsForm) []string {
	c := newCanvas(f.Width, f.Height)
	if form.draw != nil {
		form.draw(c, f.top())
	}
	return textOf(c.rows(Look{}))
}

// Detail names each look-read figure it draws by what it is, all accounts'
// use and room their own, never the pool's, and THIS WEEK's and BY MODEL's,
// the period's, of the account shown, or all accounts, "", by their rows, a
// version's share by its name.
func TestDetailNamesTheNumbersItDraws(t *testing.T) {
	doc := usageWorld(extraOff)
	page := detailsPage(t)
	tests := []struct {
		name, chosen string
		want         map[Number]string
		none         []Number
	}{
		{
			name: "all accounts",
			want: map[Number]string{
				{Kind: NumberAllUsed, Of: "5h"}: "49%", {Kind: NumberAllRoom, Of: "5h"}: "1.5",
				{Kind: NumberAllUsed, Of: "7d"}: "59%", {Kind: NumberAllRoom, Of: "7d"}: "1.2",
				{Kind: NumberPeriod, Of: "requests"}: "5,410", {Kind: NumberPeriod, Of: "Opus 5.5 share"}: "57%",
			},
			none: []Number{{Kind: NumberUsed, Of: "5h"}, {Kind: NumberRoom, Of: "5h"}},
		},
		{
			name: "an account", chosen: "work",
			want: map[Number]string{
				{Kind: NumberUsed, Account: "work", Of: "5h"}: "95%", {Kind: NumberUsed, Account: "work", Of: "7d"}: "34%",
				{Kind: NumberPeriod, Account: "work", Of: "requests"}: "1,204", {Kind: NumberPeriod, Account: "work", Of: "worth"}: "$1844.0×",
			},
			none: []Number{{Kind: NumberUsed, Account: "personal", Of: "5h"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detailFrame(Look{}, framedAccounts, tt.chosen, "", page, nil).Numbers(doc, routedAt)
			for n, want := range tt.want {
				if got[n] != want {
					t.Errorf("%+v reads %q, want %q", n, got[n], want)
				}
			}
			for _, n := range tt.none {
				if said, ok := got[n]; ok {
					t.Errorf("%+v is named, reading %q", n, said)
				}
			}
		})
	}
}

// Detail fills every account's bars, whichever is chosen, so one chosen
// next eases from where it stood, and all accounts', by their windows' keys
// alone.
func TestDetailFillsEveryAccountsBarsAndAllAccounts(t *testing.T) {
	doc := usageWorld(extraOff, func(d *status.Document) { d.Accounts[2].TokenSet = false })
	filled := detailFrame(Look{}, framedAccounts, "work", "", nil, nil).Filled(doc, routedAt)
	want := map[Ref]float64{
		{Account: "work", Window: "5h"}: 0.95, {Account: "work", Window: "7d"}: 0.34, {Account: "work", Window: "7d_oi"}: 0.12,
		{Account: "personal", Window: "5h"}: 0.39, {Account: "personal", Window: "7d"}: 0.66, {Account: "personal", Window: "7d_oi"}: 0,
		{Window: "5h"}: (0.95 + 0.39) / 2, {Window: "7d"}: (0.34 + 0.66) / 2, {Window: "7d_oi"}: 0.12 / 2,
	}
	if len(filled) != len(want) {
		t.Errorf("Detail fills %v, want %v: none of side's, without a token", filled, want)
	}
	for ref, used := range want {
		if got, ok := filled[ref]; !ok || math.Abs(got-used) > 1e-9 {
			t.Errorf("%+v fills %v, want %v", ref, got, used)
		}
	}
}

// Detail's USAGE draws each bar as far as it has eased, as the Overview's:
// an account's by its window, all accounts' by its window's key alone.
func TestDetailsUSAGEEasesItsBars(t *testing.T) {
	doc := usageWorld(extraOff)
	for _, tt := range []struct {
		name, chosen string
		of           Ref
	}{
		{name: "an account's", chosen: "work", of: Ref{Account: "work", Window: "5h"}},
		{name: "all accounts'", of: Ref{Window: "5h"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := detailFrame(Look{}, framedAccounts, tt.chosen, "", nil, nil)
			f.Eased = map[Ref]float64{tt.of: 0.2}
			if g := f.detailDrawn(doc, routedAt, laidIn(f, doc)).gauge("5h"); g.bar.eased == nil || *g.bar.eased != 0.2 {
				t.Errorf("the 5-hour bar is drawn eased to %v, want 0.2", g.bar.eased)
			}
		})
	}
}

// All accounts' bar of a window fades where every account it counts is
// stale, its token refused or its last read failed; not while any is read.
func TestAllAccountsBarFadesWhereEveryAccountsIsStale(t *testing.T) {
	failed := func(ids ...string) func(*status.Document) {
		return func(d *status.Document) {
			for i, a := range d.Accounts {
				if slices.Contains(ids, a.ID) {
					d.Accounts[i].Error = "timed out"
				}
			}
		}
	}
	for _, tt := range []struct {
		name  string
		doc   status.Document
		stale bool
	}{
		{name: "every account's last read failed", doc: usageWorld(failed("work", "personal", "side")), stale: true},
		{name: "one read", doc: usageWorld(failed("work", "personal")), stale: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if g := allGauge(tt.doc.At(routedAt, claudeLike), "7d"); g.bar.empty || g.bar.stale != tt.stale {
				t.Errorf("all accounts' week is drawn %+v, want stale %t", g.bar, tt.stale)
			}
		})
	}
}

// BY MODEL stands two rows under the lower of USAGE's last window and THIS
// WEEK's last item: under THIS WEEK's while USAGE has three windows or
// fewer, and under USAGE's past them.
func TestDetailsBYMODELStandsUnderUSAGEAndTHISWEEK(t *testing.T) {
	oi := func(used float64) quota.Window { return windowOf("7d_oi", "Fable week", used, 3*quota.Day) }
	opus := func(used float64) quota.Window { return windowOf("7d_opus", "Opus week", used, 3*quota.Day) }
	for _, tt := range []struct {
		name    string
		windows []quota.Window
		want    int
	}{
		{name: "two windows", windows: []quota.Window{sessionOf(0.2, time.Hour), weekOf(0.3, 3*quota.Day)}, want: 9},
		{name: "three windows", windows: []quota.Window{sessionOf(0.2, time.Hour), weekOf(0.3, 3*quota.Day), oi(0.1)}, want: 9},
		{name: "four windows", windows: []quota.Window{sessionOf(0.2, time.Hour), weekOf(0.3, 3*quota.Day), oi(0.1), opus(0.1)}, want: 11},
	} {
		t.Run(tt.name, func(t *testing.T) {
			doc := routerDoc("work", 0, readAccount("work", tt.windows...), readAccount("personal", tt.windows...))
			f := detailFrame(Look{}, framedAccounts[:2], "", "", detailsPage(t), nil)
			rows := textOf(f.Draw(doc, now))
			if got := rowWith(rows, "BY MODEL") - f.top(); got != tt.want {
				t.Errorf("BY MODEL is on the panel's row %d, want %d", got, tt.want)
			}
		})
	}
}

// Detail's head picks out how the account stands while its state in a word
// is newly changed, and the list its state's glyph, over the chosen row's
// light, as Compare's heads do, never for a change ROUTING alone sees; and
// LATELY a line while what it tells is fresh: on bg.attention, as far faded
// back as each has gone.
func TestDetailCuesAStateNewlyChangedAndAFreshLine(t *testing.T) {
	doc := usageWorld(extraOff, happened(onAuto()...))
	f := detailFrame(Screen(builtin(t, "nord")), framedAccounts, "work", "", nil, nil)
	told := views.AccountLately(f.Moment(doc, routedAt), "work", latelyLines)[0]
	f.StateChanged, f.Fresh = map[string]float64{"work": 0.25}, map[int]float64{told.ID: 0.5}
	f.Changed = map[string]float64{"personal": 0.25}
	d := f.detailDrawn(doc, routedAt, laidIn(f, doc))
	c := d.panel()
	rows := textOf(c.rows(Look{}))
	state := d.l.panel + listName(d.account, 1).fitted(d.f.edge()).width() + headApart
	if []rune(rows[0])[state] == ' ' {
		t.Fatalf("the head reads %q, want its state from column %d", rows[0], state)
	}
	lately := rowWith(rows, "LATELY")
	list := growing(f.Width)
	d.list(list, 0, f.viewRows())
	tests := []struct {
		name string
		c    *canvas
		x, y int
		fade float64
	}{
		{name: "the head's state", c: c, x: state, y: 0, fade: 0.25},
		{name: "LATELY's first line", c: c, x: d.l.cx(), y: lately, fade: 0.5},
		{name: "work's glyph in the list, chosen", c: list, x: framedLayout().dotAt(), y: listApart, fade: 0.25},
	}
	for _, tt := range tests {
		if got := tt.c.at(tt.x, tt.y).ink.on; got != (hue{token: theme.BgAttention, fade: tt.fade}) {
			t.Errorf("%s is on %+v, want bg.attention faded %v", tt.name, got, tt.fade)
		}
	}
	for _, unchanged := range []struct {
		name string
		c    *canvas
		x, y int
	}{
		{name: "LATELY's second line", c: c, x: d.l.cx(), y: lately + 1},
		{name: "the head's name", c: c, x: d.l.panel, y: 0},
		{name: "the cell before the head's state", c: c, x: state - 1, y: 0},
		{name: "personal's glyph, its routing alone changed", c: list, x: framedLayout().dotAt(), y: 2 * listApart},
	} {
		if got := unchanged.c.at(unchanged.x, unchanged.y).ink.on; got.token == theme.BgAttention {
			t.Errorf("%s is on %+v, want it not cued", unchanged.name, got)
		}
	}
}

// A session in flight as last known, the router not answering, is said in
// SESSIONS as it was, in text.subtle and not bold, its ○ at rest.
func TestDetailsSESSIONSSaysWhatsInFlightAsLastKnownAsItWas(t *testing.T) {
	const doingAt = 18
	draw := func(a views.Activity) *canvas {
		c := newCanvas(40, 1)
		detailDrawn{m: status.Moment{Now: routedAt}}.session(c, views.Running{Session: idD28C, Family: "opus", Activity: a}, 0, 0, false)
		return c
	}
	for _, doing := range []views.Doing{views.Asking, views.Answering, views.Throttled, views.Refused} {
		live := views.Activity{Doing: doing, Status: 429, Chars: 4936, LastSeen: routedAt.Add(-time.Hour)}
		stale := live
		stale.Stale = true
		said := live.Short()
		c := draw(stale)
		if got, known := glyphsAt(c, doingAt, 0, len([]rune(said))), glyphsAt(draw(live), doingAt, 0, len([]rune(said))); got != said || known != said {
			t.Errorf("doing %d, SESSIONS reads %q as last known and %q as known, want %q, as it was", doing, got, known, said)
		}
		if got := c.at(doingAt, 0).ink; got != dimInk {
			t.Errorf("doing %d, as last known, it's in %+v, want %+v", doing, got, dimInk)
		}
		if got, known := glyphsAt(c, 2, 0, 1), glyphsAt(draw(live), 2, 0, 1); got != "○" || known != "●" {
			t.Errorf("doing %d, its dot is %q as last known and %q as known, want ○, at rest, and ●", doing, got, known)
		}
	}
}

// laidOf is Detail's wide layout in the frame of doc at routedAt, as
// detailLaid lays it, and whether its leasts fit.
func laidOf(f Frame, doc status.Document) (detailLayout, bool) {
	return f.detailLaid(f.Moment(doc, routedAt))
}

// laidIn is Detail's wide layout in the frame of doc at routedAt.
func laidIn(f Frame, doc status.Document) detailLayout {
	l, _ := laidOf(f, doc)
	return l
}
