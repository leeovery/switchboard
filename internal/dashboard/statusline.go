package dashboard

import (
	"slices"

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
// parts: the router's state, and what a line short of room shortens it to; a
// restart it has due; the mode; the sessions it has routed in the last hour;
// and where new sessions go. Each part but the state is nil where the line
// leaves it out: a document probed has no mode, sessions or new sessions.
type statusLine struct {
	state, short                         line
	restart, mode, sessions, destination line
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
// any, that nothing has.
func destination(doc status.Document) line {
	read := slices.ContainsFunc(doc.Accounts, func(a status.Account) bool { return !a.FetchedAt.IsZero() })
	switch {
	case !read:
		return line{{"nothing read yet", dimInk}}
	case doc.Best == "":
		return line{{"no account has room", exhaustedInk}}
	default:
		return line{{"▲ " + named(doc, doc.Best), nameInk}}
	}
}

// fit is the line in width cells at most: in full where it fits; else as a
// phone spaces it; then without the sessions, then without where new
// sessions go, then without a restart due, so the state and the mode show,
// then with its state shortened; and where even that doesn't fit, cut short.
func (s statusLine) fit(width int) line {
	forms := s.forms()
	for _, l := range forms {
		if l.width() <= width {
			return l
		}
	}
	return forms[len(forms)-1].fit(width)
}

// forms are the line's forms, each shorter than the one before.
func (s statusLine) forms() []line {
	going := led(phoneDestinationLead, s.destination)
	return []line{
		joined(statusApart, s.state, s.restart, s.mode, s.sessions, led(destinationLead, s.destination)),
		joined(phoneApart, s.state, s.restart, s.mode, s.sessions, going),
		joined(phoneApart, s.state, s.restart, s.mode, going),
		joined(phoneApart, s.state, s.restart, s.mode),
		joined(phoneApart, s.state, s.mode),
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
