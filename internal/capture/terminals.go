package capture

import (
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// terminalNames are the names of the terminal sessions the frames' sessions
// were started in, by their ids, as run told the router them. The rest were
// started outside every terminal session run reads, and are named by their
// worktree's name, or their id.
var terminalNames = map[string]string{
	idD28C: "router-fix",
	id7F3A: "api-auth",
	idDB8A: "terraform",
	id41E0: "handbook",
	idB9C4: "dashboard",
	id5B19: "ios-build",
	id2B6D: "billing",
	id5E7D: "billing",
	id3D7A: "guides",
	id8A2C: "guides",
}

// inTerminals are the lines given, each of a session terminalNames names
// naming its terminal session, as run told the router it, and each other as
// it is.
func inTerminals(lines []ledger.Line) []ledger.Line {
	for i := range lines {
		if name, ok := terminalNames[lines[i].Session]; ok {
			lines[i].Name = name
		}
	}
	return lines
}

// routedInTerminals are the sessions given, as the router lists them, each
// terminalNames names naming its terminal session beside its directory, and
// each other as it is.
func routedInTerminals(sessions []status.Session) []status.Session {
	for _, s := range sessions {
		name, ok := terminalNames[s.ID]
		if !ok {
			continue
		}
		for i := range s.Assignments {
			s.Assignments[i].SessionName = name
		}
	}
	return sessions
}
