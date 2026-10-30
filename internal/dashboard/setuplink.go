package dashboard

import (
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/setup"
)

// SetupLinkPage is what the claude link step's page has of its own, beside
// what the step found, which the summary holds: whether the step asks the
// user's login shell now which claude it runs; whether the step has ended,
// installing the session hook, which it does once, whatever it found; and
// the session hook's row, once it's installed.
type SetupLinkPage struct {
	Asking, Ended bool
	Hook          *setup.Said
}

// What the claude link page says of its own: what the claude command is
// for; what it says while it asks the shell; the label of its row where the
// step failed; and where asking again can tell the link is put right by
// hand, how to have it ask.
const (
	linkPurpose  = "The claude command starts Claude Code through switchboard, so every session is routed."
	linkAsking   = "Asking your shell, as a new terminal starts it, which claude it runs"
	linkNotSetUp = "Not set up"
	pressR       = "Then press r: setup asks your shell again, as a new terminal would start it."
)

// The claude link page's keys, where it leaves the link to put right by
// hand: d copies the directory, as c copies the line, and r asks the shell
// again; and enter leaves it for now.
var (
	copyTheDir = Key{Key: "d", Does: "copy the directory"}
	askAgain   = Key{Key: "r", Does: "check again"}
	leaveIt    = onward("leave it for now")
)

// linkPage is the claude link step's page: CLAUDE LINK, what the claude
// command is for, and what the step found, a fact a row, the link's first.
// While the step asks the shell, it says so in the shell's place; where the
// step failed, it says why. Once the step has ended, the session hook's row
// follows the shell's, once it's installed; till then, where the shell runs
// another claude, what's left to put right by hand, and the keys to copy it
// and to ask again, as the step's remedy has them, and enter leaves it for
// now. Its onward key moves on to the Skill.
func (s Setup) linkPage() setupPage {
	p := setupPage{name: "CLAUDE LINK", purpose: []line{{{linkPurpose, secondaryInk}}}, onward: onward("next: " + SetupSkill.Title())}
	found := s.Link
	if found != nil {
		p.facts = slices.Clone(found.Said)
	}
	switch {
	case s.Failure != "":
		p.facts = append(p.facts, setup.Said{Mark: setup.Failed, Label: linkNotSetUp, Detail: s.Failure})
	case s.Linking.Asking:
		p.underWay = waiting(s.Now, linkAsking)
	case found == nil:
	case s.Linking.Ended:
		p.facts = append(p.facts, found.Shell.Said)
		if s.Linking.Hook != nil {
			p.facts = append(p.facts, *s.Linking.Hook)
		}
	default:
		p.facts = append(p.facts, found.Shell.Said)
		if r, ok := s.remedy(); ok {
			p.byHand, p.keys = remedied(r), remedyKeys(r)
			p.onward = leaveIt
		}
	}
	return p
}

// remedy is what the claude link step leaves to put right by hand, and
// whether its page offers it: once the step has found the shell runs
// another claude, till the step ends, while it isn't asking again, and
// hasn't failed.
func (s Setup) remedy() (setup.Remedy, bool) {
	if s.Link == nil || s.Linking.Asking || s.Linking.Ended || s.Failure != "" {
		return setup.Remedy{}, false
	}
	r := s.Link.Shell.Remedy()
	return r, len(r.Parts) > 0 || r.Checkable
}

// remedied is how the page says to put r right by hand: each of its parts,
// in setup's words; then to press r, where asking the shell again can tell.
func remedied(r setup.Remedy) []handPart {
	var parts []handPart
	for _, part := range r.Parts {
		parts = append(parts, handPart{says: part.Says, put: part.Put})
	}
	if r.Checkable {
		parts = append(parts, handPart{says: pressR, aside: true})
	}
	return parts
}

// remedyKeys are the page's keys for r: c to copy the line, d the
// directory, and r to ask again, each where r has it.
func remedyKeys(r setup.Remedy) []Key {
	var keys []Key
	if r.Line != "" {
		keys = append(keys, copyLine)
	}
	if r.Dir != "" {
		keys = append(keys, copyTheDir)
	}
	if r.Checkable {
		keys = append(keys, askAgain)
	}
	return keys
}

// linkCopies is what the claude link page copies at the key: c the line,
// and d the directory, where it offers them; "" otherwise.
func (s Setup) linkCopies(key string) string {
	r, ok := s.remedy()
	switch {
	case !ok:
		return ""
	case key == copyLine.Key:
		return r.Line
	case key == copyTheDir.Key:
		return r.Dir
	}
	return ""
}

// handPart is a part of what a page says to put right by hand: what to do,
// in text.secondary, or muted where it's an aside; and under it, what to put
// in place, to copy, where there's anything.
type handPart struct {
	says  string
	put   string
	aside bool
}

// The parts of a row of what's to put in place by hand, as handParts lays
// them out.
const (
	handIndent = iota
	handPut
)

// handPutAt is the column what's to put in place by hand is set out from,
// in from what's said of it.
const handPutAt = 6

// handParts are a row of what's to put in place by hand, across width cells:
// handPutAt cells, then what's to put in place, taking the rest.
func handParts(width int) []flex.Span {
	spans, _ := flex.Lay(width, []flex.Part{
		handIndent: {Least: handPutAt, Natural: handPutAt},
		handPut:    {Least: 1, Grows: true},
	}...)
	return spans
}

// draw draws the part from row y, width cells wide: what it says, from
// setupIndent, wrapped where it's wider than the page; then a row on, what's
// to put in place, in accent.mode, wrapped, words wider than its room
// broken, never cut, as it's to be copied whole. It returns the row after
// the last it drew on.
func (h handPart) draw(c *canvas, y, width int) int {
	k := secondaryInk
	if h.aside {
		k = mutedInk
	}
	for _, l := range (line{{h.says, k}}).wrap(width - setupIndent) {
		c.line(setupIndent, y, l)
		y++
	}
	if h.put == "" {
		return y
	}
	put := handParts(width)[handPut]
	y++
	for part := range strings.SplitSeq(ansi.Wrap(h.put, put.Size, ""), "\n") {
		c.line(put.At, y, line{{part, accentInk}})
		y++
	}
	return y
}
