package setup

import (
	"unicode"
	"unicode/utf8"

	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/service"
)

// Mark is how what a step found stands: done, a warning, or failed.
type Mark int

const (
	Done Mark = iota
	Warning
	Failed
)

// Said is what a step tells of one thing it found or did, in both the forms
// it's told in: a mark, a label of a few words and the detail after it, as
// setup's screen draws a row, and the sentence the line walk prints.
type Said struct {
	Mark Mark
	// Label is "" for a Said that's the line walk's alone, which the screen
	// doesn't draw.
	Label string
	// Detail is the rest, which the screen wraps.
	Detail string
	// Sentence is the line walk's, as it prints it: a line, or more; "" for a
	// Said that's the screen's alone, which the line walk doesn't print.
	Sentence string
}

// Serviced is what the service step told, in turn, and the health of the
// service's router, where the step saw it answer: nil where it didn't. Swap
// is how far the step's swap of the service got, as service.Install says,
// where the step began one, which runs to its end whatever stops the step.
type Serviced struct {
	Said   []Said
	Router *router.Health
	Swap   service.Swap
}

// capital is text with its first letter a capital, as a sentence starts.
func capital(text string) string {
	if text == "" {
		return text
	}
	first, size := utf8.DecodeRuneInString(text)
	return string(unicode.ToUpper(first)) + text[size:]
}

// lowered is text with its first letter a small one, as a label reads
// within a sentence.
func lowered(text string) string {
	if text == "" {
		return text
	}
	first, size := utf8.DecodeRuneInString(text)
	return string(unicode.ToLower(first)) + text[size:]
}
