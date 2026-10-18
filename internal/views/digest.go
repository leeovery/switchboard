package views

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/leeovery/switchboard/internal/ledger"
)

// Digests are the views' ledger.Digester, which the router's ledger digests
// each finished day with: each session the day names held as of its end, as
// the store would hold it, and stored, as a day's digest stores it. They
// open nothing: the ledger hands them every line they fold.
type Digests struct{}

// Version is the version of the states Digest gives, DigestVersion.
func (Digests) Version() int {
	return DigestVersion
}

// Digest gives the state of each session day hands it at the day's end, by
// its id, as stateAsOf has it: none of one whose base won't decode or
// restore, nor of one with a request that arrived after the day, as a change
// of time zone files one, which stored refuses; its readers read those
// whole, as they read a session of no state. It fails once ctx ends, looked
// at between sessions.
func (Digests) Digest(ctx context.Context, day ledger.DigestDay) (map[string]json.RawMessage, error) {
	d := LedgerDay{Start: day.Start, Date: day.Date}
	states := make(map[string]json.RawMessage, len(day.Sessions))
	for id, from := range day.Sessions {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("digest the sessions of %s: %w", day.Date, err)
		}
		// A state left out is no worse than no digest: the session is read
		// whole, never told otherwise than its lines tell it.
		if state, err := stateAsOf(from, d); err == nil {
			states[id] = state
		}
	}
	return states, nil
}

// stateAsOf is the state of the session from gives at the end of the day d,
// as heldAsOf holds it on d, sealed at the day's end, as a day's digest
// holds it, as state has it. Nothing priced is stored, so the built-in
// table prices it, whatever the config's: the state is the same whichever
// does. It fails as heldAsOf does, and where state refuses it, as of a line
// of it arrived after the day.
func stateAsOf(from ledger.SessionFrom, d LedgerDay) (json.RawMessage, error) {
	s, err := heldAsOf(from, ledger.Pricing, d, endOfDay(d))
	if err != nil {
		return nil, err
	}
	return s.state()
}
