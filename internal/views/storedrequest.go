package views

import (
	"slices"

	"github.com/leeovery/switchboard/internal/ledger"
)

// storedRequest is all a request decodes of its line, as decodedRequest has
// it, as a stored form holds it, of a held request or of a line's record:
// what it spent in ledger.Spend's own stored form, the tools its answer
// called as a list, and the accounts it went out on, an empty list none, as
// decodedOf has it. What a held request works out of that, how long its
// writes to the cache last and its sum, is worked out again as it's
// restored; and it's restored unpriced, as heldRequestOf gives it, for the
// store to price at its own day. Its keys are short and its zeros left out.
type storedRequest struct {
	storedFlight
	Dir      string        `json:"d,omitempty"`
	Model    string        `json:"m,omitempty"`
	Account  string        `json:"ac,omitempty"`
	Reason   string        `json:"rs,omitempty"`
	From     string        `json:"f,omitempty"`
	Class    string        `json:"c,omitempty"`
	AgentID  string        `json:"ag,omitempty"`
	Geo      string        `json:"g,omitempty"`
	Prompt   string        `json:"p,omitempty"`
	Stop     string        `json:"st,omitempty"`
	Tools    []string      `json:"tl,omitempty"`
	Upstream bool          `json:"u,omitzero"`
	Tried    []storedTried `json:"t,omitzero"`
	Spend    ledger.Spend  `json:"sp,omitzero"`
}

// storedTried is an account a request went out on that couldn't serve it,
// and why it was left, as a stored form holds it.
type storedTried struct {
	Account string `json:"a,omitempty"`
	Why     string `json:"w,omitempty"`
}

// storedFlight is a flight as a stored form holds it: its request's id, and
// when it arrived and ended.
type storedFlight struct {
	Request string        `json:"r,omitempty"`
	At      storedInstant `json:"a,omitzero"`
	End     storedInstant `json:"e,omitzero"`
}

// storedInstant is an instant as a stored form holds it: the seconds since
// the epoch and the nanoseconds after, exactly, whatever its year.
type storedInstant struct {
	Sec  int64 `json:"s,omitzero"`
	Nsec int32 `json:"n,omitzero"`
}

// stored is r as a stored form holds it: all it decodes of its line, as
// decoded has it.
func (r *heldRequest) stored() storedRequest {
	d := r.decoded()
	return d.stored()
}

// decoded is all r decodes of its line, as newHeldRequest was given it: its
// names as their handles hold them, and the tools its answer called as a
// list, none of none.
func (r *heldRequest) decoded() decodedRequest {
	return decodedRequest{
		flight: r.flight, dir: r.dir.Value(), model: r.model.Value(), account: r.account.Value(), reason: r.reason.Value(), from: r.from.Value(),
		class: r.class.Value(), agentID: r.agentID.Value(), geo: r.geo.Value(), prompt: r.prompt.Value(), stop: r.stop.Value(),
		tools: slices.Collect(r.calls()), upstream: r.upstream, tried: r.tried, spend: r.spend,
	}
}

// restored is the held request s stores, put together as heldRequestOf puts
// one together of its line: unpriced, exactly as it was decoded.
func (s *storedRequest) restored() heldRequest {
	return newHeldRequest(s.decoded())
}

// stored is d as a stored form holds it.
func (d *decodedRequest) stored() storedRequest {
	return storedRequest{
		storedFlight: d.flight.stored(), Dir: d.dir, Model: d.model, Account: d.account, Reason: d.reason, From: d.from, Class: d.class,
		AgentID: d.agentID, Geo: d.geo, Prompt: d.prompt, Stop: d.stop, Tools: d.tools, Upstream: d.upstream, Tried: storedTriedOf(d.tried),
		Spend: d.spend,
	}
}

// decoded is all s stores of what a request decodes of its line, exactly as
// decodedOf decoded it.
func (s *storedRequest) decoded() decodedRequest {
	return decodedRequest{
		flight: s.flight(), dir: s.Dir, model: s.Model, account: s.Account, reason: s.Reason, from: s.From, class: s.Class, agentID: s.AgentID,
		geo: s.Geo, prompt: s.Prompt, stop: s.Stop, tools: s.Tools, upstream: s.Upstream, tried: triedOf(s.Tried), spend: s.Spend,
	}
}

// storedTriedOf is tried as a stored form holds it: none of none.
func storedTriedOf(tried []ledger.Tried) []storedTried {
	return eachAs(tried, func(t ledger.Tried) storedTried { return storedTried(t) })
}

// triedOf is the accounts stored holds: none of none, and of an empty list,
// as a digest an earlier release wrote may hold, as decodedOf has it.
func triedOf(stored []storedTried) []ledger.Tried {
	return eachAs(stored, func(t storedTried) ledger.Tried { return ledger.Tried(t) })
}

// eachAs is items, each as as gives it, in order: none of none, or of an
// empty list, as a stored form keeps no empty list.
func eachAs[T, U any](items []T, as func(T) U) []U {
	if len(items) == 0 {
		return nil
	}
	each := make([]U, len(items))
	for i, item := range items {
		each[i] = as(item)
	}
	return each
}

// stored is f as a stored form holds it.
func (f flight) stored() storedFlight {
	return storedFlight{Request: f.request, At: f.at.stored(), End: f.end.stored()}
}

// flight is the flight s stores.
func (s storedFlight) flight() flight {
	return flight{request: s.Request, at: s.At.instant(), end: s.End.instant()}
}

// stored is i as a stored form holds it.
func (i instant) stored() storedInstant {
	return storedInstant{Sec: i.sec, Nsec: i.nsec}
}

// instant is the instant s stores.
func (s storedInstant) instant() instant {
	return instant{sec: s.Sec, nsec: s.Nsec}
}
