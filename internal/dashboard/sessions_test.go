package dashboard

import (
	"cmp"
	"fmt"
	"maps"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// switchboard is a frame of the Sessions view of a terminal the size given,
// as text alone, of the sessions given, the request stream telling of their
// requests as traffic has it.
func switchboard(width, height int, sessions []status.Session, traffic Traffic) Frame {
	f := frameOf(width, height)
	f.View, f.Sessions, f.Traffic = Sessions, sessions, traffic
	return f
}

// bayOfFrame is the switchboard the frame f lays doc out as, under its title
// and heading, failing the test where it lays it out as a plain list.
func bayOfFrame(t *testing.T, f Frame, doc status.Document) bay {
	t.Helper()
	b, ok := f.bayOf(doc, now, f.above(newCanvas(f.Width, f.Height), doc, now))
	if !ok {
		t.Fatalf("at %d×%d, the view is a plain list, want a switchboard", f.Width, f.Height)
	}
	return b
}

// drawnBay is the switchboard the frame f lays doc out as, drawn on a canvas
// of its own, from its top.
func drawnBay(t *testing.T, f Frame, doc status.Document) (bay, *canvas) {
	t.Helper()
	b := bayOfFrame(t, f, doc)
	c := newCanvas(f.Width, b.content)
	f.drawBay(c, doc, b, now)
	return b, c
}

// rowOf is row y of the canvas as text, its trailing blanks trimmed.
func rowOf(c *canvas, y int) string {
	return strings.TrimRight(c.row(y, Look{}), " ")
}

// callAt is the call of the bay with the session's id shown on the account
// with the given id.
func callAt(t *testing.T, b bay, shown, account string) callRow {
	t.Helper()
	i := slices.IndexFunc(b.calls, func(r callRow) bool {
		return sessionID(r.session.ID) == shown && r.assignment.Account == account
	})
	if i < 0 {
		t.Fatalf("there's no call of %s on %s", shown, account)
	}
	return b.calls[i]
}

// cordOf is the bay's cord from the call of the session's id shown on the
// account with the given id.
func cordOf(t *testing.T, b bay, shown, account string) cord {
	t.Helper()
	i := slices.IndexFunc(b.cords, func(k cord) bool {
		return sessionID(k.plug.Seat.Session) == shown && k.plug.Account == account
	})
	if i < 0 {
		t.Fatalf("there's no cord of %s to %s", shown, account)
	}
	return b.cords[i]
}

func TestTheCallsAreGroupedByTheLineTheyreOn(t *testing.T) {
	b := bayOfFrame(t, switchboard(160, 40, flippingSessions(), Traffic{}), threeRouted())

	type placed struct {
		row  int
		call string
		jack int
	}
	var got []placed
	for _, r := range b.calls {
		got = append(got, placed{row: r.row, call: sessionID(r.session.ID) + " " + r.assignment.Name() + " on " + r.assignment.Account, jack: r.jack})
	}
	// work's, the one put there last first, db8a's sonnet and d28c's at once,
	// as the router lists them; then, a blank row on, side's, db8a's opus,
	// split from its sonnet, among them; personal, with none, takes no rows.
	want := []placed{
		{row: 1, call: "db8a sonnet on work", jack: 0},
		{row: 2, call: "d28c opus on work", jack: 1},
		{row: 3, call: "7f3a haiku on work", jack: 2},
		{row: 5, call: "c61b opus on side", jack: 0},
		{row: 6, call: "db8a opus on side", jack: 1},
		{row: 7, call: "41e0 sonnet on side", jack: 2},
	}
	if !slices.Equal(got, want) {
		t.Errorf("the calls are\n%+v\nwant\n%+v", got, want)
	}
}

func TestACallKeepsItsRowHoweverTheRouterListsTheSessions(t *testing.T) {
	sessions := []status.Session{
		{ID: id7F3A, Assignments: []status.Assignment{seatOf(haiku, "work", "new", 2*time.Hour, 5*time.Second)}},
		{ID: idD28C, Assignments: []status.Assignment{seatOf(opus, "work", "new", time.Hour, time.Minute)}},
		{ID: idDB8A, Assignments: []status.Assignment{seatOf(sonnet, "work", "new", 30*time.Minute, 9*time.Minute)}},
	}
	turned := slices.Clone(sessions)
	slices.Reverse(turned)
	order := func(listed []status.Session) []string {
		var ids []string
		for _, r := range bayOfFrame(t, switchboard(160, 40, listed, Traffic{}), threeRouted()).calls {
			ids = append(ids, sessionID(r.session.ID))
		}
		return ids
	}

	want := []string{"db8a", "d28c", "7f3a"}
	for _, listed := range [][]status.Session{sessions, turned} {
		if got := order(listed); !slices.Equal(got, want) {
			t.Errorf("listed %v, the calls run %v, want %v, the one put there last first", order(listed), got, want)
		}
	}
}

func TestTheCallsKeepTheOrderTheWatchKeeps(t *testing.T) {
	f := switchboard(160, 40, flippingSessions(), Traffic{})
	f.Order = Order{"work": {{idD28C, opus}, {"5b19aa00", opus}, {id7F3A, haiku}}}
	b := bayOfFrame(t, f, threeRouted())

	var got []string
	for _, r := range b.calls {
		if r.assignment.Account == "work" {
			got = append(got, fmt.Sprintf("%s on row %d", sessionID(r.session.ID), r.row))
		}
	}
	// d28c and 7f3a in the order kept, closing up over 5b19, which the router
	// no longer lists; then db8a's sonnet, which the order doesn't place, at
	// the foot.
	if want := []string{"d28c on row 1", "7f3a on row 2", "db8a on row 3"}; !slices.Equal(got, want) {
		t.Errorf("work's calls are %q, want %q", got, want)
	}
}

func TestTheOrderKeptIsTheCallsAsDrawn(t *testing.T) {
	move := Move{Seat: Seat{Session: idD28C, Model: opus}, From: "work", To: "side", At: now}
	f := switchboard(160, 40, flippingSessions(), Traffic{Moves: []Move{move}})

	want := Order{
		"work": {{idDB8A, sonnet}, {idD28C, opus}, {id7F3A, haiku}},
		"side": {{idC61B, opus}, {idDB8A, opus}, {id41E0, sonnet}, {idD28C, opus}},
	}
	if got := f.OrderOf(threeRouted()); !maps.EqualFunc(got, want, slices.Equal) {
		t.Errorf("the order kept is %+v, want %+v: d28c's placeholder on its row, which it keeps till a listing shows its move done, and d28c on side, where it joined the foot", got, want)
	}
}

func TestAPlaceholderKeepsItsRowTillAListingShowsItsMoveDone(t *testing.T) {
	move := Move{Seat: Seat{Session: idD28C, Model: opus}, From: "work", To: "side", At: now}
	drawn := switchboard(160, 40, flippingSessions(), Traffic{Moves: []Move{move}})
	work := func(f Frame) []string {
		var calls []string
		for _, r := range bayOfFrame(t, f, threeRouted()).calls {
			if r.assignment.Account == "work" {
				calls = append(calls, fmt.Sprintf("%s on row %d, gone %v", sessionID(r.session.ID), r.row, r.gone))
			}
		}
		return calls
	}

	// The next look lists d28c on work still, the router listing it before
	// it moved, and turns over the order it lists the sessions in.
	next := drawn
	next.Order = drawn.OrderOf(threeRouted())
	next.Sessions = slices.Clone(flippingSessions())
	slices.Reverse(next.Sessions)
	if got, want := work(next), work(drawn); !slices.Equal(got, want) {
		t.Errorf("work's calls are %q, want %q, d28c's placeholder on its row", got, want)
	}
}

func TestAMoveBringsOnTheSeatOfASessionNotListedItsModelNamed(t *testing.T) {
	move := Move{Seat: Seat{Session: "5b19aa00", Model: opus}, From: "work", To: "side", At: now}
	b, c := drawnBay(t, switchboard(160, 40, flippingSessions(), Traffic{Moves: []Move{move}}), threeRouted())

	if got := rowOf(c, callAt(t, b, "5b19", "side").row); !strings.HasPrefix(got, "  5b19  opus") {
		t.Errorf("its row reads %q, want its model named as the router names it for the sessions it lists", got)
	}
}

func TestSessionsIsASwitchboardWhereItsCordsFit(t *testing.T) {
	tests := []struct {
		name  string
		frame Frame
		doc   status.Document
		want  bool
	}{
		{name: "three accounts", frame: switchboard(160, 40, flippingSessions(), Traffic{}), doc: threeRouted(), want: true},
		{name: "one account: a plain list", frame: switchboard(160, 40, flippingSessions(), Traffic{}), doc: routerDoc("work", 3, pressedAccount("work"))},
		{name: "under cordsFrom columns: a plain list", frame: switchboard(cordsFrom-1, 40, flippingSessions(), Traffic{}), doc: threeRouted()},
	}
	for _, tt := range tests {
		f := tt.frame
		if _, got := f.bayOf(tt.doc, now, f.above(newCanvas(f.Width, f.Height), tt.doc, now)); got != tt.want {
			t.Errorf("%s: laid out as a switchboard %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestACallsIDIsBrightAndBoldWhileItsBusy(t *testing.T) {
	b, c := drawnBay(t, switchboard(160, 40, flippingSessions(), Traffic{}), threeRouted())

	tests := []struct {
		call string
		want ink
	}{
		{call: "d28c", want: titleInk},
		{call: "7f3a", want: ink{token: theme.TextMuted}},
	}
	for _, tt := range tests {
		r := callAt(t, b, tt.call, "work")
		for x := callsAt; x < callsAt+len(tt.call); x++ {
			if got := c.cells[r.row][x].ink; got != tt.want {
				t.Errorf("%s's id, at %d, is %+v, want %+v", tt.call, x, got, tt.want)
				break
			}
		}
	}
}

func TestEachLineHasAPanelAJackAWindowGrowingARowForEachCallPastThem(t *testing.T) {
	sessions := flippingSessions()
	for _, id := range []string{"aaaa1111", "bbbb2222", "cccc3333"} {
		sessions = append(sessions, status.Session{ID: id, Assignments: []status.Assignment{seatOf(opus, "side", "new", time.Hour, time.Hour)}})
	}
	b := bayOfFrame(t, switchboard(160, 50, sessions, Traffic{}), threeRouted())

	type panelled struct {
		account   string
		top, rows int
	}
	var got []panelled
	for _, p := range b.panels {
		got = append(got, panelled{account: p.account.ID, top: p.top, rows: p.rows})
	}
	// Two windows shown each, Fable's week used by none: work's three calls
	// grow it a row, and side's six it four; personal has none.
	want := []panelled{{account: "work", top: 0, rows: 3}, {account: "personal", top: 6, rows: 2}, {account: "side", top: 11, rows: 6}}
	if !slices.Equal(got, want) {
		t.Errorf("the panels are %+v, want %+v", got, want)
	}
	for _, k := range b.cords {
		p := b.panels[k.place-1]
		if k.to <= p.top || k.to > p.top+p.rows {
			t.Errorf("the cord of %s ends on row %d, outside its panel's rows %d to %d", k.plug.Seat.Shown(), k.to, p.top+1, p.top+p.rows)
		}
	}
}

func TestCordsRunRightThenDownBendingSoNoneCross(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 21))
	const trials = 400
	switchboards := 0
	for trial := range trials {
		doc, sessions, traffic := arrangement(r)
		width := 90 + r.IntN(130)
		f := switchboard(width, 20+r.IntN(40), sessions, traffic)
		top := f.above(newCanvas(f.Width, f.Height), doc, now)
		b, ok := f.bayOf(doc, now, top)
		if !ok {
			continue
		}
		switchboards++
		name := fmt.Sprintf("trial %d, %d accounts, %d calls, %d wide", trial, len(doc.Accounts), len(b.calls), width)
		cordsHoldTheirRule(t, name, b)
	}
	if switchboards < trials*3/4 {
		t.Errorf("%d of %d arrangements were laid out as switchboards, want most", switchboards, trials)
	}
}

// cordsHoldTheirRule checks the bay's cords against the rule they're drawn
// by: each runs right, then down, to a jack of its own; those that bend do so
// between the plugs and the cords hanging loose from the jacks, the one that
// starts higher further right; and no two share a cell, so none crosses
// another.
func cordsHoldTheirRule(t *testing.T, name string, b bay) {
	t.Helper()
	taken := make(map[point]string)
	jacks := make(map[int]bool)
	lastBend := b.x
	for _, k := range b.cords {
		who := k.plug.Seat.Shown() + " on " + k.plug.Account
		switch {
		case k.to < k.from:
			t.Fatalf("%s: the cord of %s runs up, from row %d to %d", name, who, k.from, k.to)
		case jacks[k.to]:
			t.Fatalf("%s: two cords end at the jack on row %d", name, k.to)
		case k.to != k.from && (k.bend <= plugAt || k.bend > b.x-looseCells-bendClear):
			t.Fatalf("%s: the cord of %s bends at %d, outside %d to %d", name, who, k.bend, plugAt+1, b.x-looseCells-bendClear)
		case k.to != k.from && k.bend >= lastBend:
			t.Fatalf("%s: the cord of %s bends at %d, not left of the one above it, at %d", name, who, k.bend, lastBend)
		}
		jacks[k.to] = true
		if k.to != k.from {
			lastBend = k.bend
		}
		for _, p := range k.path(b.x) {
			if other, ok := taken[p]; ok {
				t.Fatalf("%s: the cords of %s and %s cross at %d along row %d", name, other, who, p.x, p.y)
			}
			taken[p] = who
		}
	}
}

// arrangement is a router's document of between two and seven accounts, the
// sessions it lists, each on between none and five of them, some split
// across two, and the moves the request stream tells of, re-patching some,
// drawn from r.
func arrangement(r *rand.Rand) (status.Document, []status.Session, Traffic) {
	n := 2 + r.IntN(6)
	accounts := make([]status.Account, n)
	for i := range accounts {
		accounts[i] = readAccount(fmt.Sprintf("account-%d", i), sessionOf(0.2, 3*time.Hour), weekOf(0.3, 4*day))
		if r.IntN(3) == 0 {
			accounts[i].Windows = append(accounts[i].Windows, fableOf(0.1, 4*day))
		}
	}
	var sessions []status.Session
	for i := range accounts {
		for range r.IntN(6) {
			s := status.Session{ID: fmt.Sprintf("%08x", r.Uint32())}
			made := time.Duration(r.IntN(120)) * time.Minute
			s.Assignments = append(s.Assignments, seatOf(opus, accounts[i].ID, "new", made, time.Duration(r.IntN(20))*time.Minute))
			if r.IntN(4) == 0 {
				s.Assignments = append(s.Assignments, seatOf(sonnet, accounts[r.IntN(n)].ID, "new", made, time.Minute))
			}
			sessions = append(sessions, s)
		}
	}
	var traffic Traffic
	for _, s := range sessions {
		if a := s.Assignments[0]; r.IntN(5) == 0 {
			to := accounts[r.IntN(n)].ID
			traffic.Moves = append(traffic.Moves, Move{Seat: Seat{Session: s.ID, Model: a.Model}, From: a.Account, To: to, At: now})
		}
	}
	return routerDoc(accounts[0].ID, len(sessions), accounts...), sessions, traffic
}

func TestACordThatStartsHigherBendsFurtherRight(t *testing.T) {
	sessions := func(n int) []status.Session {
		var listed []status.Session
		for i := range n {
			listed = append(listed, status.Session{ID: fmt.Sprintf("%04x0000", 0x1000+i), Assignments: []status.Assignment{seatOf(opus, "side", "new", time.Duration(n-i)*time.Minute, time.Minute)}})
		}
		return listed
	}
	tests := []struct {
		name  string
		width int
		calls int
		// want are the columns the cords bend at, from the one that starts
		// highest, or nil where they don't fit, and the view is a plain list.
		want []int
	}{
		{name: "the frames' three, bendsApart apart, from 12 short of the lines", width: 160, calls: 3, want: []int{84, 79, 74}},
		{name: "too many to be bendsApart apart: closer", width: 110, calls: 5, want: []int{34, 33, 32, 31, 30}},
		{name: "too many to bend a cell apart beside the bars' lines: beside the narrower lines without them", width: 110, calls: 6, want: []int{58, 53, 48, 43, 38, 33}},
		{name: "too many to bend a cell apart even then: a plain list", width: 110, calls: 40},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := switchboard(tt.width, 60, sessions(tt.calls), Traffic{})
			doc := threeRouted()
			b, ok := f.bayOf(doc, now, f.above(newCanvas(f.Width, f.Height), doc, now))
			if tt.want == nil {
				if ok {
					t.Errorf("laid out as a switchboard, bending at %v, want a plain list", bends(b))
				}
				return
			}
			if got := bends(b); !ok || !slices.Equal(got, tt.want) {
				t.Errorf("the cords bend at %v, want %v", got, tt.want)
			}
		})
	}
}

// bends are the columns the bay's bent cords bend at, from the one that
// starts highest.
func bends(b bay) []int {
	var at []int
	for _, k := range b.cords {
		if k.to != k.from {
			at = append(at, k.bend)
		}
	}
	return at
}

func TestACordIsItsLinesColourDimmedWhileItsCallIsIdle(t *testing.T) {
	f := switchboard(160, 40, flippingSessions(), Traffic{})
	f.Look = Screen(builtin(t, "nord"))
	b, c := drawnBay(t, f, threeRouted())

	tests := []struct {
		call, account string
		want          ink
	}{
		{call: "d28c", account: "work", want: ink{token: theme.VizSeries1}},
		{call: "7f3a", account: "work", want: ink{token: theme.VizSeries1, fade: idleFade}},
		{call: "c61b", account: "side", want: ink{token: theme.VizSeries3}},
		{call: "41e0", account: "side", want: ink{token: theme.VizSeries3, fade: idleFade}},
	}
	for _, tt := range tests {
		k := cordOf(t, b, tt.call, tt.account)
		for i, p := range k.path(b.x) {
			if got := c.cells[p.y][p.x].ink; got != tt.want {
				t.Errorf("the cord of %s, at its cell %d, is %+v, want %+v", tt.call, i, got, tt.want)
				break
			}
		}
	}
}

func TestAnIdleCallsCordIsDrawnLightWhereTheLookCantDimIt(t *testing.T) {
	f := switchboard(160, 40, flippingSessions(), Traffic{})
	f.Look = NoColour()
	b, c := drawnBay(t, f, threeRouted())

	if got := rowOf(c, callAt(t, b, "7f3a", "work").row); !strings.Contains(got, "●──────") {
		t.Errorf("7f3a, idle, reads %q, want its cord light", got)
	}
	if got := rowOf(c, callAt(t, b, "d28c", "work").row); !strings.Contains(got, "●━━━━━━") {
		t.Errorf("d28c, busy, reads %q, want its cord heavy", got)
	}
}

func TestTheLinesPanels(t *testing.T) {
	f := switchboard(160, 40, flippingSessions(), Traffic{})
	doc := threeRouted()
	doc.Accounts[0].Primary = true
	_, c := drawnBay(t, f, doc)

	// Its place and name, in capitals, and its state, as its card leads with
	// it, its badges at the right; a row each window, its bar, its use and
	// where it's heading, and a third grown for its third call; and when its
	// windows reset.
	want := []string{
		"┌─ 1 · WORK ─ ● under pressure ──────────────────── ◆ primary ─┐",
		"◉  Session  █████████┃███▉▒▒▒▒▒▒▒▒▒▒  58%  → out ~14:42        │",
		"◉  Week     ████████▏▒┃▒▒▒▒▒▒▒▒░░░░░  34%  → 79%               │",
		"◉                                                              │",
		"└─ resets  session 16:12  ·  week Fri 13:12 ───────────────────┘",
	}
	for i, w := range want {
		if got := rowOf(c, i); !strings.HasSuffix(got, w) {
			t.Errorf("row %d reads %q, want it to end %q", i, got, w)
		}
	}
}

func TestAPanelsUseIsTonedAsItRises(t *testing.T) {
	tests := []struct {
		used float64
		want theme.Token
	}{
		{used: 0.69, want: theme.TextPrimary}, {used: 0.7, want: theme.VizRamp2}, {used: 0.89, want: theme.VizRamp2},
		{used: 0.9, want: theme.VizRamp4}, {used: 1, want: theme.VizRamp4},
	}
	for _, tt := range tests {
		s := standing{window: weekOf(tt.used, day)}
		if got := panelUse(s); got.ink != (ink{token: tt.want, bold: true}) {
			t.Errorf("at %.0f%%, the use is %+v, want %s, bold", tt.used*100, got.ink, tt.want)
		}
	}
}

func TestStubsHangFromALinesFreeJacksWhileItsLimitHolds(t *testing.T) {
	limit := func(count int, until time.Duration) status.Document {
		doc := threeRouted()
		doc.Accounts[1].Limit.ID, doc.Accounts[1].Limit.Until = 1, now.Add(until).UTC()
		doc.Events = []status.Event{{ID: 1, At: now.Add(-time.Hour).UTC(), Kind: status.EventLimit, Account: "personal", Windows: []string{"5h"}, Until: now.Add(until).UTC(), Count: count, To: "side", Limit: 1}}
		return doc
	}
	another := limit(2, time.Hour)
	another.Events = slices.Concat([]status.Event{{ID: 2, At: now.Add(-30 * time.Minute).UTC(), Kind: status.EventLimit, Account: "personal", Windows: []string{"7d_oi"}, Until: now.Add(48 * time.Hour).UTC(), Count: 1, To: "side", Limit: 2}}, another.Events)
	tests := []struct {
		name string
		doc  status.Document
		// want are personal's jacks' rows, as they read to their jack.
		want []string
	}{
		{name: "a stub each, each a cell shorter", doc: limit(2, time.Hour), want: []string{"╌╌╌╌╌╌╌○", "╌╌╌╌╌╌○"}},
		{name: "as many as there are free jacks", doc: limit(5, time.Hour), want: []string{"╌╌╌╌╌╌╌○", "╌╌╌╌╌╌○"}},
		{name: "the holding limit's, by its identity, though another's event is newer", doc: another, want: []string{"╌╌╌╌╌╌╌○", "╌╌╌╌╌╌○"}},
		{name: "none once the limit has lifted", doc: limit(2, -time.Minute), want: []string{"○", "○"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := switchboard(160, 40, flippingSessions(), Traffic{})
			b, c := drawnBay(t, f, tt.doc)
			p := b.panels[1]
			for i, want := range tt.want {
				row := rowOf(c, p.top+1+i)
				if got := strings.TrimSpace(ansi.Cut(row, b.x-stubCells-1, b.x+1)); got != want {
					t.Errorf("jack %d reads %q, want %q", i, got, want)
				}
			}
		})
	}
}

func TestACallsRowSaysWhatItsRequestIsDoing(t *testing.T) {
	plug := Plug{Account: "work", Seat: Seat{Session: idD28C, Model: opus}}
	tests := []struct {
		name string
		call Call
		// want is what the row says after when it was last seen, and its ink;
		// and jack the glyph its jack is drawn as, and its ink.
		want     span
		jack     string
		jackInk  ink
		shimmers bool
	}{
		{name: "gone out", call: Call{Doing: Asking}, want: span{"↑ ask", nameInk}, jack: pluggedJack, jackInk: ink{token: theme.VizSeries1}},
		{name: "gone out on an account a move just brought it to", call: Call{Doing: Asking, New: true}, want: span{"↪ new", nameInk}, jack: pluggedJack, jackInk: ink{token: theme.VizSeries1}},
		{
			name: "its answer streaming, its tokens estimated, every fourth cell lit and its jack", call: Call{Doing: Streaming, Tokens: 1234, Shimmer: 1},
			want: span{"↓ ~1.2k", titleInk}, jack: pluggedJack, jackInk: titleInk, shimmers: true,
		},
		{name: "its answer ended, its tokens exact", call: Call{Doing: Answered, Tokens: 1234, Exact: true}, want: span{"↓ 1.2k", titleInk}, jack: pluggedJack, jackInk: ink{token: theme.VizSeries1}},
		{name: "refused at a limit", call: Call{Doing: Refused, Status: 429}, want: span{"✕ 429", exhaustedInk}, jack: refusedJack, jackInk: exhaustedInk},
		{name: "refused alone", call: Call{Doing: Refused, Status: 403}, want: span{"✕ 403", exhaustedInk}, jack: refusedJack, jackInk: exhaustedInk},
		{name: "throttled, dim", call: Call{Doing: Throttled, Status: 429}, want: span{"… 429", dimInk}, jack: pluggedJack, jackInk: ink{token: theme.VizSeries1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := switchboard(160, 40, flippingSessions(), Traffic{Calls: map[Plug]Call{plug: tt.call}})
			b, c := drawnBay(t, f, threeRouted())
			r := callAt(t, b, "d28c", "work")
			at := callsAt + idColumn + modelColumn + seenColumn
			for i, glyph := range strings.Split(tt.want.text, "") {
				if got := c.cells[r.row][at+i]; got.glyph != glyph || got.ink != tt.want.ink {
					t.Fatalf("the row reads %q, its cell %d %+v, want %q in %+v", rowOf(c, r.row), i, got, tt.want.text, tt.want.ink)
				}
			}
			path := cordOf(t, b, "d28c", "work").path(b.x)
			if got := c.cells[r.row][b.x]; got.glyph != tt.jack || got.ink != tt.jackInk {
				t.Errorf("its jack is %+v, want %q in %+v", got, tt.jack, tt.jackInk)
			}
			for i := 1; i < len(path)-1; i++ {
				lit := c.cells[path[i].y][path[i].x].ink.glow > 0
				if want := tt.shimmers && (i+1)%shimmerEvery == 0; lit != want {
					t.Errorf("its cord's cell %d lit is %v, want %v", i, lit, want)
				}
			}
		})
	}
}

func TestAPulseTravelsACord(t *testing.T) {
	plug := Plug{Account: "work", Seat: Seat{Session: idD28C, Model: opus}}
	tests := []struct {
		name  string
		call  Call
		token theme.Token
		// head is how far along the cord's path its head is, and behind
		// which way the rest of it lies.
		head, behind int
	}{
		{name: "out, as a request goes", call: Call{Doing: Asking, Pulsing: true, Pulse: Pulse{Along: 0.5}}, token: theme.VizSeries1, head: 33, behind: -1},
		{name: "back, as its answer ends", call: Call{Doing: Answered, Pulsing: true, Pulse: Pulse{Along: 0.25, Back: true}}, token: theme.VizSeries1, head: 51, behind: 1},
		{name: "back, red, as it's refused", call: Call{Doing: Refused, Status: 429, Pulsing: true, Pulse: Pulse{Along: 0.25, Back: true, Red: true}}, token: theme.StateDestructive, head: 51, behind: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := switchboard(160, 40, flippingSessions(), Traffic{Calls: map[Plug]Call{plug: tt.call}})
			b, c := drawnBay(t, f, threeRouted())
			path := cordOf(t, b, "d28c", "work").path(b.x)
			if len(path) != 68 {
				t.Fatalf("the cord's path is %d cells, want 68", len(path))
			}
			for k := range pulseLength {
				p := path[tt.head+k*tt.behind]
				want := ink{token: tt.token, glow: trailGlow - glowStep*float64(k)}
				if k == 0 {
					want = titleInk
				}
				if got := c.cells[p.y][p.x].ink; got != want {
					t.Errorf("the pulse's cell %d, at %d, is %+v, want %+v", k, p.x, got, want)
				}
			}
			past := path[tt.head+pulseLength*tt.behind]
			if got := c.cells[past.y][past.x].ink; got.glow != 0 {
				t.Errorf("the cell past the pulse, at %d, is %+v, want the cord's own", past.x, got)
			}
		})
	}
}

func TestAMoveRePatchesTheCall(t *testing.T) {
	seat := Seat{Session: idD28C, Model: opus}
	move := Move{Seat: seat, From: "work", To: "side", At: now.Add(-time.Second), Reason: "moved: work hit its limit", Held: true}
	side := Plug{Account: "side", Seat: seat}
	traffic := Traffic{Calls: map[Plug]Call{side: {Doing: Asking, New: true}}, Seen: map[Plug]time.Time{side: now}, Moves: []Move{move}}
	f := switchboard(160, 40, flippingSessions(), traffic)
	doc := threeRouted()
	doc.GeneratedAt = now.Add(-4 * time.Second).UTC()
	before := bayOfFrame(t, switchboard(160, 40, flippingSessions(), Traffic{}), doc)
	b, c := drawnBay(t, f, doc)

	gone, was := callAt(t, b, "d28c", "work"), callAt(t, before, "d28c", "work")
	if !gone.gone || gone.row != was.row || gone.jack != was.jack {
		t.Errorf("d28c on work is %+v, want it gone, on its row, %d, and its jack, %d, as before", gone, was.row, was.jack)
	}
	if got := ansi.Cut(rowOf(c, gone.row), 0, b.x+1); !strings.HasPrefix(got, "  d28c  ↪ moved to side") || !strings.HasSuffix(got, " ╌╌╌╌╌╌╌╌╌○") {
		t.Errorf("its row reads %q, want a placeholder, and its cord hanging loose from its jack", got)
	}
	for _, r := range before.calls {
		if r.assignment.Account == "work" && r.seat() != seat && callAt(t, b, sessionID(r.session.ID), "work").row != r.row {
			t.Errorf("%s on work moved from row %d, want the other calls put", sessionID(r.session.ID), r.row)
		}
	}
	brought := callAt(t, b, "d28c", "side")
	if last := b.calls[len(b.calls)-1]; last.seat() != seat || brought.jack != 3 {
		t.Errorf("d28c joins side's calls as %+v, want it after the rest, on side's fourth jack", brought)
	}
	if got := rowOf(c, brought.row); !strings.Contains(got, "d28c  opus    now  ↪ new") {
		t.Errorf("its row on side reads %q, want it new", got)
	}
	if got := rowOf(c, b.log.row+1); !strings.HasPrefix(got, "  13:11  ▸ d28c moved work → side: work reached its limit, so") {
		t.Errorf("LOG's first line reads %q, want it to tell of the move, and why", got)
	}
}

func TestLOGSaysWhyASessionMoved(t *testing.T) {
	doc := threeRouted()
	tests := []struct {
		reason, want string
	}{
		{reason: "moved: work hit its limit", want: ": work reached its limit, so its request was retried on side"},
		{reason: "moved: work was refused", want: ": work was refused, so its request was retried on side"},
		{reason: "moved: work was throttled, personal under pressure", want: ": work was throttled, so its request was retried on side, passing over personal under pressure"},
		{reason: "moved: work has no room", want: ": work has no room"},
		{reason: "pinned", want: " (pin)"},
	}
	for _, tt := range tests {
		e := status.Event{Kind: status.EventMoved, Session: idD28C, From: "work", To: "side", Reason: tt.reason}
		var got strings.Builder
		retriedWhy(e, doc).draw(&got, Look{})
		if got.String() != tt.want {
			t.Errorf("moved for %q, LOG says %q, want %q", tt.reason, got.String(), tt.want)
		}
	}
}

func TestAMovesCordFadesOnceTheSessionsAreListedAgain(t *testing.T) {
	seat := Seat{Session: idD28C, Model: opus}
	sessions := flippingSessions()
	for i, s := range sessions {
		if s.ID == idD28C {
			sessions[i].Assignments[0].Account = "side"
		}
	}
	tests := []struct {
		name string
		fade float64
		look Look
		// want is the cord hanging from work's free jack, "" for none.
		want string
	}{
		{name: "fading", fade: 0.4, look: Look{}, want: "╌╌╌╌╌╌╌╌╌○"},
		{name: "faded", fade: 1, look: Look{}, want: "○"},
		{name: "halfway, where the look can't fade it: gone", fade: 0.5, look: NoColour(), want: "○"},
	}
	// Fable's week used, each line has three jacks, and work, left with two
	// calls, one free.
	doc := threeRouted()
	doc.Accounts[2].Windows = append(doc.Accounts[2].Windows, fableOf(0.1, 4*day))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			move := Move{Seat: seat, From: "work", To: "side", At: now.Add(-3 * time.Second), Listed: true, Fade: tt.fade}
			f := switchboard(160, 40, sessions, Traffic{Moves: []Move{move}})
			f.Look = tt.look
			b, c := drawnBay(t, f, doc)
			if slices.ContainsFunc(b.calls, func(r callRow) bool { return r.gone }) {
				t.Errorf("the calls are %+v, want no placeholder, the sessions listed since", b.calls)
			}
			free := b.panels[0].top + 3
			if got := strings.TrimSpace(ansi.Strip(ansi.Cut(rowOf(c, free), b.x-looseCells-1, b.x+1))); got != tt.want {
				t.Errorf("work's free jack reads %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLOGListsEachMoveAndWhy(t *testing.T) {
	doc := flipping()
	doc.GeneratedAt = now.Add(-4 * time.Second).UTC()
	pending := Move{Seat: Seat{Session: idC61B, Model: opus}, From: "side", To: "work", At: now, Reason: "moved: side was refused"}
	f := switchboard(160, 40, flippingSessions(), Traffic{Moves: []Move{pending}})
	b, c := drawnBay(t, f, doc)

	var got []string
	for i := range b.log.lines {
		got = append(got, strings.TrimRight(ansi.Cut(rowOf(c, b.log.row+1+i), 0, b.log.end), " "))
	}
	want := []string{
		"  13:12  ▸ c61b moved side → work: side was refused, so its request was retried on work",
		"  13:11  ▲ c61b started on side, the best",
		"  12:42  ● work came under pressure: its session runs out ~14:42 at its last-30-min rate",
		"  12:22  ▸ 41e0 moved personal → side: personal has no room",
		"  12:22  ▸ db8a moved personal → side: personal has no room",
	}
	told := len(got) == len(want)
	for i := range want {
		told = told && tells(got[i], want[i], b.log.end)
	}
	if !told {
		t.Errorf("LOG reads\n%s\nwant\n%s\neach whole, or cut short at its end, column %d", strings.Join(got, "\n"), strings.Join(want, "\n"), b.log.end)
	}
	if label := strings.TrimRight(ansi.Cut(rowOf(c, b.log.row), 0, b.log.end), " "); label != "  LOG" {
		t.Errorf("LOG's label reads %q", label)
	}
}

// tells reports whether a row of LOG, its blanks trimmed, tells of an event
// as said: the whole of it, or as much as fits before LOG's end, cut short
// with an ellipsis, a blank before it trimmed.
func tells(row, said string, end int) bool {
	if row == said {
		return true
	}
	kept, cut := strings.CutSuffix(row, ellipsis)
	return cut && strings.HasPrefix(said, kept) && ansi.StringWidth(row) >= end-1
}

func TestLOGLeavesAMoveToTheDocumentBuiltSinceIt(t *testing.T) {
	doc := flipping()
	told := Move{Seat: Seat{Session: idC61B, Model: opus}, From: "side", To: "work", At: doc.GeneratedAt.Add(-time.Second), Reason: "pinned"}
	f := switchboard(160, 40, flippingSessions(), Traffic{Moves: []Move{told}})

	if events := f.logged(doc, now, logLines); events[0].ID == 0 {
		t.Errorf("LOG's first is %+v, want the document's newest, the move told of before it was built being the document's to tell", events[0])
	}
}

func TestLOGEndsShortOfTheCordsThatTurnDownBesideIt(t *testing.T) {
	f, doc := switchboard(160, 40, flippingSessions(), Traffic{}), flipping()
	b := bayOfFrame(t, f, doc)
	logged := newCanvas(f.Width, b.content)
	f.drawLog(logged, doc, b.log, now)
	corded := make(map[point]bool)
	for _, k := range b.cords {
		for _, p := range k.path(b.x) {
			corded[p] = true
		}
	}

	beside := 0
	for y := b.log.row + 1; y <= b.log.row+b.log.lines; y++ {
		for x := range f.Width {
			if corded[point{x, y}] {
				beside++
			}
			if cell := logged.at(x, y); cell.glyph == " " {
				continue
			}
			if corded[point{x - 1, y}] || corded[point{x, y}] || corded[point{x + 1, y}] {
				t.Errorf("LOG's row %d reads %q, its cell at %d meeting a cord's", y, rowOf(logged, y), x)
			}
		}
	}
	if beside == 0 {
		t.Fatal("no cord turns down beside LOG, want the frame to test it against one")
	}
}

func TestTheSessionsViewScrollsWhereItDoesntFit(t *testing.T) {
	doc := accountsOf(8, true)
	f := switchboard(160, 30, nil, Traffic{})
	scrolling := f.Scrolling(doc, now)
	if scrolling.Most == 0 {
		t.Fatalf("eight accounts at 160×30 scroll %+v, want them scrolling", scrolling)
	}
	rows := f.Draw(doc, now)

	if got := rows[28]; !strings.Contains(got, "▼ 5 more accounts below · j/k or wheel to scroll") {
		t.Errorf("the line over the footer reads %q, want what's out of view", got)
	}
	if got := rows[9]; !strings.HasSuffix(got, "│┃") {
		t.Errorf("row 10 reads %q, want the lines a cell short of the frame's edge, the scrollbar beside them", got)
	}
	f.Scroll = scrolling.Most
	if got := f.Draw(doc, now)[28]; !strings.Contains(got, "▲ 5 more accounts above") {
		t.Errorf("scrolled to the foot, the line over the footer reads %q, want the accounts above", got)
	}
}

func TestWhatTravelsTheCordsMovesOnScreenAlone(t *testing.T) {
	plug := func(session, account string) Plug {
		return Plug{Account: account, Seat: Seat{Session: session, Model: opus}}
	}
	tests := []struct {
		name   string
		height int
		scroll bool
		calls  map[Plug]Call
		moves  []Move
		want   Motion
	}{
		{name: "a pulse along a cord in view: frame by frame", calls: map[Plug]Call{plug(idD28C, "work"): {Doing: Asking, Pulsing: true}}, want: Motion{Smooth: true}},
		{name: "an answer streaming back along one: its shimmer, a step at a time", calls: map[Plug]Call{plug(idD28C, "work"): {Doing: Streaming}}, want: Motion{Shimmer: true}},
		{name: "refused, its pulse back: still", calls: map[Plug]Call{plug(idD28C, "work"): {Doing: Refused, Status: 429}}},
		{name: "a session the router hasn't listed, with no cord: still", calls: map[Plug]Call{plug("5b19aa00", "work"): {Doing: Streaming, Pulsing: true}}},
		{name: "a cord scrolled out of view: still", height: 14, calls: map[Plug]Call{plug(idC61B, "side"): {Doing: Asking, Pulsing: true}}},
		{name: "scrolled into view", height: 14, scroll: true, calls: map[Plug]Call{plug(idC61B, "side"): {Doing: Asking, Pulsing: true}}, want: Motion{Smooth: true}},
		{
			name:  "a cord a move let go, fading from a jack in view",
			moves: []Move{{Seat: Seat{Session: idD28C, Model: opus}, From: "work", To: "side", At: now, Listed: true, Fade: 0.4}},
			want:  Motion{Smooth: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessions := flippingSessions()
			for _, m := range tt.moves {
				sessions = listedOn(sessions, m.Seat.Session, m.To)
			}
			// Fable's week used, each line has three jacks, and work, left
			// with two calls by a move, one free.
			doc := threeRouted()
			doc.Accounts[2].Windows = append(doc.Accounts[2].Windows, fableOf(0.1, 4*day))
			f := switchboard(160, cmp.Or(tt.height, 40), sessions, Traffic{Live: true, Calls: tt.calls, Moves: tt.moves})
			if tt.scroll {
				f.Scroll = f.Scrolling(doc, now).Most
			}
			if got := f.Motion(doc, now); got != tt.want {
				t.Errorf("Motion = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// listedOn are the sessions given, the one with the given id listed on the
// account to.
func listedOn(sessions []status.Session, id, to string) []status.Session {
	sessions = slices.Clone(sessions)
	for i, s := range sessions {
		if s.ID == id {
			sessions[i].Assignments = slices.Clone(s.Assignments)
			sessions[i].Assignments[0].Account = to
		}
	}
	return sessions
}

func TestWhatTravelsTheCordsUnderTheHelpMovesNothing(t *testing.T) {
	traffic := Traffic{Live: true, Calls: map[Plug]Call{{Account: "side", Seat: Seat{Session: idC61B, Model: opus}}: {Doing: Asking, Pulsing: true}}}
	f := switchboard(160, 40, flippingSessions(), traffic)
	f.Help = []Key{{Key: "?", Does: "these keys, and the key to the glyphs, with room enough for every one of them and more besides"}}

	b := bayOfFrame(t, f, threeRouted())
	top := f.above(newCanvas(f.Width, f.Height), threeRouted(), now) + 1
	for _, p := range cordOf(t, b, "c61b", "side").path(b.x) {
		if !f.helpCovers().covers(p.x, top+p.y, 1, 1) {
			t.Fatalf("the help doesn't cover c61b's cord at %d along row %d, want it to, to test what it covers", p.x, top+p.y)
		}
	}
	if got := f.Motion(threeRouted(), now); got.Smooth {
		t.Errorf("Motion = %+v, want nothing moving, the cord under the help", got)
	}
}

func TestAPlaceholderIsCutToTheCallsColumns(t *testing.T) {
	doc := threeRouted()
	doc.Accounts[2].Label = "a side account with a long name"
	move := Move{Seat: Seat{Session: idD28C, Model: opus}, From: "work", To: "side", At: now}
	b, c := drawnBay(t, switchboard(160, 40, flippingSessions(), Traffic{Moves: []Move{move}}), doc)

	said := strings.TrimRight(ansi.Cut(rowOf(c, callAt(t, b, "d28c", "work").row), 0, b.x-looseCells), " ")
	if !strings.HasPrefix(said, "  d28c  ↪ moved to a side") || !strings.HasSuffix(said, ellipsis) || ansi.StringWidth(said) > plugAt {
		t.Errorf("its placeholder reads %q, want it cut short, with an ellipsis, before the plugs' column, %d", said, plugAt)
	}
}

func TestSessionsDrawsNothingOverItsHeadingOrItsFooter(t *testing.T) {
	for _, doc := range []status.Document{threeRouted(), routerDoc("work", 3, pressedAccount("work"))} {
		for height := 1; height <= 14; height++ {
			f := switchboard(160, height, flippingSessions(), Traffic{})
			bare := newCanvas(f.Width, f.Height)
			top := f.above(bare, doc, now)
			f.footer(bare, doc, height-1)
			want, got := bare.rows(Look{}), f.Draw(doc, now)
			for y := range height {
				if (y < top || y == height-1) && got[y] != want[y] {
					t.Errorf("%d accounts, %d rows tall: row %d reads %q, want %q, as the heading or the footer has it", len(doc.Accounts), height, y+1, got[y], want[y])
				}
			}
		}
	}
}

func TestAPlainListThatScrollsWithEveryPanelInViewSaysNothingOfScrolling(t *testing.T) {
	doc := routerDoc("work", 3, pressedAccount("work"))
	f := switchboard(160, 15, flippingSessions(), Traffic{})
	rows := f.Draw(doc, now)

	if f.Scrolling(doc, now).Most == 0 {
		t.Fatal("the plain list doesn't scroll, want it to, to test what the line over the footer says")
	}
	if got := rows[f.Height-2]; strings.Contains(got, "scroll") || got != strings.Repeat("─", 160) {
		t.Errorf("the line over the footer reads %q, want a rule, no account out of view", got)
	}
	if got := rows[f.Height-3]; !strings.HasSuffix(got, "┃") && !strings.HasSuffix(got, "│") {
		t.Errorf("the row over it reads %q, want the scrollbar beside the view", got)
	}
}

func TestAnUnreadAccountsPanelSaysSoAsItsCardDoes(t *testing.T) {
	doc := threeRouted()
	doc.Accounts[1] = status.Account{ID: "personal", Label: "personal", TokenSet: true}
	f := switchboard(160, 40, flippingSessions(), Traffic{})
	b, c := drawnBay(t, f, doc)
	p := b.panels[1]

	says := strings.Index(rowOf(c, p.top), "not read yet")
	if says < 0 {
		t.Fatalf("personal's top edge reads %q, want it not read yet", rowOf(c, p.top))
	}
	if got := c.cells[p.top][ansi.StringWidth(rowOf(c, p.top)[:says])].ink; got != dimInk {
		t.Errorf("its not read yet is in %+v, want dim, as its card has it", got)
	}
	if got := strings.TrimSpace(ansi.Cut(rowOf(c, p.bottom()), b.x, b.x+b.width)); got != "└"+strings.Repeat("─", b.width-2)+"┘" {
		t.Errorf("its bottom edge reads %q, want it whole, nothing to say of its resets", got)
	}
}

func TestTheLineOverTheSessionsFooterIsARule(t *testing.T) {
	rows := switchboard(160, 40, flippingSessions(), Traffic{}).Draw(threeRouted(), now)

	if got := rows[38]; got != strings.Repeat("─", 160) {
		t.Errorf("row 39 reads %q, want a rule from side to side", got)
	}
}

func TestNarrowerTerminalsSessions(t *testing.T) {
	tests := []struct {
		name  string
		width int
		// want is what the view has, and wantNo what it hasn't.
		want, wantNo []string
	}{
		{
			name: "from 110 columns, the lines linesWide wide, their windows' bars", width: 110,
			want: []string{"LINES  accounts", "◉  Session  ███"}, wantNo: []string{"Session    58%"},
		},
		{
			name: "under 110, the bars giving way to the windows' use", width: 109,
			want: []string{"LINES  accounts", "◉  Session    58%  → out ~14:42", "└─ session 16:12 · week Fri 13:12 "}, wantNo: []string{"███"},
		},
		{
			name: "under 90, a plain list, each line's sessions under its panel", width: 89,
			want: []string{"│  Session    58%", "● d28c  opus    seen       now", "╰ here since 11:12", "LOG"}, wantNo: []string{"CALLS", "LINES", "◉"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			screen := strings.Join(switchboard(tt.width, 60, flippingSessions(), Traffic{}).Draw(threeRouted(), now), "\n")
			for _, want := range tt.want {
				if !strings.Contains(screen, want) {
					t.Errorf("the view reads\n%s\nwant %q", screen, want)
				}
			}
			for _, unwanted := range tt.wantNo {
				if strings.Contains(screen, unwanted) {
					t.Errorf("the view reads\n%s\nwant no %q", screen, unwanted)
				}
			}
		})
	}
}

func TestThePlainListShowsTheStreamsMoves(t *testing.T) {
	d28c := Seat{Session: idD28C, Model: opus}
	traffic := Traffic{
		Live:  true,
		Calls: map[Plug]Call{{Account: "side", Seat: d28c}: {Doing: Asking, Since: now.Add(-38 * time.Second), New: true}},
		Moves: []Move{{Seat: d28c, From: "work", To: "side", At: now.Add(-time.Minute), Reason: "moved: work hit its limit"}},
	}
	rows := switchboard(89, 60, flippingSessions(), traffic).Draw(threeRouted(), now)

	at := func(row string) int {
		return slices.IndexFunc(rows, func(r string) bool { return strings.Contains(r, row) })
	}
	d28cRow, sides := at("● d28c  opus    waiting    38s"), at("3 · SIDE")
	if d28cRow < 0 || sides < 0 || d28cRow < sides {
		t.Errorf("the plain list is\n%s\nwant d28c under side's panel, waiting there, as the stream moved it", strings.Join(rows, "\n"))
	}
}

func TestWithOneAccountSessionsIsAPlainList(t *testing.T) {
	doc := routerDoc("work", 3, pressedAccount("work"))
	rows := switchboard(160, 30, flippingSessions(), Traffic{}).Draw(doc, now)

	want := []string{
		" ┌─ 1 · WORK ─ ● under pressure ────────────────────────────────┐",
		" │  Session  █████████┃███▉▒▒▒▒▒▒▒▒▒▒  58%  → out ~14:42        │",
		" │  Week     ████████▏▒┃▒▒▒▒▒▒▒▒░░░░░  34%  → 79%               │",
		" └─ resets  session 16:12  ·  week Fri 13:12 ───────────────────┘",
		"    ● db8a  sonnet  seen       now",
		"      ╰ its opus is on side",
	}
	for i, w := range want {
		if got := strings.TrimRight(rows[4+i], " "); got != w {
			t.Errorf("row %d reads %q, want %q", 5+i, got, w)
		}
	}
}

func TestARequestIsInFlightWhileItAsksStreamsOrIsThrottled(t *testing.T) {
	tests := []struct {
		doing Doing
		want  bool
	}{
		{doing: Asking, want: true}, {doing: Streaming, want: true}, {doing: Throttled, want: true},
		{doing: Answered}, {doing: Refused}, {doing: 0},
	}
	for _, tt := range tests {
		if got := tt.doing.InFlight(); got != tt.want {
			t.Errorf("Doing(%d).InFlight() = %v, want %v", tt.doing, got, tt.want)
		}
	}
}

func TestTokensAreCountedAsBrieflyAsARowHasRoomFor(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{n: 0, want: "0"}, {n: 999, want: "999"}, {n: 1000, want: "1.0k"}, {n: 1234, want: "1.2k"},
		{n: 9949, want: "9.9k"}, {n: 9950, want: "10k"}, {n: 34567, want: "35k"}, {n: 999499, want: "999k"},
		{n: 999500, want: "1.0M"}, {n: 1234567, want: "1.2M"},
	}
	for _, tt := range tests {
		if got := tokens(tt.n); got != tt.want {
			t.Errorf("tokens(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestABackAndACallRoundWhenItWasLastSeenAlike(t *testing.T) {
	for ago := time.Minute; ago <= time.Hour; ago += 7 * time.Second {
		if back, call := lapsed(now.Add(-ago), now), seenAgo(now, now.Add(-ago)); back != call {
			t.Errorf("seen %v ago, a back's row reads %q and a call's %q, want them alike", ago, back, call)
		}
	}
}

func TestACallSaysWhenItWasLastSeen(t *testing.T) {
	tests := []struct {
		ago  time.Duration
		want string
	}{
		{ago: 0, want: "now"}, {ago: 29 * time.Second, want: "now"}, {ago: 30 * time.Second, want: "1m"},
		{ago: 50 * time.Second, want: "1m"}, {ago: 9*time.Minute + 20*time.Second, want: "9m"},
		{ago: 59*time.Minute + 29*time.Second, want: "59m"}, {ago: 59*time.Minute + 30*time.Second, want: "1h"},
	}
	for _, tt := range tests {
		if got := seenAgo(now, now.Add(-tt.ago)); got != tt.want {
			t.Errorf("seen %v ago reads %q, want %q", tt.ago, got, tt.want)
		}
	}
}
