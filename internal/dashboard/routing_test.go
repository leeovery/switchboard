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

// badHourRouting is the world of a bad hour: new sessions pinned to personal
// and side since 13:40; personal at its limit till 15:54; work's token
// refused since 14:41, nothing read of it since 14:20; and as the document
// counts them, six sessions on side.
func badHourRouting() status.Document {
	doc, _ := routedWorld(true)
	doc.Accounts[1].Primary = true
	doc.Accounts[0].Refused = status.Refusal{Until: routedClock(1, 14, 51), Status: 401}
	doc.Accounts[0].FetchedAt = routedClock(1, 14, 20)
	doc.Pin = status.Pin{Accounts: []string{"personal", "side"}, Since: routedClock(1, 13, 40)}
	doc.Accounts[2].Sessions = 6
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
	for id, n := range map[string]int{"personal": 2, "side": 4, "client": 3, "lab": 2, "extra": 1, "studio": 2} {
		doc.Accounts[docPlace(doc, id)-1].Sessions = n
	}
	return doc.WorkedOut(claudeLike)
}

// routingDrawn are the cells ROUTING of doc at routedAt draws in f, as a
// terminal shows them, its body from row y, rows tall.
func routingDrawn(t *testing.T, f Frame, doc status.Document, y, rows int) [][]seen {
	t.Helper()
	c := newCanvas(f.Width, f.Height)
	f.routingSection(doc, routedAt).draw(c, bounds{y: y, width: f.Width, rows: rows})
	return screenOf(t, c.rows(f.Look))
}

// spanning are the spots of the cells from column first to last on each row
// given.
func spanning(first, last int, rows ...int) []spot {
	spots := make([]spot, len(rows))
	for i, row := range rows {
		spots[i] = spot{row: row, first: first, last: last}
	}
	return spots
}

func TestROUTINGIsDrawnAsItsFramesAre(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	auto, listed := autoRouting()
	tests := []struct {
		stem     string
		doc      status.Document
		sessions []status.Session
		width    int
		y, rows  int
		// traces are the cells of the traces and their rates, which draw
		// the frames' own data, held by their tests; and names those of
		// the names of accounts out of the pool, which USAGE fades.
		traces, names []spot
	}{
		{stem: "overview-default-160", doc: auto, sessions: listed, width: 160, y: 20, rows: 9, traces: spanning(92, 159, 20, 21, 23, 24, 26, 27)},
		{
			stem: "overview-pinned-160", doc: pinnedRouting(), width: 160, y: 20, rows: 9,
			traces: spanning(92, 159, 20, 21, 23, 24, 26, 27), names: spanning(0, 15, 20, 26),
		},
		{
			stem: "overview-bad-hour-160", doc: badHourRouting(), width: 160, y: 20, rows: 9,
			traces: spanning(92, 159, 20, 21, 23, 24, 26, 27), names: spanning(0, 15, 20),
		},
		{stem: "overview-everything-160x39", doc: auto, sessions: listed, width: 160, y: 30, rows: 4, traces: spanning(92, 159, 30, 31, 32)},
		{stem: "overview-ten-accounts-160", doc: tenRouting(), width: 160, y: 34, rows: 11, traces: spanning(92, 159, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43)},
		{stem: "overview-phone-52", doc: auto, sessions: listed, width: 52, y: 44, rows: 8, traces: spanning(16, 51, 45, 48, 51)},
	}
	for _, tt := range tests {
		t.Run(tt.stem, func(t *testing.T) {
			f := Frame{Width: tt.width, Height: 66, Look: nord, View: Overview, Policy: claudeLike, Sessions: tt.sessions}
			in := region{top: tt.y, bottom: tt.y + tt.rows - 1, left: 0, right: tt.width - 1}
			heldTo(t, tt.stem, reference(t, tt.stem), routingDrawn(t, f, tt.doc, tt.y, tt.rows), in, shadeOf(nord.Canvas()), tt.traces, tt.names)
		})
	}
}

func TestROUTINGBesideUSAGEsBlocksIsDrawnAsItsFrameIs(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	doc, sessions := autoRouting()
	f := Frame{Width: 160, Height: 42, Look: nord, View: Overview, Policy: claudeLike, Sessions: sessions, By: ByAccount}
	c := newCanvas(f.Width, f.Height)
	f.routingBeside(c, f.routingTableOf(doc, routedAt), blocksBeside{x: 107, blocks: []rowAt{{"work", 6, 5}, {"personal", 13, 5}, {"side", 20, 5}, {"", 27, 5}}})

	// The frame draws work's state whole to the edge, where the design cuts
	// its words by whole clauses to end two cells short of it.
	whole := append(spanning(107, 159, 10, 17, 24, 31), spot{row: 6, first: 132, last: 159})
	frame, drawn := reference(t, "overview-by-account-160"), screenOf(t, c.rows(nord))
	// Row 26 is the pool's rule, which USAGE's blocks draw.
	for _, in := range []region{{top: 6, bottom: 25, left: 107, right: 159}, {top: 27, bottom: 31, left: 107, right: 159}} {
		heldTo(t, "overview-by-account-160", frame, drawn, in, shadeOf(nord.Canvas()), whole, nil)
	}
}

func TestROUTINGsFormsLayItsRowsOut(t *testing.T) {
	doc, sessions := autoRouting()
	lane := func(n int) string { return strings.Repeat(traceLane, n) }
	// From 110 columns, at 120, a trace is 20 cells, from column 92, and its
	// rate ends at the last column, the caption cut to it; at 109, it's from
	// the name column to 89.
	wide := func(name, state, sessions string) map[int]string {
		return map[int]string{0: name, 16: state, sessionsAt: sessions}
	}
	traced := func(texts map[int]string) string {
		texts[traceFrom], texts[115] = lane(20), "0/min"
		return textsAt(120, texts)
	}
	work, personal, side := "● at its cap · back 17:10 · new sessions go elsewhere", "● open", "● open · new sessions come here"
	caption := textsAt(120, map[int]string{traceFrom: "requests a minute, last 30…"})
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
			name: "under 110 columns: two rows an account and a blank row, its short state, its trace to 20 cells short of the edge", width: 109, rows: 8,
			want: []string{
				" 1 work         ● at its cap · 17:10", textsAt(109, map[int]string{16: lane(73), 92: "0 sessions  0/min"}), "",
				" 2 personal ◆   ● open", textsAt(109, map[int]string{16: lane(73), 92: "2 sessions  0/min"}), "",
				" 3 side ▲       ● open · next", textsAt(109, map[int]string{16: lane(73), 92: "4 sessions  0/min"}),
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
			want: textsAt(traceFrom, map[int]string{0: "1 work", 16: "● at its cap · back 17:10 · new sessions go elsewhere", sessionsAt: "0 sessions"}),
		},
		{
			name: "pinned: its clause left off whole, to end two cells or more before ● pinned at 68", doc: capped(true),
			want: textsAt(traceFrom, map[int]string{0: "1 work", 16: "● at its cap", pinnedAt: "● pinned", sessionsAt: "0 sessions"}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Width: 160, Height: 40, View: Overview, Policy: claudeLike}
			c := newCanvas(f.Width, 4)
			f.routingSection(tt.doc, routedAt).draw(c, bounds{width: f.Width, rows: 4})
			row := []rune(textOf(c.rows(Look{}))[0])
			if got := textsAt(traceFrom, map[int]string{0: string(row[:traceFrom])}); got != tt.want {
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
		{name: "under 110 columns", frame: Frame{Width: 109}, least: 10, most: 10},
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
	if _, lane := traceInks(true); c.at(traceFrom, 1).ink != lane {
		t.Errorf("work's empty trace is in %+v, want %+v, faded", c.at(traceFrom, 1).ink, lane)
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
			if got := countedStates(status.CountStates(tt.accounts), tt.width); !slices.Equal(got, tt.want) {
				t.Errorf("countedStates() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestROUTINGsLinesBesideABlockAreTwoRowsApartWhereItHasTheRows(t *testing.T) {
	tests := []struct {
		windows int
		want    []int
	}{
		{windows: 4, want: []int{0, 2, 4}},
		{windows: 3, want: []int{0, 2, 4}},
		{windows: 2, want: []int{0, 1, 2}},
		{windows: 1, want: []int{0, 1}},
	}
	for _, tt := range tests {
		if got := besideRows(2*tt.windows - 1); !slices.Equal(got, tt.want) {
			t.Errorf("beside a block of %d windows: rows %v down, want %v", tt.windows, got, tt.want)
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
		{name: "under 110 columns", width: 109, rows: 8, want: map[string][]int{"work": {10, 11}, "personal": {13, 14}, "side": {16, 17}}},
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
