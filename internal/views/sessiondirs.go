package views

import (
	"cmp"
	"slices"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// SessionDirs are what LATELY names its sessions by: the sessions the router
// lists, and those the watch holds. Their zero value names every session by
// its id.
type SessionDirs struct {
	routed []status.Session
	held   HeldSessions
}

// SessionDirsOf are the directories of the sessions the router lists,
// routed, and of those held holds.
func SessionDirsOf(routed []status.Session, held HeldSessions) SessionDirs {
	return SessionDirs{routed: routed, held: held}
}

// of is the directory the session with the given id runs in, as the List
// has it: as the router gives it, where it lists it; else as the lines held
// last name it; "" where neither names one.
func (d SessionDirs) of(id string) string {
	if i := slices.IndexFunc(d.routed, func(s status.Session) bool { return s.ID == id }); i >= 0 {
		if dir := routedDir(d.routed[i]); dir != "" {
			return dir
		}
	}
	return d.held.dirOf(id)
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
