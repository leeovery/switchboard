package score

import "time"

// atRisk is the share of its perishable window an account must be heading to
// leave unused at its reset before a session moves to it at its compaction:
// past it, that's room that would otherwise be lost.
const atRisk = 0.02

// Verdict is what AtCompaction judges of moving a session at its compaction.
type Verdict int

const (
	// StaysUnknown keeps the session where it is, as the reset of either
	// account's perishable window isn't known, or has passed since it was
	// read; or, its reset coming first, how fast the account chosen's has
	// been used lately isn't known; or no account was chosen to move it to.
	StaysUnknown Verdict = iota
	// StaysSame keeps it where it is, as that's the account chosen.
	StaysSame
	// StaysResetsLater keeps it where it is, as the account chosen's
	// perishable window resets no sooner than its own account's.
	StaysResetsLater
	// StaysNotAtRisk keeps it where it is, as the account chosen would leave
	// no more than atRisk of its perishable window unused at its reset.
	StaysNotAtRisk
	// Moves moves it to the account chosen.
	Moves
)

// Move is how AtCompaction judges moving a session at its compaction, and
// what it judged by.
type Move struct {
	Verdict Verdict
	// To is the account chosen, "" when none was.
	To string
	// Left is the share of To's perishable window it would leave unused at
	// its reset, as LeftAtReset says: zero when that isn't known.
	Left float64
}

// AtCompaction judges, at now, whether a session on own, the account it's
// on, moves to choice, the account a new session would go to, at the first
// request of its main conversation after its compaction, which has little of
// its cache left to lose: where choice is another account, its perishable
// window resets before own's does, and, used from now on at its recent rate,
// which must be known, it would leave more than atRisk of that window unused
// at its reset, as LeftAtReset says.
func (p Policy) AtCompaction(own, choice Candidate, now time.Time) Move {
	m := Move{To: choice.ID}
	if choice.ID == own.ID {
		m.Verdict = StaysSame
		return m
	}
	ours, oursKnown := p.perishableReset(own, now)
	theirs, theirsKnown := p.perishableReset(choice, now)
	left, leftKnown := p.LeftAtReset(choice, now)
	m.Left = left
	switch {
	case !oursKnown || !theirsKnown:
		m.Verdict = StaysUnknown
	case !theirs.Before(ours):
		m.Verdict = StaysResetsLater
	case !leftKnown:
		m.Verdict = StaysUnknown
	case left <= atRisk+Tolerance:
		m.Verdict = StaysNotAtRisk
	default:
		m.Verdict = Moves
	}
	return m
}

// LeftAtReset returns the share of the candidate's perishable window it would
// leave unused at the window's reset, used from now on at its PerishableRate,
// as ProjectAt says, its room ending at its reserve: none where that brings
// the window to its reserve first. It reports false where that can't be
// said: the window is missing, its reset isn't known, or it has reset since
// it was read; or its PerishableRate isn't known, as a rate made up would
// make up what's left.
func (p Policy) LeftAtReset(c Candidate, now time.Time) (float64, bool) {
	w, ok := find(c.Windows, p.Perishable)
	if !ok || w.ResetsAt.IsZero() || !c.PerishableKnown {
		return 0, false
	}
	switch at := ProjectAt(w, c.PerishableRate, now); at.Kind {
	case OnPace:
		return max(1-c.Reserve-at.AtReset, 0), true
	case RunsOut, Exhausted:
		return 0, true
	}
	return 0, false
}

// perishableReset returns when the candidate's perishable window resets,
// reporting false where that isn't known: the window is missing, its reset
// isn't known, or it has reset since it was read.
func (p Policy) perishableReset(c Candidate, now time.Time) (time.Time, bool) {
	w, ok := find(c.Windows, p.Perishable)
	if !ok || w.ResetsAt.IsZero() || w.ResetBy(now) {
		return time.Time{}, false
	}
	return w.ResetsAt, true
}
