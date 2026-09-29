package setup

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/leeovery/switchboard/internal/redact"
)

// errNoAnswer is what setup stops with when the user's input ends before
// they answer.
var errNoAnswer = errors.New("the input ended without an answer, so setup stops here: run it again to carry on")

// Terminal is the terminal the user is at.
type Terminal struct {
	// In is what the user types: an answer a line.
	In io.Reader
	// Out is where setup asks, and says what it finds and does.
	Out io.Writer
	// Hidden reads a line the user types without showing it, as a token is
	// typed.
	Hidden func() ([]byte, error)
}

// sayf tells the user a line. One that can't be shown is let go: what it
// tells of goes ahead all the same.
func (t Terminal) sayf(format string, args ...any) {
	_, _ = fmt.Fprintf(t.Out, format, args...)
	_, _ = fmt.Fprintln(t.Out)
}

// heading sets the step numbered n apart, by its title.
func (t Terminal) heading(n int, title string) {
	t.sayf("\n%d. %s", n, title)
}

// ask asks question, and returns the answer, its spaces trimmed. It fails
// with errNoAnswer when the input ends first. An answer shaped like a token,
// pasted where it shows, is never taken, nor repeated: it's asked again.
func (t Terminal) ask(question string) (string, error) {
	for {
		_, _ = io.WriteString(t.Out, question)
		answer, err := t.line()
		switch {
		case errors.Is(err, io.EOF):
			_, _ = fmt.Fprintln(t.Out)
			return "", errNoAnswer
		case err != nil:
			return "", fmt.Errorf("read the answer: %w", err)
		case redact.HoldsToken(answer):
			t.sayf("That looks like a token, which isn't asked for here: setup asks for each token where it won't show.")
			continue
		}
		return strings.TrimSpace(answer), nil
	}
}

// confirm asks a question that takes yes or no, and asks again until it has
// one: Enter gives yes when yes is set, else no.
func (t Terminal) confirm(question string, yes bool) (bool, error) {
	choices := " [y/N] "
	if yes {
		choices = " [Y/n] "
	}
	for {
		answer, err := t.ask(question + choices)
		if err != nil {
			return false, err
		}
		switch strings.ToLower(answer) {
		case "":
			return yes, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
		t.sayf("Answer y or n.")
	}
}

// line reads what the user types up to the end of the line, without it. It
// reads a byte at a time, so it never takes what's typed past the line,
// which a hidden read, reading the terminal itself, wouldn't see.
func (t Terminal) line() (string, error) {
	var line []byte
	b := make([]byte, 1)
	for {
		n, err := t.In.Read(b)
		if n == 1 {
			if b[0] == '\n' {
				return string(line), nil
			}
			line = append(line, b[0])
		}
		switch {
		case errors.Is(err, io.EOF) && len(line) > 0:
			return string(line), nil
		case err != nil:
			return "", err
		}
	}
}
