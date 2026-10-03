package dashboard

import (
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// chunk is a part of what a slot of the heading says, and whether it starts a
// line of its own.
type chunk struct {
	text  line
	fresh bool
}

// fresh is a chunk that starts a line of its own.
func fresh(l line) chunk {
	return chunk{text: l, fresh: true}
}

// flow lays chunks out on lines as wide as widths says, a line for each
// width at most. A chunk follows on the line before, after sep, where it
// fits and doesn't start a line of its own; else it takes the next, broken
// onto the lines after as broken says where it's too long. With no line
// left, it follows on the last all the same, which is cut short.
func flow(chunks []chunk, widths []int, sep string) []line {
	var lines []line
	for _, c := range chunks {
		n := len(lines)
		var follow line
		if n > 0 {
			follow = slices.Concat(lines[n-1], line{{sep, mutedInk}}, c.text)
		}
		switch {
		case len(c.text) == 0:
		case n > 0 && !c.fresh && follow.width() <= widths[n-1]:
			lines[n-1] = follow
		case n < len(widths):
			lines = append(lines, broken(c.text, widths[n:])...)
		case n > 0:
			lines[n-1] = follow.fit(widths[n-1])
		}
	}
	return lines
}

// broken lays l out on lines as wide as widths says, a line for each width
// at most: as much as fits on each, between words, where it's one span, its
// last line taking the rest, cut short; else on its first line alone, cut
// short.
func broken(l line, widths []int) []line {
	if l.width() <= widths[0] || len(l) != 1 {
		return []line{l.fit(widths[0])}
	}
	var lines []line
	words := strings.Fields(l[0].text)
	for i := 0; len(words) > 0 && i < len(widths); i++ {
		taken := 1
		for taken < len(words) && ansi.StringWidth(strings.Join(words[:taken+1], " ")) <= widths[i] {
			taken++
		}
		if i == len(widths)-1 {
			taken = len(words)
		}
		lines = append(lines, line{{truncate(strings.Join(words[:taken], " "), widths[i]), l[0].ink}})
		words = words[taken:]
	}
	return lines
}

// together runs the chunks together on one line, sep between each.
func together(chunks []chunk, sep string) line {
	var l line
	for _, c := range chunks {
		if len(c.text) == 0 {
			continue
		}
		if l != nil {
			l = append(l, span{sep, mutedInk})
		}
		l = append(l, c.text...)
	}
	return l
}
