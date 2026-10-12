package dashboard

import "slices"

// The badges at the right of a panel's top edge: the primary account's, the
// pin's on each account the global pin names, and the next's on the account
// new sessions go to.
const (
	primaryBadge = "◆ primary"
	pinBadge     = "● pinned"
	nextBadge    = "▲ next"
)

// leavingOff is the badges, with ◆ primary left off, then ▲ next, until fit
// reports that they fit: ● pinned, the pin's only mark on a panel, is never
// left off.
func leavingOff(badges []span, fit func([]span) bool) []span {
	for _, off := range []string{primaryBadge, nextBadge} {
		if fit(badges) {
			break
		}
		badges = slices.DeleteFunc(slices.Clone(badges), func(b span) bool { return b.text == off })
	}
	return badges
}

// badgeLine is the badges in a line, a dot in the border's ink between each.
func badgeLine(badges []span, border ink) line {
	var l line
	for i, b := range badges {
		if i > 0 {
			l = append(l, span{" · ", border})
		}
		l = append(l, b)
	}
	return l
}
