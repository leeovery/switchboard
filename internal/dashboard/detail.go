package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

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
			{"back in " + status.Countdown(now, p.At), exhaustedInk},
			{clock(now, p.At), exhaustedInk},
		}
	}
	var parts []span
	if phrase := status.Projection(now, p); phrase != "" {
		parts = append(parts, span{phrase, projectionInk(w, p)})
	}
	if !w.ResetsAt.IsZero() {
		parts = append(parts, span{status.Resets(now, w.ResetsAt), dimInk}, span{clock(now, w.ResetsAt), dimInk})
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
		return ink{color: orange}
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

// clock shows t in now's time zone, such as "Mon 18:10".
func clock(now, t time.Time) string {
	return status.Clock(t.In(now.Location()))
}
