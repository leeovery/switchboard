package dashboard

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// id9C2E's neighbours on the board: the sessions the frames of ten accounts
// add.
const (
	id1F9A = "1f9a3b7c-5d2e-4f80-9a6b-c4d1e2f3a5b6"
	id6C2E = "6c2e8d4f-1a3b-4c5d-8e9f-0a1b2c3d4e5f"
	id0D4B = "0d4b7e2a-9c1f-4a3d-b5e6-7f8a9b0c1d2e"
	idB7E5 = "b7e51c3d-2f4a-4b6c-8d9e-0f1a2b3c4d5e"
	id93FA = "93fa4e6b-7c8d-4e9f-a0b1-c2d3e4f5a6b7"
	id4AE8 = "4ae80b1c-3d5e-4f6a-9b7c-8d9e0f1a2b3c"
	idC0D1 = "c0d1e2f3-4a5b-4c6d-8e7f-9a0b1c2d3e4f"
	idF3B1 = "f3b1a2c3-d4e5-4f6a-8b7c-9d0e1f2a3b4c"
)

// boardCall is a call as the watch builds it: the session with the given id
// on the account given, in the directory given, doing what a says, and come
// from the account from, "" where no move brought it.
func boardCall(session, account, dir string, a views.Activity, from string) views.Call {
	return views.Call{Account: account, Session: session, Family: "opus", Models: []string{opus}, Activity: a, Dir: dir, From: from}
}

// idleSince is a session idle since the time given, its last answer ended
// then.
func idleSince(at time.Time) views.Activity {
	return views.Activity{Since: at, LastSeen: at}
}

// inFlight is a session doing d, its request gone out at the time given,
// answered with the status given where it's refused or throttled.
func inFlight(d views.Doing, sent time.Time, status int) views.Activity {
	return views.Activity{Doing: d, Since: sent, LastSeen: sent, Status: status}
}

// lastKnown is a, as last known, the router no longer answering.
func lastKnown(a views.Activity) views.Activity {
	a.Stale = true
	return a
}

// boardLook is a look at the Switchboard: the calls the watch built at it,
// and the stubs each account's limit hangs, by its id.
type boardLook struct {
	calls []views.Call
	stubs map[string]int
}

// boardOf is Sessions' Switchboard as the watch holds it after the looks
// given, each after the last, as views.Patch keeps who sits where, the last
// look's calls drawn.
func boardOf(looks ...boardLook) SessionsShown {
	var s SessionsShown
	for _, look := range looks {
		s.Patched = views.Patch(s.Patched, look.calls, look.stubs)
		s.Calls = look.calls
	}
	return s
}

// atClock is the time given on Thursday 1 October 2026, routedAt's day.
func atClock(hour, minute, second int) time.Time {
	return time.Date(2026, 10, 1, hour, minute, second, 0, time.UTC)
}

// sceneCalls are the calls of the 14:42:07 scene, as RunningOf lists them:
// on personal, 41e0, idle 34s, and 9c2e, idle 9m; on side, c61b answering,
// and moved from work as it reached its cap, d28c asking, 7f3a answering,
// and db8a, idle 50s.
func sceneCalls() []views.Call {
	return []views.Call{
		boardCall(id41E0, "personal", "~/Code/docs", idleSince(routedAt.Add(-34*time.Second)), ""),
		boardCall(id9C2E, "personal", "~/Code/website", idleSince(atClock(14, 33, 7)), ""),
		boardCall(idC61B, "side", "~/Code/mobile", inFlight(views.Answering, atClock(14, 41, 50), 0), ""),
		boardCall(idD28C, "side", "~/Code/switchboard", inFlight(views.Asking, atClock(14, 42, 5), 0), "work"),
		boardCall(id7F3A, "side", "~/Code/api", inFlight(views.Answering, atClock(14, 41, 58), 0), "work"),
		boardCall(idDB8A, "side", "~/Code/infra", idleSince(routedAt.Add(-50*time.Second)), "work"),
	}
}

// readAt has the world read a minute before at, its document built then, and
// personal's week running out ~Sat 06:00 at its recent rate, as usageWorld
// has it at routedAt.
func readAt(at time.Time) func(*status.Document) {
	return func(d *status.Document) {
		d.GeneratedAt = at
		for i := range d.Accounts {
			d.Accounts[i].FetchedAt = at.Add(-time.Minute)
		}
		runsOut := time.Date(2026, 10, 3, 6, 0, 30, 0, time.UTC)
		account("personal", func(a *status.Account) { a.Rates[0].Rate = 0.34 / runsOut.Sub(at).Hours() })(d)
	}
}

// personalAtItsLimit has personal at its 5-hour limit, reached at reached, till
// until, as its window resets, its event counting the sessions it has moved,
// to side.
func personalAtItsLimit(reached, until time.Time, moved int) func(*status.Document) {
	const limit = 12
	return func(d *status.Document) {
		account("personal", func(a *status.Account) {
			a.Windows[0] = quota.Window{Key: "5h", Label: "Session", Utilization: 1, Status: quota.StatusRejected, ResetsAt: until}
			a.Limit = status.Limit{ID: limit, Windows: []string{"5h"}, Until: until}
		})(d)
		d.Events = append(d.Events, status.Event{
			ID: 7, At: reached, Kind: status.EventLimit, Account: "personal", Windows: []string{"5h"}, To: "side", Until: until, Count: moved, Limit: limit,
		})
	}
}

// sidesSession has side's 5-hour window used as given.
func sidesSession(used float64) func(*status.Document) {
	return account("side", func(a *status.Account) { a.Windows[0].Utilization = used })
}

// limitWorld is the auto world at the time given, personal at its 5-hour
// limit since 16:05:41 till 16:30, its event counting the sessions it has
// moved, and side's 5-hour at 31%.
func limitWorld(at time.Time, moved int) status.Document {
	return usageWorld(readAt(at), personalAtItsLimit(atClock(16, 5, 41), atClock(16, 30, 0), moved), sidesSession(0.31))
}

// at1605 are the calls at 16:05:41, as RunningOf lists them, 41e0 doing
// what doing41E0 says: on personal, 41e0 and 9c2e, idle since 15:52:30; on
// side, c61b answering, and moved from work, d28c, idle since 16:05:36,
// db8a since 16:04:55, and 7f3a since 16:03:12.
func at1605(doing41E0 views.Activity) []views.Call {
	return []views.Call{
		boardCall(id41E0, "personal", "~/Code/docs", doing41E0, ""),
		boardCall(id9C2E, "personal", "~/Code/website", idleSince(atClock(15, 52, 30)), ""),
		boardCall(idC61B, "side", "~/Code/mobile", inFlight(views.Answering, atClock(16, 5, 30), 0), ""),
		boardCall(idD28C, "side", "~/Code/switchboard", idleSince(atClock(16, 5, 36)), "work"),
		boardCall(idDB8A, "side", "~/Code/infra", idleSince(atClock(16, 4, 55)), "work"),
		boardCall(id7F3A, "side", "~/Code/api", idleSince(atClock(16, 3, 12)), "work"),
	}
}

// movedAt1605 are the calls at 16:05:42, 41e0 moved from personal to side,
// asking there, seen last of them.
func movedAt1605() []views.Call {
	calls := at1605(views.Activity{})[1:]
	moved := boardCall(id41E0, "side", "~/Code/docs", inFlight(views.Asking, atClock(16, 5, 42), 0), "personal")
	return slices.Insert(calls, 1, moved)
}

// pinnedWorld is the world of the pinned frame: new sessions pinned to
// personal and side, side where they go; work at its cap, out of the pool;
// and personal at its 5-hour limit since 14:12 till 15:54, its event
// counting the two sessions it moved.
func pinnedWorld() status.Document {
	return usageWorld(usagePinned("personal", "side"), func(d *status.Document) { d.Best = "side" },
		personalAtItsLimit(atClock(14, 12, 0), atClock(15, 54, 0), 2))
}

// pinnedCalls are the calls of the pinned frame, each on side: c61b
// answering, d28c asking, moved from work, 7f3a answering, db8a idle, moved
// from work, and 41e0 and 9c2e, idle, moved from personal at its limit.
func pinnedCalls() []views.Call {
	calls := sceneCalls()
	for i := range calls {
		calls[i].Account = "side"
	}
	calls[0].From, calls[1].From, calls[4].From = "personal", "personal", ""
	return calls
}

// aloneCalls are the calls of the frame of one account, each on work: 41e0,
// idle 34s, 9c2e, idle 9m, c61b answering, d28c asking and 7f3a answering.
func aloneCalls() []views.Call {
	calls := sceneCalls()[:5]
	for i := range calls {
		calls[i].Account, calls[i].From = "work", ""
	}
	return calls
}

// primaryAlone is aloneDoc with work the primary, which ◆ marks with one
// account as with several.
func primaryAlone() status.Document {
	doc := aloneDoc()
	doc.Accounts[0].Primary = true
	return doc.WorkedOut(claudeLike)
}

// pacedAccount is an account of the frame of ten, read a minute before
// routedAt: its 5-hour window used as given, its even pace at the cell given
// of a bar of tenBar cells, and its week alike.
func pacedAccount(id string, used float64, cell int, weekUsed float64, weekCell int) status.Account {
	const tenBar = 22
	resets := func(cell int, length time.Duration) time.Time {
		return routedAt.Add(time.Duration((1 - float64(cell)/tenBar) * float64(length)))
	}
	return usageAccount(id, used, resets(cell, 5*time.Hour), weekUsed, 0, resets(weekCell, 7*24*time.Hour))
}

// boardOfTen is the world of the frame of ten accounts: usageWorld's three,
// then client, spare, lab, under pressure, running out ~16:05, team, at its
// 5-hour limit till 15:30, extra, ops and studio.
func boardOfTen() status.Document {
	runsOut, since := atClock(16, 5, 0), routedAt.Add(-30*time.Minute)
	lab := pacedAccount("lab", 0.71, 14, 0.58, 11)
	rate := (1 - 0.71) / runsOut.Sub(routedAt).Hours()
	lab.Rates = []status.Rate{{Window: "5h", Rate: rate, Since: since}}
	lab.Pressure = status.Pressure{Window: "5h", Rate: rate, Recent: true, Since: since, RunsOut: runsOut, Under: true}
	team := pacedAccount("team", 1, 15, 0.83, 13)
	team.Windows[0] = quota.Window{Key: "5h", Label: "Session", Utilization: 1, Status: quota.StatusRejected, ResetsAt: atClock(15, 30, 0)}
	team.Limit = status.Limit{Windows: []string{"5h"}, Until: atClock(15, 30, 0)}
	more := []status.Account{
		pacedAccount("client", 0.52, 9, 0.47, 13), pacedAccount("spare", 0.04, 1, 0.05, 7), lab, team,
		pacedAccount("extra", 0.22, 8, 0.12, 4), pacedAccount("ops", 0.06, 3, 0.29, 10), pacedAccount("studio", 0.33, 12, 0.41, 8),
	}
	return usageWorld(func(d *status.Document) { d.Accounts = append(d.Accounts, more...) })
}

// tenCalls are the calls of the frame of ten accounts, as the frame lists
// them: the scene's six; on client, 1f9a, 6c2e and 0d4b; on lab, b7e5 and
// 93fa; on extra, 4ae8; and on studio, c0d1 and f3b1, each idle as long as
// the frame says.
func tenCalls() []views.Call {
	idle := func(id, account, dir string, ago time.Duration) views.Call {
		return boardCall(id, account, dir, idleSince(routedAt.Add(-ago)), "")
	}
	return append(sceneCalls(),
		idle(id1F9A, "client", "~/Code/billing", 2*time.Minute), idle(id6C2E, "client", "~/Code/ios-app", 14*time.Minute),
		idle(id0D4B, "client", "~/Code/scraper", 41*time.Second), idle(idB7E5, "lab", "~/Code/notebooks", 6*time.Minute),
		idle(id93FA, "lab", "~/Code/evals", time.Minute), idle(id4AE8, "extra", "~/Code/blog", 22*time.Minute),
		idle(idC0D1, "studio", "~/Code/design", 3*time.Minute), idle(idF3B1, "studio", "~/Code/site-v2", 8*time.Second),
	)
}

// seenLastFirst are the calls given, each account's seen last first, as
// RunningOf lists them.
func seenLastFirst(calls []views.Call) []views.Call {
	sorted := slices.Clone(calls)
	slices.SortStableFunc(sorted, func(a, b views.Call) int {
		if a.Account != b.Account {
			return 0
		}
		return b.Activity.LastSeen.Compare(a.Activity.LastSeen)
	})
	return sorted
}

// streamingOn is what the stream tells of a call whose answer streams back,
// its shimmer as the frames freeze it.
var streamingOn = Call{Doing: Streaming, Shimmer: 2}

// askingOn is what the stream tells of a call whose request goes out, its
// pulse's head at cell head of a cord last cells long, as the frames
// freeze it.
func askingOn(head, last int) Call {
	return Call{Doing: Asking, Pulsing: true, Pulse: Pulse{Along: float64(head) / float64(last)}}
}

// refusedOn is what the stream tells of a call whose request was refused at
// its account's limit, its red pulse's head at cell head of a cord last
// cells long, counted from the jack, as the frames freeze it.
func refusedOn(head, last int) Call {
	return Call{Doing: Refused, Pulsing: true, Pulse: Pulse{Along: float64(head) / float64(last), Back: true, Red: true}}
}

// The cells of the Switchboard's frames passed over, as the frames draw them
// otherwise than the design or than the world.
var (
	// boardSidesPace is where the frames place side's 5-hour even pace a
	// cell on, as USAGE's frames place each window's as if its pace were a
	// whole percent, evenPace160.
	boardSidesPace = spot{23, 103, 104}
	// boardSidesWeek is where the frames give side's week its allowance,
	// where the world has it run out, as USAGE's frames do, sidesWeek.
	boardSidesWeek = spot{25, 137, 158}
	// fableAt1442 are where the frames at 16:05 give each Fable week the
	// allowance it had at 14:42, a percent a day less than the world's.
	fableAt1442 = []spot{{11, 137, 138}, {19, 137, 138}, {27, 137, 138}}
	// alonePrimary is where the frame of one account marks work ▲ alone, and
	// its key names no ◆, where, by the owner's call of 11 October 2026, ◆
	// marks the primary with one account as with several.
	alonePrimary = []spot{{6, 93, 95}, {15, 109, 143}}
	// personalsPinnedPace is where the pinned frame places personal's
	// 5-hour even pace as if its window reset at 16:30, as the auto world's,
	// where its world's resets at 15:54, as its limit lifts.
	personalsPinnedPace = spot{15, 118, 122}
	// teamsPace is where the frame of ten places team's 5-hour even pace as
	// if its window reset at about 16:17, where its world's resets at 15:30,
	// as its limit lifts.
	teamsPace = spot{22, 113, 116}
)

func TestTheSwitchboardIsDrawnAsItsFrames(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	tests := []struct {
		stem    string
		doc     status.Document
		now     time.Time
		board   SessionsShown
		traffic Traffic
		whole   []spot
	}{
		{
			stem: "sessions-switchboard-160", doc: usageWorld(), now: routedAt, board: boardOf(boardLook{calls: sceneCalls()}),
			traffic: Traffic{Calls: map[Plug]Call{
				{Session: idC61B, Account: "side"}: streamingOn,
				{Session: id7F3A, Account: "side"}: streamingOn,
				{Session: idD28C, Account: "side"}: askingOn(24, 49),
			}},
			whole: []spot{boardSidesPace, boardSidesWeek},
		},
		{
			stem: "sessions-refused-160", doc: limitWorld(atClock(16, 5, 41), 0), now: atClock(16, 5, 41),
			board: boardOf(boardLook{calls: sceneCalls()}, boardLook{calls: at1605(inFlight(views.Refused, atClock(16, 5, 40), 429))}),
			traffic: Traffic{Calls: map[Plug]Call{
				{Session: id41E0, Account: "personal"}: refusedOn(14, 44),
				{Session: idC61B, Account: "side"}:     streamingOn,
			}},
			whole: append([]spot{boardSidesWeek}, fableAt1442...),
		},
		{
			stem: "sessions-moved-160", doc: limitWorld(atClock(16, 5, 42), 1), now: atClock(16, 5, 42),
			board: boardOf(
				boardLook{calls: sceneCalls()},
				boardLook{calls: at1605(inFlight(views.Refused, atClock(16, 5, 40), 429))},
				boardLook{calls: movedAt1605(), stubs: map[string]int{"personal": 1}},
			),
			traffic: Traffic{Calls: map[Plug]Call{
				{Session: id41E0, Account: "side"}: askingOn(7, 49),
				{Session: idC61B, Account: "side"}: streamingOn,
			}},
			whole: append([]spot{boardSidesWeek}, fableAt1442...),
		},
		{
			stem: "sessions-pinned-160", doc: pinnedWorld(), now: routedAt,
			board: boardOf(boardLook{calls: sceneCalls()}, boardLook{calls: pinnedCalls(), stubs: map[string]int{"personal": 2}}),
			traffic: Traffic{Calls: map[Plug]Call{
				{Session: idC61B, Account: "side"}: streamingOn,
				{Session: id7F3A, Account: "side"}: streamingOn,
				{Session: idD28C, Account: "side"}: askingOn(26, 52),
			}},
			whole: []spot{boardSidesPace, boardSidesWeek, personalsPinnedPace},
		},
		{
			stem: "sessions-ten-accounts-160", doc: boardOfTen(), now: routedAt,
			board: boardOf(boardLook{calls: tenCalls()}, boardLook{calls: seenLastFirst(tenCalls())}),
			traffic: Traffic{Calls: map[Plug]Call{
				{Session: idC61B, Account: "side"}: streamingOn,
				{Session: id7F3A, Account: "side"}: streamingOn,
				{Session: idD28C, Account: "side"}: askingOn(18, 37),
			}},
			whole: []spot{teamsPace},
		},
		{
			stem: "sessions-one-account-160", doc: primaryAlone(), now: routedAt, board: boardOf(boardLook{calls: aloneCalls()}),
			traffic: Traffic{Calls: map[Plug]Call{
				{Session: idC61B, Account: "work"}: streamingOn,
				{Session: id7F3A, Account: "work"}: streamingOn,
				{Session: idD28C, Account: "work"}: askingOn(18, 36),
			}},
			whole: alonePrimary,
		},
	}
	for _, tt := range tests {
		t.Run(tt.stem, func(t *testing.T) {
			f := Frame{Width: 160, Height: 37, Look: nord, View: Sessions, Policy: claudeLike, SessionsTab: tt.board, Traffic: tt.traffic}
			drawn := screenOf(t, f.Draw(tt.doc, tt.now))
			heldTo(t, tt.stem, reference(t, tt.stem), drawn, region{top: 6, bottom: 34, right: 159}, shadeOf(nord.Canvas()), tt.whole, nil)
		})
	}
}

// sceneTraffic is what the stream tells at 14:42:07, as the frames freeze
// it: c61b's and 7f3a's answers streaming back, and d28c's request going out.
func sceneTraffic() Traffic {
	return Traffic{Calls: map[Plug]Call{
		{Session: idC61B, Account: "side"}: streamingOn,
		{Session: id7F3A, Account: "side"}: streamingOn,
		{Session: idD28C, Account: "side"}: askingOn(24, 49),
	}}
}

// boardFrame is a frame of Sessions' Switchboard, width by height, in the
// look given, the board as given, what travels it as traffic tells.
func boardFrame(width, height int, look Look, board SessionsShown, traffic Traffic) Frame {
	return Frame{Width: width, Height: height, Look: look, View: Sessions, Policy: claudeLike, SessionsTab: board, Traffic: traffic}
}

// sceneFrame is the 14:42:07 scene's Switchboard, width by height, in the
// look given.
func sceneFrame(width, height int, look Look) Frame {
	return boardFrame(width, height, look, boardOf(boardLook{calls: sceneCalls()}), sceneTraffic())
}

// textFrom is the text of row y of c from column x to before column end.
func textFrom(c *canvas, x, end, y int) string {
	var b strings.Builder
	for i := x; i < end; i++ {
		b.WriteString(c.at(i, y).glyph)
	}
	return b.String()
}

// With no accounts, the Switchboard has no lines, and draws no key for
// them: no even pace, cap or marks under nothing.
func TestTheSwitchboardOfNoAccountsDrawsNoKey(t *testing.T) {
	doc := routerDoc("", 0)
	for _, width := range []int{160, 120} {
		f := boardFrame(width, 37, Look{}, boardOf(boardLook{}), Traffic{})
		rows := textOf(f.Draw(doc, routedAt))
		if slices.ContainsFunc(rows, func(row string) bool { return strings.Contains(row, "even pace") }) {
			t.Errorf("at %d columns, the Switchboard of no accounts reads\n%s\nwant no key", width, strings.Join(rows, "\n"))
		}
	}
}

func TestACallSaysItsSessionWhatItsDoingAndWhereItCameFrom(t *testing.T) {
	sent := routedAt.Add(-2 * time.Second)
	tests := []struct {
		name       string
		call       views.Call
		want       string
		id, doing  ink
		cameFromAt int
	}{
		{
			name: "asking, moved from work", call: boardCall(idD28C, "side", "~/Code/switchboard", inFlight(views.Asking, sent, 0), "work"),
			want: "d28c switchboard  ↑ asking     ▸ from work      ", id: titleInk, doing: keyInk, cameFromAt: 31,
		},
		{
			name: "answering", call: boardCall(idC61B, "side", "~/Code/mobile", inFlight(views.Answering, sent, 0), ""),
			want: "c61b mobile       ↓ answering                   ", id: titleInk, doing: keyInk,
		},
		{
			name: "refused, a 403, its id still bold", call: boardCall(idC61B, "side", "~/Code/mobile", inFlight(views.Refused, sent, 403), ""),
			want: "c61b mobile       ✕ 403                         ", id: titleInk, doing: exhaustedInk,
		},
		{
			name: "throttled, sent again on its account, its id bold", call: boardCall(idC61B, "side", "~/Code/mobile", inFlight(views.Throttled, sent, 429), ""),
			want: "c61b mobile       … 429                         ", id: titleInk, doing: dimInk,
		},
		{
			name: "answering as last known, the router not answering: as it was, in text.subtle, its id at rest",
			call: boardCall(idC61B, "side", "~/Code/mobile", lastKnown(inFlight(views.Answering, sent, 0)), ""),
			want: "c61b mobile       ↓ answering                   ", id: mutedInk, doing: dimInk,
		},
		{
			name: "refused as last known: as it was, in text.subtle, its id at rest",
			call: boardCall(idC61B, "side", "~/Code/mobile", lastKnown(inFlight(views.Refused, sent, 429)), ""),
			want: "c61b mobile       ✕ 429                         ", id: mutedInk, doing: dimInk,
		},
		{
			name: "idle, as the dashboard writes spans", call: boardCall(id9C2E, "side", "~/Code/website", idleSince(routedAt.Add(-65*time.Second)), ""),
			want: "9c2e website      idle 1m                       ", id: mutedInk, doing: faintInk,
		},
		{
			name: "idle under a minute, in seconds", call: boardCall(id9C2E, "side", "~/Code/website", idleSince(routedAt.Add(-34*time.Second)), ""),
			want: "9c2e website      idle 34s                      ", id: mutedInk, doing: faintInk,
		},
		{
			name: "no directory: the id alone", call: boardCall(id9C2E, "side", "", idleSince(routedAt.Add(-34*time.Second)), ""),
			want: "9c2e              idle 34s                      ", id: mutedInk, doing: faintInk,
		},
		{
			name: "a long directory and account, each cut with …", call: boardCall(id9C2E, "side", "~/Code/notebooks-archive", idleSince(routedAt.Add(-34*time.Second)), "personal-2"),
			want: "9c2e notebooks-a… idle 34s     ▸ from persona…  ", id: mutedInk, doing: faintInk, cameFromAt: 31,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := boardFrame(160, 37, Look{}, boardOf(boardLook{calls: []views.Call{tt.call}}), Traffic{})
			c := f.page(usageWorld(), routedAt)
			if got := textFrom(c, 0, 48, 6); got != tt.want {
				t.Errorf("the call reads\n%q\nwant\n%q", got, tt.want)
			}
			if got := c.at(48, 6).glyph; got != plugGlyph {
				t.Errorf("its plug, at 48, is %q, want %s", got, plugGlyph)
			}
			if got, want := c.at(49, 6).ink, cordInk(tt.id == titleInk); got != want {
				t.Errorf("its cord is in %+v, want %+v, lit while its id is bold, else at rest", got, want)
			}
			for _, at := range []struct {
				what string
				x    int
				want ink
			}{{"its id", 0, tt.id}, {"what it's doing", 18, tt.doing}} {
				if got := c.at(at.x, 6).ink; got != at.want {
					t.Errorf("%s, at %d, is in %+v, want %+v", at.what, at.x, got, at.want)
				}
			}
			if dir := c.at(5, 6); dir.glyph != " " && dir.ink != mutedInk {
				t.Errorf("its directory, at 5, is in %+v, want text.muted", dir.ink)
			}
			if tt.cameFromAt == 0 {
				return
			}
			for x, want := range map[int]ink{tt.cameFromAt: keyMarkInk, tt.cameFromAt + 2: mutedInk, tt.cameFromAt + 7: secondaryInk} {
				if got := c.at(x, 6).ink; got != want {
					t.Errorf("where it came from, at %d, is in %+v, want %+v", x, got, want)
				}
			}
		})
	}
}

// With no session running, as while no router lists any, the Switchboard's
// first row says so in text.faint, and its lines stand with every jack free.
func TestWithNoSessionRunningTheLinesStandAlone(t *testing.T) {
	c := boardFrame(160, 37, Look{}, SessionsShown{}, Traffic{}).page(usageWorld(), routedAt)
	if got := rowOf(c, 6); !strings.HasPrefix(got, noSessionsRunning+strings.Repeat(" ", 65)+freeJack+" 1 work") {
		t.Errorf("the view's first row reads %q, want %q, then work's line", got, noSessionsRunning)
	}
	if got := c.at(0, 6).ink; got != faintInk {
		t.Errorf("it's in %+v, want text.faint", got)
	}
	for row := 6; row < 28; row++ {
		if got := c.at(84, row).glyph; got != freeJack && got != " " {
			t.Errorf("row %d's jack is %q, want every jack free", row+1, got)
		}
	}
}

// The scene's calls' rows at 160 by 37: 41e0's, at rest, and c61b's, at
// work; and the row 41e0's cord ends on, at personal's first jack.
const (
	restingRow = 6
	workingRow = 9
	restingEnd = 14
)

func TestACordsColourSaysWhatsHappeningNeverWhichAccount(t *testing.T) {
	tests := []struct {
		name           string
		slug           string
		look           Look
		resting, works string
		turn           string
	}{
		{name: "a look that blends: the idle cord dimmed", slug: "nord", resting: "●━━━", works: "●━━━", turn: "┓"},
		{name: "a light theme: dimmed against its own canvas", slug: "tokyo-night-day", resting: "●━━━", works: "●━━━", turn: "┓"},
		{name: "the terminal's colours: the idle cord light", slug: theme.Terminal, resting: "●───", works: "●━━━", turn: "┐"},
		{name: "no colour: the idle cord light", resting: "●───", works: "●━━━", turn: "┐"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			look := NoColour()
			if tt.slug != "" {
				look = Screen(builtin(t, tt.slug))
			}
			c := boardFrame(160, 37, look, boardOf(boardLook{calls: sceneCalls()}), Traffic{}).page(usageWorld(), routedAt)
			for row, want := range map[int]string{restingRow: tt.resting, workingRow: tt.works} {
				if got := textFrom(c, 48, 52, row); got != want {
					t.Errorf("the cord on row %d reads %q, want %q", row+1, got, want)
				}
			}
			if got := textFrom(c, 72, 73, restingRow); got != tt.turn {
				t.Errorf("the idle cord turns down as %q, want %q", got, tt.turn)
			}
			for row, want := range map[int]ink{restingRow: cordInk(false), workingRow: cordInk(true)} {
				if got := c.at(60, row).ink; got != want {
					t.Errorf("the cord on row %d is in %+v, want %+v", row+1, got, want)
				}
			}
			if got := c.at(84, restingEnd); got.glyph != pluggedJack || got.ink != cordInk(false) {
				t.Errorf("the idle cord's jack is %q in %+v, want %s in its cord's ink", got.glyph, got.ink, pluggedJack)
			}
			if tt.slug == "" || !look.blends() {
				return
			}
			dimmed, _ := look.Blend(builtin(t, tt.slug).Colour(theme.AccentKey), restingCord)
			if got := screenOf(t, c.rows(look))[restingRow][60].fg; !got.near(shadeOf(dimmed)) {
				t.Errorf("the idle cord is %s, want accent.key %.2f of the way to the look's canvas, %s", got, restingCord, shadeOf(dimmed))
			}
		})
	}
}

// stubsOn is the Switchboard of a world with personal at its limit, the
// sessions its event says it moved gone, and no call on it, the stubs hung.
func stubsOn(moved int) (Frame, status.Document) {
	doc := usageWorld(personalAtItsLimit(atClock(14, 12, 0), atClock(15, 54, 0), moved))
	board := boardOf(boardLook{stubs: map[string]int{"personal": moved}})
	return boardFrame(160, 37, Look{}, board, Traffic{}), doc
}

// A stub hangs to its jack's left for each session the limit's event moved,
// red: 7 cells, then a cell shorter for each next, never under a cell; its
// jack shows ○, and the line grows a row for each stub past its rows.
func TestStubsHangForEachSessionTheLimitMoved(t *testing.T) {
	for _, moved := range []int{1, 3, 8} {
		t.Run(strconv.Itoa(moved), func(t *testing.T) {
			f, doc := stubsOn(moved)
			c := f.page(doc, routedAt)
			personal, _ := f.switchboardLaid(doc, routedAt).line("personal")
			top := f.top() + personal.top
			for jack := range moved {
				cells := max(7-jack, 1)
				want := strings.Repeat(" ", 7-cells) + strings.Repeat(hangingGlyph, cells) + freeJack
				if got := textFrom(c, 77, 85, top+jack); got != want {
					t.Errorf("jack %d reads %q, want %q", jack, got, want)
				}
				if got := c.at(83, top+jack).ink; got != errorInk {
					t.Errorf("stub %d is in %+v, want state.destructive", jack, got)
				}
			}
			if got := textFrom(c, 77, 85, top+max(moved, 6)); strings.TrimSpace(got) != "" {
				t.Errorf("the row under personal's line reads %q, want nothing hanging", got)
			}
		})
	}
}

// letGo is the Switchboard after d28c moved from work to side with no
// limit forcing it, what travels its cords as traffic tells.
func letGo(look Look, traffic Traffic) Frame {
	before := []views.Call{boardCall(idD28C, "work", "~/Code/api", idleSince(routedAt.Add(-time.Minute)), "")}
	after := []views.Call{boardCall(idD28C, "side", "~/Code/api", inFlight(views.Asking, routedAt, 0), "work")}
	return boardFrame(160, 37, look, boardOf(boardLook{calls: before}, boardLook{calls: after}), traffic)
}

// A cord a move no limit forced let go hangs 9 cells from the jack its
// session left, in text.subtle, as far faded as the stream tells, and marked
// as hanging loose, for its fading to move frames on; gone once it has faded,
// or halfway where the look can't fade it.
func TestALooseCordHangsFromTheJackItsSessionLeftFading(t *testing.T) {
	tests := []struct {
		name  string
		look  Look
		fade  float64
		hangs bool
	}{
		{name: "just let go", look: Screen(builtin(t, "nord")), fade: 0, hangs: true},
		{name: "fading", look: Screen(builtin(t, "nord")), fade: 0.6, hangs: true},
		{name: "faded", look: Screen(builtin(t, "nord")), fade: 1},
		{name: "no colour, before halfway", look: NoColour(), fade: 0.4, hangs: true},
		{name: "no colour, from halfway", look: NoColour(), fade: 0.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loose := Loose{Session: idD28C, From: "work", Fade: tt.fade}
			c := letGo(tt.look, Traffic{Loose: []Loose{loose}}).page(usageWorld(), routedAt)
			want := strings.Repeat(" ", looseCells) + freeJack
			if tt.hangs {
				want = strings.Repeat(hangingGlyph, looseCells) + freeJack
			}
			if got := textFrom(c, 84-looseCells, 85, 6); got != want {
				t.Fatalf("work's first jack reads %q, want %q", got, want)
			}
			if !tt.hangs {
				return
			}
			k := dimInk
			k.fade = tt.fade
			if here := c.at(84-looseCells, 6); here.ink != k || !here.mark.Value().loose {
				t.Errorf("the loose cord is in %+v, marked %+v, want text.subtle faded %.1f, marked loose", here.ink, here.mark.Value(), tt.fade)
			}
		})
	}
}

// The call chosen is picked out on bg.selection over its own columns alone,
// from the row's first to the one before its plug, their glyphs and colours
// as they are; its plug, its cord, and the line and bars its row runs on to
// are drawn as they are unchosen, as is every other row: work's week and
// Fable week bars, on d28c's and 7f3a's rows, keep their colours. At every
// width the cords draw at, the plug moves, and the choice with it.
func TestTheCallChosenIsPickedOutOverItsOwnColumnsAlone(t *testing.T) {
	selection := hue{token: theme.BgSelection}
	for _, width := range []int{100, 129, 160, 200} {
		for _, chosen := range []Plug{{Session: idD28C, Account: "side"}, {Session: id7F3A, Account: "side"}} {
			f := sceneFrame(width, 37, Look{})
			f.SessionsTab.Chosen = chosen
			c := f.page(usageWorld(), routedAt)
			unchosen := sceneFrame(width, 37, Look{}).page(usageWorld(), routedAt)
			laid := f.switchboardLaid(usageWorld(), routedAt)
			i := slices.IndexFunc(laid.calls, func(lc laidCall) bool { return lc.session == chosen.Session })
			row, plug := f.top()+laid.calls[i].cord.from, laid.columns.plug
			for y := f.top(); y < f.top()+24; y++ {
				for x := range width {
					got, was := c.at(x, y), unchosen.at(x, y)
					if y == row && x < plug {
						was.ink.on = selection
					}
					if got.glyph != was.glyph || got.ink != was.ink {
						t.Fatalf("at %d, %s chosen, row %d, column %d is %q in %+v, want %q in %+v", width, status.ShownID(chosen.Session), y+1, x+1, got.glyph, got.ink, was.glyph, was.ink)
					}
				}
			}
		}
	}
}

// Each call the view shows is a target, from its id to its plug, which a
// click on chooses, then opens: its session's, on its account.
func TestTheSwitchboardsCallsAreItsTargets(t *testing.T) {
	f := sceneFrame(160, 37, Look{})
	var got []Target
	for _, target := range f.Targets(usageWorld(), routedAt) {
		if target.On == OnSession {
			got = append(got, target)
		}
	}
	at := func(row int, session, account string) Target {
		return Target{Y: row, Width: 49, On: OnSession, Session: session, Account: account}
	}
	want := []Target{
		at(6, id41E0, "personal"), at(7, id9C2E, "personal"),
		at(9, idC61B, "side"), at(10, idD28C, "side"), at(11, id7F3A, "side"), at(12, idDB8A, "side"),
	}
	if !slices.Equal(got, want) {
		t.Errorf("the calls' targets are\n%+v\nwant\n%+v", got, want)
	}
}

// What moves on the Switchboard is what travels the cords it draws: a pulse
// along one smoothly, a shimmer down one a step at a time, a loose cord
// fading; nothing travelling, nothing moves.
func TestWhatTravelsTheCordsDrawnMovesFrames(t *testing.T) {
	d28c, c61b := Plug{Session: idD28C, Account: "side"}, Plug{Session: idC61B, Account: "side"}
	tests := []struct {
		name    string
		traffic Traffic
		want    Motion
	}{
		{name: "nothing travelling", want: Motion{}},
		{name: "a pulse", traffic: Traffic{Calls: map[Plug]Call{d28c: askingOn(24, 49)}}, want: Motion{Smooth: true}},
		{name: "a shimmer", traffic: Traffic{Calls: map[Plug]Call{c61b: streamingOn}}, want: Motion{Shimmer: true}},
		{name: "a session the board doesn't draw", traffic: Traffic{Calls: map[Plug]Call{{Session: "gone", Account: "side"}: askingOn(1, 2)}}, want: Motion{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := boardFrame(160, 37, Look{}, boardOf(boardLook{calls: sceneCalls()}), tt.traffic)
			if got := f.Motion(usageWorld(), routedAt); got != tt.want {
				t.Errorf("Motion() = %+v, want %+v", got, tt.want)
			}
		})
	}
	t.Run("a cord let go, fading", func(t *testing.T) {
		f := letGo(Look{}, Traffic{Loose: []Loose{{Session: idD28C, From: "work", Fade: 0.3}}})
		if got := f.Motion(usageWorld(), routedAt); got != (Motion{Smooth: true}) {
			t.Errorf("Motion() = %+v, want it smooth", got)
		}
	})
}

// The key is the Overview's, on its two rows under two blank rows after the
// last line, a cell after the lines' column; a row that would pass the last
// column starts at column 0. It names the cap where the bars draw its mark.
func TestTheKeyIsTheOverviewsACellAfterTheLines(t *testing.T) {
	const pace = "┃ even pace: where you would be now, used evenly"
	const marks = "◆ primary    ▲ new sessions go here"
	const capped = "╎ cap: work at 95%    " + marks
	tests := []struct {
		width         int
		first, second int
		marks         string
	}{
		{width: 160, first: 87, second: 87, marks: capped},
		{width: 200, first: 127, second: 127, marks: capped},
		{width: 140, first: 72, second: 72, marks: capped},
		{width: 110, first: 61, second: 0, marks: capped},
		{width: 100, first: 0, second: 59, marks: marks},
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.width), func(t *testing.T) {
			f := sceneFrame(tt.width, 37, Look{})
			laid := f.switchboardLaid(usageWorld(), routedAt)
			c := f.page(usageWorld(), routedAt)
			y := f.top() + laid.keyAt
			for i, want := range []struct {
				x    int
				text string
			}{{tt.first, pace}, {tt.second, tt.marks}} {
				row := rowOf(c, y+i)
				if got := strings.TrimLeft(row, " "); got != want.text || ansi.StringWidth(row)-ansi.StringWidth(got) != want.x {
					t.Errorf("the key's row %d reads %q, want %q from column %d", i+1, row, want.text, want.x)
				}
			}
		})
	}
}
