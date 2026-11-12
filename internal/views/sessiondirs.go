package views

import (
	"cmp"
	"slices"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// SessionDirs are what LATELY names its sessions by, beside the directory
// an event gives: the sessions the router lists, and those the watch holds.
// Their zero value names a session by its event's directory alone.
type SessionDirs struct {
	routed []status.Session
	held   HeldSessions
}

// SessionDirsOf are the directories of the sessions the router lists,
// routed, and of those held holds.
func SessionDirsOf(routed []status.Session, held HeldSessions) SessionDirs {
	return SessionDirs{routed: routed, held: held}
}

// of is the directory the session an event names ran in: as the event gives
// it, so a line's name never changes once told; else as the List has it, as
// listedDir says, of the router's listing and the lines held; "" where none
// names one.
func (d SessionDirs) of(e status.Event) string {
	if e.Dir != "" {
		return e.Dir
	}
	var routed status.Session
	if i := slices.IndexFunc(d.routed, func(s status.Session) bool { return s.ID == e.Session }); i >= 0 {
		routed = d.routed[i]
	}
	return listedDir(routed, d.held.dirOf(e.Session))
}

// sessionNamed is the session with the given id, running in dir, named in a
// sentence: by its directory, as SessionDirName names it, where dir names
// one, else as status.ShownID names it; none for "".
func sessionNamed(id, dir string) status.Words {
	if prose.Clean(id) == "" {
		return nil
	}
	return clause(cmp.Or(SessionDirName(dir), status.ShownID(id)))
}
