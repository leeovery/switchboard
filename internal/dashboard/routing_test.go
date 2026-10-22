package dashboard

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

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
		name string
		doc  status.Document
		want string
	}{
		{
			name: "whole, ending two cells or more before its sessions at 79", doc: capped(false),
			want: textsAt(92, map[int]string{0: "1 work", 16: "● at its cap · back 17:10 · new sessions go elsewhere", 79: "0 sessions"}),
		},
		{
			name: "pinned: its clause left off whole, to end two cells or more before ● pinned at 68", doc: capped(true),
			want: textsAt(92, map[int]string{0: "1 work", 16: "● at its cap", 68: "● pinned", 79: "0 sessions"}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Width: 173, Height: 40, View: Overview, Policy: claudeLike}
			c := newCanvas(f.Width, 4)
			f.routingSection(tt.doc, routedAt).draw(c, bounds{width: f.Width, rows: 4})
			row := []rune(textOf(c.rows(Look{}))[0])
			if got := textsAt(92, map[int]string{0: string(row[:92])}); got != tt.want {
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
		alone bool
		want  line
	}{
		{name: "auto", doc: auto, want: line{{"auto", strongInk}}},
		{
			name: "pinned to an account", doc: pinned("personal"),
			want: slices.Concat(line{{"pinned to personal", alertInk}, {" since 13:40", dimInk}}, back),
		},
		{
			name: "pinned to several", doc: pinned("personal", "side"),
			want: slices.Concat(line{{"pinned to personal and side", alertInk}, {" since 13:40", dimInk}}, back),
		},
		{name: "the mode alone, as by account and on a phone", doc: pinned("personal"), alone: true, want: line{{"pinned to personal", alertInk}}},
		{name: "probed: no mode", doc: probe},
	}
	for _, tt := range tests {
		if got := routingMode(tt.doc.At(routedAt, claudeLike), tt.alone); !slices.Equal(got, tt.want) {
			t.Errorf("%s: %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestROUTINGsHeadingSaysTheModeAloneBesideUSAGEOnAPhoneAndFolded(t *testing.T) {
	doc := pinnedRouting()
	full, alone := routingMode(doc.At(routedAt, claudeLike), false), routingMode(doc.At(routedAt, claudeLike), true)
	tests := []struct {
		name        string
		frame       Frame
		description line
	}{
		{name: "by window", frame: Frame{Width: 160}, description: full},
		{name: "on a phone", frame: Frame{Width: 52}, description: alone},
		{name: "by account, beside USAGE", frame: Frame{Width: 160, By: ByAccount}, description: alone},
		{name: "by account under 150 columns, a section of its own", frame: Frame{Width: 149, By: ByAccount}, description: full},
	}
	for _, tt := range tests {
		s := tt.frame.routingSection(doc, routedAt)
		if got := s.description(); !slices.Equal(got, tt.description) {
			t.Errorf("%s: described as %v, want %v", tt.name, got, tt.description)
		}
		if got := s.folded(); !slices.Equal(got, alone) {
			t.Errorf("%s: folded to %v, want the mode alone, %v", tt.name, got, alone)
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
