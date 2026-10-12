package dashboard

import (
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// The numbers the tests name most.
var (
	workSession = Number{Kind: NumberUsed, Account: "work", Of: "5h"}
	workRate    = Number{Kind: NumberRate, Account: "work"}
	routedCount = Number{Kind: NumberRouted}
)

// sessionsShown is frameOf the size given, SESSIONS shown besides the
// sections shown at first, of overviewWorld's sessions and what the stream
// tells of them.
func sessionsShown(width, height int) (Frame, status.Document) {
	doc, sessions, traffic := overviewWorld()
	f := frameOf(width, height)
	f.Sections = f.Sections.Toggled(SectionSessions)
	f.Sessions, f.Traffic = sessions, traffic
	return f, doc
}

func TestTheOverviewNamesEachNumberItDrawsByWhatItIs(t *testing.T) {
	byAccount := frameOf(160, 60)
	byAccount.By = ByAccount
	alone := routerDoc("work", 0, readAccount("work", sessionOf(0.6, 2*time.Hour), weekOf(0.4, 3*day)))
	withSessions, overview := sessionsShown(160, 60)
	tests := []struct {
		name string
		f    Frame
		doc  status.Document
		at   time.Time
		want map[Number]string
	}{
		{
			name: "USAGE's percentages, its words' allowances and its pool's room; ROUTING's sessions and rates; the status line's sessions",
			f:    frameOf(160, 60), doc: threeRouted(), at: now,
			want: map[Number]string{
				workSession: "58%", {Kind: NumberUsed, Account: "personal", Of: "7d"}: "89%", {Kind: NumberUsed, Of: "5h"}: "57%",
				{Kind: NumberAllowance, Account: "side", Of: "5h"}: "22%/h", {Kind: NumberAllowance, Account: "work", Of: "7d"}: "16%/day",
				{Kind: NumberRoom, Of: "5h"}: "1.3", {Kind: NumberRoom, Of: "7d"}: "1.4",
				{Kind: NumberSessions, Account: "work"}: "0 sessions", workRate: "0/min",
				routedCount: "5 sessions",
			},
		},
		{
			name: "by account, the pool's states counted, its sessions and its rate beside its block",
			f:    byAccount, doc: threeRouted(), at: now,
			want: map[Number]string{
				{Kind: NumberStates, Of: "open"}: "1 open", {Kind: NumberStates, Of: "pressure"}: "1 pressure", {Kind: NumberStates, Of: "limit"}: "1 limit",
				{Kind: NumberSessions}: "0 sessions", {Kind: NumberRate}: "0/min", {Kind: NumberSessions, Account: "side"}: "0 sessions",
			},
		},
		{
			name: "a lone account's room, as USAGE's description says it",
			f:    frameOf(160, 60), doc: alone, at: now,
			want: map[Number]string{{Kind: NumberRoom, Account: "work", Of: "5h"}: "0.4", {Kind: NumberRoom, Account: "work", Of: "7d"}: "0.6"},
		},
		{
			name: "under 110 columns, ROUTING's sessions and rate on its second row",
			f:    frameOf(100, 60), doc: threeRouted(), at: now,
			want: map[Number]string{{Kind: NumberSessions, Account: "work"}: "0 sessions", workRate: "0/min"},
		},
		{
			name: "SESSIONS' tokens, as the stream counts them",
			f:    withSessions, doc: overview, at: routedAt,
			want: map[Number]string{
				{Kind: NumberTokens, Account: "side", Of: idC61B}: "↓ ~1.2k", {Kind: NumberTokens, Account: "personal", Of: id41E0}: "↓ ~860",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.f.Numbers(tt.doc, tt.at)
			for n, want := range tt.want {
				if got[n] != want {
					t.Errorf("%+v reads %q, want %q", n, got[n], want)
				}
			}
		})
	}
}

func TestWhatIsntANumberIsNeverNamed(t *testing.T) {
	tokenless := threeRouted()
	tokenless.Accounts[2].TokenSet = false
	quiet := threeRouted()
	quiet.Sessions = 0
	withSessions, overview := sessionsShown(160, 60)
	tests := []struct {
		name string
		f    Frame
		doc  status.Document
		at   time.Time
		none func(Number) bool
	}{
		{
			name: "an empty bar's percentage, —", f: frameOf(160, 60), doc: tokenless, at: now,
			none: func(n Number) bool { return n.Kind == NumberUsed && n.Account == "side" },
		},
		{
			name: "the status line's sessions, where it has none", f: frameOf(160, 60), doc: quiet, at: now,
			none: func(n Number) bool { return n.Kind == NumberRouted },
		},
		{
			name: "what a session asking is doing, ↑ ask", f: withSessions, doc: overview, at: routedAt,
			none: func(n Number) bool { return n.Kind == NumberTokens && n.Of == idD28C },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if named := slices.Collect(maps.Keys(tt.f.Numbers(tt.doc, tt.at))); slices.ContainsFunc(named, tt.none) {
				t.Errorf("named %+v, want none of what isn't a number", slices.DeleteFunc(named, func(n Number) bool { return !tt.none(n) }))
			}
		})
	}
}

func TestAnsweringWithNothingCountedIsNoNumber(t *testing.T) {
	plug := Plug{Account: "side", Seat: Seat{Session: idD28C, Model: opus}}
	f := Frame{Traffic: Traffic{Live: true, Calls: map[Plug]Call{plug: {Doing: Streaming}}}}
	c := newCanvas(10, 1)
	c.line(0, 0, line{f.runningDoing(views.Running{Account: "side", Session: idD28C, Models: []string{opus}})})
	if got := c.numbers(); len(got) > 0 {
		t.Errorf("↓ alone is drawn as %v, want no number", got)
	}
}

func TestWhatTheClockMovesIsNoNumber(t *testing.T) {
	f, doc := frameOf(160, 60), threeRouted()
	later := now.Add(time.Minute)
	if slices.Equal(f.Draw(doc, now), f.Draw(doc, later)) {
		t.Fatal("the page drawn a minute on is as it was, want its clock and countdowns moved on")
	}
	if was, is := f.Numbers(doc, now), f.Numbers(doc, later); !maps.Equal(was, is) {
		t.Errorf("a minute on, the numbers are\n%v\nwant them as they were\n%v", is, was)
	}
}

func TestOnlyTheNumbersOnScreenAreNamed(t *testing.T) {
	ten := tenRouting()
	scrolled := frameOf(160, 30)
	scrolled.Sections = Sections{}.Toggled(SectionRouting).Toggled(SectionComing).Toggled(SectionLately)
	folded, overview := sessionsShown(160, 30)
	tests := []struct {
		name          string
		f             Frame
		doc           status.Document
		at            time.Time
		shown, unseen Number
	}{
		{
			name: "USAGE scrolled: an account below what fits",
			f:    scrolled, doc: ten, at: routedAt,
			shown: Number{Kind: NumberUsed, Account: "work", Of: "5h"}, unseen: Number{Kind: NumberUsed, Account: "studio", Of: "5h"},
		},
		{
			name: "SESSIONS folded to its heading",
			f:    folded, doc: overview, at: routedAt,
			shown: Number{Kind: NumberSessions, Account: "side"}, unseen: Number{Kind: NumberTokens, Account: "side", Of: idC61B},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.f.Numbers(tt.doc, tt.at)
			if _, ok := got[tt.shown]; !ok {
				t.Errorf("%+v isn't named, want it, on screen", tt.shown)
			}
			if _, ok := got[tt.unseen]; ok {
				t.Errorf("%+v is named, want it not, off screen", tt.unseen)
			}
		})
	}
}

func TestANumberBrightensItsCellsAloneAsFarAsItsBrightened(t *testing.T) {
	brightened := func(look Look) *canvas {
		f := frameOf(160, 60)
		f.Look = look
		f.Brightened = map[Number]float64{workRate: 0.5, workSession: 0}
		c := f.page(threeRouted(), now)
		c.brighten(f.Brightened, f.Look)
		return c
	}
	tests := []struct {
		name string
		look Look
		lit  func(ink) bool
	}{
		{name: "mixed halfway toward text.primary", look: Screen(builtin(t, "nord")), lit: func(k ink) bool { return k.glow == 0.5 && !k.bold }},
		{name: "without colour, bold", look: NoColour(), lit: func(k ink) bool { return k.bold && k.glow == 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plain := frameOf(160, 60)
			plain.Look = tt.look
			unlit, c := plain.page(threeRouted(), now), brightened(tt.look)
			lit := 0
			for y, row := range c.cells {
				for x, here := range row {
					switch n, _ := here.numbered(); {
					case n == workRate && !tt.lit(here.ink):
						t.Errorf("work's rate's cell %d of row %d is in %+v, want it brightened", x, y, here.ink)
					case n == workRate:
						lit++
					case here.ink != unlit.cells[y][x].ink:
						t.Errorf("cell %d of row %d, of %+v, is in %+v, want it as it was, %+v", x, y, n, here.ink, unlit.cells[y][x].ink)
					}
				}
			}
			if lit != len("0/min") {
				t.Errorf("%d of work's rate's cells are brightened, want all %d", lit, len("0/min"))
			}
		})
	}
}

func TestANumberBrighteningOnScreenMovesTheFrame(t *testing.T) {
	ten := tenRouting()
	scrolled := frameOf(160, 30)
	scrolled.Sections = Sections{}.Toggled(SectionRouting).Toggled(SectionComing).Toggled(SectionLately)
	tests := []struct {
		name       string
		brightened map[Number]float64
		want       bool
	}{
		{name: "none brightening"},
		{name: "a number on screen brightening", brightened: map[Number]float64{workSession: 0.5}, want: true},
		{name: "a number scrolled out of view brightening", brightened: map[Number]float64{{Kind: NumberUsed, Account: "studio", Of: "5h"}: 0.5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := scrolled
			f.Brightened = tt.brightened
			if got := f.Motion(ten, routedAt); got.Smooth != tt.want {
				t.Errorf("Motion is %+v, want Smooth %t", got, tt.want)
			}
		})
	}
}
