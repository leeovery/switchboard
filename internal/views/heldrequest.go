package views

import (
	"cmp"
	"hash/maphash"
	"iter"
	"slices"
	"strings"
	"time"
	"unique"

	"github.com/leeovery/switchboard/internal/ledger"
)

// heldRequest is a session's request as its story and its page take it in,
// its line decoded once: its flight; what its line names it by, each name
// interned, as a session's requests name the same again and again, the
// prompt it serves and why its answer stopped among them, and the tools its
// answer called, interned as one, as calls repeat; whether it went upstream;
// the accounts it went out on that couldn't serve it; what it spent; how long
// its writes to the cache last; and what it was worth at a day's prices. A
// story is told again from it without decoding its line again, and it's a
// fraction of its line's size, so the store holds it in place of the line.
type heldRequest struct {
	flight
	dir, model, account, reason, from, class, agentID, geo unique.Handle[string]
	prompt, stop, tools                                    unique.Handle[string]
	upstream                                               bool
	tried                                                  []ledger.Tried
	spend                                                  ledger.Spend
	// life is how long its writes to the cache last, as ledger.Spend's
	// CacheLife has it: none where it wrote nothing to the cache.
	life time.Duration
	// worth is what it was worth at the prices of the day it's priced at, as
	// ledger.Table.SummedSpend gives it, priced where the table priced it:
	// none, and not priced, till it's priced, as a story the Log tells never
	// is.
	worth  ledger.Worth
	priced bool
	// sum is all it decodes of its line, summed, as summed has it.
	sum uint64
}

// decodedRequest is all a held request decodes of its line, as its line
// gives it or its stored form holds it: its flight, the names its line gives
// it, the tools its answer called, whether it went upstream, the accounts it
// went out on that couldn't serve it, and what it spent.
type decodedRequest struct {
	flight
	dir, model, account, reason, from, class, agentID, geo, prompt, stop string
	tools                                                                []string
	upstream                                                             bool
	tried                                                                []ledger.Tried
	spend                                                                ledger.Spend
}

// heldRequestOf returns the request of the line l, decoded once, unpriced.
func heldRequestOf(l *ledger.Line) heldRequest {
	return newHeldRequest(decodedRequest{
		flight: flightOf(l), dir: l.Dir, model: l.Model, account: l.Account, reason: l.Reason, from: l.From, class: l.Class, agentID: l.AgentID,
		geo: l.Shape.InferenceGeo, prompt: l.Prompt, stop: l.Answer.Stop, tools: l.Answer.Tools, upstream: l.Upstream(), tried: l.Tried, spend: ledger.SpendOf(l),
	})
}

// newHeldRequest returns the request d decodes, unpriced: its names
// interned, how long its writes to the cache last worked out of its spend,
// and its sum of all it holds. It's the one place a held request is put
// together, whether of its line or of its stored form.
func newHeldRequest(d decodedRequest) heldRequest {
	r := heldRequest{
		flight: d.flight, upstream: d.upstream, tried: d.tried, spend: d.spend,
		dir: unique.Make(d.dir), model: unique.Make(d.model), account: unique.Make(d.account), reason: unique.Make(d.reason),
		from: unique.Make(d.from), class: unique.Make(d.class), agentID: unique.Make(d.agentID), geo: unique.Make(d.geo),
		prompt: unique.Make(d.prompt), stop: unique.Make(d.stop), tools: unique.Make(strings.Join(d.tools, toolsApart)),
	}
	r.life, _ = r.spend.CacheLife()
	r.sum = r.summed()
	return r
}

// toolsApart is what parts the names of the tools an answer called as a
// held request interns them as one: no tool's name holds it.
const toolsApart = "\x00"

// calls are the names of the tools r's answer called, in order.
func (r *heldRequest) calls() iter.Seq[string] {
	return func(yield func(string) bool) {
		if all := r.tools.Value(); all != "" {
			for name := range strings.SplitSeq(all, toolsApart) {
				if !yield(name) {
					return
				}
			}
		}
	}
}

// sumsSeed seeds every sum of a line, as lineSumOf has it, and of a held
// request, as summed has it: one seed for the run, as a sum is held no longer
// than the run, and worked out again of a request restored from its stored
// form.
var sumsSeed = maphash.MakeSeed()

// summed is a sum of all r decodes of its line, as it holds it, its spend as
// ledger.Spend's Sum has it in place of the usage it's decoded of: two held
// requests whose sums differ hold otherwise, and two whose sums are the same
// hold the same, but by a chance of one in 2^64. So a request restored from
// its stored form sums as the one decoded of its line did, and two lines of
// a request that say otherwise only of what none of it decodes, as their
// usages' bytes, sum alike. It's never compared with a line's sum, as
// lineSumOf has it.
func (r *heldRequest) summed() uint64 {
	var h maphash.Hash
	h.SetSeed(sumsSeed)
	maphash.WriteComparable(&h, struct {
		flight
		dir, model, account, reason, from, class, agentID, geo, prompt, stop, tools string
		upstream                                                                    bool
		tried                                                                       int
		spend                                                                       uint64
	}{
		r.flight, r.dir.Value(), r.model.Value(), r.account.Value(), r.reason.Value(), r.from.Value(), r.class.Value(), r.agentID.Value(),
		r.geo.Value(), r.prompt.Value(), r.stop.Value(), r.tools.Value(), r.upstream, len(r.tried), r.spend.Sum(sumsSeed),
	})
	for _, t := range r.tried {
		maphash.WriteComparable(&h, t)
	}
	return h.Sum64()
}

// lineSumOf is a sum of what the line l says of its request: its id, its
// session, the names it gives, its kind, when it arrived and ended, whether
// it went upstream, the accounts it went out on that couldn't serve it, the
// tools its answer called, and its usage, byte for byte: two lines whose
// sums differ say otherwise of their request, and two whose sums are the
// same say the same of it, but by a chance of one in 2^64. It's never
// compared with a held request's sum, as summed has it.
func lineSumOf(l *ledger.Line) uint64 {
	var h maphash.Hash
	h.SetSeed(sumsSeed)
	maphash.WriteComparable(&h, struct {
		request, session, dir, model, account, reason, from, class, agentID, geo, kind, prompt, stop string
		at, end                                                                                      instant
		upstream                                                                                     bool
		tried, tools                                                                                 int
	}{
		l.Request, l.Session, l.Dir, l.Model, l.Account, l.Reason, l.From, l.Class, l.AgentID, l.Shape.InferenceGeo, l.Kind, l.Prompt, l.Answer.Stop,
		instantOf(l.At), instantOf(ended(l)), l.Upstream(), len(l.Tried), len(l.Answer.Tools),
	})
	for _, t := range l.Tried {
		maphash.WriteComparable(&h, t)
	}
	for _, tool := range l.Answer.Tools {
		maphash.WriteComparable(&h, tool)
	}
	_, _ = h.Write(l.Usage)
	return h.Sum64()
}

// heldRequestsOf returns the requests among lines, a session's, as isRequest
// says, each decoded once, unpriced, in the order every story takes them in,
// as inOrder has it, whatever the order of lines.
func heldRequestsOf(lines []ledger.Line) []heldRequest {
	var requests []heldRequest
	for i := range lines {
		if isRequest(&lines[i]) {
			requests = append(requests, heldRequestOf(&lines[i]))
		}
	}
	slices.SortStableFunc(requests, inOrder)
	return requests
}

// pricedRequestsOf returns the requests among lines, as heldRequestsOf has
// them, each priced by prices at the local day with the given date.
func pricedRequestsOf(lines []ledger.Line, prices ledger.Table, date string) []heldRequest {
	return pricedAt(heldRequestsOf(lines), prices, date)
}

// pricedAt returns requests, each priced by prices at the local day with the
// given date, as pricedOn has it, afresh.
func pricedAt(requests []heldRequest, prices ledger.Table, date string) []heldRequest {
	priced := slices.Clone(requests)
	for i := range priced {
		priced[i] = priced[i].pricedOn(prices, date)
	}
	return priced
}

// pricedOn is r priced by prices at the local day with the given date, as
// ledger.Table.SummedSpend prices it.
func (r heldRequest) pricedOn(prices ledger.Table, date string) heldRequest {
	r.worth, r.priced = prices.SummedSpend(r.model.Value(), r.geo.Value(), r.spend, date)
	return r
}

// worthSaid is what r was worth, as a page sums it: as it was priced, none
// where it wasn't.
func (r *heldRequest) worthSaid() Worth {
	var w Worth
	w.addSummed(r.worth, r.priced)
	return w
}

// withRequests returns held, a session's requests in order, as inOrder has
// it, with those added among them, afresh.
func withRequests(held, added []heldRequest) []heldRequest {
	all := slices.Concat(held, added)
	slices.SortStableFunc(all, inOrder)
	return all
}

// inOrder orders a session's requests as every story takes them in: by when
// each arrived, then when it ended, then by its id, so a story is told the
// same however its lines were read, which file or place each came from.
func inOrder(a, b heldRequest) int {
	return a.compare(b.flight)
}

// compare orders f before o as inOrder orders their requests.
func (f flight) compare(o flight) int {
	return cmp.Or(f.at.compare(o.at), f.end.compare(o.end), strings.Compare(f.request, o.request))
}

// metered reports whether r's answer gave usage, as ledger.Spend's Tokens
// says.
func (r *heldRequest) metered() bool {
	_, ok := r.spend.Tokens()
	return ok
}

// thread returns the thread of a session's requests r is of, as its cache
// follows them: a subagent's, by its id; the session's own conversation, its
// compactions, which read it, among them; a side request's, as a title, by
// its class and model; and, of a request from before requests said their
// class, which tells no thread from another, its model's.
func (r *heldRequest) thread() string {
	agentID, class, model := r.agentID.Value(), r.class.Value(), r.model.Value()
	switch {
	case agentID != "":
		return "agent " + agentID
	case class == ledger.ClassMain || class == classCompaction:
		return "conversation"
	case class == "":
		return "model " + model
	}
	return class + " " + model
}

// flight is a request as a view tells it from another: its id, and when it
// arrived and ended, as sameRequest has them.
type flight struct {
	request string
	at, end instant
}

// flightOf returns the flight of the request of the line l.
func flightOf(l *ledger.Line) flight {
	return flight{request: l.Request, at: instantOf(l.At), end: instantOf(ended(l))}
}

// arrived is when f's request arrived, in UTC, as a line gives it.
func (f flight) arrived() time.Time {
	return f.at.time()
}

// ended is when f's request ended, in UTC.
func (f flight) ended() time.Time {
	return f.end.time()
}

// instant is a moment as a flight holds it, exactly, whatever its year: the
// seconds since the epoch and the nanoseconds after, as time.Unix takes them.
// Nanoseconds alone hold only the years 1678 to 2262, and a hand-edited line
// may arrive at no time at all, the zero time, or far off.
type instant struct {
	sec  int64
	nsec int32
}

// instantOf returns the instant t is.
func instantOf(t time.Time) instant {
	return instant{sec: t.Unix(), nsec: int32(t.Nanosecond())}
}

// time is i in UTC: the zero time where i is the zero time's.
func (i instant) time() time.Time {
	return time.Unix(i.sec, int64(i.nsec)).UTC()
}

// compare orders instants, the earlier first.
func (i instant) compare(o instant) int {
	return cmp.Or(cmp.Compare(i.sec, o.sec), cmp.Compare(i.nsec, o.nsec))
}

// is reports whether f is of the request with the given id that arrived, or
// went upstream, at at, as sameRequest has it of a line.
func (f flight) is(id string, at time.Time) bool {
	return f.request == id && f.inFlightAt(at)
}

// inFlightAt reports whether f's request was in flight at at, as inFlightAt
// has it of a line.
func (f flight) inFlightAt(at time.Time) bool {
	return inFlight(at, f.arrived(), f.ended())
}

// key is f as a request is known from another, as keyOf has it of its line,
// reporting false where it has no id.
func (f flight) key() (lineKey, bool) {
	return lineKey{request: f.request, at: f.at}, f.request != ""
}
