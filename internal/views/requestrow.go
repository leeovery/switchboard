package views

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// RequestRow is a request's row, the Log's, as every list of requests
// gives one: the Log's Requests, a turn opened on a session's page, and
// requests. It holds the facts typed, each printed form left to its one
// owner: status.Seconds for First and Took, prose.Tokens for Read, Written
// and Out, and dashboard.WorthCents for Worth; status.ShownID or
// status.ShortID for Session, status.DirName for Dir, and the account's name
// for Account. It says its stopped and its routing itself, as no drawer
// works words out of a line. It's built from the request's line, with its
// session's story, as LogRows builds it, or, before its line comes, from
// what the request stream told of it.
type RequestRow struct {
	// Request is the router's id for it.
	Request string
	// At is when it arrived, as its line gives it; before its line comes,
	// when it was sent, as the stream told.
	At time.Time
	// Kind is what it asked.
	Kind RowKind
	// Session and Dir are its session's whole id, and the directory the
	// session was started in, as its line or the stream names them: each ""
	// where it names none.
	Session, Dir string
	// Model is the model it asked for, its whole id.
	Model string
	// Account is the account whose answer its client got, or, before its
	// line comes, the one it went out on last: "" where the router answered
	// it itself.
	Account string
	// Stage is how far it has come, which says what of the rest is known.
	Stage RowStage
	// Status is what it was answered, as its line gives it, 0 where it ended
	// before an answer came; or, ended before its line came, as the stream
	// told. Canceled is set where its client went away before its end, and
	// CutOff where the router cut it off as it stopped, as its line says.
	Status           int
	Canceled, CutOff bool
	// Failed is set where it failed: as ledger.Line.Failed says, or, ended
	// before its line came, as ledger.Failing says of the status told; and
	// where it was lost.
	Failed bool
	// First is how long after it arrived its answer's first byte came, and
	// Took how long it took in all, as its line gives them; before its line
	// comes, since it was sent, as the stream told, Took counting to when
	// the row was built while it's in flight. Each is nil where it isn't
	// known: First while it asks, where no byte of an answer came, as to a
	// request the router answered itself, and where it was lost; Took where
	// it was lost.
	First, Took *time.Duration
	// Chars counts the characters its answer has streamed so far while it's
	// answering, as the stream told: zero till it counts any.
	Chars int
	// Read, Written and Out are the tokens it read from the cache, wrote to
	// it and put out, as its answer's usage counts them, where Metered says
	// its answer gave usage.
	Read, Written, Out int
	Metered            bool
	// Rewrote is set where it wrote its session's context again on the
	// account a move took the session to: where the cache would have run out
	// anyway, it wrote nothing staying wouldn't have, and it's left unset.
	Rewrote bool
	// Worth is what it would have cost through the API, at today's prices.
	Worth Worth
	// Stop is why its answer stopped, as its line gives it, as end_turn;
	// Tool the first tool it called, and More how many more it called.
	Stop, Tool string
	More       int
	// NewSession is set where it's its session's first request.
	NewSession bool
	// Routing is how the router chose its account, as its line tells it:
	// none before its line comes, as the stream tells no reason.
	Routing RoutingSaid
	// Mark is its mark.
	Mark RowMark
}

// RowKind is what a request asked, as its row tells it.
type RowKind int

const (
	// RowRequest is one the views count among the requests, as
	// ledger.Line.Counted counts them: the zero RowKind.
	RowRequest RowKind = iota
	// RowCheck is Claude Code's quota check, hidden until it's shown.
	RowCheck
	// RowCount is a count of a request's tokens, hidden until it's shown.
	RowCount
)

// RowStage is how far a request has come, as its row tells it.
type RowStage int

const (
	// RowLined is a request whose line has come, every fact of it its
	// line's: the zero RowStage.
	RowLined RowStage = iota
	// RowAsking is a request in flight, before its answer's first byte, as
	// the stream told: one refused or throttled on an account, to go out
	// again, among them.
	RowAsking
	// RowAnswering is a request in flight whose answer streams back.
	RowAnswering
	// RowEnded is a request whose end the stream told of, its line yet to
	// come: its status and times are as the stream told, the rest not known.
	RowEnded
	// RowLost is a request the stream told of in flight that ended untold,
	// as one cut off as its router went away does, with no line: it failed,
	// nothing of its answer known.
	RowLost
)

// InFlight reports whether the request is in flight: asking or answering.
func (s RowStage) InFlight() bool {
	return s == RowAsking || s == RowAnswering
}

// RowMark is a row's mark.
type RowMark int

const (
	// NoMark is no mark, as a request that stayed and succeeded has.
	NoMark RowMark = iota
	// MarkAsking is ↑: in flight, before its answer's first byte.
	MarkAsking
	// MarkAnswering is ↓: in flight, its answer streaming.
	MarkAnswering
	// MarkFailed is ■: it failed.
	MarkFailed
	// MarkMoved is ▸: it moved its session.
	MarkMoved
	// MarkNew is ▲: its session's first request.
	MarkNew
	// MarkHidden is ·: a check or a count, shown.
	MarkHidden
)

// rowMarks are each mark's glyph, by the mark.
var rowMarks = [...]string{NoMark: "", MarkAsking: "↑", MarkAnswering: "↓", MarkFailed: "■", MarkMoved: "▸", MarkNew: "▲", MarkHidden: "·"}

// String is the mark's glyph: "" for none.
func (m RowMark) String() string {
	return rowMarks[m]
}

// marking is what says a row's mark: what its request asked, how far it has
// come, and whether it failed, moved its session, or is its session's first.
type marking struct {
	kind                 RowKind
	stage                RowStage
	failed, moved, first bool
}

// mark is the first mark that applies, as the Log orders them: a check's or
// a count's ·; ↑ in flight, asking; ↓ in flight, answering; ■ failed; ▸ it
// moved its session; ▲ its session's first request; else none.
func (m marking) mark() RowMark {
	switch {
	case m.kind != RowRequest:
		return MarkHidden
	case m.stage == RowAsking:
		return MarkAsking
	case m.stage == RowAnswering:
		return MarkAnswering
	case m.failed:
		return MarkFailed
	case m.moved:
		return MarkMoved
	case m.first:
		return MarkNew
	}
	return NoMark
}

// stage is how far the request a stands for has come, as its row tells it:
// answering where a's answer streams; else asking, while a's in flight, as
// one refused or throttled on an account, to go out again, is; else as if
// its line had come, nothing in flight.
func (a Activity) stage() RowStage {
	switch {
	case a.Doing == Answering:
		return RowAnswering
	case a.InFlight():
		return RowAsking
	}
	return RowLined
}

// RoutingSaid is how the router chose a request's account, or brought a
// session to the account it's on, in the views' words: the Log's routing,
// as a turn opened and the List say it too.
type RoutingSaid struct {
	// Words are the router's reason, as status.RoutingWords says it: "from
	// work, at its cap", or of a session's first request, "new session"; of
	// a request sent again after an account couldn't serve it, where it went
	// out before, as status.TriedWords says it: "after work's limit"; and of
	// a session on the List pinned there on its own, "pinned here". None
	// where it stayed where its session was.
	Words status.Words
	// From is the account the request, or the move that brought the session,
	// took the session from: "" where it moved none.
	From string
	// Cost is what writing the session's context again on the account it
	// moved to cost, as its story settles a move: nil where it moved none,
	// that isn't known, or its cache would have run out anyway. It's paid,
	// so it's said exactly, with no ≈.
	Cost *ledger.Picodollars
}

// routingOf is how the router chose the account of the request l, as its
// session's story tells it: whether it's the session's first request, and
// the move it made, nil where it made none. Where it was sent again after an
// account couldn't serve it, where it went out before; else, where it moved
// its session, the reason; else, where it's the session's first, "new
// session". The cost is its move's.
func routingOf(l *ledger.Line, first bool, made *move) RoutingSaid {
	said := RoutingSaid{From: l.From}
	switch {
	case len(l.Tried) > 0:
		tried := make([]status.Tried, len(l.Tried))
		for i, t := range l.Tried {
			tried[i] = status.Tried(t)
		}
		said.Words = status.TriedWords(tried...)
	case l.From != "":
		said.Words = status.RoutingWords(l.Reason, status.Facts{From: l.From, To: l.Account})
	case first:
		said.Words = newSession()
	}
	if made != nil {
		said.Cost = made.cost
	}
	return said
}

// newSession is a session's first request's routing: "new session".
func newSession() status.Words {
	return status.RoutingWords(status.ReasonNew, status.Facts{})
}

// cameBy has the session brought to Account by the move m, nil where no move
// of today's brought it there: Moved, as the List gives m; and its mark and
// routing, as the Log's row of its request says them. Its mark is of those
// the List shows: ↑ while a request of it asks and ↓ while it's answered, as
// the row of its busiest request in flight says, else ▸ where m brought it.
// Its routing is the row's of m's request, and what m cost; else, where it's
// pinned to Account on its own, "pinned here".
func (l *ListedSession) cameBy(m *move) {
	l.Mark = marking{stage: l.Activity.stage(), moved: m != nil}.mark()
	switch {
	case m != nil:
		l.Moved = &Move{At: m.at.UTC(), From: m.from, Reason: m.reason, Cost: m.cost}
		l.Routing = routingOf(m.by, false, m)
	case slices.ContainsFunc(l.Models, func(s SessionModel) bool { return s.Account == l.Account && s.Reason == status.ReasonPinned }):
		l.Routing = RoutingSaid{Words: status.SessionWords(status.ReasonPinned, status.Facts{})}
	}
}

// lineRow is the row of the request l, a line of the session s tells, nil
// for one of no session, its worth at the prices on the local day today.
func lineRow(l *ledger.Line, s *story, prices ledger.Table, today LedgerDay) RequestRow {
	r := RequestRow{
		Request: l.Request, At: l.At, Kind: kindOf(l), Session: l.Session, Dir: l.Dir, Model: l.Model, Account: l.Account,
		Status: l.Status, Canceled: l.Canceled, CutOff: l.CutOff, Failed: l.Failed(), Took: millis(l.TotalMS),
		Worth: summed(prices, l, today.Date), Stop: l.Answer.Stop,
	}
	if l.FirstMS != nil {
		r.First = millis(*l.FirstMS)
	}
	if tokens, ok := l.Tokens(); ok {
		r.Read, r.Written, r.Out, r.Metered = tokens.CacheRead, tokens.CacheWrite, tokens.Output, true
	}
	if tools := l.Answer.Tools; len(tools) > 0 {
		r.Tool, r.More = tools[0], len(tools)-1
	}
	var made *move
	if s != nil {
		r.NewSession, made, r.Rewrote = s.toldOf(l)
	}
	r.Routing = routingOf(l, r.NewSession, made)
	r.Mark = r.marking().mark()
	return r
}

// kindOf is what the request l asked, as its row tells it.
func kindOf(l *ledger.Line) RowKind {
	switch l.Kind {
	case ledger.KindCheck:
		return RowCheck
	case ledger.KindCount:
		return RowCount
	}
	return RowRequest
}

// millis is a duration of n milliseconds.
func millis(n int64) *time.Duration {
	return new(time.Duration(n) * time.Millisecond)
}

// since is how long after from to came, none before it.
func since(from, to time.Time) *time.Duration {
	return new(max(to.Sub(from), 0))
}

// toldRow is the row of the request t, as the stream told of it before its
// line came, at now: lost; ended, its status as told; else in flight, its
// took counting to now.
func toldRow(t Told, now time.Time) RequestRow {
	r := RequestRow{Request: t.Request, At: t.Sent, Kind: toldKind(t), Session: t.Session, Dir: t.Dir, Model: t.Model, Account: t.Account}
	switch {
	case t.Lost:
		r.Stage, r.Failed = RowLost, true
	case !t.Ended.IsZero():
		r.Stage, r.Status, r.Failed, r.Took = RowEnded, t.Status, ledger.Failing(t.Status), since(t.Sent, t.Ended)
	default:
		a := t.doing()
		r.Stage, r.Chars, r.Took = a.stage(), a.Chars, since(t.Sent, now)
	}
	if !t.First.IsZero() && !t.Lost {
		r.First = since(t.Sent, t.First)
	}
	r.Mark = r.marking().mark()
	return r
}

// toldKind is what the request t asked, as the stream told.
func toldKind(t Told) RowKind {
	switch {
	case t.Check:
		return RowCheck
	case t.Count:
		return RowCount
	}
	return RowRequest
}

// marking is what says the row's mark: it moved its session where its
// routing names the account it left.
func (r RequestRow) marking() marking {
	return marking{kind: r.Kind, stage: r.Stage, failed: r.Failed, moved: r.Routing.From != "", first: r.NewSession}
}

// RowWords are what a request's row says in a column, in the parts it's
// drawn in.
type RowWords []RowPart

// RowPart is a run of RowWords, of the kind it is, which says how it's
// drawn.
type RowPart struct {
	Text string
	Kind RowPartKind
}

// RowPartKind is what a RowPart is.
type RowPartKind int

const (
	// RowSaid is words said, as end_turn, from, new session, or 5.1k
	// characters.
	RowSaid RowPartKind = iota
	// RowBetween stands between what's said and what's named, as the · after
	// tool_use.
	RowBetween
	// RowNamed is what's named: a tool, or an account.
	RowNamed
	// RowDoing is what a request in flight is doing: asking, or answering.
	RowDoing
)

// String runs the words together, as a row drawn without colour shows them.
func (w RowWords) String() string {
	var b strings.Builder
	for _, p := range w {
		b.WriteString(p.Text)
	}
	return b.String()
}

// lostWords say a request was lost: no line came of it, its router gone.
const lostWords = "no answer: the router went away"

// Stopped is why the request's answer stopped, as the Log's stopped says it:
// end_turn; tool_use · Bash, the first tool it called, and +1 after it where
// it called one more; in flight, asking, or answering · 5.1k characters, its
// characters so far, once any are counted, as prose.Characters counts; lost, no
// answer: the router went away. None for a check or a count, and where
// nothing's known of why it stopped.
func (r RequestRow) Stopped() RowWords {
	switch {
	case r.Kind != RowRequest:
		return nil
	case r.Stage == RowAnswering && r.Chars > 0:
		return RowWords{{Text: status.Answering, Kind: RowDoing}, {Text: " · " + prose.Characters(r.Chars) + " characters"}}
	case r.Stage.InFlight():
		return RowWords{r.doing()}
	case r.Stage == RowLost:
		return RowWords{{Text: lostWords}}
	}
	var w RowWords
	if r.Stop != "" {
		w = append(w, RowPart{Text: status.Clean(r.Stop)})
	}
	if r.Tool == "" {
		return w
	}
	if w != nil {
		w = append(w, RowPart{Text: " · ", Kind: RowBetween})
	}
	return append(w, r.tools()...)
}

// PhoneStopped is the request's stopped as a phone's Log says it, its routing
// folded in: in flight, asking or answering alone; lost, as Stopped says;
// where it moved its session, from work; its session's first, new session;
// else the tool it called alone, as Bash, and +1 where it called one more;
// else why its answer stopped. None for a check or a count.
func (r RequestRow) PhoneStopped() RowWords {
	switch {
	case r.Kind != RowRequest:
		return nil
	case r.Stage.InFlight():
		return RowWords{r.doing()}
	case r.Stage == RowLost:
		return r.Stopped()
	case r.Routing.From != "":
		return rowWordsOf(status.FromWords(r.Routing.From))
	case r.NewSession:
		return rowWordsOf(newSession())
	case r.Tool != "":
		return r.tools()
	case r.Stop != "":
		return RowWords{{Text: status.Clean(r.Stop)}}
	}
	return nil
}

// doing is what the request in flight is doing: asking, or answering.
func (r RequestRow) doing() RowPart {
	if r.Stage == RowAnswering {
		return RowPart{Text: status.Answering, Kind: RowDoing}
	}
	return RowPart{Text: status.Asking, Kind: RowDoing}
}

// tools are the first tool the request called, and how many more.
func (r RequestRow) tools() RowWords {
	w := RowWords{{Text: status.Clean(r.Tool), Kind: RowNamed}}
	if r.More > 0 {
		w = append(w, RowPart{Text: " +" + strconv.Itoa(r.More)})
	}
	return w
}

// rowWordsOf are status's words as a row draws them, each account named.
func rowWordsOf(w status.Words) RowWords {
	said := make(RowWords, len(w))
	for i, p := range w {
		said[i] = RowPart{Text: p.Text}
		if p.Kind == status.PartAccount {
			said[i].Kind = RowNamed
		}
	}
	return said
}
