package capture

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/router"
)

// The answers the frames draw streaming back, as the characters the request
// stream counts of them: 1.2k tokens' worth, and ~860, at four characters a
// token.
const (
	chars1200 = 1200 * 4
	chars860  = 860 * 4
)

// request is a routed request the fixtures' router tells of, by its id: its
// session's model, and the account it goes out on.
type request struct {
	id, session, model, account string
}

// told is the request stream's event of the kind given of the request, at a
// time, on its first time upstream.
func (r request) told(kind string, at time.Time) router.StreamEvent {
	return router.StreamEvent{At: at.UTC(), Kind: kind, Request: r.id, Attempt: 1, Session: r.session, Model: r.model, Account: r.account}
}

// The Overview's requests in flight (dash3.SESSIONS), as the auto world's
// router lists their sessions: 41e0's on personal, of sonnet, and c61b's and
// d28c's on side, of opus; and how long before the frames' moment each was
// sent, which the router lists as when it last saw its session.
const (
	sent41E0 = 9 * time.Second
	sentC61B = 6 * time.Second
	sentD28C = 400 * time.Millisecond
)

var (
	inFlight41E0 = request{id: "00000401", session: id41E0, model: sonnet, account: "personal"}
	inFlightC61B = request{id: "00000402", session: idC61B, model: opus, account: "side"}
	inFlightD28C = request{id: "00000403", session: idD28C, model: opus, account: "side"}
)

// inFlight is what the auto world's request stream tells of at now: 41e0's
// request, sent 9s ago, its answer streaming since 8.1s ago, ~860 tokens of
// it so far; c61b's, sent 6s ago, streaming since 5.52s ago, 1.2k tokens so
// far; and d28c's, sent 0.4s ago, asking still. The answers' progress is
// told 0.25s ago, after d28c's ask, so the stream saw c61b last on side, as
// the frames order its sessions.
func inFlight(now time.Time) []router.StreamEvent {
	answering := func(r request, sent, first time.Duration, chars int) []router.StreamEvent {
		progress := r.told(router.StreamProgress, ago(now, 250*time.Millisecond))
		progress.Chars = chars
		return []router.StreamEvent{r.told(router.StreamSent, ago(now, sent)), r.told(router.StreamFirst, ago(now, first)), progress}
	}
	told := slices.Concat(
		answering(inFlight41E0, sent41E0, 8100*time.Millisecond, chars860),
		answering(inFlightC61B, sentC61B, 5520*time.Millisecond, chars1200),
		[]router.StreamEvent{inFlightD28C.told(router.StreamSent, ago(now, sentD28C))},
	)
	slices.SortStableFunc(told, func(a, b router.StreamEvent) int { return a.At.Compare(b.At) })
	return told
}
