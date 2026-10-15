package claude

import "github.com/leeovery/switchboard/internal/score"

// Policy is Claude's say in scoring accounts: its windows, each named for the
// part it plays. Each call gives a Policy of its own, so no caller changes
// another's.
func Policy() score.Policy {
	return score.Policy{
		Shared:     SharedWindows(),
		Perishable: PerishableWindow,
		Tiebreak:   TiebreakWindow,
		Started:    StartedWindow,
		Pressure:   PressureWindow,
	}
}

// SharedWindows are the windows that apply to every model: an account's
// five-hour session and its week count every request it makes, and every
// response reports them. Any other window, such as Fable's weekly cap, counts
// only its own models' requests. Each call gives a slice of its own, so no
// caller changes another's.
func SharedWindows() []string {
	return []string{FiveHourWindow, WeekWindow}
}

// FiveHourWindow is an account's five-hour session: the window five hours
// long every model shares, as usage's --5h names it.
const FiveHourWindow = "5h"

// WeekWindow is an account's week: the window a week long every model shares,
// which History's Weeks counts the accounts' weeks by.
const WeekWindow = "7d"

// PerishableWindow is the window an account's perishability is measured on:
// the week, the shared window with days between resets. Using the account
// whose week resets soonest wastes the least; the five-hour window comes round
// too often to steer by.
const PerishableWindow = WeekWindow

// TiebreakWindow is the window whose reset decides between accounts whose
// weeks score near enough equal: the five-hour session, whose allowance left
// at its reset is lost.
const TiebreakWindow = FiveHourWindow

// StartedWindow is the window a request starts when it isn't running: the
// five-hour session, which begins at an account's first request after its
// last one ended, and resets five hours later. A probe is a request, so
// probing an account whose session has lapsed starts it.
const StartedWindow = FiveHourWindow

// PressureWindow is the window whose pace of use is watched: the five-hour
// session, which several busy sessions on one account run out together, each
// then rebuilding its cache on another account at once.
const PressureWindow = FiveHourWindow
