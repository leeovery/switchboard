package backdrop_test

import (
	"bytes"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"
	"weak"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard/backdrop"
)

func TestTheMouseMovingOrLetGoNeverReachesTheScreen(t *testing.T) {
	tests := []struct {
		name   string
		msg    tea.Msg
		heeded bool
	}{
		{name: "a drag, a cell on", msg: tea.MouseMotionMsg{X: 4, Y: 7, Button: tea.MouseLeft}},
		{name: "a button let go", msg: tea.MouseReleaseMsg{X: 4, Y: 7, Button: tea.MouseLeft}},
		{name: "a click", msg: tea.MouseClickMsg{X: 4, Y: 7, Button: tea.MouseLeft}, heeded: true},
		{name: "the wheel", msg: tea.MouseWheelMsg{X: 4, Y: 7, Button: tea.MouseWheelDown}, heeded: true},
		{name: "a key", msg: tea.KeyPressMsg{Code: 'j', Text: "j"}, heeded: true},
	}
	for _, tt := range tests {
		if got := backdrop.Heeded(nil, tt.msg); (got != nil) != tt.heeded || tt.heeded && got != tt.msg {
			t.Errorf("%s reaches the screen as %v; want it to reach it %v", tt.name, got, tt.heeded)
		}
	}
}

// stepped is a screen counting the steps it's told to take, which quits
// once it has taken one.
type stepped struct {
	steps int
}

// step is a step a stepped screen takes.
type step struct{}

func (s stepped) Init() tea.Cmd {
	return func() tea.Msg { return step{} }
}

func (s stepped) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(step); ok {
		s.steps++
		return s, tea.Quit
	}
	return s, nil
}

func (s stepped) View() tea.View {
	return tea.NewView("")
}

func (s stepped) Backdrop() backdrop.Backdrop {
	return backdrop.Backdrop{}
}

func TestAFilterHeedingTheScreenIsGivenItAsItsLastStepLeftIt(t *testing.T) {
	// Bubble Tea reads these itself: set, they have it write a trace, or a
	// log of a panic it recovers from in the working directory.
	t.Setenv("TEA_TRACE", "")
	t.Setenv("TEA_DEBUG", "")
	quitting := -1
	heed := backdrop.Heeding(func(s stepped, msg tea.Msg) tea.Msg {
		if _, ok := msg.(tea.QuitMsg); ok {
			quitting = s.steps
		}
		return msg
	})
	var out bytes.Buffer
	last, err := backdrop.Run(stepped{}, &out, func() {}, tea.WithInput(nil), tea.WithoutSignals(), tea.WithFilter(heed))
	if err != nil || last.steps != 1 {
		t.Fatalf("Run() = %+v, %v; want the screen quit after its step", last, err)
	}
	if quitting != 1 {
		t.Errorf("the filter saw the screen quit having taken %d steps, want the one it took", quitting)
	}
}

func TestAFilterHeedingTheScreenPassesOverAModelRunDidntWrap(t *testing.T) {
	heed := backdrop.Heeding(func(stepped, tea.Msg) tea.Msg {
		t.Error("the filter was told of a model Run didn't wrap")
		return nil
	})
	if got := heed(stepped{}, step{}); got != (step{}) {
		t.Errorf("the filter gave %v, want the message as it was", got)
	}
}

// payload is what a screen holds, as the dashboard's model holds its ledger.
type payload struct {
	read []byte
}

// replacing is a screen whose first step replaces the payload it started
// with, then has it collected and notes, in let, whether the payload it
// started with, first, was let go: as a turn has the dashboard's model let
// go of the ledger it opened with.
type replacing struct {
	held  *payload
	first weak.Pointer[payload]
	let   *bool
}

// collected is what the screen found of the payload it started with, once
// it was replaced and collected.
type collected struct {
	let bool
}

func (s replacing) Init() tea.Cmd {
	return func() tea.Msg { return step{} }
}

func (s replacing) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case step:
		s.held = &payload{read: make([]byte, 1<<10)}
		first := s.first
		return s, func() tea.Msg {
			runtime.GC()
			return collected{let: first.Value() == nil}
		}
	case collected:
		*s.let = msg.let
		return s, tea.Quit
	}
	return s, nil
}

func (s replacing) View() tea.View {
	return tea.NewView("")
}

func (s replacing) Backdrop() backdrop.Backdrop {
	return backdrop.Backdrop{}
}

// startingWithPayload is a replacing screen and the payload it starts with,
// which nothing but the screen holds.
func startingWithPayload(let *bool) replacing {
	held := &payload{read: make([]byte, 1<<10)}
	return replacing{held: held, first: weak.Make(held), let: let}
}

// Nothing of a screen a step has left behind stays reachable while Bubble
// Tea runs: the payload a screen started with, once a step replaces it, is
// collected, though Bubble Tea keeps the model it was started with till it
// stops.
func TestAScreenAStepLeftBehindIsLetGoWhileItRuns(t *testing.T) {
	t.Setenv("TEA_TRACE", "")
	t.Setenv("TEA_DEBUG", "")
	var let bool
	var out bytes.Buffer
	if _, err := backdrop.Run(startingWithPayload(&let), &out, func() {}, tea.WithInput(nil), tea.WithoutSignals()); err != nil {
		t.Fatal(err)
	}
	if !let {
		t.Error("the payload the screen started with was still held once a step replaced it, want it let go")
	}
}

// stopping is a screen whose background is set, which stops, at its first
// step, as stop has it.
type stopping struct {
	back backdrop.Backdrop
	stop func(stopping) (tea.Model, tea.Cmd)
}

func (s stopping) Init() tea.Cmd {
	return func() tea.Msg { return step{} }
}

func (s stopping) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(step); ok {
		return s.stop(s)
	}
	return s, nil
}

func (s stopping) View() tea.View {
	return tea.NewView("")
}

func (s stopping) Backdrop() backdrop.Backdrop {
	return s.back
}

// However a screen stops that Bubble Tea catches, Run puts back the
// terminal's background, then, last, once Bubble Tea has written its last,
// throws away what was typed and never read, once.
func TestRunThrowsAwayWhatWasTypedUnreadLastHoweverTheScreenStops(t *testing.T) {
	t.Setenv("TEA_TRACE", "")
	t.Setenv("TEA_DEBUG", "")
	stops := []struct {
		name      string
		stop      func(stopping) (tea.Model, tea.Cmd)
		wantError bool
	}{
		{name: "a quit", stop: func(s stopping) (tea.Model, tea.Cmd) { return s, tea.Quit }},
		{name: "an interrupt", stop: func(s stopping) (tea.Model, tea.Cmd) { return s, tea.Interrupt }, wantError: true},
		{name: "a panic Bubble Tea recovers from", stop: func(stopping) (tea.Model, tea.Cmd) { panic("a bug") }, wantError: true},
	}
	for _, tt := range stops {
		t.Run(tt.name, func(t *testing.T) {
			screen := stopping{back: backdrop.Backdrop{}.DrawIn(lakeTheme(t)), stop: tt.stop}
			var out bytes.Buffer
			var unread []string
			_, err := backdrop.Run(screen, &out, func() { unread = append(unread, out.String()) }, tea.WithInput(nil), tea.WithoutSignals())

			if (err != nil) != tt.wantError {
				t.Errorf("Run() error = %v, want an error: %v", err, tt.wantError)
			}
			if !strings.HasSuffix(out.String(), ansi.ResetBackgroundColor) {
				t.Errorf("the screen's output ends %q, want the background put back", out.String())
			}
			if len(unread) != 1 || unread[0] != out.String() {
				t.Errorf("what was typed unread was thrown away %d times, the output then %q; want once, after the screen's last", len(unread), unread)
			}
		})
	}
}

// What a screen reads that isn't a terminal is left as it is: nothing of it
// is thrown away as the screen stops.
func TestUnreadLeavesWhatIsntATerminalAlone(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	if _, err := io.WriteString(w, "typed"); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	backdrop.Unread(r)()
	if got, err := io.ReadAll(r); err != nil || string(got) != "typed" {
		t.Errorf("the pipe holds %q, %v, want what was written to it, unread", got, err)
	}
}
