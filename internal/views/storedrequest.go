package views

import (
	"slices"

	"github.com/leeovery/switchboard/internal/ledger"
)

// storedRequest is a held request as a stored form holds it: all it
// decodes of its line, what it spent in ledger.Spend's own stored form, the
// tools its answer called as a list, and the accounts it went out on, an
// empty list none, as decodedOf has it. What's worked out of that, how long
// its writes to the cache last and its sum, is worked out again as it's
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

// stored is r as a stored form holds it.
func (r *heldRequest) stored() storedRequest {
	return storedRequest{
		storedFlight: r.flight.stored(), Dir: r.dir.Value(), Model: r.model.Value(), Account: r.account.Value(), Reason: r.reason.Value(),
		From: r.from.Value(), Class: r.class.Value(), AgentID: r.agentID.Value(), Geo: r.geo.Value(), Prompt: r.prompt.Value(),
		Stop: r.stop.Value(), Tools: slices.Collect(r.calls()), Upstream: r.upstream, Tried: storedTriedOf(r.tried), Spend: r.spend,
	}
}

// restored is the held request s stores, put together as heldRequestOf puts
// one together of its line: unpriced, exactly as it was decoded.
func (s *storedRequest) restored() heldRequest {
	return newHeldRequest(decodedRequest{
		flight: s.flight(), dir: s.Dir, model: s.Model, account: s.Account, reason: s.Reason, from: s.From, class: s.Class, agentID: s.AgentID,
		geo: s.Geo, prompt: s.Prompt, stop: s.Stop, tools: s.Tools, upstream: s.Upstream, tried: triedOf(s.Tried), spend: s.Spend,
	})
}

// storedTriedOf is tried as a stored form holds it: none of none.
func storedTriedOf(tried []ledger.Tried) []storedTried {
	if len(tried) == 0 {
		return nil
	}
	stored := make([]storedTried, len(tried))
	for i, t := range tried {
		stored[i] = storedTried{Account: t.Account, Why: t.Why}
	}
	return stored
}

// triedOf is the accounts stored holds: none of none, and of an empty list,
// as a digest an earlier release wrote may hold, as decodedOf has it.
func triedOf(stored []storedTried) []ledger.Tried {
	if len(stored) == 0 {
		return nil
	}
	tried := make([]ledger.Tried, len(stored))
	for i, t := range stored {
		tried[i] = ledger.Tried{Account: t.Account, Why: t.Why}
	}
	return tried
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
