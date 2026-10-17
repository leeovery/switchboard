package watch

import (
	"github.com/leeovery/switchboard/internal/ledger"
)

// traced is m with the Overview's traces counting today's lines a read of
// the ledger's tail read, whoever it was read for: they hold no mark of
// their own, so the ledger is asked for today's lines by the tail alone,
// and the request stream, as it opens, has a read of the tail owed to them,
// as streamOpened has it. Lines given again, as a read that gives every one
// of today's afresh gives them, count once, the traces counting each
// request once.
func (m Model) traced(lines []ledger.Held) Model {
	m.traces = m.traces.Read(lines, m.now())
	return m
}
