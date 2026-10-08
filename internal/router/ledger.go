package router

import (
	"cmp"
	"context"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
)

// requestLedger is the request ledger, as the router keeps it: a line for
// each routed request, noted as the request ends, which run's goroutine
// writes once Run has opened it in the state directory, kept as long as the
// config says, and a summary of each day once it has ended, made with the
// readings history. Noting a line never holds a request up: one noted before
// the ledger is opened, or past its queue's end, is dropped.
type requestLedger struct {
	keep time.Duration
	now  func() time.Time
	// opened is the ledger once it's opened, nil before.
	opened atomic.Pointer[ledger.Ledger]
}

// newRequestLedger returns a request ledger kept as settings say, by now's
// clock: a zero Keep keeps a day's lines for config.DefaultLedgerKeep.
func newRequestLedger(settings config.Ledger, now func() time.Time) *requestLedger {
	return &requestLedger{keep: cmp.Or(settings.Keep, config.DefaultLedgerKeep), now: now}
}

// open has the ledger kept in dir from now on, its days summarised with the
// readings readings gives.
func (l *requestLedger) open(dir string, readings ledger.Readings) {
	l.opened.Store(ledger.Open(dir, l.keep, l.now, readings, logger))
}

// note queues a request's line for run to write. It never waits.
func (l *requestLedger) note(line *ledger.Line) {
	if opened := l.opened.Load(); opened != nil {
		opened.Note(line)
	}
}

// run writes the lines queued, and keeps the ledger's files, until ctx ends,
// when it writes what's still queued. The ledger must be open.
func (l *requestLedger) run(ctx context.Context) {
	l.opened.Load().Run(ctx)
}

// summariseEnded has the ledger, once it's opened, summarise the days that
// have ended at now, as ledger.Ledger.SummariseEnded says: the readings
// history's writer has it do so just before each prune of its files, so no
// reading a summary needs goes before its day is summarised.
func (l *requestLedger) summariseEnded(now time.Time) {
	if opened := l.opened.Load(); opened != nil {
		opened.SummariseEnded(now)
	}
}

// line is the request ledger's line of the routed request ex, once it's
// done, as f finished it, h its header: its account, reason and from those
// of the account whose answer the client got, as it was picked. The answer's
// counting is done with it.
func (p *proxy) line(ex *exchange, h http.Header, f finish) *ledger.Line {
	answering := ex.answering()
	line := &ledger.Line{
		At:       ex.arrived,
		Request:  ex.id,
		Kind:     ex.kind(),
		Session:  ex.req.Session,
		Dir:      ex.req.Dir,
		Model:    ex.req.Model,
		Account:  answering.account,
		Reason:   answering.reason,
		From:     answering.from,
		Tried:    tried(ex.req.Tried),
		Status:   ex.status,
		Canceled: f.canceled,
		CutOff:   f.cutOff,
		Attempts: ex.attempts,
		TotalMS:  f.took.Milliseconds(),
		Agent:    h.Get("User-Agent"),
		Betas:    p.provider.Betas(h),
		Shape:    ex.shape,
	}
	if ex.tap != nil {
		line.FirstMS, line.Reply = ex.tap.firstMS(ex.started), ex.tap.reply
	}
	return line
}

// kind is what the routed request asked, as the request ledger has it: a
// count of tokens, sent to a path that spends no quota; the client's quota
// check; else a message.
func (ex *exchange) kind() string {
	switch {
	case !ex.spends:
		return ledger.KindCount
	case ex.req.Check:
		return ledger.KindCheck
	}
	return ledger.KindMessage
}

// tried returns the accounts a request went out on that couldn't serve it,
// and why, as the request ledger has them.
func tried(attempts []Attempt) []ledger.Tried {
	if len(attempts) == 0 {
		return nil
	}
	t := make([]ledger.Tried, len(attempts))
	for i, a := range attempts {
		t[i] = ledger.Tried{Account: a.Account, Why: a.Why}
	}
	return t
}
