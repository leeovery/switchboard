package capture

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

// idA11C is a session a test starts, beside b3e9.
const idA11C = "a11c5e08-2d4b-4f1a-8c3e-9b7d6a5f4e21"

func TestTheRouterAnswersAsTheScenarioHasPlayedUpToItsClock(t *testing.T) {
	s := testScenario(starts(1, idB3E9, opus, "~/Code/cli", brief))
	tl, c := timelineOf(s, 0)

	before, health, err := tl.Read(t.Context(), status.Read{})
	if err != nil {
		t.Fatal(err)
	}
	c.advance(2 * time.Second)
	after, again, _ := tl.Read(t.Context(), status.Read{})
	sessions, _ := tl.Sessions(t.Context())

	if before.Sessions != 6 || after.Sessions != 7 {
		t.Errorf("the router counts %d sessions, then %d, want 6, then 7, b3e9 started", before.Sessions, after.Sessions)
	}
	if start := tl.base.start; !after.GeneratedAt.Equal(start.Add(2*time.Second)) || after.Events[0].Session != idB3E9 {
		t.Errorf("the document read 2s in was built at %s, its newest event %+v, want built then, telling of b3e9 starting", after.GeneratedAt, after.Events[0])
	}
	if after.Events[1].ID != before.Events[0].ID || after.Events[0].ID != before.Events[0].ID+1 {
		t.Errorf("the router numbers its events %d, then %d, after %d, want each to keep its id, the new one next", after.Events[1].ID, after.Events[0].ID, before.Events[0].ID)
	}
	if b3e9 := listed(t, sessions, idB3E9); b3e9.Assignments[0].Dir != "~/Code/cli" || b3e9.Assignments[0].Account != "side" {
		t.Errorf("the router lists b3e9 as %+v, want it on side, in its directory", b3e9)
	}
	if !health.Same(again) || !health.StartedAt.Equal(routerRun(tl.base.start)) || !tl.RouterAnswers(t.Context()) {
		t.Error("read again, the router is another, or doesn't answer, want the same router throughout, the world's run, answering")
	}
}

func TestOrdersAreTakenFromWhenTheyreGiven(t *testing.T) {
	s := testScenario(starts(0.5, idA11C, sonnet, "~/Code/cli", brief), starts(3, idB3E9, opus, "~/Code/cli", brief))
	tl, c := timelineOf(s, time.Second)

	if err := tl.Pin(t.Context(), []string{"personal"}, false); err != nil {
		t.Fatal(err)
	}
	if err := tl.PinSession(t.Context(), idC61B, "personal"); err != nil {
		t.Fatal(err)
	}
	c.advance(3 * time.Second)

	doc, _, _ := tl.Read(t.Context(), status.Read{})
	if !doc.Pin.Has("personal") || !doc.Pin.Since.Equal(tl.base.start.Add(time.Second)) || doc.Best != "personal" {
		t.Errorf("the document's pin is %+v, its best %q, want personal's, given 1s in, and personal", doc.Pin, doc.Best)
	}
	sessions, _ := tl.Sessions(t.Context())
	if on := listed(t, sessions, idA11C).Assignments[0].Account; on != "side" {
		t.Errorf("a11c, started before the pin, is on %s, want side, the best then", on)
	}
	if b3e9 := listed(t, sessions, idB3E9).Assignments[0]; b3e9.Account != "personal" || b3e9.Reason != status.ReasonGlobalPin {
		t.Errorf("b3e9, started after the pin, is listed %+v, want on personal, as pinned", b3e9)
	}
	if c61b := listed(t, sessions, idC61B); c61b.Pin != "personal" || c61b.Assignments[0].Account != "side" || c61b.Assignments[0].Pinned {
		t.Errorf("the router lists c61b as %+v, want its own pin to personal, its opus on side till its next request", c61b)
	}
	if err := tl.Unpin(t.Context()); err != nil {
		t.Fatal(err)
	}
	if doc, _, _ := tl.Read(t.Context(), status.Read{}); !doc.Pin.IsZero() || doc.Best != "side" {
		t.Errorf("unpinned, the document's pin is %+v, its best %q, want none, and side", doc.Pin, doc.Best)
	}
}

// The card's Pin and Unpin, and a session's own pin and its clearing, are
// told of as the router tells of them, given from the dashboard; clearing
// what was never given tells nothing.
func TestOrdersAreToldOfAsTheRouterTellsOfThem(t *testing.T) {
	tl, c := timelineOf(testScenario(), time.Second)
	for _, give := range []func() error{
		func() error { return tl.Unpin(t.Context()) },
		func() error { return tl.UnpinSession(t.Context(), idC61B) },
		func() error { return tl.Pin(t.Context(), []string{"side"}, true) },
		func() error { return tl.PinSession(t.Context(), idC61B, "personal") },
		func() error { return tl.UnpinSession(t.Context(), idC61B) },
		func() error { return tl.Unpin(t.Context()) },
	} {
		if err := give(); err != nil {
			t.Fatal(err)
		}
		c.advance(time.Second)
	}

	doc, _, _ := tl.Read(t.Context(), status.Read{})
	var told []status.Event
	for _, e := range doc.Events {
		if e.ID > tl.base.events {
			e.ID, e.At = 0, time.Time{}
			told = append(told, e)
		}
	}
	slices.Reverse(told)
	want := []status.Event{
		{Kind: status.EventPin, Accounts: []string{"side"}, Account: "side", Move: true, By: "dashboard"},
		{Kind: status.EventPin, Account: "personal", Session: idC61B, By: "dashboard"},
		{Kind: status.EventAuto, Account: "personal", Session: idC61B, By: "dashboard"},
		{Kind: status.EventAuto, Accounts: []string{"side"}, By: "dashboard"},
	}
	if len(told) != len(want) {
		t.Fatalf("the router tells of %+v, want %+v", told, want)
	}
	for i := range want {
		if got := told[i]; got.Kind != want[i].Kind || !slices.Equal(got.Accounts, want[i].Accounts) || got.Account != want[i].Account ||
			got.Move != want[i].Move || got.Session != want[i].Session || got.By != want[i].By {
			t.Errorf("the router tells %+v, want %+v", got, want[i])
		}
	}
}

func TestASessionUnpinnedOnceItsPinMovedItIsRoutedOnItsMerits(t *testing.T) {
	s := testScenario(asks(2, idDB8A, opus, brief), asks(5, idDB8A, opus, brief))
	tl, c := timelineOf(s, time.Second)

	if err := tl.PinSession(t.Context(), idDB8A, "personal"); err != nil {
		t.Fatal(err)
	}
	c.advance(1500 * time.Millisecond)
	sessions, _ := tl.Sessions(t.Context())
	if db8a := listed(t, sessions, idDB8A); db8a.Pin != "personal" || db8a.Assignments[0].Account != "personal" ||
		!db8a.Assignments[0].Pinned || !db8a.Assignments[0].PinnedAt.Equal(tl.base.start.Add(time.Second)) {
		t.Errorf("its pin having moved it, the router lists db8a as %+v, want it on personal, pinned there since 1s", db8a)
	}
	if err := tl.UnpinSession(t.Context(), idDB8A); err != nil {
		t.Fatal(err)
	}
	c.advance(3 * time.Second)

	sessions, _ = tl.Sessions(t.Context())
	if db8a := listed(t, sessions, idDB8A); db8a.Pin != "" || db8a.Assignments[0].Account != "personal" ||
		db8a.Assignments[0].Pinned || !db8a.Assignments[0].PinnedAt.IsZero() {
		t.Errorf("unpinned, the router lists db8a as %+v, want it on personal, where its pin moved it, pinned nowhere", db8a)
	}
	if moves := movesOf(tl, idDB8A); !slices.Equal(moves, []string{"side → personal, pinned"}) {
		t.Errorf("db8a moved %q, want once, by its pin, its next request staying on personal", moves)
	}
}

func TestAPinThatMovesRunningSessionsMovesEachOnce(t *testing.T) {
	s := testScenario(
		asks(2, idDB8A, opus, brief),
		asks(3, id7F3A, sonnet, brief),
		asks(5, idDB8A, opus, brief),
		asks(7, idDB8A, opus, brief),
		asks(8, id7F3A, sonnet, brief),
	)
	tl, c := timelineOf(s, time.Second)

	if err := tl.Pin(t.Context(), []string{"personal"}, true); err != nil {
		t.Fatal(err)
	}
	c.advance(3 * time.Second)
	if err := tl.PinSession(t.Context(), idDB8A, "side"); err != nil {
		t.Fatal(err)
	}
	c.advance(2 * time.Second)
	if err := tl.UnpinSession(t.Context(), idDB8A); err != nil {
		t.Fatal(err)
	}

	// db8a, put back on side by its own pin after the pin that moved it, stays
	// there once that's cleared: the router moves a session once a pin.
	if moves := movesOf(tl, idDB8A); !slices.Equal(moves, []string{"side → personal, moved by pin", "personal → side, pinned"}) {
		t.Errorf("db8a moved %q, want to personal by the pin, back to side by its own pin, and no more", moves)
	}
	if moves := movesOf(tl, id7F3A); !slices.Equal(moves, []string{"side → personal, moved by pin"}) {
		t.Errorf("7f3a moved %q, want to personal by the pin, once", moves)
	}
}

func TestTheStreamTellsOfTheRequestsInFlightThenOfEachEventAsItsTimeComes(t *testing.T) {
	s := testScenario(asks(1, idDB8A, opus, brief), asks(2, id7F3A, sonnet, brief))
	tl, c := timelineOf(s, 1500*time.Millisecond)
	tl.wait = c.wait
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	events, err := tl.Stream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var joined []string
	for range 3 {
		e := receive(t, events)
		if e.Kind != router.StreamInFlight {
			t.Errorf("the stream opens telling %+v, want what's in flight", e)
		}
		joined = append(joined, status.ShortID(e.Session)[:4])
	}
	if want := []string{"c61b", "d28c", "db8a"}; !slices.Equal(joined, want) {
		t.Errorf("the stream opens telling of %q in flight, want %q, the first sent first", joined, want)
	}
	start, joinedAt := tl.base.start, c.read()
	told := tl.base.played(nil, time.Time{}).told
	next := told[slices.IndexFunc(told, func(e router.StreamEvent) bool { return e.At.After(joinedAt) })]
	var after []router.StreamEvent
	for len(after) < 12 {
		after = append(after, receive(t, events))
		if e := after[len(after)-1]; e.At.After(c.read()) {
			t.Errorf("the stream told %+v at %s, before its time", e, c.read())
		}
	}
	if after[0] != next || c.waits()[0] != next.At.Sub(joinedAt) {
		t.Errorf("then, having waited %v, it tells %+v, want %+v, as its time comes", c.waits(), after[0], next)
	}
	if !slices.IsSortedFunc(after, func(a, b router.StreamEvent) int { return a.At.Compare(b.At) }) {
		t.Errorf("it tells %+v out of order, want each event as its time comes", after)
	}
	if i := slices.IndexFunc(after, func(e router.StreamEvent) bool { return e.Kind == router.StreamSent }); i < 0 ||
		after[i].Session != id7F3A || !after[i].At.Equal(start.Add(2*time.Second)) {
		t.Errorf("it tells %+v, want 7f3a's request going out among it, 2s in", after)
	}
	cancel()
	for range events {
	}
}

func TestAnOrderGivenWhileTheStreamIsOpenChangesWhatItTellsNext(t *testing.T) {
	s := testScenario(asks(1, idDB8A, opus, brief))
	held := make(chan struct{})
	tl, c := timelineOf(s, 0)
	c.held = held
	tl.wait = c.wait
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	events, _ := tl.Stream(ctx)
	if err := tl.PinSession(t.Context(), idDB8A, "personal"); err != nil {
		t.Fatal(err)
	}
	close(held)

	var moved router.StreamEvent
	for moved.Kind != router.StreamMoved {
		moved = receive(t, events)
	}
	if moved.Session != idDB8A || moved.From != "side" || moved.To != "personal" || moved.Reason != status.ReasonPinned {
		t.Errorf("the stream tells %+v, want db8a moved from side to personal, by its pin", moved)
	}
	if sent := receive(t, events); sent.Kind != router.StreamSent || sent.Account != "personal" {
		t.Errorf("then %+v, want its request going out on personal", sent)
	}
}

func TestTheStreamClosesAsItsContextEnds(t *testing.T) {
	tl, _ := timelineOf(testScenario(), 0)
	tl.wait = waitFor
	ctx, cancel := context.WithCancel(t.Context())

	events, _ := tl.Stream(ctx)
	cancel()
	for {
		select {
		case _, open := <-events:
			if !open {
				return
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the stream stayed open once its context ended, want it closed")
		}
	}
}

// A scenario's world is built before its clock starts, so its first frame
// reads its start, however long the world took to build.
func TestTheClockStartsAtTheScenariosFirstFrame(t *testing.T) {
	s := testScenario()
	clock := &testClock{now: time.Date(2026, time.October, 14, 9, 0, 0, 0, time.UTC)}
	worlds := s.worlds
	s.worlds = func() accountWorlds {
		clock.advance(time.Hour)
		return worlds()
	}

	l := s.begun(t.Context(), clock.read)
	if start := worldsNow().now; !l.now().Equal(start) {
		t.Errorf("as it starts, the scenario's clock reads %s, want its start, %s, its world built before", l.now(), start)
	}
	clock.advance(12 * time.Second)
	if start := worldsNow().now; !l.now().Equal(start.Add(12 * time.Second)) {
		t.Errorf("12s in, the scenario's clock reads %s, want 12s on from its start", l.now())
	}
}

func TestTheClockRunsFromTheScenariosStart(t *testing.T) {
	p := played(t, testScenario())
	if frame := p.screen(); !strings.Contains(frame, "Thu 1 Oct  14:42:07") {
		t.Fatalf("starting, it draws\n%s\nwant it drawn at once, at the scenario's start", frame)
	}
	p.until(12)
	p.l = drive(p.l, nil, reached{})
	if frame := p.screen(); !strings.Contains(frame, "Thu 1 Oct  14:42:19") {
		t.Errorf("12s in, it draws\n%s\nwant its clock 12s on", frame)
	}
}

func TestKeysAreShownAsTheyrePressedTillTheyveHadTheirTime(t *testing.T) {
	p := played(t, testScenario())

	p.press(0, tea.KeyPressMsg{Code: tea.KeyRight}, tea.KeyPressMsg{Code: tea.KeyRight})
	p.clock.advance(typedFor - time.Millisecond)
	p.l = drive(p.l, nil, tea.KeyPressMsg{Code: 'w', Text: "w"})
	if last := lastRow(p.l); !strings.HasSuffix(last, " →   →   w  ") {
		t.Errorf("the last row reads %q, want the keys pressed, in turn, at its right", last)
	}
	if last := lastRow(p.l); strings.Contains(last, "read") || !strings.Contains(last, "q quit") {
		t.Errorf("the last row reads %q, want the key line's keys, but nothing of how long ago it was read, which the keys cut into", last)
	}
	p.clock.advance(typedFor - time.Millisecond)
	if last := lastRow(p.l); !strings.HasSuffix(last, " w  ") {
		t.Errorf("as w has its time, the last row reads %q, want it still shown", last)
	}
	p.clock.advance(time.Millisecond)
	if last := lastRow(p.l); strings.HasSuffix(last, " w  ") {
		t.Errorf("once the keys have had their time, the last row reads %q, want them gone", last)
	}
}

func TestTheLatestKeysPressedAreShown(t *testing.T) {
	var keys typed
	at := localMoment()
	for _, key := range []string{"1", "2", "3", "4", "5"} {
		keys = keys.pressed(key, at)
		at = at.Add(typedFor / 2)
	}
	if got := keys.shown(at); !slices.Equal(got, []string{"2", "3", "4", "5"}) {
		t.Errorf("having pressed five keys in turn, %q are shown, want the last %d", got, mostTyped)
	}
	if got := keys.pressed("6", at.Add(typedFor)).shown(at.Add(typedFor)); !slices.Equal(got, []string{"6"}) {
		t.Errorf("a key pressed once the others have had their time shows %q, want it alone", got)
	}
}

func TestKeysAreNamedAsTheKeyLineNamesThem(t *testing.T) {
	tests := []struct {
		key  tea.KeyPressMsg
		want string
	}{
		{key: tea.KeyPressMsg{Code: tea.KeyTab}, want: "tab"},
		{key: tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}, want: "shift-tab"},
		{key: tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}, want: "space"},
		{key: tea.KeyPressMsg{Code: tea.KeyUp}, want: "↑"},
		{key: tea.KeyPressMsg{Code: tea.KeyDown}, want: "↓"},
		{key: tea.KeyPressMsg{Code: tea.KeyLeft}, want: "←"},
		{key: tea.KeyPressMsg{Code: tea.KeyRight}, want: "→"},
		{key: tea.KeyPressMsg{Code: tea.KeyEnter}, want: "enter"},
		{key: tea.KeyPressMsg{Code: tea.KeyEscape}, want: "esc"},
		{key: tea.KeyPressMsg{Code: tea.KeyPgDown}, want: "PgDn"},
		{key: tea.KeyPressMsg{Code: 'w', Text: "w"}, want: "w"},
		{key: tea.KeyPressMsg{Code: '2', Text: "2"}, want: "2"},
	}
	for _, tt := range tests {
		if got := keyName(tt.key); got != tt.want {
			t.Errorf("keyName(%v) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestKeycapsAreDrawnInTheThemeShown(t *testing.T) {
	p := played(t, testScenario())
	p.press(0, tea.KeyPressMsg{Code: 'g', Text: "g"})

	content := p.l.View().Content
	if last := content[strings.LastIndex(content, "\n")+1:]; !strings.Contains(last, "48;2;129;161;193") {
		t.Errorf("the keycap is drawn %q, want it on nord's accent.key, the theme shown", last)
	}
	plain := played(t, testScenario().WithoutColour())
	plain.press(0, tea.KeyPressMsg{Code: 'g', Text: "g"})
	if last := lastRow(plain.l); !strings.HasSuffix(last, " g  ") {
		t.Errorf("without colour, the last row reads %q, want the keycap all the same", last)
	}
}

// The watch reads the readings history as the scenario has it: its base's,
// then those its answers have read, each once it's read.
func TestTheWatchReadsTheReadingsTheScenarioReadsAfterItsBases(t *testing.T) {
	hundredth := answer{wait: brief.wait, streams: brief.streams, chars: int(0.01 / sessionPerChar)}
	tl, c := timelineOf(testScenario(asks(4, idDB8A, opus, hundredth)), 0)
	_, read, _ := tl.readers()
	start := tl.base.start
	from, to := start.Add(-time.Hour), start.Add(time.Hour)

	before := slices.Collect(read.Between(from, to))
	c.advance(5 * time.Second)
	after := slices.Collect(read.Between(from, to))
	tail := read.Tail(from, start.Add(-time.Minute), to)()

	first := start.Add(4*time.Second + hundredth.wait)
	if len(after) != len(before)+1 || !slices.Equal(after[:len(before)], before) {
		t.Fatalf("read again, the readings are %d, want the %d before and one more", len(after), len(before))
	}
	for _, got := range []readings.Reading{after[len(after)-1], tail[len(tail)-1]} {
		if got.Account != "side" || got.Key != fiveHourKey || got.Utilization != .14 || !got.At.Equal(first) {
			t.Errorf("the last reading read is %+v, want side's 5-hour window at 14%%, as the answer's first byte came", got)
		}
	}
}

// Where the router says where auto would send a session's next request, it
// says so as its own rule chooses at each listing, the global pin counting:
// 9c2e kept on personal, then, a pin moving running sessions to side given,
// sent there.
func TestWhereAutoWouldSendASessionIsAsTheRouterChoosesIt(t *testing.T) {
	b := testScenario().based()
	at := b.start.Add(time.Second)
	pinned := order{at: at, give: func(w *world) { w.setPin([]string{"side"}, true, at) }}
	for _, tt := range []struct {
		orders []order
		want   string
	}{
		{want: "personal"},
		{orders: []order{pinned}, want: "side"},
	} {
		p := b.played(tt.orders, at)
		sessions := p.world.listing(at, p.told)
		if got := listed(t, sessions, id9C2E).Assignments[0].Auto; got != tt.want {
			t.Errorf("with %d orders, the router says auto would send 9c2e to %q, want %q", len(tt.orders), got, tt.want)
		}
		if got := listed(t, sessions, id41E0).Assignments[0].Auto; got != "" {
			t.Errorf("the router says auto would send 41e0 to %q, want nothing, as its world says nothing of it", got)
		}
	}
}

// timelineOf is the router the scenario plays as, its base built, and its
// clock, at the given time into it, waiting as the clock's wait does.
func timelineOf(s Scenario, at time.Duration) (*timeline, *testClock) {
	b := s.based()
	c := &testClock{now: b.start.Add(at)}
	return &timeline{base: b, now: c.read, wait: c.wait}, c
}

// heldStream is the watch's waits on the request stream as a test holds
// them: none runs, so the stream never blocks a test driving the model.
func heldStream(tea.Cmd) tea.Cmd {
	return nil
}

// testClock is a scenario's clock in a test: a wait moves it on at once, but
// where held, the first, which waits till held is closed; and it keeps how
// long each wait was.
type testClock struct {
	mu     sync.Mutex
	now    time.Time
	held   chan struct{}
	waited []time.Duration
}

// read reads the clock.
func (c *testClock) read() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// advance moves the clock on by d.
func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// set moves the clock on to t, where it's later.
func (c *testClock) set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t.After(c.now) {
		c.now = t
	}
}

// since reads how long the clock has run from start, as a scenario's elapsed
// does: safe to read as the stream it opens reads it, from a goroutine of its
// own.
func (c *testClock) since(start time.Time) func() time.Duration {
	return func() time.Duration { return c.read().Sub(start) }
}

// wait moves the clock on by d, as a stream's wait does, once held is
// closed where it's the first: reporting false where ctx ends first.
func (c *testClock) wait(ctx context.Context, d time.Duration) bool {
	c.mu.Lock()
	held := c.held
	c.held = nil
	c.waited = append(c.waited, d)
	c.mu.Unlock()
	if held != nil {
		select {
		case <-held:
		case <-ctx.Done():
			return false
		}
	}
	c.advance(d)
	return ctx.Err() == nil
}

// waits are how long each wait so far was.
func (c *testClock) waits() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.waited)
}

// movesOf are the moves of the session's requests the timeline's router
// tells of as its scenario plays to its end, with the orders given it, each
// as from → to, and why.
func movesOf(tl *timeline, session string) []string {
	_, orders := tl.moment()
	var moves []string
	for _, e := range tl.base.played(orders, time.Time{}).told {
		if e.Kind == router.StreamMoved && e.Session == session {
			moves = append(moves, e.From+" → "+e.To+", "+e.Reason)
		}
	}
	return moves
}

// receive is what the stream tells next, failing the test should it tell
// nothing for a while.
func receive(t *testing.T, events <-chan router.StreamEvent) router.StreamEvent {
	t.Helper()
	select {
	case e := <-events:
		return e
	case <-time.After(5 * time.Second):
		t.Fatal("the stream told nothing more")
		return router.StreamEvent{}
	}
}

// drive gives the live model what cmd sends back, then each message, and
// whatever their commands send back, until none is left, as Bubble Tea would,
// but at once: its timers held, it reads the router as it stands.
func drive(l live, cmd tea.Cmd, msgs ...tea.Msg) live {
	queue := append(run(cmd), msgs...)
	for len(queue) > 0 {
		next, cmd := l.Update(queue[0])
		l = next.(live)
		queue = append(queue[1:], run(cmd)...)
	}
	return l
}

// lastRow is the live model's last row, as text alone.
func lastRow(l live) string {
	rows := strings.Split(ansi.Strip(l.View().Content), "\n")
	return rows[len(rows)-1]
}
