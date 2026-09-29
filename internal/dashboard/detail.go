package dashboard

import (
	"fmt"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// secondsWithin is how near an exhausted window's return has to be for its
// countdown to show seconds.
const secondsWithin = 10 * time.Minute

// detail says where a window is heading and when it resets, as fully as width
// cells allow: the reset's clock gives way first, then its countdown, and
// what's left is cut short. It never wraps.
func detail(w quota.Window, p score.Projection, now time.Time, width int) line {
	parts := detailParts(w, p, now)
	for n := len(parts); n > 1; n-- {
		if l := joined(parts[:n]); l.width() <= width {
			return l
		}
	}
	return joined(parts[:1]).fit(width)
}

// detailParts are what a window's detail line can say, most telling first.
func detailParts(w quota.Window, p score.Projection, now time.Time) []span {
	if p.Kind == score.Exhausted {
		if p.At.IsZero() {
			return []span{{"exhausted", exhaustedInk}}
		}
		return []span{
			{"back in " + backIn(now, p.At), exhaustedInk},
			{status.Clock(now, p.At), exhaustedInk},
		}
	}
	var parts []span
	if phrase := status.Projection(now, p); phrase != "" {
		parts = append(parts, span{phrase, projectionInk(w, p)})
	}
	if !w.ResetsAt.IsZero() {
		parts = append(parts, span{status.Resets(now, w.ResetsAt), dimInk}, span{status.Clock(now, w.ResetsAt), dimInk})
	}
	if len(parts) == 0 {
		parts = append(parts, span{"reset time unknown", dimInk})
	}
	return parts
}

// projectionInk is how loudly a projection speaks: running out is a
// warning, red once the window is in the red, while keeping pace is quiet.
func projectionInk(w quota.Window, p score.Projection) ink {
	switch {
	case p.Kind != score.RunsOut:
		return dimInk
	case w.Utilization >= redFrom:
		return ink{color: red}
	default:
		return warningInk
	}
}

// joined runs parts together, each after the first set off by a dot in its
// own ink.
func joined(parts []span) line {
	l := line{parts[0]}
	for _, p := range parts[1:] {
		l = append(l, span{" · " + p.text, p.ink})
	}
	return l
}

// backIn counts down from now to an exhausted window's return at t: in
// minutes and seconds, such as "07:42", within its last ten minutes, and as
// status.Countdown does before them.
func backIn(now, t time.Time) string {
	if !inSeconds(now, t) {
		return status.Countdown(now, t)
	}
	left := t.Sub(now)
	return fmt.Sprintf("%02d:%02d", int(left/time.Minute), int(left%time.Minute/time.Second))
}

// inSeconds reports whether a countdown from now to t shows seconds: t is
// less than ten minutes away.
func inSeconds(now, t time.Time) bool {
	left := t.Sub(now)
	return left > 0 && left < secondsWithin
}

// CountsSeconds reports whether a frame of doc drawn at now counts down in
// seconds, as it does to an exhausted window that's back in under ten
// minutes. A live view redraws every second while it does.
func CountsSeconds(doc status.Document, now time.Time) bool {
	for _, a := range doc.Accounts {
		for _, w := range a.Windows {
			if p := score.Project(w, now); p.Kind == score.Exhausted && inSeconds(now, p.At) {
				return true
			}
		}
	}
	return false
}
