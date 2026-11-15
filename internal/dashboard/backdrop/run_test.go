package backdrop_test

import (
	"bytes"
	"runtime"
	"testing"
	"weak"

	tea "charm.land/bubbletea/v2"

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
	last, err := backdrop.Run(stepped{}, &out, tea.WithInput(nil), tea.WithoutSignals(), tea.WithFilter(heed))
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
	if _, err := backdrop.Run(startingWithPayload(&let), &out, tea.WithInput(nil), tea.WithoutSignals()); err != nil {
		t.Fatal(err)
	}
	if !let {
		t.Error("the payload the screen started with was still held once a step replaced it, want it let go")
	}
}
