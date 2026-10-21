package views

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// SessionDirCells are the most cells a session's directory takes where the
// Log names a session by its id and its directory, as status.DirName cuts
// it: its tables' session column, its panel's sessions, and an event's
// card.
const SessionDirCells = 12

// SessionDirMost are the most cells a session's directory takes where a
// column of sessions grows to name it whole, as status.DirName cuts it:
// Sessions' List's.
const SessionDirMost = 24

// SessionDirName names the directory a session runs in as the Log names a
// session by it: its last part, cut to SessionDirCells, as status.DirName
// cuts it; "" for none.
func SessionDirName(dir string) string {
	return status.DirName(dir, SessionDirCells)
}

// LogSessionSources are what the Log's panel's sessions are built from.
type LogSessionSources struct {
	// Read is the Log's read, which its Requests are built from, its lines
	// telling when each session's first today came; and Held the store,
	// whose stories tell each session as the List tells it.
	Read LogRead
	Held HeldSessions
	// Lines and Events count each session's lines today and its events
	// today, by its whole id, as the Log counts them: the panel's sessions
	// are those either counts.
	Lines, Events map[string]int
	// Ticked are the sessions the panel's filter ticks, by their whole ids,
	// which join its sessions though neither count counts them, as one
	// ticked before midnight, or of another day the find picked: each once.
	Ticked []string
	// Found are the sessions the Log's find picked, as FindSessions gives
	// them: what's known of one ticked that the read holds no lines of.
	Found []FoundSession
	// Running are the sessions the router lists, as GET /sessions gives
	// them: none without the router.
	Running []status.Session
	// Unlisted is set where no router lists the sessions, as the List's
	// sources say.
	Unlisted bool
	// Stream is what the request stream told.
	Stream Stream
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
	// Started is when its first line today came, as the Log counts its
	// lines: zero where none has come today.
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
// nothing, while it's busy, as Activity.Busy says, or in flight as last
// known, as it was; idle; or ended. since is when it started, when it was
// last at work, idle, or when its last request came, ended: zero for the
// others. It's said at the moment it's drawn, as idle comes once the busy
// minute has passed, with nothing read.
func (s LogSession) Doing(now time.Time) (doing LogDoing, since time.Time) {
	a := s.Activity
	switch {
	case a.Waiting():
		return LogAsking, time.Time{}
	case !s.Started.IsZero() && now.Sub(s.Started) < busyWithin:
		return LogStarted, s.Started
	case a.Busy(now) || a.InFlight():
		return LogBusy, time.Time{}
	case a.Doing == Ended:
		return LogEnded, a.Since
	}
	return LogIdle, a.Since
}

// LogSessions are today's sessions as the Log's panel lists them: those src
// counts lines or events of today, each as the List has it, from its story,
// as src's Read holds it told, and what the router and the request stream
// say of it, started at its first line today; and those src ticks that it
// counts none of, as tickedSession has them, so a session the filter keeps
// always has a tick; those that haven't ended first, seen last first, then
// those ended, the latest ended first. Each session's story is the store's,
// at src's now, as the List's is.
func LogSessions(src LogSessionSources) []LogSession {
	by := newLister(src.Running, src.Unlisted, src.Stream, src.Now)
	counted := countedIDs(src.Lines, src.Events)
	firsts := src.Read.firstsOn(Today(src.Now))
	held := src.Held.at(src.Now)
	var sessions []LogSession
	for _, id := range counted {
		listed := by.listed(id, held.storyOf(id))
		sessions = append(sessions, LogSession{
			ID: id, Dir: listed.Dir, Account: listed.Account, Lines: src.Lines[id], Events: src.Events[id],
			Activity: listed.Activity, Started: firsts[id],
		})
	}
	for _, id := range notAmong(src.Ticked, counted) {
		sessions = append(sessions, tickedSession(by, id, src.Found, held.storyOf(id)))
	}
	slices.SortFunc(sessions, inLogOrder)
	return sessions
}

// tickedSession is the session with the given id, ticked, as the panel lists
// it with no lines nor events today, so not started today: as the List has
// it, from its story s, where the store holds its lines; else as the find
// found it, among found, its directory as its day's lines named it,
// ended at its last request, its account not known; else its id alone, with
// what the router and the request stream say of it.
func tickedSession(by lister, id string, found []FoundSession, s *story) LogSession {
	listed := by.listed(id, s)
	ticked := LogSession{ID: id, Dir: listed.Dir, Account: listed.Account, Activity: listed.Activity}
	i := slices.IndexFunc(found, func(f FoundSession) bool { return f.ID == id })
	if i < 0 {
		return ticked
	}
	ticked.Dir = cmp.Or(ticked.Dir, found[i].Dir)
	if !s.begun {
		ticked.Activity = Activity{Doing: Ended, Since: found[i].Last, LastSeen: found[i].Last}
	}
	return ticked
}

// notAmong are the ids given that aren't among those, nor "", each once, in
// the order of their ids.
func notAmong(ids, among []string) []string {
	not := slices.DeleteFunc(slices.Clone(ids), func(id string) bool { return id == "" || slices.Contains(among, id) })
	slices.Sort(not)
	return slices.Compact(not)
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
