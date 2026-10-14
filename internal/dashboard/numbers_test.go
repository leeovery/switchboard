package dashboard

import (
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
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
	doc, sessions, stream := overviewWorld()
	f := frameOf(width, height)
	f.Sections = f.Sections.Toggled(SectionSessions)
	f.Sessions, f.Stream = sessions, stream
	return f, doc
}

func TestTheOverviewNamesEachNumberItDrawsByWhatItIs(t *testing.T) {
	byAccount := frameOf(160, 60)
	byAccount.By = ByAccount
	alone := routerDoc("work", 0, readAccount("work", sessionOf(0.6, 2*time.Hour), weekOf(0.4, 3*quota.Day)))
	withSessions, overview := sessionsShown(160, 60)
	sent := []views.Told{{Request: "a", Session: idC61B, Account: "side", Sent: routedAt}, {Request: "b", Session: idC61B, Account: "side", Sent: routedAt}}
	withSessions.Traces = views.Traces{}.Told(sent, routedAt)
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
			name: "SESSIONS' rates, each session's of its account, by its id",
			f:    withSessions, doc: overview, at: routedAt,
			want: map[Number]string{
				{Kind: NumberRate, Account: "side", Of: idC61B}: "2/min", {Kind: NumberRate, Account: "side", Of: idD28C}: "0/min",
				{Kind: NumberRate, Account: "personal", Of: id41E0}: "0/min", {Kind: NumberRate, Account: "side"}: "2/min",
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

// Each kind of number is its own, none the zero kind, so a figure of one
// kind never brightens as another's changes.
func TestNoTwoKindsOfNumberShareAValue(t *testing.T) {
	kinds := map[string]NumberKind{
		"NumberUsed":          NumberUsed,
		"NumberAllowance":     NumberAllowance,
		"NumberRoom":          NumberRoom,
		"NumberSessions":      NumberSessions,
		"NumberRate":          NumberRate,
		"NumberStates":        NumberStates,
		"NumberAccountStates": NumberAccountStates,
		"NumberAllUsed":       NumberAllUsed,
		"NumberAllRoom":       NumberAllRoom,
		"NumberRouted":        NumberRouted,
		"NumberYear":          NumberYear,
		"NumberShareOfUse":    NumberShareOfUse,
		"NumberLimitsHit":     NumberLimitsHit,
		"NumberTokens":        NumberTokens,
		"NumberTokensSplit":   NumberTokensSplit,
		"NumberMoved":         NumberMoved,
		"NumberPeriod":        NumberPeriod,
	}
	named := make(map[NumberKind]string)
	for _, name := range slices.Sorted(maps.Keys(kinds)) {
		kind := kinds[name]
		if kind == 0 {
			t.Errorf("%s is the zero kind, which names no number", name)
		}
		if other, ok := named[kind]; ok {
			t.Errorf("%s and %s are both kind %d", other, name, kind)
		}
		named[kind] = name
	}
}

func TestWhatIsntANumberIsNeverNamed(t *testing.T) {
	tokenless := threeRouted()
	tokenless.Accounts[2].TokenSet = false
	quiet := threeRouted()
	quiet.Sessions = 0
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if named := slices.Collect(maps.Keys(tt.f.Numbers(tt.doc, tt.at))); slices.ContainsFunc(named, tt.none) {
				t.Errorf("named %+v, want none of what isn't a number", slices.DeleteFunc(named, func(n Number) bool { return !tt.none(n) }))
			}
		})
	}
}

// An answer's tokens count up as it streams, as the clock's numbers do: as
// a number, they'd hold the page lit for as long as anything streams.
func TestAnAnswersTokensAreNoNumber(t *testing.T) {
	f, doc := sessionsShown(160, 60)
	if view := strings.Join(f.Draw(doc, routedAt), "\n"); !strings.Contains(view, "↓ ~1.2k") {
		t.Fatalf("SESSIONS is drawn\n%s\nwant c61b's answer counted, ↓ ~1.2k", view)
	}
	for n, drawn := range f.Numbers(doc, routedAt) {
		if strings.Contains(drawn, "↓") {
			t.Errorf("%+v reads %q, want an answer's tokens no number", n, drawn)
		}
	}
}

// The watch compares the page's numbers for the request stream's events only
// where they send a request, which the traces count, or move a seat, which
// re-patches the sessions counted: whatever else the stream tells, as the
// traffic and as the views read it, has to change no number, or it'd change
// unlit. So every field of each but its moves is set here, each call and
// each request doing one thing of each.
func TestTheTrafficChangesNoNumberButByItsMoves(t *testing.T) {
	f, doc := sessionsShown(160, 60)
	plug := func(account, session, model string) Plug {
		return Plug{Account: account, Seat: Seat{Session: session, Model: model}}
	}
	f.Traffic = Traffic{Live: true, Calls: map[Plug]Call{
		plug("side", idC61B, opus):       {Doing: Streaming, Tokens: 1200},
		plug("side", idD28C, haiku):      {Doing: Asking, Since: routedAt, New: true, Pulse: Pulse{Along: 0.5}, Pulsing: true},
		plug("side", id7F3A, sonnet):     {Doing: Answered, Tokens: 3400, Exact: true},
		plug("side", idDB8A, opus):       {Doing: Refused, Status: 429, Pulse: Pulse{Along: 0.5, Back: true, Red: true}, Pulsing: true},
		plug("personal", id9C2E, opus):   {Doing: Throttled, Status: 529, Shimmer: 3},
		plug("personal", id41E0, sonnet): {Doing: Streaming, Tokens: 860},
	}}
	f.Traffic.Seen = map[Plug]time.Time{plug("side", idDB8A, opus): routedAt}
	told := func(request, account, session, model string) views.Told {
		return views.Told{Request: request, Session: session, Model: model, Account: account, Sent: routedAt.Add(-20 * time.Second), Heard: routedAt}
	}
	asking, answered, lost := told("r4", "side", idD28C, haiku), told("r5", "side", id7F3A, sonnet), told("r6", "side", id7F3A, sonnet)
	answered.First, answered.Chars, answered.Ended, answered.Status = routedAt.Add(-19*time.Second), 3400*views.CharsPerToken, routedAt, 200
	lost.Lost = true
	refused, throttled, check := told("r7", "side", idDB8A, opus), told("r8", "personal", id9C2E, opus), told("r9", "work", "", "")
	refused.Verdict, refused.Status = views.VerdictLimited, 429
	throttled.Verdict, throttled.Status = views.VerdictThrottled, 529
	check.Check = true
	for _, r := range []views.Told{asking, answered, lost, refused, throttled, check} {
		f.Stream.Requests[r.Request] = r
	}
	setButItsMoves(t, "traffic", f.Traffic)
	setButItsMoves(t, "stream", f.Stream)
	quiet := f
	quiet.Traffic, quiet.Stream = Traffic{}, views.Stream{}
	if slices.Equal(f.Draw(doc, routedAt), quiet.Draw(doc, routedAt)) {
		t.Fatal("the page is drawn alike with the traffic and without it, want SESSIONS drawing what it tells")
	}
	if changed := changedOf(quiet.Numbers(doc, routedAt), f.Numbers(doc, routedAt)); len(changed) > 0 {
		t.Errorf("the traffic changed %+v, want no number changed", changed)
	}
}

// setButItsMoves fails the test where the stream, as the traffic or as the
// views read it, leaves a field but its Moves unset.
func setButItsMoves(t *testing.T, named string, stream any) {
	t.Helper()
	v := reflect.ValueOf(stream)
	for i := range v.NumField() {
		if name := v.Type().Field(i).Name; name != "Moves" && v.Field(i).IsZero() {
			t.Fatalf("the %s leaves %s unset, want it set, for the test to hold it to changing no number", named, name)
		}
	}
}

// changedOf are the numbers drawn both before, was, and after, is, that
// read differently, as the watch brightens them.
func changedOf(was, is map[Number]string) []Number {
	var changed []Number
	for n, drawn := range is {
		if before, ok := was[n]; ok && before != drawn {
			changed = append(changed, n)
		}
	}
	return changed
}

// The watch compares the page's numbers for a request sent, or a move, only
// where StreamNumbered says the page may draw one they change: every page,
// with any sections shown, grouped either way, is held to it.
func TestOnlyAPageStreamNumberedDrawsANumberTheStreamChanges(t *testing.T) {
	sent := []views.Told{
		{Request: "a", Session: idC61B, Account: "side", Sent: routedAt},
		{Request: "b", Session: id41E0, Account: "personal", Sent: routedAt},
	}
	move := Move{Seat: Seat{Session: idD28C, Model: opus}, From: "side", To: "personal", At: routedAt}
	moved := views.Moved{Session: idD28C, Model: opus, From: "side", To: "personal", At: routedAt}
	usageAlone := Sections{}.Toggled(SectionRouting).Toggled(SectionComing).Toggled(SectionLately)
	shown := []Sections{{}, Sections{}.Toggled(SectionSessions), usageAlone, usageAlone.Toggled(SectionRouting), usageAlone.Toggled(SectionSessions)}
	changing := 0
	for _, view := range Views() {
		for _, sections := range shown {
			for _, by := range []By{ByWindow, ByAccount} {
				quiet, doc := sessionsShown(160, 60)
				quiet.View, quiet.Sections, quiet.By = view, sections, by
				streamed := quiet
				streamed.Traces = views.Traces{}.Told(sent, routedAt)
				streamed.Traffic.Moves, streamed.Stream.Moves = []Move{move}, []views.Moved{moved}
				changed := changedOf(quiet.Numbers(doc, routedAt), streamed.Numbers(doc, routedAt))
				if len(changed) == 0 {
					continue
				}
				changing++
				if !streamed.StreamNumbered() {
					t.Errorf("on %s, %v shown, by %v, the stream changed %+v; want the page StreamNumbered", view, sections.Names(), by, changed)
				}
			}
		}
	}
	if changing == 0 {
		t.Fatal("the stream changed no number on any page, want it changing ROUTING's and SESSIONS'")
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
			shown: Number{Kind: NumberSessions, Account: "side"}, unseen: Number{Kind: NumberRate, Account: "side", Of: idC61B},
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
