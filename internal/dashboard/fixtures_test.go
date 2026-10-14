package dashboard

import (
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// The sample sessions' ids, each a Claude Code session's.
const (
	idD28C = "d28c5e17-3a9b-4f60-8c21-7e4d9b0a6f13"
	idDB8A = "db8a71e3-9f04-4c6d-b2a8-5e1f7c39d860"
	id7F3A = "7f3a0c94-5d2e-4b18-a6f7-31c8e9d24b05"
	idC61B = "c61b2f8d-04a7-4e93-9b5c-d81e6a7f2c34"
	id41E0 = "41e0b6c2-8d3f-4a71-95e4-2c7b0f9a1d68"
)

// The sample sessions' models.
const (
	opus   = "claude-opus-5-5"
	sonnet = "claude-sonnet-5-5"
	haiku  = "claude-haiku-4-5"
)

// seatOf is a session's model on an account, as the router assigns it: put
// there made before now, for the reason given, and last seen seen before
// now.
func seatOf(model, account, reason string, made, seen time.Duration) status.Assignment {
	family := strings.Split(model, "-")[1]
	return status.Assignment{
		Model: model, Family: family, Account: account, Reason: reason,
		AssignedAt: now.Add(-made).UTC(), LastSeen: now.Add(-seen).UTC(),
	}
}

// flippingSessions are the sessions the router lists of threeRouted, the
// one seen last first: idDB8A, whose sonnet is on work and whose opus a
// limit moved to side; idD28C, on work since it started; idC61B, new on
// side; 41e0, which the limit moved to side, idle; and 7f3a, on work, idle.
func flippingSessions() []status.Session {
	return []status.Session{
		{ID: idDB8A, Assignments: []status.Assignment{
			seatOf(sonnet, "work", "new", 2*time.Hour, 3*time.Second),
			seatOf(opus, "side", "moved: personal has no room", 50*time.Minute, 4*time.Second),
		}},
		{ID: idC61B, Assignments: []status.Assignment{seatOf(opus, "side", "new", time.Minute, 5*time.Second)}},
		{ID: idD28C, Assignments: []status.Assignment{seatOf(opus, "work", "new", 2*time.Hour, 50*time.Second)}},
		{ID: id41E0, Assignments: []status.Assignment{seatOf(sonnet, "side", "moved: personal has no room", 50*time.Minute, 4*time.Minute)}},
		{ID: id7F3A, Assignments: []status.Assignment{seatOf(haiku, "work", "new", 3*time.Hour, 9*time.Minute)}},
	}
}

// rowOf is row y of the canvas as text, its trailing blanks trimmed.
func rowOf(c *canvas, y int) string {
	return strings.TrimRight(c.row(y, Look{}), " ")
}
