package views

import (
	"cmp"
	"slices"
	"sort"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/score"
)

// TraceSpan is how far back a trace goes: the last 30 minutes.
const TraceSpan = 30 * time.Minute

// tracesKept is how far back the requests the traces count are kept: their
// span, and the minute before it, which the requests a minute at its oldest
// sample count.
const tracesKept = TraceSpan + time.Minute

// cellSamples is how many samples of requests a minute a trace's cell is the
// mean of, one at the end of each like part of its slice of the TraceSpan.
const cellSamples = 4

// Traces are the requests the Overview's traces count: those that spend
// quota and went upstream, each once, by its id, as it's sent, on the
// account it went out on; Claude Code's quota checks and token counts never.
// They're by account, and by session, a session's carrying its whole
// TraceSpan across its moves. Read and Told count them, from today's lines
// and the request stream; what changes them makes them afresh, so a frame
// holding them never sees them change.
type Traces struct {
	// Accounts are each account's, by its id.
	Accounts map[string]Trace
	// Sessions are each session's, by its id.
	Sessions map[string]Trace
	// requests are the requests counted, by their ids.
	requests map[string]counted
}

// counted is a request the traces count: when it was first sent, the
// account it went out on last, and its session's id, "" where it names none.
type counted struct {
	at               time.Time
	account, session string
}

// Sent is a request going upstream, as the request stream tells of it: the
// router's id for it, its session's, "" where it names none, the account it
// goes out on, and when it went.
type Sent struct {
	Request, Session, Account string
	At                        time.Time
}

// Read is t with the requests of lines, today's, counted at now, as Told
// counts them: those that spend quota and went upstream, each on the account
// whose answer its client got, as of when it arrived; never a quota check, a
// count of tokens or one the router answered itself.
func (t Traces) Read(lines []ledger.Held, now time.Time) Traces {
	var sent []Sent
	for _, l := range lines {
		if l.Kind == ledger.KindMessage && l.Attempts > 0 {
			sent = append(sent, Sent{Request: l.Request, Session: l.Session, Account: l.Account, At: l.At})
		}
	}
	return t.Told(sent, now)
}

// Told is t with the requests sent counted at now: each once, by its id,
// whether the request stream or its line tells of it first. One told of
// again, as one going out again on another account after a refusal, keeps
// when it was first sent, and is counted on the account it went out on last.
// The requests sent more than TraceSpan and a minute before now are let go
// of, as no trace reaches them.
func (t Traces) Told(sent []Sent, now time.Time) Traces {
	if len(sent) == 0 {
		return t
	}
	from := now.Add(-tracesKept)
	requests := make(map[string]counted, len(t.requests)+len(sent))
	for id, c := range t.requests {
		if !c.at.Before(from) {
			requests[id] = c
		}
	}
	for _, s := range sent {
		c, ok := requests[s.Request]
		if !ok {
			if s.At.Before(from) {
				continue
			}
			c.at = s.At
		}
		c.account, c.session = cmp.Or(s.Account, c.account), cmp.Or(c.session, s.Session)
		requests[s.Request] = c
	}
	return tracesOf(requests)
}

// tracesOf are the traces of the requests counted, by their ids.
func tracesOf(requests map[string]counted) Traces {
	t := Traces{Accounts: make(map[string]Trace), Sessions: make(map[string]Trace), requests: requests}
	for _, c := range requests {
		sentOn(t.Accounts, c.account, c.at)
		sentOn(t.Sessions, c.session, c.at)
	}
	for _, traces := range []map[string]Trace{t.Accounts, t.Sessions} {
		for _, trace := range traces {
			slices.SortFunc(trace.Sent, time.Time.Compare)
		}
	}
	return t
}

// sentOn adds a request sent at at to the trace with the given id among
// traces: none where the id is "".
func sentOn(traces map[string]Trace, id string, at time.Time) {
	if id == "" {
		return
	}
	trace := traces[id]
	trace.Sent = append(trace.Sent, at)
	traces[id] = trace
}

// Trace is the requests an account, or a session, sent over the TraceSpan:
// when each went out, the oldest first.
type Trace struct {
	Sent []time.Time
}

// Cells are the trace n cells wide at now, the newest at the right: each its
// requests a minute over its slice of the TraceSpan, the mean of cellSamples
// samples, one at the end of each like part of the slice. The slices are the
// clock's, each starting on a whole multiple of its length, so the trace
// steps left a cell as each passes; the newest is the one now falls in, its
// samples yet to come taken at now, so it fills as requests are sent.
func (t Trace) Cells(n int, now time.Time) []float64 {
	cells := make([]float64, max(n, 0))
	if n <= 0 {
		return cells
	}
	slice := TraceSpan / time.Duration(n)
	end := now.Truncate(slice).Add(slice)
	for i := range cells {
		start := end.Add(-time.Duration(n-i) * slice)
		sum := 0
		for k := 1; k <= cellSamples; k++ {
			sum += t.perMinute(score.Earlier(start.Add(time.Duration(k)*slice/cellSamples), now))
		}
		cells[i] = float64(sum) / cellSamples
	}
	return cells
}

// Rate is the trace's requests of the last minute to now.
func (t Trace) Rate(now time.Time) int {
	return t.perMinute(now)
}

// perMinute is how many of the trace's requests were sent in the minute to
// at, at itself included.
func (t Trace) perMinute(at time.Time) int {
	return t.sentBy(at) - t.sentBy(at.Add(-time.Minute))
}

// sentBy is how many of the trace's requests were sent by at, at included.
func (t Trace) sentBy(at time.Time) int {
	return sort.Search(len(t.Sent), func(i int) bool { return t.Sent[i].After(at) })
}
