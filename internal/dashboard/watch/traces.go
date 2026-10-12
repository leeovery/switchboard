package watch

import (
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/views"
)

// tracedLines are today's lines read for the Overview's traces as the
// request stream opened, those the day's files gained since it last did,
// and the Mark they're read to.
type tracedLines struct {
	lines []ledger.Held
	read  ledger.Mark
}

// readTraces reads the requests today's lines tell of for the Overview's
// traces, for the request stream to go on from, as openStream has it once
// the stream has opened: after it, so no request ends unread between the
// two. Each time the stream opens, as when it's joined again, it reads only
// what the day's files gained since it last did; and nothing where the watch
// reads no ledger.
func (m Model) readTraces() func() tracedLines {
	l, mark := m.cfg.Ledger, m.stream.read
	return func() tracedLines {
		if l == nil {
			return tracedLines{read: mark}
		}
		lines, next, _ := l.Today(mark)
		return tracedLines{lines: lines, read: next}
	}
}

// traced takes in today's lines read for the traces as the request stream
// opened.
func (m Model) traced(t tracedLines) Model {
	m.traces, m.stream.read = m.traces.Read(t.lines, m.now()), t.read
	return m
}

// toldTraces takes in the requests the stream told of going upstream, for
// the traces.
func (m Model) toldTraces(events []router.StreamEvent) Model {
	m.traces = m.traces.Told(sentOf(events), m.now())
	return m
}

// sentOf are the requests the events tell of going upstream, as the traces
// count them: each as it goes out, or as it was last sent, in flight as the
// stream opens; never the client's quota check, or a count of tokens.
func sentOf(events []router.StreamEvent) []views.Sent {
	var sent []views.Sent
	for _, e := range events {
		if at, ok := sentAt(e); ok {
			sent = append(sent, views.Sent{Request: e.Request, Session: e.Session, Account: e.Account, At: at})
		}
	}
	return sent
}

// sentAt is when the request e tells of went upstream, reporting false
// where e tells of none going, or of the client's quota check, or a count of
// tokens, which spend no quota.
func sentAt(e router.StreamEvent) (time.Time, bool) {
	switch {
	case e.Check, e.Count:
		return time.Time{}, false
	case e.Kind == router.StreamSent:
		return e.At, true
	case e.Kind == router.StreamInFlight:
		return e.SentAt, true
	}
	return time.Time{}, false
}
