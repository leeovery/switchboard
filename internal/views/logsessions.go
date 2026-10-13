package views

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// LogSessionSources are what the Log's panel's sessions are built from.
type LogSessionSources struct {
	// Read holds today's lines, and every line of their sessions, whatever
	// day it's of, as the Log reads them.
	Read LogRead
	// Lines and Events count each session's lines today and its events
	// today, by its whole id, as the Log counts them: the panel's sessions
	// are those either counts.
	Lines, Events map[string]int
	// Running are the sessions the router lists, as GET /sessions gives
	// them: none without the router.
	Running []status.Session
	// Unlisted is set where no router lists the sessions, as the List's
	// sources say.
	Unlisted bool
	// Stream is what the request stream told.
	Stream Stream
	// Prices are the price table its sessions' stories are told by.
	Prices ledger.Table
	Now    time.Time
}

// LogSession is a session of today's as the Log's panel lists it, each fact
// the List's of it, so the two never disagree.
type LogSession struct {
	// ID is its whole id, and Dir the directory it runs in, as the List has
	// it: "" where none names one.
	ID, Dir string
	// Account is the account it's on now, as the List names it, following
	// its main thread: "" where none is known.
	Account string
	// Lines and Events are its lines today and its events today, as the Log
	// counts them.
	Lines, Events int
	// Activity is what it's doing, as the List has it, one rule with the
	// Overview's SESSIONS.
	Activity Activity
	// Started is when its first request came, today or on an earlier day:
	// zero where none has come yet.
	Started time.Time
}

// LogDoing is what the panel's doing column says of a session.
type LogDoing int

const (
	// LogBusy says nothing: it's busy, but neither asking nor just started.
	LogBusy LogDoing = iota
	// LogAsking is a request of it waiting for its answer, as
	// Activity.Waiting says: asking, throttled or refused.
	LogAsking
	// LogStarted is its first request come within the busy minute.
	LogStarted
	// LogIdle is it idle, busy no more.
	LogIdle
	// LogEnded is it ended.
	LogEnded
)

// Doing is what the panel's doing column says of the session at now, the
// first that holds: asking, while a request of it waits, as Activity.Waiting
// says; started, while its first request came within the busy minute;
// nothing, while it's busy, as Activity.Busy says; idle; or ended. since is
// when it started, when it was last at work, idle, or when its last request
// came, ended: zero for the others. It's said at the moment it's drawn, as
// idle comes once the busy minute has passed, with nothing read.
func (s LogSession) Doing(now time.Time) (doing LogDoing, since time.Time) {
	a := s.Activity
	switch {
	case a.Waiting():
		return LogAsking, time.Time{}
	case !s.Started.IsZero() && now.Sub(s.Started) < busyWithin:
		return LogStarted, s.Started
	case a.Busy(now):
		return LogBusy, time.Time{}
	case a.Doing == Ended:
		return LogEnded, a.Since
	}
	return LogIdle, a.Since
}

// LogSessions are today's sessions as the Log's panel lists them: those src
// counts lines or events of today, each as the List has it, from its story,
// as its lines, of whatever day, tell it, and what the router and the
// request stream say of it; those that haven't ended first, seen last
// first, then those ended, the latest ended first.
func LogSessions(src LogSessionSources) []LogSession {
	today := Today(src.Now)
	by := newLister(src.Running, src.Unlisted, src.Stream, src.Now)
	var sessions []LogSession
	for _, id := range countedIDs(src.Lines, src.Events) {
		s := storyOf(requestsOf(src.Read.sessions[id], src.Prices, today.Date), src.Prices, today)
		listed := by.listed(id, s)
		sessions = append(sessions, LogSession{
			ID: id, Dir: listed.Dir, Account: listed.Account, Lines: src.Lines[id], Events: src.Events[id],
			Activity: listed.Activity, Started: listed.Started,
		})
	}
	slices.SortFunc(sessions, inLogOrder)
	return sessions
}

// countedIDs are the ids of the sessions either count counts any of, each
// once, in the order of their ids.
func countedIDs(counts ...map[string]int) []string {
	var ids []string
	for _, c := range counts {
		for id, n := range c {
			if id != "" && n > 0 {
				ids = append(ids, id)
			}
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}

// inLogOrder orders the panel's sessions: those that haven't ended first,
// seen last first, as the router lists them; then those ended, the latest
// ended first; each by its id where nothing else tells them apart.
func inLogOrder(a, b LogSession) int {
	if ae, be := a.Activity.Doing == Ended, b.Activity.Doing == Ended; ae != be {
		if ae {
			return 1
		}
		return -1
	}
	return cmp.Or(b.Activity.LastSeen.Compare(a.Activity.LastSeen), strings.Compare(a.ID, b.ID))
}
