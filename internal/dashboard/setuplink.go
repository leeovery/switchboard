package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/setup"
)

// SetupLinkPage is what the claude link step's page has of its own, beside
// what the step found, which the summary holds: whether the step asks the
// user's login shell now which claude it runs; whether the step has ended,
// installing the session hook, which it does once, whatever it found; and
// the session hook's row, once it's installed; and the card offering to add
// switchboard's line to the startup file, where it's open.
type SetupLinkPage struct {
	Asking, Ended bool
	Hook          *setup.Said
	Offer         *SetupOfferCard
}

// SetupOfferCard is the card offering to add switchboard's line to the end
// of the startup file the claude link step found without it, open over the
// claude link page, as it stands: whether the line's being added, when it
// takes no key but q.
type SetupOfferCard struct {
	Adding bool
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
// another claude, till the step ends, while it isn't asking again, nor
// offering to add the line, and hasn't failed.
func (s Setup) remedy() (setup.Remedy, bool) {
	if s.Link == nil || s.Linking.Asking || s.Linking.Ended || s.Linking.Offer != nil || s.Failure != "" {
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
	for _, l := range wholeLines(h.put, put.Size) {
		c.line(put.At, y, l)
		y++
	}
	return y
}

// wholeLines are what's to put in place, to copy, in accent.mode, wrapped
// width cells wide, words wider than that broken, never cut, as it's to be
// copied whole.
func wholeLines(put string, width int) []line {
	var lines []line
	for part := range strings.SplitSeq(ansi.Wrap(put, width, ""), "\n") {
		lines = append(lines, line{{part, accentInk}})
	}
	return lines
}

// indented are lines, each set in by cells.
func indented(lines []line, by int) []line {
	in := make([]line, len(lines))
	for i, l := range lines {
		in[i] = append(line{spaces(by)}, l...)
	}
	return in
}

// offerCardWidth is the natural width of the offer card, frame 8's, its
// edges included.
const offerCardWidth = 128

// How far into the offer card's content what it says is set in, and the line
// it offers to add, as frame 8 has them.
const (
	offerSaysAt = 1
	offerLineAt = 3
)

// What the offer card says, the startup file named where each %s is: its
// title; what setup can do; what the line does, and what it leaves; and
// while it's added, that it is.
const (
	offerTitle    = "ADD A LINE TO %s"
	offerCan      = "Setup can add one line to the end of %s, so your shell finds switchboard's claude first:"
	offerHandsOff = "The line hands off to a file of switchboard's own, which puts its claude first and loads its completions."
	offerLeaves   = "It does nothing once switchboard is gone, and nothing else in %s changes."
	offerAdding   = "Adding the line to the end of %s"
)

// offerKeys are the offer card's keys.
var offerKeys = []Key{{Key: "enter", Does: "add it"}, {Key: "n", Does: "I'll add it myself"}}

// offerCard is the offer card open over the claude link page, at the
// screen's time, width cells wide as the dashboard Card's rule has it: nil
// where none is.
func (s Setup) offerCard(width int) *setupCard {
	if s.Linking.Offer == nil || s.Link == nil {
		return nil
	}
	cardWidth, _ := cardAcross(width, offerCardWidth)
	return s.Linking.Offer.card(cardWidth, s.Now, s.Link.Shell)
}

// card is the offer card at now, width cells wide, of the startup file and
// the line sh names: ADD A LINE TO and the file in its top edge; what setup
// can do, in text.secondary, or while the line's added, the spinner and that
// it is; a row on, the line, in accent.mode, wrapped whole; a row on, what
// the line does, and what it leaves, muted; and its keys, but q quit alone
// while the line's added.
func (o SetupOfferCard) card(width int, now time.Time, sh setup.ShellFound) *setupCard {
	content := width - 2*cardInset
	says := func(text string, k ink) []line {
		return indented(line{{text, k}}.wrap(content-offerSaysAt), offerSaysAt)
	}
	body, keys := says(fmt.Sprintf(offerCan, sh.Where), secondaryInk), offerKeys
	if o.Adding {
		body, keys = indented([]line{waiting(now, fmt.Sprintf(offerAdding, sh.Where))}, offerSaysAt), []Key{Quit}
	}
	body = append(append(body, nil), indented(wholeLines(sh.Line, content-offerLineAt), offerLineAt)...)
	body = append(append(body, nil), says(offerHandsOff, mutedInk)...)
	body = append(body, says(fmt.Sprintf(offerLeaves, sh.Where), mutedInk)...)
	return &setupCard{card: Card{title: fmt.Sprintf(offerTitle, sh.Where), width: width, body: body, keys: keys}}
}
