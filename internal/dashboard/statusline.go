package dashboard

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// The spans that set the status line's parts apart: in full, and as a phone
// spaces them.
var (
	statusApart = span{"  ·  ", mutedInk}
	phoneApart  = span{" · ", mutedInk}
)

// The words that lead where new sessions go: in full, and as a phone says
// them.
const (
	destinationLead      = "new sessions → "
	phoneDestinationLead = "new → "
)

// statusLine is how the router stands, as the status line says it, in its
// parts: the router's state, since when, where it's a state that has a
// since, and what a line short of room shortens the two to; a restart it has
// due; the mode; the sessions it has routed in the last hour; and where new
// sessions go. Each part but the state is nil where the line leaves it out: a
// document probed has no mode, sessions or new sessions.
type statusLine struct {
	state, since, short                  line
	restart, mode, sessions, destination line
}

// routerStatus draws the page's status line of doc at now, as pageStatus
// has it, on its row, from the first column, where that's over the key
// line's. On a phone, it's always as a phone spaces it.
func (f Frame) routerStatus(c *canvas, doc status.Document, now time.Time) {
	y := f.statusRow()
	if y >= f.Height-1 {
		return
	}
	forms := f.pageStatus(doc, now).forms()
	if f.phone() {
		forms = forms[1:]
	}
	c.line(0, y, firstFit(f.Width, forms...))
}

// pageStatus is the page's status line of doc at now: the router's, as
// newStatusLine has it, auto said where it has no global pin; or probed, as
// probedLine has it. While the router doesn't answer, from when the frame
// lost it, its state says so, the rest as its last document had them, or
// before any document, that alone. A read that failed otherwise says why in
// the state's place, as failing has it. Before any document, and before
// either, it's blank.
func (f Frame) pageStatus(doc status.Document, now time.Time) statusLine {
	var s statusLine
	switch doc.Source {
	case status.SourceProbe:
		return f.failing(probedLine(doc), now)
	case status.SourceRouter:
		s = newStatusLine(doc)
		if s.mode == nil {
			s.mode = line{{"auto", secondaryInk}}
		}
	}
	if f.Lost.IsZero() {
		return f.failing(s, now)
	}
	lost := f.Lost.In(now.Location())
	s.state = line{{"■ ", errorInk}, {"the router isn't answering", secondaryInk}}
	s.since = line{{"since " + lost.Format("15:04:05"), mutedInk}}
	s.short = line{{"■ ", errorInk}, {"no router", secondaryInk}, {" since " + lost.Format("15:04"), mutedInk}}
	return s
}

// Failure is why a read failed, and when the next is due.
type Failure struct {
	Reason string
	Next   time.Time
}

// failing is s with the frame's failed read, where it has one, in its
// state's place: why, and when the next read is due, at now.
func (f Frame) failing(s statusLine, now time.Time) statusLine {
	if f.Failed.Reason == "" {
		return s
	}
	next := "next " + status.TimeOfDay(now, f.Failed.Next)
	s.state = line{{"! ", errorInk}, {"couldn't read usage: " + status.Clean(f.Failed.Reason), secondaryInk}}
	s.since = line{{next, mutedInk}}
	s.short = line{{"! ", errorInk}, {"couldn't read usage", secondaryInk}, {" " + next, mutedInk}}
	return s
}

// newStatusLine is doc's status line, its mode shown only while the router
// has a global pin.
func newStatusLine(doc status.Document) statusLine {
	if doc.Source != status.SourceRouter {
		return probedLine(doc)
	}
	s := statusLine{sessions: line{{status.SessionCount(doc.Sessions), mutedInk}}, destination: destination(doc)}
	s.state = line{{"● ", positiveInk}, {"healthy", secondaryInk}}
	if !doc.Router.Healthy {
		s.state = line{{"● unhealthy", errorInk}}
	}
	s.short = s.state
	if doc.Restart.Due() {
		s.restart = line{{"restart due (" + status.Clean(doc.Restart.Reason) + ")", warningInk}}
	}
	if !doc.Pin.IsZero() {
		s.mode = line{{"pinned to " + names(doc, doc.Pin.Accounts), alertInk}}
	}
	return s
}

// probedLine is the status line of doc, probed rather than the router's: why
// the router's document wasn't read, or that probing was asked for.
func probedLine(doc status.Document) statusLine {
	why := ", as asked"
	switch doc.Fallback.Router {
	case status.RouterNotRunning:
		why = " · router not running"
	case status.RouterUnhealthy:
		why = " · " + because("router unhealthy", doc.Fallback.Reason)
	}
	return statusLine{state: line{{"○ probed" + why, dimInk}}, short: line{{"○ probed", dimInk}}}
}

// destination says where the router sends new sessions: ▲ and the account,
// or, where none has room, that none has, or before anything has been read of
// any, that nothing has; a lone account it names regardless, as its row
// carries ▲ regardless.
func destination(doc status.Document) line {
	read := slices.ContainsFunc(doc.Accounts, func(a status.Account) bool { return !a.FetchedAt.IsZero() })
	to := bestOf(doc)
	switch {
	case len(doc.Accounts) != 1 && !read:
		return line{{"nothing read yet", dimInk}}
	case to == "":
		return line{{"no account has room", exhaustedInk}}
	default:
		return line{{"▲ " + named(doc, to), nameInk}}
	}
}

// bestOf is the id of the account ▲ marks as where new sessions go:
// doc's best, "" where none has room; or with one account, that one, whatever
// its room, as there's nowhere else for them to go. A probed document marks
// none of several, as where new sessions go is the router's to say, and its
// best is worked out without the router's pin.
func bestOf(doc status.Document) string {
	switch {
	case len(doc.Accounts) == 1:
		return doc.Accounts[0].ID
	case doc.Source != status.SourceRouter:
		return ""
	}
	return doc.Best
}

// fit is the line in width cells at most: in full where it fits; else as a
// phone spaces it; then without the sessions, then without where new
// sessions go, then without a restart due, so the state and the mode show,
// then with its state shortened; and where even that doesn't fit, cut short.
func (s statusLine) fit(width int) line {
	return firstFit(width, s.forms()...)
}

// forms are the line's forms, as fit tries them, each shorter than the one
// before.
func (s statusLine) forms() []line {
	going := led(phoneDestinationLead, s.destination)
	return []line{
		joined(statusApart, s.state, s.since, s.restart, s.mode, s.sessions, led(destinationLead, s.destination)),
		joined(phoneApart, s.state, s.since, s.restart, s.mode, s.sessions, going),
		joined(phoneApart, s.state, s.since, s.restart, s.mode, going),
		joined(phoneApart, s.state, s.since, s.restart, s.mode),
		joined(phoneApart, s.state, s.since, s.mode),
		joined(phoneApart, s.short, s.mode),
	}
}

// led is l after the muted words lead: nil where l is.
func led(lead string, l line) line {
	if l == nil {
		return nil
	}
	return slices.Concat(line{{lead, mutedInk}}, l)
}

// joined runs the parts that aren't nil together, apart between each.
func joined(apart span, parts ...line) line {
	var l line
	for _, p := range parts {
		if p == nil {
			continue
		}
		if l != nil {
			l = append(l, apart)
		}
		l = append(l, p...)
	}
	return l
}
