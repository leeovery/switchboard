package backdrop

import (
	"io"

	tea "charm.land/bubbletea/v2"
)

// Screen is a model of a screen drawn full on the terminal, which may set
// the terminal's background.
type Screen interface {
	tea.Model
	// Backdrop is the terminal's background as the screen has it.
	Backdrop() Backdrop
}

// Run runs screen in Bubble Tea, on out, as opts say, until it stops, and
// puts back the terminal's background as the last step the screen took left
// it, however it stops that Bubble Tea catches: a quit, an interrupt, a
// terminate signal, which it takes as a quit, or a panic it recovers from.
// It returns the screen as that last step left it, and how Bubble Tea's run
// ended.
func Run[S Screen](screen S, out io.Writer, opts ...tea.ProgramOption) (S, error) {
	last := screen
	_, err := tea.NewProgram(tracked[S]{last: &last}, append(opts, tea.WithOutput(out))...).Run()
	last.Backdrop().PutBack(out)
	return last, err
}

// Heeded is msg, for a screen to take, but for the mouse moving, or a
// button let go, which nothing on a screen acts on: a filter dropping them
// before the screen takes them, so a drag, which tells of every cell it
// crosses, never has the screen laid out and drawn again for each.
func Heeded(_ tea.Model, msg tea.Msg) tea.Msg {
	switch msg.(type) {
	case tea.MouseMotionMsg, tea.MouseReleaseMsg:
		return nil
	}
	return msg
}

// Heeding is heed as Bubble Tea's filter of a screen Run runs, given the
// screen as its last step left it, where Bubble Tea gives the filter the
// screen as Run wraps it.
func Heeding[S Screen](heed func(S, tea.Msg) tea.Msg) func(tea.Model, tea.Msg) tea.Msg {
	return func(m tea.Model, msg tea.Msg) tea.Msg {
		t, ok := m.(tracked[S])
		if !ok {
			return msg
		}
		return heed(*t.last, msg)
	}
}

// tracked is the screen as Bubble Tea runs it, each step it takes noted in
// last: Bubble Tea gives back no model from a panic it recovers from, and
// the background the screen set is put back all the same. It holds the
// screen in last alone, which each step overwrites, as Bubble Tea keeps the
// model it was started with till it stops: so nothing of a screen a step
// has left behind stays reachable through it.
type tracked[S Screen] struct {
	last *S
}

// Init starts the screen as it starts itself.
func (t tracked[S]) Init() tea.Cmd {
	return (*t.last).Init()
}

// Update takes in a message as the screen does, noting where it leaves it.
func (t tracked[S]) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := (*t.last).Update(msg)
	*t.last = next.(S)
	return t, cmd
}

// View draws the screen as it draws itself.
func (t tracked[S]) View() tea.View {
	return (*t.last).View()
}
