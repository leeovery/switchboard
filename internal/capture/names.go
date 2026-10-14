package capture

import "time"

// The frames' sessions every world shares, each a Claude Code session's id,
// which the frames cut to its first four characters.
const (
	idD28C = "d28c5e17-3a9b-4f60-8c21-7e4d9b0a6f13"
	id7F3A = "7f3a0c94-5d2e-4b18-a6f7-31c8e9d24b05"
	idDB8A = "db8a71e3-9f04-4c6d-b2a8-5e1f7c39d860"
	idC61B = "c61b2f8d-04a7-4e93-9b5c-d81e6a7f2c34"
	id41E0 = "41e0b6c2-8d3f-4a71-95e4-2c7b0f9a1d68"
)

// The models the frames' sessions use.
const (
	opus   = "claude-opus-5-5"
	sonnet = "claude-sonnet-5-5"
)

// The windows' keys, as Claude names them.
const (
	fiveHourKey = "5h"
	weekKey     = "7d"
	fableKey    = "7d_oi"
)

// readAgo is how long before the moment drawn the router's document was
// read.
const readAgo = 4 * time.Second

// ago is d before now.
func ago(now time.Time, d time.Duration) time.Time {
	return now.Add(-d)
}

// personalLimit is the identity of personal's limit of yesterday evening, as
// the router numbers its limits from 1 as it starts: the run's first.
const personalLimit = 1

// forced marks, as its ForcedBy, a move a world's router tells of as one a
// cap, a limit or a refusal forced, which ofTheRun counts in it.
const forced = -1
