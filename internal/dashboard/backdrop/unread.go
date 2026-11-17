package backdrop

import "github.com/charmbracelet/x/term"

// Unread throws away, once a screen reading the terminal f stops, what was
// typed at it and is yet to be read, as Run has it: nothing where f isn't a
// terminal, as a screen then reads one of Bubble Tea's own opening, nor off
// macOS, as dropTyped has it.
func Unread(f term.File) func() {
	return func() {
		if !term.IsTerminal(f.Fd()) {
			return
		}
		if err := dropTyped(f.Fd()); err != nil {
			logger.Warn("throw away what was typed unread", "err", err)
		}
	}
}
