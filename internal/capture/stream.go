package capture

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

// chars1200 is an answer the frames draw streaming back, as the characters
// the request stream counts of it: 1.2k tokens' worth, at four characters a
// token.
const chars1200 = 1200 * 4

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

// storyboardAt is when the storyboard's frames are drawn, of the day now is:
// 14:43:02, as its last frame's LOG has the move it tells of at 14:43.
func storyboardAt(now time.Time) time.Time {
	return on(now, 1, 14, 43).Add(2 * time.Second)
}

// storyboard is the router the storyboard's frames draw at now: work,
// personal and side, but side with c61b and db8a alone on it, as the
// storyboard has them; and where limited is set, work at its session's limit,
// reached at 14:43, which holds until the session resets at 17:10, the
// router telling of it, the sessions it moves not yet counted.
func storyboard(now time.Time, limited bool) source {
	w, s := work(now), side(now)
	s.seats = s.seats[:2]
	events := lately(now)
	if limited {
		reached := on(now, 1, 14, 43)
		w.session = fiveHours{used: 1, resets: w.session.resets, limited: reached, limit: workLimit}
		events = slices.Insert(events, 0, status.Event{At: reached, Kind: status.EventLimit, Account: w.id, Windows: []string{fiveHourKey}, Until: w.session.resets, Limit: workLimit})
	}
	return newSource(now, []sample{w, personal(now), s}, "side", events...)
}

// The storyboard's requests: c61b's on side, and d28c's on work, which work's
// limit refuses, and on side, where the router sends it again.
var (
	c61bOnSide = request{id: "00000201", session: idC61B, model: opus, account: "side"}
	d28cOnWork = request{id: "00000301", session: idD28C, model: opus, account: "work"}
	d28cOnSide = request{id: "00000301", session: idD28C, model: opus, account: "side"}
)

// requestOut is the storyboard's first frame, at now: c61b sent a request
// 0.3s ago, its pulse over half way along its cord to side.
func requestOut(now time.Time) source {
	return storyboard(now, false).telling(c61bOnSide.told(router.StreamSent, ago(now, 300*time.Millisecond)))
}

// streamingBack is the storyboard's second frame, at now: c61b's request,
// sent 6s ago, its answer streaming back since its first byte came 5.52s
// ago, 4,800 characters of it so far, 1.2k tokens, its shimmer 69 steps on.
func streamingBack(now time.Time) source {
	progress := c61bOnSide.told(router.StreamProgress, ago(now, 250*time.Millisecond))
	progress.Chars = chars1200
	return storyboard(now, false).telling(
		c61bOnSide.told(router.StreamSent, ago(now, 6*time.Second)),
		c61bOnSide.told(router.StreamFirst, ago(now, 5520*time.Millisecond)),
		progress,
	)
}

// refused is the storyboard's third frame, at now: d28c's request, sent 0.4s
// ago, refused with a 429 as work reached its limit, 0.14s ago, its red pulse
// a quarter of the way back along its cord.
func refused(now time.Time) source {
	limited := d28cOnWork.told(router.StreamLimited, ago(now, 140*time.Millisecond))
	limited.Status = 429
	return storyboard(now, true).telling(d28cOnWork.told(router.StreamSent, ago(now, 400*time.Millisecond)), limited)
}

// repatched is the storyboard's last frame, at now: d28c's request, refused
// by work's limit 1.07s ago, moved to side, and sent again there, as the
// router retries it; re-patched once its refusal bounced back, 0.53s ago, its
// pulse almost at side's jack.
func repatched(now time.Time) source {
	refusedAt := ago(now, 1070*time.Millisecond)
	limited := d28cOnWork.told(router.StreamLimited, refusedAt)
	limited.Status = 429
	moved := d28cOnSide.told(router.StreamMoved, refusedAt)
	moved.From, moved.To, moved.Reason = "work", "side", "moved: work hit its limit"
	again := d28cOnSide.told(router.StreamSent, refusedAt)
	again.Attempt = 2
	return storyboard(now, true).telling(d28cOnWork.told(router.StreamSent, ago(now, 1300*time.Millisecond)), limited, moved, again)
}
