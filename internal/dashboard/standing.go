package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// standing is how an account's window stands at a moment, as its panel shows
// it, as the moment has it: how much of it is used, where it's heading, where
// it runs out, and whether it's at its limit or not started.
type standing struct {
	// account is the id of the account whose window it is.
	account string
	w       status.WindowNow
}

// ref is the window's, by its account and key.
func (s standing) ref() Ref {
	return Ref{Account: s.account, Window: s.w.Key}
}

// lapsed reports whether it stands not started: it isn't running, and reads
// empty, until a request starts it.
func (s standing) lapsed() bool {
	return s.w.Kind == status.WindowNotStarted
}

// runsOut reports whether it runs out before it resets at the pace it's
// heading at, as its Out says, whatever holds it.
func (s standing) runsOut() bool {
	return !s.w.Out.At.IsZero()
}

// projected is the share of the window it's heading for by its reset, as a
// bar shows it beyond its use, as its Projected has it: reporting false where
// nothing says, and while it's held at its limit or not started.
func (s standing) projected() (float64, bool) {
	if s.w.Projected == nil {
		return 0, false
	}
	return *s.w.Projected, true
}

// paceCell is the cell of the window's bar, cells long, where even use
// across the window would put it, at its even pace: noMarker, leaving the
// marker off, where that can't be said, and while it's held at its limit or
// not started.
func (s standing) paceCell(cells int) int {
	if s.w.Pace == nil || s.w.Held || s.lapsed() {
		return noMarker
	}
	return cellAt(*s.w.Pace, cells)
}

// headed says where the window is heading at now, its times as when shows
// them: that it's back at a time, at its limit; that it runs out at a time,
// as in "→ out ~16:05", with tilde as given; that it's heading for a share
// by its reset, as in "→ 87%", calling for attention where that nears where
// its account runs out, as its Nearing has it; or that it hasn't started.
// It's empty where nothing says.
func (s standing) headed(now time.Time, tilde string, when func(now, t time.Time) string) span {
	switch {
	case s.lapsed():
		return span{"not started", dimInk}
	case s.w.Held && !s.w.Back.IsZero():
		return span{"back " + when(now, s.w.Back), exhaustedInk}
	case s.w.Held:
		return span{}
	case s.runsOut():
		return span{"→ out " + tilde + when(now, s.w.Out.At), alertInk}
	}
	share, ok := s.projected()
	switch {
	case !ok:
		return span{}
	case s.w.Nearing:
		return span{"→ " + prose.Percent(share), warningInk}
	default:
		return span{"→ " + prose.Percent(share), mutedInk}
	}
}
