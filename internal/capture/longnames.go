package capture

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// longLabels are the labels the world of long names gives the scene's
// accounts, by their ids: 14, 17 and 22 cells, longer than any view gives
// a label at the owner's sizes.
var longLabels = map[string]string{
	"work":     "work-team-plan",
	"personal": "personal-research",
	"side":     "side-hustle-consulting",
}

// longDirs are the directories the world of long names runs the frames'
// sessions in, by the frames' own: their names 23 to 31 cells, longer than
// the Log gives a directory, so its cuts show; the List shows the 23-cell
// ones whole and cuts the rest at its most.
var longDirs = map[string]string{
	"~/Code/switchboard": "~/Code/switchboard-dashboard-fluid",
	"~/Code/mobile":      "~/Code/mobile-app-offline-sync",
	"~/Code/api":         "~/Code/api-gateway-rate-limits",
	"~/Code/infra":       "~/Code/infra-terraform-modules",
	"~/Code/docs":        "~/Code/docs-site-getting-started",
	"~/Code/website":     "~/Code/website-marketing-redesign-2026",
}

// longNames is the scene's world with long names, so a view's cut name shows
// in its frame: each account labelled as longLabels has it, and each
// session, as the router lists it, as the ledger holds its lines and as the
// router records its events, in the directory and project longDirs gives in
// place of its own, so an event's directory agrees with its session's lines.
func (a accountWorlds) longNames() accountWorld {
	return a.kept("longNames", func() accountWorld {
		w := a.scene()
		for i, account := range w.accounts {
			w.accounts[i].Label = longLabels[account.ID]
		}
		routed := w.routed
		w.routed = func(now time.Time) []status.Session { return inLongDirs(routed(now)) }
		w.lines = func(now time.Time) []ledger.Line {
			lines := sessionsLines(now)
			for i := range lines {
				lines[i].Dir, lines[i].Project = longDir(lines[i].Dir), longDir(lines[i].Project)
			}
			return lines
		}
		w.events, w.ownEvents = eventsInLongDirs(w.events), true
		return w
	})
}

// eventsInLongDirs are the events given, each in the directory and project
// longDir gives in place of its own.
func eventsInLongDirs(told []status.Event) []status.Event {
	long := slices.Clone(told)
	for i := range long {
		long[i].Dir, long[i].Project = longDir(long[i].Dir), longDir(long[i].Project)
	}
	return long
}

// inLongDirs are the sessions given, each assignment in the directory and
// project longDir gives in place of its own.
func inLongDirs(sessions []status.Session) []status.Session {
	for _, s := range sessions {
		for i := range s.Assignments {
			s.Assignments[i].Dir, s.Assignments[i].Project = longDir(s.Assignments[i].Dir), longDir(s.Assignments[i].Project)
		}
	}
	return sessions
}

// longDir is the directory longDirs gives in place of dir, or dir itself,
// where it gives none, as of a request of no session's.
func longDir(dir string) string {
	if long, ok := longDirs[dir]; ok {
		return long
	}
	return dir
}
