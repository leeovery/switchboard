package watch

import (
	"context"
	"maps"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/status"
)

// Once reads what the dashboard is drawn from, once, as a watch's first read
// reads it: the document, as r asks, and from the router, the sessions it
// lists, and GET /history for every window the document has, carried on by
// the document's own readings, as a watch carries it on. Probing, from a
// router from before GET /history, or of a window whose history the router
// didn't give, as when asking for it failed, there's no history, and the
// charts draw the room each window has now: a reading of the document's
// alone, with none to follow, draws no more.
func Once(ctx context.Context, source Source, r status.Read) (dashboard.Snapshot, error) {
	got := fetchFrom(ctx, source, r)
	if got.err != nil {
		return dashboard.Snapshot{}, got.err
	}
	snap := dashboard.Snapshot{Doc: got.doc, Sessions: got.sessions}
	if routed(got.doc) {
		h := history{}.answered(histories(ctx, source, got.doc, 0))
		given := h.drawn(got.doc)
		if !h.outdated {
			h = h.saw(got.doc)
		}
		snap.History, snap.Outdated = h.drawn(got.doc), h.outdated
		maps.DeleteFunc(snap.History, func(ref dashboard.Ref, _ dashboard.Trail) bool {
			_, ok := given[ref]
			return !ok
		})
	}
	return snap, nil
}
