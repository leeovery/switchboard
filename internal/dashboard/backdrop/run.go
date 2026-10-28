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
	_, err := tea.NewProgram(tracked[S]{screen: screen, last: &last}, append(opts, tea.WithOutput(out))...).Run()
	last.Backdrop().PutBack(out)
	return last, err
}

// tracked is the screen as Bubble Tea runs it, each step it takes noted in
// last: Bubble Tea gives back no model from a panic it recovers from, and
// the background the screen set is put back all the same.
type tracked[S Screen] struct {
	screen S
	last   *S
}

// Init starts the screen as it starts itself.
func (t tracked[S]) Init() tea.Cmd {
	return t.screen.Init()
}

// Update takes in a message as the screen does, noting where it leaves it.
func (t tracked[S]) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := t.screen.Update(msg)
	*t.last = next.(S)
	return tracked[S]{screen: *t.last, last: t.last}, cmd
}

// View draws the screen as it draws itself.
func (t tracked[S]) View() tea.View {
	return t.screen.View()
}
