package watch

import (
	"context"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/status"
)

// Snapshot is what the dashboard is drawn from, read once, as usage reads it
// to print the dashboard: the document; the sessions the router lists, for
// the cards' dots; the windows' history, for the charts; and whether the
// router is from before it gave its history and told of events.
type Snapshot struct {
	Doc      status.Document
	Sessions []status.Session
	History  dashboard.History
	Outdated bool
}

// Once reads what the dashboard is drawn from, once, as a watch's first read
// reads it: the document, as r asks, and from the router, the sessions it
// lists, and GET /history for every window the document has, carried on by
// the document's own readings, as a watch carries it on. Probing, or from a
// router from before GET /history, there's no history, and the charts draw
// the room each window has now.
func Once(ctx context.Context, source Source, r Read) (Snapshot, error) {
	got := fetchFrom(ctx, source, r, time.Now)
	if got.err != nil {
		return Snapshot{}, got.err
	}
	snap := Snapshot{Doc: got.doc, Sessions: got.sessions}
	if routed(got.doc) {
		h := history{}.answered(histories(ctx, source, got.doc, 0))
		if !h.outdated {
			h = h.saw(got.doc)
		}
		snap.History, snap.Outdated = h.drawn(got.doc), h.outdated
	}
	return snap, nil
}
