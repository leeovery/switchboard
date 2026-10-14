package dashboard

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
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
// newStatusLine has it, auto said where it has no global pin, and the
// sessions it has routed numbered as such, where it has routed any; or
// probed, as probedLine has it. While the router doesn't answer, from when
// the frame lost it, its state says so, the rest as its last document had
// them, or before any document, that alone. A read that failed otherwise
// says why in the state's place, as failing has it. Before any document, and
// before either, it's blank.
func (f Frame) pageStatus(doc status.Document, now time.Time) statusLine {
	m := f.Moment(doc, now)
	var s statusLine
	switch {
	case m.Probed:
		return f.failing(probedLine(m), now)
	case m.Routed:
		s = newStatusLine(m)
		if s.mode == nil {
			s.mode = line{{"auto", secondaryInk}}
		}
		if m.Sessions > 0 {
			s.sessions = s.sessions.numbered(Number{Kind: NumberRouted})
		}
	}
	if f.Lost.IsZero() {
		return f.failing(s, now)
	}
	s.state = line{{"■ ", errorInk}, {"the router isn't answering", secondaryInk}}
	s.since = line{{"since " + prose.ToTheSecond(now, f.Lost), mutedInk}}
	s.short = line{{"■ ", errorInk}, {"no router", secondaryInk}, {" since " + prose.TimeOfDay(now, f.Lost), mutedInk}}
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
	next := "next " + prose.TimeOfDay(now, f.Failed.Next)
	s.state = line{{"! ", errorInk}, {"couldn't read usage: " + prose.Clean(f.Failed.Reason), secondaryInk}}
	s.since = line{{next, mutedInk}}
	s.short = line{{"! ", errorInk}, {"couldn't read usage", secondaryInk}, {" " + next, mutedInk}}
	return s
}

// newStatusLine is the status line of the moment m, its mode shown only while
// the router has a global pin.
func newStatusLine(m status.Moment) statusLine {
	if !m.Routed {
		return probedLine(m)
	}
	s := statusLine{sessions: line{{prose.SessionCount(m.Sessions), mutedInk}}, destination: destination(m)}
	s.state = line{{"● ", positiveInk}, {"healthy", secondaryInk}}
	if !m.Health.Healthy {
		s.state = line{{"● unhealthy", errorInk}}
	}
	s.short = s.state
	if m.Restart.Due() {
		s.restart = line{{"restart due (" + prose.Clean(m.Restart.Reason) + ")", warningInk}}
	}
	if !m.Pin.IsZero() {
		s.mode = line{{"pinned to " + names(m, m.Pin.Accounts), alertInk}}
	}
	return s
}

// probedLine is the status line of the moment m, probed rather than the
// router's: why the router's document wasn't read, or that probing was asked
// for.
func probedLine(m status.Moment) statusLine {
	why := ", as asked"
	switch m.Fallback.Router {
	case status.RouterNotRunning:
		why = " · router not running"
	case status.RouterUnhealthy:
		why = " · " + because("router unhealthy", m.Fallback.Reason)
	}
	return statusLine{state: line{{"○ probed" + why, dimInk}}, short: line{{"○ probed", dimInk}}}
}

// destination says where the router sends new sessions, as m's Next has it:
// ▲ and the account, or, where none has room, that none has, or before
// anything has been read of any, that nothing has; a lone account it names
// regardless, as its row carries ▲ regardless.
func destination(m status.Moment) line {
	read := slices.ContainsFunc(m.Accounts, func(a status.AccountNow) bool { return a.Read })
	switch {
	case len(m.Accounts) != 1 && !read:
		return line{{"nothing read yet", dimInk}}
	case m.Next == "":
		return line{{"no account has room", exhaustedInk}}
	default:
		return line{{"▲ " + named(m, m.Next), nameInk}}
	}
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
