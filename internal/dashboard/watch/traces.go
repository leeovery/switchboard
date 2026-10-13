package watch

import (
	"github.com/leeovery/switchboard/internal/ledger"
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
