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

	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

func TestTheRouterAnswersAsTheScenarioHasPlayedUpToItsClock(t *testing.T) {
	s := testScenario(starts(1, idB3E9, opus, brief))
	c := &testClock{now: s.start}
	tl := &timeline{scenario: s, now: c.read}

	before, health, err := tl.Read(t.Context(), status.Read{})
	if err != nil {
		t.Fatal(err)
	}
	c.advance(2 * time.Second)
	after, again, _ := tl.Read(t.Context(), status.Read{})
	sessions, _ := tl.Sessions(t.Context())

	if before.Sessions != 5 || after.Sessions != 6 {
		t.Errorf("the router counts %d sessions, then %d, want 5, then 6, b3e9 started", before.Sessions, after.Sessions)
	}
	if !after.GeneratedAt.Equal(s.start.Add(2*time.Second)) || after.Events[0].Session != idB3E9 {
		t.Errorf("the document read 2s in was built at %s, its newest event %+v, want built then, telling of b3e9 starting", after.GeneratedAt, after.Events[0])
	}
	if after.Events[1].ID != before.Events[0].ID {
		t.Errorf("the router numbers its events afresh, from %d to %d, want each to keep its id", before.Events[0].ID, after.Events[1].ID)
	}
	if !slices.ContainsFunc(sessions, func(l status.Session) bool { return l.ID == idB3E9 }) {
		t.Errorf("the router lists %+v, want b3e9 among them", sessions)
	}
	if !health.Same(again) || !tl.RouterAnswers(t.Context()) {
		t.Error("read again, the router is another, or doesn't answer, want the same router throughout, answering")
	}
}

func TestOrdersAreTakenFromWhenTheyreGiven(t *testing.T) {
	s := testScenario(starts(0.5, id9E21, sonnet, brief), starts(3, idB3E9, opus, brief))
	c := &testClock{now: s.start}
	tl := &timeline{scenario: s, now: c.read}

	c.advance(time.Second)
	if err := tl.Pin(t.Context(), []string{"work"}, false); err != nil {
		t.Fatal(err)
	}
	if err := tl.PinSession(t.Context(), idC61B, "work"); err != nil {
		t.Fatal(err)
	}
	c.advance(3 * time.Second)

	doc, _, _ := tl.Read(t.Context(), status.Read{})
	if !doc.Pin.Has("work") || !doc.Pin.Since.Equal(s.start.Add(time.Second)) || doc.Best != "work" {
		t.Errorf("the document's pin is %+v, its best %q, want work's, given 1s in, and work", doc.Pin, doc.Best)
	}
	sessions, _ := tl.Sessions(t.Context())
	if on := listed(t, sessions, id9E21).Assignments[0].Account; on != "side" {
		t.Errorf("9e21, started before the pin, is on %s, want side, the best then", on)
	}
	if on := listed(t, sessions, idB3E9).Assignments[0].Account; on != "work" {
		t.Errorf("b3e9, started after the pin, is on %s, want work", on)
	}
	if c61b := listed(t, sessions, idC61B); c61b.Pin != "work" || c61b.Assignments[0].Account != "side" || c61b.Assignments[0].Pinned {
		t.Errorf("the router lists c61b as %+v, want its own pin to work, its opus on side till its next request", c61b)
	}
	if err := tl.Unpin(t.Context()); err != nil {
		t.Fatal(err)
	}
	if doc, _, _ := tl.Read(t.Context(), status.Read{}); !doc.Pin.IsZero() || doc.Best != "side" {
		t.Errorf("unpinned, the document's pin is %+v, its best %q, want none, and side", doc.Pin, doc.Best)
	}
}

func TestASessionUnpinnedOnceItsPinMovedItIsRoutedOnItsMerits(t *testing.T) {
	s := testScenario(asks(2, idD28C, opus, brief), asks(5, idD28C, opus, brief))
	c := &testClock{now: s.start.Add(time.Second)}
	tl := &timeline{scenario: s, now: c.read}

	if err := tl.PinSession(t.Context(), idD28C, "side"); err != nil {
		t.Fatal(err)
	}
	c.advance(1500 * time.Millisecond)
	sessions, _ := tl.Sessions(t.Context())
	if d28c := listed(t, sessions, idD28C); d28c.Pin != "side" || d28c.Assignments[0].Account != "side" ||
		!d28c.Assignments[0].Pinned || !d28c.Assignments[0].PinnedAt.Equal(s.start.Add(time.Second)) {
		t.Errorf("its pin having moved it, the router lists d28c as %+v, want it on side, pinned there since 1s", d28c)
	}
	if err := tl.UnpinSession(t.Context(), idD28C); err != nil {
		t.Fatal(err)
	}
	c.advance(3 * time.Second)

	sessions, _ = tl.Sessions(t.Context())
	if d28c := listed(t, sessions, idD28C); d28c.Pin != "" || d28c.Assignments[0].Account != "side" ||
		d28c.Assignments[0].Pinned || !d28c.Assignments[0].PinnedAt.IsZero() {
		t.Errorf("unpinned, the router lists d28c as %+v, want it on side, where its pin moved it, pinned nowhere", d28c)
	}
	if moves := movesOf(tl, idD28C); !slices.Equal(moves, []string{"work → side, pinned"}) {
		t.Errorf("d28c moved %q, want once, by its pin, its next request staying on side", moves)
	}
}

func TestAPinThatMovesRunningSessionsMovesEachOnce(t *testing.T) {
	s := testScenario(
		asks(2, idD28C, opus, brief),
		asks(3, id7F3A, haiku, brief),
		asks(5, idD28C, opus, brief),
		asks(7, idD28C, opus, brief),
		asks(8, id7F3A, haiku, brief),
	)
	c := &testClock{now: s.start.Add(time.Second)}
	tl := &timeline{scenario: s, now: c.read}

	if err := tl.Pin(t.Context(), []string{"side"}, true); err != nil {
		t.Fatal(err)
	}
	c.advance(3 * time.Second)
	if err := tl.PinSession(t.Context(), idD28C, "work"); err != nil {
		t.Fatal(err)
	}
	c.advance(2 * time.Second)
	if err := tl.UnpinSession(t.Context(), idD28C); err != nil {
		t.Fatal(err)
	}

	// d28c, put back on work by its own pin after the pin that moved it, stays
	// there once that's cleared: the router moves a session once a pin.
	if moves := movesOf(tl, idD28C); !slices.Equal(moves, []string{"work → side, moved by pin", "side → work, pinned"}) {
		t.Errorf("d28c moved %q, want to side by the pin, back to work by its own pin, and no more", moves)
	}
	if moves := movesOf(tl, id7F3A); !slices.Equal(moves, []string{"work → side, moved by pin"}) {
		t.Errorf("7f3a moved %q, want to side by the pin, once", moves)
	}
}

func TestTheStreamTellsOfTheRequestsInFlightThenOfEachEventAsItsTimeComes(t *testing.T) {
	s := testScenario(asks(1, idDB8A, sonnet, brief), asks(2, idD28C, opus, brief))
	c := &testClock{now: s.start.Add(1500 * time.Millisecond)}
	tl := &timeline{scenario: s, now: c.read, wait: c.wait}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	events, err := tl.Stream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var told []router.StreamEvent
	for len(told) < 12 {
		told = append(told, receive(t, events))
		if e := told[len(told)-1]; e.At.After(c.read()) {
			t.Errorf("the stream told %+v at %s, before its time", e, c.read())
		}
	}
	if joined := told[0]; joined.Kind != router.StreamInFlight || joined.Session != idDB8A || !joined.SentAt.Equal(s.start.Add(time.Second)) {
		t.Errorf("the stream opens telling %+v, want db8a's request in flight, sent a second in", joined)
	}
	if first := told[1]; first.Kind != router.StreamFirst || !first.At.Equal(s.start.Add(1700*time.Millisecond)) || c.waits()[0] != 200*time.Millisecond {
		t.Errorf("then, having waited %v, it tells %+v, want db8a's answer's first byte, 200ms on", c.waits(), first)
	}
	if !slices.IsSortedFunc(told[1:], func(a, b router.StreamEvent) int { return a.At.Compare(b.At) }) {
		t.Errorf("it tells %+v out of order, want each event as its time comes", told)
	}
	if sent := told[3]; sent.Kind != router.StreamSent || sent.Session != idD28C || !sent.At.Equal(s.start.Add(2*time.Second)) {
		t.Errorf("it tells %+v fourth, want d28c's request going out, 2s in", sent)
	}
	cancel()
	for range events {
	}
}

func TestAnOrderGivenWhileTheStreamIsOpenChangesWhatItTellsNext(t *testing.T) {
	s := testScenario(asks(1, idD28C, opus, brief))
	held := make(chan struct{})
	c := &testClock{now: s.start, held: held}
	tl := &timeline{scenario: s, now: c.read, wait: c.wait}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	events, _ := tl.Stream(ctx)
	if err := tl.PinSession(t.Context(), idD28C, "side"); err != nil {
		t.Fatal(err)
	}
	close(held)

	moved, sent := receive(t, events), receive(t, events)
	if moved.Kind != router.StreamMoved || moved.From != "work" || moved.To != "side" || moved.Reason != status.ReasonPinned {
		t.Errorf("the stream tells %+v, want d28c moved from work to side, by its pin", moved)
	}
	if sent.Kind != router.StreamSent || sent.Account != "side" {
		t.Errorf("then %+v, want its request going out on side", sent)
	}
}

func TestTheStreamClosesAsItsContextEnds(t *testing.T) {
	s := testScenario()
	tl := &timeline{scenario: s, now: (&testClock{now: s.start}).read, wait: waitFor}
	ctx, cancel := context.WithCancel(t.Context())

	events, _ := tl.Stream(ctx)
	cancel()
	select {
	case _, open := <-events:
		if open {
			t.Error("the stream told of something, with nothing to tell, want it closed")
		}
	case <-time.After(5 * time.Second):
		t.Error("the stream stayed open once its context ended, want it closed")
	}
}

func TestTheClockRunsFromTheScenariosStart(t *testing.T) {
	s := testScenario()
	var elapsed time.Duration
	l := s.play(func() time.Duration { return elapsed }, waitFor, held)

	l = drive(l, l.Init())
	if frame := ansi.Strip(l.View().Content); !strings.Contains(frame, "Thu 1 Oct  14:42:07") {
		t.Fatalf("starting, it draws\n%s\nwant it drawn at once, at the scenario's start", frame)
	}
	elapsed = 12 * time.Second
	if frame := ansi.Strip(l.View().Content); !strings.Contains(frame, "Thu 1 Oct  14:42:19") {
		t.Errorf("12s in, it draws\n%s\nwant its clock 12s on", frame)
	}
}

func TestKeysAreShownAsTheyrePressedTillTheyveHadTheirTime(t *testing.T) {
	s := testScenario()
	var elapsed time.Duration
	l := s.play(func() time.Duration { return elapsed }, waitFor, held)
	l = drive(l, l.Init())

	l = drive(l, nil, tea.KeyPressMsg{Code: tea.KeyRight}, tea.KeyPressMsg{Code: tea.KeyRight})
	elapsed = typedFor - time.Millisecond
	l = drive(l, nil, tea.KeyPressMsg{Code: 'w', Text: "w"})
	if last := lastRow(l); !strings.HasSuffix(last, " →   →   w  ") {
		t.Errorf("the last row reads %q, want the keys pressed, in turn, at its right", last)
	}
	if last := lastRow(l); strings.Contains(last, "read") || !strings.Contains(last, "q quit") {
		t.Errorf("the last row reads %q, want the footer's keys, but nothing of how long ago it was read, which the keys cut into", last)
	}
	elapsed += typedFor - time.Millisecond
	if last := lastRow(l); !strings.HasSuffix(last, " w  ") {
		t.Errorf("as w has its time, the last row reads %q, want it still shown", last)
	}
	elapsed += time.Millisecond
	if last := lastRow(l); strings.HasSuffix(last, " w  ") {
		t.Errorf("once the keys have had their time, the last row reads %q, want them gone", last)
	}
}

func TestTheLatestKeysPressedAreShown(t *testing.T) {
	var keys typed
	at := moment(time.UTC)
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

func TestKeysAreNamedAsTheFooterNamesThem(t *testing.T) {
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
		{key: tea.KeyPressMsg{Code: tea.KeyEnter}, want: "⏎"},
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
	s := testScenario()
	l := s.play(func() time.Duration { return 0 }, waitFor, held)
	l = drive(l, l.Init(), tea.KeyPressMsg{Code: 'g', Text: "g"})

	content := l.View().Content
	if last := content[strings.LastIndex(content, "\n")+1:]; !strings.Contains(last, "48;2;129;161;193") {
		t.Errorf("the keycap is drawn %q, want it on nord's accent.key, the theme shown", last)
	}
	plain := s.WithoutColour().play(func() time.Duration { return 0 }, waitFor, held)
	plain = drive(plain, plain.Init(), tea.KeyPressMsg{Code: 'g', Text: "g"})
	if last := lastRow(plain); !strings.HasSuffix(last, " g  ") {
		t.Errorf("without colour, the last row reads %q, want the keycap all the same", last)
	}
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
	for _, e := range tl.scenario.played(orders, time.Time{}).told {
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
