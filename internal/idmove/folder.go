package idmove

import (
	"context"
	"strings"

	"github.com/leeovery/switchboard/internal/identity"
)

// MoveFolder moves the history in dir, laid out as a state directory lays out
// its history, as import stages an exported folder, onto the ids record has
// made: each account it names by an id one of record's had before
// switchboard made ids is renamed to that account's id, as the move renames
// it, and every other is left as it is. So a folder exported before the move
// reads as one exported after, and one exported after, naming the new ids,
// reads as it is, as no id made is one an account had before. It stops,
// failing, once ctx ends.
func MoveFolder(ctx context.Context, dir string, record identity.Record) error {
	rename := formerIDs(record)
	if rename.none() {
		return nil
	}
	m := &Mover{StateDir: dir}
	return m.renameHistory(ctx, m.history(), rename)
}

// formerIDs are the ids record's accounts had before switchboard made ids,
// each renamed to the id it made.
func formerIDs(record identity.Record) renames {
	r := make(renames)
	for _, e := range record.Accounts {
		if e.Was != "" {
			r[strings.ToLower(e.Was)] = e.ID
		}
	}
	return r
}
