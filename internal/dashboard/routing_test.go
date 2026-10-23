package dashboard

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// autoRouting is the auto world ROUTING's frames draw at routedAt:
// routedWorld's accounts, personal the primary, and the sessions the router
// lists on each.
func autoRouting() (status.Document, []status.Session) {
	doc, sessions := routedWorld(false)
	doc.Accounts[1].Primary = true
	return doc, sessions
}

// pinnedRouting is the pinned world: the auto world's accounts, new sessions
// pinned to personal since 14:02, and as the document counts them, three
// sessions on personal and three on side.
func pinnedRouting() status.Document {
	doc, _ := autoRouting()
	doc.Pin = status.Pin{Accounts: []string{"personal"}, Since: routedClock(1, 14, 2)}
	doc.Best = "personal"
	doc.Accounts[1].Sessions, doc.Accounts[2].Sessions = 3, 3
	return doc.WorkedOut(claudeLike)
}

// tenRouting is the world of ten accounts: the auto world's three, as the
// document counts their sessions, then client; spare; lab, under pressure,
// running out at 16:05; team, at its limit till 15:30; extra, ops and
// studio.
func tenRouting() status.Document {
	doc, _ := autoRouting()
	accounts := doc.Accounts
	open := func(id string, used float64) status.Account {
		return routedAccount(id, routedWindow("5h", used, routedClock(1, 18, 0)), routedWindow("7d", 0.2, routedClock(5, 10, 0)))
	}
	lab := open("lab", 0.71)
	runsOut := routedClock(1, 16, 5).Add(30 * time.Second)
	rate := (1 - 0.71) / runsOut.Sub(routedAt).Hours()
	lab.Rates = []status.Rate{{Window: "5h", Rate: rate, Since: routedAt.Add(-score.Recent)}}
	lab.Pressure = status.Pressure{Window: "5h", Rate: rate, Recent: true, Since: routedAt.Add(-score.Recent), RunsOut: runsOut, Under: true}
	team := open("team", 1)
	team.Windows[0] = routedWindow("5h", 1, routedClock(1, 15, 30))
	team.Windows[0].Status = quota.StatusRejected
	team.Limit = status.Limit{Windows: []string{"5h"}, Until: routedClock(1, 15, 30)}
	doc.Accounts = append(accounts, open("client", 0.52), open("spare", 0.04), lab, team, open("extra", 0.22), open("ops", 0.06), open("studio", 0.33))
	m := doc.At(routedAt, claudeLike)
	for id, n := range map[string]int{"personal": 2, "side": 4, "client": 3, "lab": 2, "extra": 1, "studio": 2} {
		doc.Accounts[place(m, id)-1].Sessions = n
	}
	return doc.WorkedOut(claudeLike)
}

func TestROUTINGsFormsLayItsRowsOut(t *testing.T) {
	doc, sessions := autoRouting()
	lane := func(n int) string { return strings.Repeat(traceLane, n) }
	// On a row an account, at 120, the words are 40 cells, its sessions at 58
	// and its trace 41 cells from column 71, its rate ending at the last
	// column; on two, at 82, its trace is from the name column to 62.
	wide := func(name, state, sessions string) map[int]string {
		return map[int]string{0: name, 16: state, 58: sessions}
	}
	traced := func(texts map[int]string) string {
		texts[71], texts[115] = lane(41), "0/min"
		return textsAt(120, texts)
	}
	work, personal, side := "● at its cap · back 17:10", "● open", "● open · new sessions come here"
	caption := textsAt(120, map[int]string{71: "requests a minute, last 30 minutes"})
	tests := []struct {
		name  string
		width int
		rows  int
		want  []string
	}{
		{
			name: "size 1: a row an account, the caption under the last", width: 120, rows: 4,
			want: []string{
				traced(wide("1 work", work, "0 sessions")),
				traced(wide("2 personal ◆", personal, "2 sessions")),
				traced(wide("3 side ▲", side, "4 sessions")),
				caption,
			},
		},
		{
			name: "size 2: two rows of trace an account, its rate on the second, and a blank row, the caption on the last", width: 120, rows: 9,
			want: []string{
				textsAt(120, wide("1 work", work, "0 sessions")), traced(map[int]string{}), "",
				textsAt(120, wide("2 personal ◆", personal, "2 sessions")), traced(map[int]string{}), "",
				textsAt(120, wide("3 side ▲", side, "4 sessions")), traced(map[int]string{}),
				caption,
			},
		},
		{
			name: "its row's parts not fitting a row: two rows an account and a blank row, its short state, its trace to 20 cells short of the edge", width: 82, rows: 8,
			want: []string{
				" 1 work         ● at its cap · 17:10", textsAt(82, map[int]string{16: lane(46), 65: "0 sessions  0/min"}), "",
				" 2 personal ◆   ● open", textsAt(82, map[int]string{16: lane(46), 65: "2 sessions  0/min"}), "",
				" 3 side ▲       ● open · next", textsAt(82, map[int]string{16: lane(46), 65: "4 sessions  0/min"}),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Width: tt.width, Height: 40, View: Overview, Policy: claudeLike, Sessions: sessions}
			c := newCanvas(f.Width, tt.rows)
			f.routingSection(doc, routedAt).draw(c, bounds{width: f.Width, rows: tt.rows})
			if got := textOf(c.rows(Look{})); !slices.Equal(got, tt.want) {
				t.Errorf("drew\n  %q\nwant\n  %q", got, tt.want)
			}
		})
	}
}

func TestAnAccountsStateIsCutByWholeClausesBeforeWhatFollowsIt(t *testing.T) {
	capped := func(pinned bool) status.Document {
		doc, _ := autoRouting()
		if pinned {
			doc.Pin = status.Pin{Accounts: []string{"work"}, Since: routedClock(1, 14, 2)}
		}
		return doc.WorkedOut(claudeLike)
	}
	tests := []struct {
		name         string
		doc          status.Document
		width, trace int
		want         string
	}{
		{
			name: "whole, ending two cells or more before its sessions at 79", doc: capped(false), width: 173, trace: 92,
			want: textsAt(92, map[int]string{0: "1 work", 16: "● at its cap · back 17:10 · new sessions go elsewhere", 79: "0 sessions"}),
		},
		{
			name: "pinned: the pin left off its words, as ● pinned at 68 says it, to end two cells or more before the mark", doc: capped(true), width: 173, trace: 92,
			want: textsAt(92, map[int]string{0: "1 work", 16: "● at its cap", 68: "● pinned", 79: "0 sessions"}),
		},
		{
			name: "pinned at 85, ● pinned left off, its lead with no room beside it: its words say the pin", doc: capped(true), width: 85, trace: 53,
			want: textsAt(53, map[int]string{0: "1 work", 16: "● at its cap · pinned", 40: "0 sessions"}),
		},
		{
			name: "pinned at 100: ● pinned beside its lead", doc: capped(true), width: 100, trace: 61,
			want: textsAt(61, map[int]string{0: "1 work", 16: "● at its cap", 37: "● pinned", 48: "0 sessions"}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Width: tt.width, Height: 40, View: Overview, Policy: claudeLike}
			c := newCanvas(f.Width, 4)
			f.routingSection(tt.doc, routedAt).draw(c, bounds{width: f.Width, rows: 4})
			row := []rune(textOf(c.rows(Look{}))[0])
			if got := textsAt(tt.trace, map[int]string{0: string(row[:tt.trace])}); got != tt.want {
				t.Errorf("drew\n  %q\nwant\n  %q", got, tt.want)
			}
		})
	}
}

func TestStandingWordsAreCutWholeFromTheEnd(t *testing.T) {
	r := status.Routing{Kind: status.RoutingCapped, Says: "at its cap", Clauses: []string{"back 17:10", "new sessions go elsewhere"}, Short: []string{"at its cap", "17:10"}}
	tests := []struct {
		name  string
		width int
		short bool
		want  string
	}{
		{name: "every clause, where they fit", width: 53, want: "● at its cap · back 17:10 · new sessions go elsewhere"},
		{name: "the last clause left off whole, a cell short", width: 52, want: "● at its cap · back 17:10"},
		{name: "every clause left off", width: 24, want: "● at its cap"},
		{name: "the first words cut short, where even they don't fit", width: 8, want: "● at it…"},
		{name: "short", width: 40, short: true, want: "● at its cap · 17:10"},
	}
	for _, tt := range tests {
		if got := standingWords(r, tt.short, tt.width).plain(); got != tt.want {
			t.Errorf("%s: %q, want %q", tt.name, got, tt.want)
		}
	}
}

// rowStates are states a row of ROUTING's says: at its cap, back at 17:10; a
// model's requests refused, its family in its words alone; and open, where
// new sessions go.
var rowStates = map[string]status.Routing{
	"capped": {
		Kind: status.RoutingCapped, Says: "at its cap", Clauses: []string{"back 17:10", "new sessions go elsewhere"},
		Short: []string{"at its cap", "17:10"}, Back: routedClock(1, 17, 10),
	},
	"refused": {Kind: status.RoutingModelRefused, Says: "refused (403, opus)", Clauses: []string{"other models still come here"}, Short: []string{"refused (403)"}},
	"next":    {Kind: status.RoutingNext, Says: "open", Clauses: []string{"new sessions come here"}, Short: []string{"open", "next"}},
}

// On a row of ROUTING's, an account's state takes the widest form that fits
// whole, of its words cut to their leading clauses, from all of them to
// their lead alone, and its short form; so its time is kept wherever a form
// keeps it, and a model's family is never lost to a short form narrower than
// its lead.
func TestARowsStateTakesTheWidestFormThatFitsWhole(t *testing.T) {
	tests := []struct {
		name, state string
		width       int
		want        string
		timed       bool
	}{
		{name: "its words whole", state: "capped", width: 53, want: "● at its cap · back 17:10 · new sessions go elsewhere", timed: true},
		{name: "its words to when it's back, a cell short of them whole", state: "capped", width: 52, want: "● at its cap · back 17:10", timed: true},
		{name: "its words to when it's back, at their width", state: "capped", width: 25, want: "● at its cap · back 17:10", timed: true},
		{name: "its short form, a cell short of its words to when it's back", state: "capped", width: 24, want: "● at its cap · 17:10", timed: true},
		{name: "its short form at its width", state: "capped", width: 20, want: "● at its cap · 17:10", timed: true},
		{name: "its lead alone, a cell short of its short form", state: "capped", width: 19, want: "● at its cap"},
		{name: "a refusal's lead, its family kept, wider than its short form", state: "refused", width: 21, want: "■ refused (403, opus)"},
		{name: "a refusal's short form, a cell short of its lead", state: "refused", width: 20, want: "■ refused (403)"},
		{name: "where new sessions go, its short form wider than its lead", state: "next", width: 30, want: "● open · next"},
	}
	for _, tt := range tests {
		got, timed := rowStanding(rowStates[tt.state], tt.width)
		if got.plain() != tt.want || timed != tt.timed {
			t.Errorf("%s: %q, timed %v; want %q, timed %v", tt.name, got.plain(), timed, tt.want, tt.timed)
		}
	}
}

// A row's forms come none wider than the one before.
func TestARowsFormsComeWidestFirst(t *testing.T) {
	for name, r := range rowStates {
		forms := rowForms(r)
		for i := 1; i < len(forms); i++ {
			if forms[i].words.width() > forms[i-1].words.width() {
				t.Errorf("%s: form %d, %q, is wider than the one before it, %q", name, i, forms[i].words.plain(), forms[i-1].words.plain())
			}
		}
	}
}

// From the first row an account to 100 columns, and at 108 and 120, a state
// keeps its time wherever a form holds it whole: work at its cap, a name
// column of 16, from 83, in its short form until its words to when it's
// back fit from 89; and of ten accounts, a name column of 17, team at its
// limit, from 84, its words cut to their lead in under 23 cells, then its
// short form from 87, until its words fit whole from 96.
func TestARowsStateKeepsItsTime(t *testing.T) {
	doc, _ := autoRouting()
	tests := []struct {
		name  string
		doc   status.Document
		label string
		from  int
		want  func(width int) string
	}{
		{name: "work at its cap", doc: doc, label: "1 work", from: 83, want: func(width int) string {
			if width >= 89 {
				return "● at its cap · back 17:10"
			}
			return "● at its cap · 17:10"
		}},
		{name: "team at its limit, of ten", doc: tenRouting(), label: "team", from: 84, want: func(width int) string {
			switch {
			case width >= 96:
				return "■ limit reached · back 15:30"
			case width >= 87:
				return "■ limit reached · 15:30"
			}
			return "■ limit reached"
		}},
	}
	for _, tt := range tests {
		widths := []int{108, 120}
		for width := tt.from; width <= 100; width++ {
			widths = append(widths, width)
		}
		for _, width := range widths {
			f := Frame{Width: width, Height: 40, View: Overview, Policy: claudeLike}
			at := f.routingColumns(f.Moment(tt.doc, routedAt))
			rows := 3 * len(tt.doc.Accounts)
			c := newCanvas(f.Width, rows)
			f.routingSection(tt.doc, routedAt).draw(c, bounds{width: f.Width, rows: rows})
			row := slices.IndexFunc(textOf(c.rows(Look{})), func(r string) bool { return strings.Contains(r, tt.label) })
			if !at.oneRow || row < 0 {
				t.Fatalf("%s at %d columns: one row %v, its row %d; want a row an account, %q on one", tt.name, width, at.oneRow, row, tt.label)
			}
			got := strings.TrimRight(string([]rune(textOf(c.rows(Look{}))[row])[at.names:at.sessions]), " ")
			if want := tt.want(width); got != want {
				t.Errorf("%s at %d columns: %q, want %q", tt.name, width, got, want)
			}
		}
	}
}

// accountRow is the row of ROUTING's of the account labelled, in doc at
// routedAt, at width columns, a row an account: its words, and whether
// ● pinned shows pinnedApart before its sessions.
func accountRow(t *testing.T, doc status.Document, label string, width int) (string, bool) {
	t.Helper()
	f := Frame{Width: width, Height: 40, View: Overview, Policy: claudeLike}
	at := f.routingColumns(f.Moment(doc, routedAt))
	rows := 3 * len(doc.Accounts)
	c := newCanvas(f.Width, rows)
	f.routingSection(doc, routedAt).draw(c, bounds{width: f.Width, rows: rows})
	drawn := textOf(c.rows(Look{}))
	i := slices.IndexFunc(drawn, func(r string) bool { return strings.Contains(r, label) })
	if !at.oneRow || i < 0 {
		t.Fatalf("at %d columns: one row %v, %q's row %d; want a row an account", width, at.oneRow, label, i)
	}
	row := []rune(drawn[i])
	mark := at.sessions - pinnedApart - len([]rune(pinnedSpan.text))
	marked := string(row[mark:at.sessions-pinnedApart]) == pinnedSpan.text
	words := row[at.names:at.sessions]
	if marked {
		words = row[at.names : mark-wordsGap]
	}
	return strings.TrimRight(string(words), " "), marked
}

// A row ranks its parts: its state's lead; its time, where it has one;
// ● pinned; then the rest. So the mark shows where the words beside it keep
// their lead whole and, where it has a time, that too; else it's left off,
// and the words take its cells, saying the pin where their forms carry it,
// as at its cap the pin spends: with a name column of 16, at 84 and 85,
// where its lead has no room beside the mark; where they don't, as at 83,
// the row doesn't, ROUTING's heading naming the accounts pinned. Of ten
// accounts, a name column of 17, from 100 columns: team at its limit keeps
// its time, the mark left off, until its short form fits beside it from
// 108; lab under pressure, until its words fit whole beside it from 120.
// (Narrower, where neither shows its time, the sweep of every width holds
// them.) Where new sessions go, the mark shows beside its lead, or its short
// form, from 83.
// And a model's requests refused keep its family wherever its lead fits,
// never giving way to a short form narrower than it.
func TestPinnedShowsOnARowWhereItsWordsKeepTheirLeadAndTime(t *testing.T) {
	capped, _ := autoRouting()
	capped.Pin = status.Pin{Accounts: []string{"work"}, Since: routedClock(1, 14, 2)}
	capped = capped.WorkedOut(claudeLike)
	ten := tenRouting()
	ten.Pin = status.Pin{Accounts: []string{"team", "lab"}, Since: routedClock(1, 14, 2)}
	ten = ten.WorkedOut(claudeLike)
	refused, _ := autoRouting()
	refused.Accounts[2].Refused = status.Refusal{Until: routedAt.Add(time.Hour), Status: 403, Family: "opus"}
	refused = refused.WorkedOut(claudeLike)
	type drawn struct {
		words  string
		marked bool
	}
	tests := []struct {
		name  string
		doc   status.Document
		label string
		want  map[int]drawn
	}{
		{name: "at its cap the pin spends", doc: capped, label: "1 work", want: map[int]drawn{
			83: {"● at its cap", false}, 85: {"● at its cap · pinned", false},
			86: {"● at its cap", true}, 100: {"● at its cap", true}, 173: {"● at its cap", true},
		}},
		{name: "where new sessions go, pinned", doc: pinnedRouting(), label: "2 personal", want: map[int]drawn{
			83: {"● open", true}, 90: {"● open · next", true},
		}},
		{name: "at its limit, pinned, of ten", doc: ten, label: "team", want: banded(100, 117, func(width int) drawn {
			switch {
			case width >= 108:
				return drawn{"■ limit reached · 15:30", true}
			case width >= 96:
				return drawn{"■ limit reached · back 15:30", false}
			}
			return drawn{"■ limit reached · 15:30", false}
		})},
		{name: "under pressure, pinned, of ten", doc: ten, label: "lab", want: banded(100, 121, func(width int) drawn {
			return drawn{"● under pressure · out ~16:05", width >= 120}
		})},
		{name: "a model's requests refused, unpinned, its family kept wherever its lead fits", doc: refused, label: "3 side", want: map[int]drawn{
			84: {"■ refused (403, opus)", false}, 108: {"■ refused (403, opus)", false}, 142: {"■ refused (403, opus)", false},
			145: {"■ refused (403, opus) · other models still come here", false},
		}},
	}
	for _, tt := range tests {
		for width, want := range tt.want {
			if words, marked := accountRow(t, tt.doc, tt.label, width); words != want.words || marked != want.marked {
				t.Errorf("%s at %d columns: %q, ● pinned %v; want %q, ● pinned %v", tt.name, width, words, marked, want.words, want.marked)
			}
		}
	}
}

// A wider frame never says less: for each pinned state that has a time,
// with name columns of 16 and 17, from 83 to 130 columns, once its time or
// ● pinned shows on its row, it shows on every wider one.
func TestAWiderRowNeverDropsAPinnedStatesTimeOrMark(t *testing.T) {
	back := routedClock(1, 17, 10)
	tests := []struct {
		name, time string
		r          status.Routing
	}{
		{name: "at its limit", time: "21:00", r: status.Routing{
			Kind: status.RoutingLimited, Says: "limit reached", Clauses: []string{"back Mon 21:00"}, Short: []string{"limit reached", "Mon 21:00"}, Back: back,
		}},
		{name: "a model at its limit", time: "17:10", r: status.Routing{
			Kind: status.RoutingModelLimited, Says: "Fable wk limit", Clauses: []string{"back 17:10", "other models still come here"}, Short: []string{"Fable wk limit"}, Back: back,
		}},
		{name: "under pressure", time: "16:05", r: status.Routing{
			Kind: status.RoutingPressed, Says: "under pressure", Clauses: []string{"out ~16:05"}, Short: []string{"under pressure"},
		}},
	}
	for _, tt := range tests {
		for _, names := range []int{16, 17} {
			timed, marked := 0, 0
			for width := 83; width <= 130; width++ {
				at := routingColumnsOf(width, names)
				if !at.oneRow {
					continue
				}
				words, mark := rowState(accountRouting{standing: tt.r, pinned: true}, at.words)
				saysTime := strings.Contains(words.plain(), tt.time)
				switch {
				case timed > 0 && !saysTime:
					t.Errorf("%s, a name column of %d: its time shows at %d columns, but not at %d: %q", tt.name, names, timed, width, words.plain())
				case marked > 0 && !mark:
					t.Errorf("%s, a name column of %d: ● pinned shows at %d columns, but not at %d", tt.name, names, marked, width)
				}
				if saysTime && timed == 0 {
					timed = width
				}
				if mark && marked == 0 {
					marked = width
				}
			}
			if timed == 0 || marked == 0 {
				t.Errorf("%s, a name column of %d: its time first shows at %d, ● pinned at %d; want both by 130", tt.name, names, timed, marked)
			}
		}
	}
}

// banded is what's wanted at each width from first to last, as want says.
func banded[T any](first, last int, want func(width int) T) map[int]T {
	band := map[int]T{}
	for width := first; width <= last; width++ {
		band[width] = want(width)
	}
	return band
}

// The pin is said once wherever ROUTING draws ● pinned: on two rows an
// account, and beside USAGE's blocks, its cap the pin spends leaves the pin
// out of its words.
func TestThePinIsSaidOnceWhereTheMarkShows(t *testing.T) {
	doc, _ := autoRouting()
	doc.Pin = status.Pin{Accounts: []string{"work"}, Since: routedClock(1, 14, 2)}
	doc = doc.WorkedOut(claudeLike)
	tests := []struct {
		name  string
		frame Frame
	}{
		{name: "two rows an account", frame: Frame{Width: 82, Height: 40, View: Overview, Policy: claudeLike, Sections: SectionsNamed([]string{"routing"})}},
		{name: "beside USAGE's blocks", frame: Frame{Width: 160, Height: 40, View: Overview, Policy: claudeLike, By: ByAccount, Sections: SectionsNamed([]string{"usage", "routing"})}},
	}
	for _, tt := range tests {
		page := strings.Join(textOf(tt.frame.overviewDrawn(doc, routedAt).rows(Look{})), "\n")
		if !strings.Contains(page, pinnedSpan.text) || !strings.Contains(page, "● at its cap") || strings.Contains(page, "at its cap · pinned") {
			t.Errorf("%s: drew\n%s\nwant ● pinned, and work at its cap saying nothing of the pin", tt.name, page)
		}
	}
}

func TestAStatesGlyphAndFirstWordsAreInItsColourTheRestMuted(t *testing.T) {
	rest := span{" · back 15:54", mutedInk}
	tests := []struct {
		kind status.RoutingKind
		want line
	}{
		{kind: status.RoutingTokenless, want: line{{"✕ says", errorInk}, rest}},
		{kind: status.RoutingLimited, want: line{{"■ says", errorInk}, rest}},
		{kind: status.RoutingRefused, want: line{{"■ says", errorInk}, rest}},
		{kind: status.RoutingUnreadable, want: line{{"! says", errorInk}, rest}},
		{kind: status.RoutingModelLimited, want: line{{"■ says", warningInk}, rest}},
		{kind: status.RoutingModelRefused, want: line{{"■ says", warningInk}, rest}},
		{kind: status.RoutingCapped, want: line{{"● says", warningInk}, rest}},
		{kind: status.RoutingCappedPinned, want: line{{"● says", warningInk}, rest}},
		{kind: status.RoutingModelCapped, want: line{{"● says", warningInk}, rest}},
		{kind: status.RoutingPressed, want: line{{"● says", warningInk}, rest}},
		{kind: status.RoutingUnread, want: line{{"○ says", dimInk}, rest}},
		{kind: status.RoutingIdle, want: line{{"○ says", dimInk}, rest}},
		{kind: status.RoutingNext, want: line{{"●", positiveInk}, {" says", secondaryInk}, rest}},
		{kind: status.RoutingOpen, want: line{{"●", positiveInk}, {" says", secondaryInk}, rest}},
	}
	for _, tt := range tests {
		r := status.Routing{Kind: tt.kind, Says: "says", Clauses: []string{"back 15:54"}}
		if got := standingWords(r, false, 80); !slices.Equal(got, tt.want) {
			t.Errorf("kind %d: %v, want %v", tt.kind, got, tt.want)
		}
	}
}

func TestTheModeIsROUTINGsDescription(t *testing.T) {
	pinned := func(ids ...string) status.Document {
		doc, _ := autoRouting()
		doc.Pin = status.Pin{Accounts: ids, Since: routedClock(1, 13, 40)}
		return doc
	}
	auto, _ := autoRouting()
	probe := auto
	probe.Source = status.SourceProbe
	back := line{spaces(3), {"p", keyInk}, {" back to auto", faintInk}}
	tests := []struct {
		name  string
		doc   status.Document
		cells int
		want  line
	}{
		{name: "auto", doc: auto, cells: 160, want: line{{"auto", strongInk}}},
		{
			name: "pinned to an account", doc: pinned("personal"), cells: 160,
			want: slices.Concat(line{{"pinned to personal", alertInk}, {" since 13:40", dimInk}}, back),
		},
		{
			name: "pinned to several", doc: pinned("personal", "side"), cells: 160,
			want: slices.Concat(line{{"pinned to personal and side", alertInk}, {" since 13:40", dimInk}}, back),
		},
		{name: "the mode alone, where its fuller words don't fit", doc: pinned("personal"), cells: 30, want: line{{"pinned to personal", alertInk}}},
		{name: "probed: no mode", doc: probe, cells: 160},
	}
	for _, tt := range tests {
		if got := routingMode(tt.doc.At(routedAt, claudeLike), tt.cells); !slices.Equal(got, tt.want) {
			t.Errorf("%s: %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestROUTINGsModeSaysItsFullerWordsFromOneWidthWhenEverItsPinWasSet(t *testing.T) {
	pinnedSince := func(since time.Time) status.Moment {
		doc, _ := autoRouting()
		doc.Pin = status.Pin{Accounts: []string{"personal"}, Since: since}
		return doc.At(routedAt, claudeLike)
	}
	alone := line{{"pinned to personal", alertInk}}
	back := line{spaces(3), {"p", keyInk}, {" back to auto", faintInk}}
	widest := alone.width() + len(" since ") + prose.DatedWidth() + back.width()
	tests := []struct {
		name string
		m    status.Moment
		fits int
	}{
		{name: "a pin set today", m: pinnedSince(routedClock(1, 13, 40)), fits: widest},
		{name: "a pin set on an earlier day", m: pinnedSince(time.Date(2026, 9, 28, 9, 5, 0, 0, time.UTC)), fits: widest},
		{name: "a pin from a router that doesn't say since when", m: pinnedSince(time.Time{}), fits: alone.width() + back.width()},
	}
	for _, tt := range tests {
		for cells := range 120 {
			got := routingMode(tt.m, cells)
			full := slices.Contains(got, span{" back to auto", faintInk})
			if full != (cells >= tt.fits) {
				t.Errorf("%s, in %d cells: says %q, want its fuller words %t: from %d cells, and only there", tt.name, cells, got.plain(), cells >= tt.fits, tt.fits)
			}
			if full && got.width() > cells {
				t.Errorf("%s, in %d cells: says %q, %d cells, want it whole", tt.name, cells, got.plain(), got.width())
			}
		}
	}
}

func TestROUTINGsHeadingSaysTheModeAloneBesideUSAGEAndFolded(t *testing.T) {
	doc := pinnedRouting()
	m := doc.At(routedAt, claudeLike)
	full, alone := routingMode(m, 160), modeAlone(m)
	tests := []struct {
		name        string
		frame       Frame
		description line
	}{
		{name: "by window", frame: Frame{Width: 160}, description: full},
		{name: "a frame of 52 columns: the heading's room decides, not the frame's width", frame: Frame{Width: 52}, description: full},
		{name: "by account, beside USAGE", frame: Frame{Width: 160, By: ByAccount}, description: alone},
		{name: "by account under 150 columns, a section of its own", frame: Frame{Width: 149, By: ByAccount}, description: full},
	}
	for _, tt := range tests {
		s := tt.frame.routingSection(doc, routedAt)
		if got := s.description(160); !slices.Equal(got, tt.description) {
			t.Errorf("%s: described as %v, want %v", tt.name, got, tt.description)
		}
		if got := s.folded(160); !slices.Equal(got, alone) {
			t.Errorf("%s: folded to %v, want the mode alone, %v", tt.name, got, alone)
		}
	}
}

// routingHeading is ROUTING's heading as the frame draws it of doc at
// routedAt, as text: none where it doesn't show.
func routingHeading(f Frame, doc status.Document) string {
	for _, p := range f.laidOut(doc, routedAt, 0) {
		if p.Section == SectionRouting {
			c := newCanvas(f.Width, p.head.y+1)
			f.sectionHeading(c, p)
			return strings.TrimRight(c.row(p.head.y, Look{}), " ")
		}
	}
	return ""
}

func TestROUTINGsHeadingSwitchesToItsFullerWordsOnceAsTheFrameWidensWhenEverItsPinWasSet(t *testing.T) {
	sinces := []time.Time{routedClock(1, 13, 40), time.Date(2026, 9, 28, 9, 5, 0, 0, time.UTC)}
	for n := 1; n <= len(accountIDs); n++ {
		for _, long := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d accounts, long names %t", n, long), func(t *testing.T) {
				t.Parallel()
				switches := make([]int, len(sinces))
				for i, since := range sinces {
					doc := latelyWorld(n, long)
					doc.Pin = status.Pin{Accounts: []string{accountIDs[0]}, Since: since}
					doc = doc.WorkedOut(claudeLike)
					for width := 40; width <= 220; width++ {
						full := strings.Contains(routingHeading(frameOf(width, 120), doc), "p back to auto")
						switch {
						case full && switches[i] == 0:
							switches[i] = width
						case !full && switches[i] > 0:
							t.Errorf("pinned since %s, at %d columns ROUTING's heading says the mode alone, want its fuller words, as from %d", since, width, switches[i])
						}
					}
				}
				if switches[0] == 0 || switches[0] != switches[1] {
					t.Errorf("ROUTING's heading says its fuller words from %d columns, pinned today, and %d, pinned on an earlier day: want one width, in 40 to 220", switches[0], switches[1])
				}
			})
		}
	}
}

func TestROUTINGIsBesideUSAGEOnlyWhereBothShowByAccountFrom150Columns(t *testing.T) {
	doc, _ := autoRouting()
	tests := []struct {
		name        string
		frame       Frame
		least, most int
	}{
		{name: "by window", frame: Frame{Width: 160}, least: 6, most: 11},
		{name: "by account, beside USAGE", frame: Frame{Width: 150, By: ByAccount}, least: 0, most: 0},
		{name: "by account under 150 columns", frame: Frame{Width: 149, By: ByAccount}, least: 6, most: 11},
		{name: "by account, USAGE hidden", frame: Frame{Width: 160, By: ByAccount, Sections: Sections{}.Toggled(SectionUsage)}, least: 6, most: 11},
		{name: "its row's parts not fitting a row", frame: Frame{Width: 82}, least: 10, most: 10},
	}
	for _, tt := range tests {
		if s := tt.frame.routingSection(doc, routedAt); s.least != tt.least || s.most != tt.most {
			t.Errorf("%s: least %d, most %d; want %d and %d", tt.name, s.least, s.most, tt.least, tt.most)
		}
	}
}

func TestAcrossAResetBetweenReadsROUTINGUSAGEAndTheRoutingCardAgree(t *testing.T) {
	// Work is at its cap in its 5-hour, read at 14:42, which resets at 17:10.
	doc, _ := autoRouting()
	work, reset := doc.Accounts[0], routedClock(1, 17, 10)
	f := Frame{Width: 160, Height: 40, View: Overview, Policy: claudeLike}
	tests := []struct {
		name   string
		now    time.Time
		capped bool
	}{
		{name: "a moment before it resets", now: reset.Add(-time.Second), capped: true},
		{name: "a moment after, before it's read again", now: reset.Add(time.Second)},
	}
	for _, tt := range tests {
		routing := f.routingTableOf(doc, tt.now).accounts[0].standing.Kind == status.RoutingCapped
		usage := strings.HasPrefix(wordsOf(windowNowOf(doc, work.ID, "5h", tt.now), tt.now).full.plain(), "at cap")
		card := RoutingRows(doc.At(tt.now, claudeLike), nil, views.Stream{})[1].Capped
		if routing != tt.capped || usage != tt.capped || card != tt.capped {
			t.Errorf("%s: ROUTING capped %t, USAGE's words %t, the routing card %t; want each %t", tt.name, routing, usage, card, tt.capped)
		}
	}
}

func TestAnAccountsSessionsAreTheListingsElseTheDocuments(t *testing.T) {
	doc, sessions := autoRouting()
	doc.Accounts[0].Sessions, doc.Accounts[1].Sessions, doc.Accounts[2].Sessions = 7, 1, 0
	tests := []struct {
		name     string
		sessions []status.Session
		want     []int
	}{
		{name: "as the router listed them", sessions: sessions, want: []int{0, 2, 4}},
		{name: "as the document counts them, the router listing none", want: []int{7, 1, 0}},
	}
	for _, tt := range tests {
		f := Frame{Width: 160, Height: 40, View: Overview, Policy: claudeLike, Sessions: tt.sessions}
		var got []int
		for _, r := range f.routingTableOf(doc, routedAt).accounts {
			got = append(got, r.sessions)
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("%s: %v sessions, want %v", tt.name, got, tt.want)
		}
	}
}

func TestSessionsAreCountedAsROUTINGSaysThem(t *testing.T) {
	tests := map[int]span{0: {"0 sessions", faintInk}, 1: {"1 session", mutedInk}, 15: {"15 sessions", mutedInk}}
	for n, want := range tests {
		if got := sessionsSpan(n); got != want {
			t.Errorf("%d: %v, want %v", n, got, want)
		}
	}
}

func TestAnAccountOutOfThePoolHasItsTraceAndRateFadedItsStateAsItIs(t *testing.T) {
	doc := pinnedRouting()
	f := Frame{Width: 160, Height: 40, View: Overview, Policy: claudeLike}
	var faded []string
	for _, r := range f.routingTableOf(doc, routedAt).accounts {
		if r.faded {
			faded = append(faded, r.id)
		}
	}
	if want := []string{"work", "side"}; !slices.Equal(faded, want) {
		t.Fatalf("faded %v, want %v, out of the pool", faded, want)
	}
	c := newCanvas(f.Width, 9)
	f.routingSection(doc, routedAt).draw(c, bounds{width: f.Width, rows: 9})
	if got := c.at(16, 0).ink; got != warningInk {
		t.Errorf("work's state is in %+v, want %+v, its own", got, warningInk)
	}
	if got := c.at(f.Width-1, 1).ink; got != faintInk {
		t.Errorf("work's rate is in %+v, want %+v", got, faintInk)
	}
	first := c.at(f.routingColumns(f.Moment(doc, routedAt)).trace, 1)
	if _, lane := traceInks(true); first.ink != lane {
		t.Errorf("work's empty trace is in %+v, want %+v, faded", first.ink, lane)
	}
}

func TestThePoolsStatesAreCountedInTheirOrderAndColours(t *testing.T) {
	of := func(kinds ...status.RoutingKind) []status.AccountNow {
		accounts := make([]status.AccountNow, len(kinds))
		for i, k := range kinds {
			accounts[i].Routing.Kind = k
		}
		return accounts
	}
	apart := span{" · ", faintInk}
	counted := func(k ink, words string) ink { return k.numbered(Number{Kind: NumberStates, Of: words}) }
	tests := []struct {
		name     string
		accounts []status.AccountNow
		width    int
		want     line
	}{
		{
			name: "the frame's", accounts: of(status.RoutingCapped, status.RoutingOpen, status.RoutingNext), width: 51,
			want: line{{"2 open", counted(secondaryInk, "open")}, apart, {"1 at cap", counted(warningInk, "at cap")}},
		},
		{
			name:     "each in its order: open, pressure, at cap, limit, refused, not read",
			accounts: of(status.RoutingUnread, status.RoutingRefused, status.RoutingLimited, status.RoutingCappedPinned, status.RoutingPressed, status.RoutingIdle),
			width:    80,
			want: line{
				{"1 open", counted(secondaryInk, "open")}, apart, {"1 pressure", counted(warningInk, "pressure")}, apart,
				{"1 at cap", counted(warningInk, "at cap")}, apart, {"1 limit", counted(errorInk, "limit")}, apart,
				{"1 refused", counted(errorInk, "refused")}, apart, {"1 not read", counted(dimInk, "not read")},
			},
		},
		{
			name: "held back from some models' requests alone: open to the rest", accounts: of(status.RoutingModelLimited, status.RoutingModelRefused, status.RoutingModelCapped), width: 51,
			want: line{{"3 open", counted(secondaryInk, "open")}},
		},
		{
			name:     "too wide: how many are open, pressure counting as open",
			accounts: of(status.RoutingOpen, status.RoutingNext, status.RoutingPressed, status.RoutingLimited), width: 20,
			want: line{{"3 of 4 open", counted(secondaryInk, "of 4 open")}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countedStates(status.CountStates(tt.accounts), NumberStates, tt.width); !slices.Equal(got, tt.want) {
				t.Errorf("countedStates() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Every account's states counted, as all accounts' head on Accounts counts
// them, are numbers of their own, apart from the pool's, wide or too wide,
// as all accounts is never the pool: one brightening never lights the other.
func TestEveryAccountsStatesCountedAreNumbersApartFromThePools(t *testing.T) {
	accounts := make([]status.AccountNow, 3)
	for i, k := range []status.RoutingKind{status.RoutingCapped, status.RoutingOpen, status.RoutingNext} {
		accounts[i].Routing.Kind = k
	}
	counted := func(k ink, words string) ink { return k.numbered(Number{Kind: NumberAccountStates, Of: words}) }
	tests := []struct {
		name  string
		width int
		want  line
	}{
		{name: "counted", width: 51, want: line{{"2 open", counted(secondaryInk, "open")}, {" · ", faintInk}, {"1 at cap", counted(warningInk, "at cap")}}},
		{name: "too wide", width: 12, want: line{{"2 of 3 open", counted(secondaryInk, "of 3 open")}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countedStates(status.CountStates(accounts), NumberAccountStates, tt.width); !slices.Equal(got, tt.want) {
				t.Errorf("countedStates() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestROUTINGsLinesBesideABlockAreTwoRowsApartWhereItHasTheRows(t *testing.T) {
	tests := []struct {
		windows int
		ruled   bool
		want    []int
	}{
		{windows: 4, want: []int{0, 2, 4}},
		{windows: 3, want: []int{0, 2, 4}},
		{windows: 2, want: []int{0, 1, 2}},
		{windows: 1, want: []int{0, 1}},
		{windows: 1, ruled: true, want: []int{0}},
		{windows: 2, ruled: true, want: []int{0, 1, 2}},
	}
	for _, tt := range tests {
		if got := besideRows(2*tt.windows-1, tt.ruled); !slices.Equal(got, tt.want) {
			t.Errorf("beside a block of %d windows, the pool's rule under it %v: rows %v down, want %v", tt.windows, tt.ruled, got, tt.want)
		}
	}
}

func TestAPinnedAccountBesideItsBlockSaysSoBeforeItsSessions(t *testing.T) {
	want := line{pinnedSpan, {"  ·  ", faintInk}, {"3 sessions", mutedInk.numbered(Number{Kind: NumberSessions})}}
	if got := besideSessions(accountRouting{pinned: true, sessions: 3}); !slices.Equal(got, want) {
		t.Errorf("%v, want %v", got, want)
	}
}

func TestAnAccountsStateNewlyChangedIsCued(t *testing.T) {
	doc, _ := autoRouting()
	words := len([]rune("● at its cap · back 17:10 · new sessions go elsewhere"))
	tests := []struct {
		name string
		look Look
		want ink
	}{
		{name: "on bg.attention, as far faded back as it's gone", look: Screen(builtin(t, "nord")), want: ink{on: hue{token: theme.BgAttention, fade: 0.25}}},
		{name: "without colour, bold", look: NoColour(), want: ink{on: hue{token: theme.BgAttention, fade: 0.25}, bold: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Width: 160, Height: 40, Look: tt.look, View: Overview, Policy: claudeLike, Changed: map[string]float64{"work": 0.25}}
			c := newCanvas(f.Width, 4)
			f.routingSection(doc, routedAt).draw(c, bounds{width: f.Width, rows: 4})
			for x := 16; x < 16+words; x++ {
				if got := c.at(x, 0).ink; got.on != tt.want.on || got.bold != tt.want.bold {
					t.Errorf("column %d of work's state is in %+v, want on %+v, bold %t", x, got, tt.want.on, tt.want.bold)
				}
			}
			if after := c.at(16+words, 0).ink; after.on != (hue{}) || after.bold {
				t.Errorf("the cell after work's state is in %+v, want it not cued", after)
			}
			if personal := c.at(16, 1).ink; personal.on != (hue{}) || personal.bold {
				t.Errorf("personal's state is in %+v, want it not cued", personal)
			}
		})
	}
}

func TestEachAccountsRowsInROUTINGAreItsTargets(t *testing.T) {
	doc, _ := autoRouting()
	tests := []struct {
		name  string
		width int
		rows  int
		want  map[string][]int
	}{
		{name: "size 1", width: 160, rows: 4, want: map[string][]int{"work": {10}, "personal": {11}, "side": {12}}},
		{name: "size 2", width: 160, rows: 9, want: map[string][]int{"work": {10, 11}, "personal": {13, 14}, "side": {16, 17}}},
		{name: "its row's parts not fitting a row", width: 82, rows: 8, want: map[string][]int{"work": {10, 11}, "personal": {13, 14}, "side": {16, 17}}},
	}
	for _, tt := range tests {
		f := Frame{Width: tt.width, Height: 40, View: Overview, Policy: claudeLike}
		got := make(map[string][]int)
		for _, target := range f.routingSection(doc, routedAt).targets(bounds{y: 10, width: tt.width, rows: tt.rows}) {
			if target.On != OnAccount || target.X != 0 || target.Width != tt.width {
				t.Errorf("%s: target %+v, want an account's, the frame's width", tt.name, target)
			}
			got[target.Account] = append(got[target.Account], target.Y)
		}
		if !maps.EqualFunc(got, tt.want, slices.Equal) {
			t.Errorf("%s: targets on rows %v, want %v", tt.name, got, tt.want)
		}
	}
}

// ROUTING and SESSIONS take a row an account and a session from the width
// their row's parts' leasts first fit, the name column, the words' 20, the
// sessions and their gaps, the trace's 24 and the rate's tail: 83 columns
// with a name column of 16, 91 with one of 24; a column less, two rows.
func TestARowOfROUTINGsIsOneWhereItsPartsLeastsFit(t *testing.T) {
	tests := []struct {
		names, width int
		oneRow       bool
	}{
		{names: 16, width: 83, oneRow: true},
		{names: 16, width: 82},
		{names: 24, width: 91, oneRow: true},
		{names: 24, width: 90},
	}
	for _, tt := range tests {
		at := routingColumnsOf(tt.width, tt.names)
		if at.oneRow != tt.oneRow {
			t.Errorf("at %d columns, a name column of %d: one row %v, want %v", tt.width, tt.names, at.oneRow, tt.oneRow)
		}
		if tt.oneRow && (at.words != wordsLeast || at.traceCells != traceLeast) {
			t.Errorf("at %d columns, a name column of %d: words %d, trace %d; want their leasts, %d and %d", tt.width, tt.names, at.words, at.traceCells, wordsLeast, traceLeast)
		}
	}
}

// ROUTING takes USAGE's name column, which only narrows where the frame does,
// so its row, once one, stays one: across 52 to 220 columns, with labels of
// 4, 8, 12, 16 and 24 cells, a wider frame never goes from one row an account
// back to two.
func TestROUTINGsRowNeverGoesBackToTwoInAWiderFrame(t *testing.T) {
	for _, cells := range []int{4, 8, 12, 16, 24} {
		doc := threeRouted()
		for i := range doc.Accounts {
			doc.Accounts[i].Label = string(rune('a'+i)) + strings.Repeat("-", cells-1)
		}
		oneRow := false
		for width := 52; width <= 220; width++ {
			f := frameOf(width, 60)
			at := f.routingColumns(f.Moment(doc, now))
			if oneRow && !at.oneRow {
				t.Errorf("labels of %d cells: at %d columns ROUTING is two rows an account, where a column narrower it was one", cells, width)
			}
			oneRow = at.oneRow
		}
	}
}

// USAGE's name column, and so ROUTING's, counts what's configured, never
// what's shown: at every width, it's the same wherever new sessions go, and
// where they go nowhere; with the sample's names, its least deciding it, and
// with a label of 12 cells, its natural.
func TestTheNameColumnIsTheSameWhereverNewSessionsGo(t *testing.T) {
	for _, label := range []string{"", "side-project"} {
		for width := 52; width <= 220; width++ {
			f := frameOf(width, 60)
			var columns []int
			for _, best := range []string{"work", "personal", "side", ""} {
				doc := usageWorld(func(d *status.Document) { d.Best = best }, account("side", func(a *status.Account) { a.Label = label }))
				columns = append(columns, f.overviewBlock(doc, now).names, f.routingColumns(f.Moment(doc, now)).names)
			}
			if slices.Min(columns) != slices.Max(columns) {
				t.Errorf("side labelled %q, at %d columns, the name columns are %v as ▲ moves, want one", label, width, columns)
			}
		}
	}
}

// A row's words and trace take half of what its fixed parts leave each, the
// odd cell the trace's, the words up to their most, the trace the rest: at
// 173 as before, the sessions at 79 and the trace at 92; at 108, the trace
// 35 cells from 65.
func TestARowOfROUTINGsSharesItsRoomBetweenItsWordsAndItsTrace(t *testing.T) {
	tests := []struct {
		width int
		want  routingColumns
	}{
		{width: 173, want: routingColumns{oneRow: true, names: 16, words: 61, sessions: 79, trace: 92, traceCells: 73}},
		{width: 108, want: routingColumns{oneRow: true, names: 16, words: 34, sessions: 52, trace: 65, traceCells: 35}},
		{width: 82, want: routingColumns{names: 16, trace: 16, traceCells: 46}},
	}
	for _, tt := range tests {
		if got := routingColumnsOf(tt.width, 16); got != tt.want {
			t.Errorf("at %d columns: %+v, want %+v", tt.width, got, tt.want)
		}
	}
}

// Wherever a row's one, its words are within their least and most, and its
// trace no less than its least, the two level, the odd cell the trace's,
// but where either is at a least or the words at their most; its sessions
// follow the words wordsGap on, and its trace ends traceTail short of the
// edge.
func TestARowOfROUTINGsWordsKeepWithinTheirLeastAndMost(t *testing.T) {
	for _, names := range []int{labelLeast, 16, labelMost} {
		for width := 52; width <= 400; width++ {
			at := routingColumnsOf(width, names)
			if !at.oneRow {
				continue
			}
			if at.words < wordsLeast || at.words > wordsMost || at.traceCells < traceLeast {
				t.Errorf("at %d columns, a name column of %d: words %d, trace %d; want words %d to %d and a trace of %d or more", width, names, at.words, at.traceCells, wordsLeast, wordsMost, traceLeast)
			}
			if level := at.traceCells - at.words; at.words > traceLeast && at.words < wordsMost && level != 0 && level != 1 {
				t.Errorf("at %d columns, a name column of %d: words %d, trace %d; want them level, the odd cell the trace's", width, names, at.words, at.traceCells)
			}
			if at.sessions != names+at.words+wordsGap || at.trace+at.traceCells != width-traceTail {
				t.Errorf("at %d columns, a name column of %d: %+v; want the sessions %d after the words, the trace ending %d short of the edge", width, names, at, wordsGap, traceTail)
			}
		}
	}
}

// SESSIONS' traces start where ROUTING's do, at every width, so the two
// columns line up; where a row's two, SESSIONS draws none.
func TestSESSIONSTracesStartWhereROUTINGsDo(t *testing.T) {
	doc, sessions, stream := overviewWorld()
	for width := 52; width <= 220; width++ {
		f := frameOf(width, 60)
		f.Sections, f.Sessions, f.Stream = SectionsNamed([]string{"routing", "sessions"}), sessions, stream
		traces := *f.overviewDrawn(doc, routedAt).traces
		want := len(doc.Accounts)
		if f.routingColumns(f.Moment(doc, routedAt)).oneRow {
			want += len(f.running(doc, routedAt))
		}
		if len(traces) != want {
			t.Fatalf("at %d columns, %d traces drawn, want %d", width, len(traces), want)
		}
		for _, tr := range traces {
			if tr.at.x != traces[0].at.x {
				t.Errorf("at %d columns, a trace starts at column %d, want %d, where ROUTING's does", width, tr.at.x, traces[0].at.x)
			}
		}
	}
}
