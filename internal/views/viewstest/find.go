package viewstest

import (
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// FindMoment is the request ledger and the router's sessions at a moment the
// Log's find has to get right, what's typed on its line then, and the
// sessions it finds once it has gone back through every day.
type FindMoment struct {
	Name string
	// Lines are the ledger's lines, every day's, oldest first, each filed
	// under the local day its request arrived on: each day's summary names
	// the sessions of its lines.
	Lines []ledger.Line
	// Sessions are those the router lists, as GET /sessions gives them.
	Sessions []status.Session
	// Typed is what's typed on the find's line, read at Now.
	Typed string
	Now   time.Time
	// Want are the sessions it finds, in order.
	Want []Found
}

// Found is a session the find finds, as a moment wants it: its id and its
// directory; the date of the day it's found on; whether it has ended; and
// when its last request that day came, zero where it hasn't ended.
type Found struct {
	Session, Dir, Date string
	Ended              bool
	Last               time.Time
}

// The find's sessions: those of today's and earlier days whose directories'
// names hold alike, and those whose ids start alike, one of them written in
// capitals.
const (
	apiServer  = "a1f0c3d2-4b5a-4e6f-8a7b-9c0d1e2f3a4b"
	apiGateway = "a2b3c4d5-6e7f-4a8b-9c0d-1e2f3a4b5c6d"
	apiary     = "a3c4d5e6-7f8a-4b9c-8d0e-2f3a4b5c6d7e"
	website    = "b2e1d4c3-5a6b-4f7e-9b8a-0d1c2e3f4a5b"
	apiOld     = "c3d2e5f4-6b7c-4a8f-8c9b-1e2d3f4a5b6c"
	apiDocs    = "d4c3f6e5-7c8d-4b9a-9d0c-2f3e4a5b6c7d"
	rapid      = "e5b4a7f6-8d9e-4c0b-8e1d-3a4f5b6c7d8e"
	mobile     = "f6a5b8a7-9e0f-4d1c-9f2e-4b5a6c7d8e9f"
	infraA     = "7f3a0b1c-2d3e-4f5a-8b6c-7d8e9f0a1b2c"
	infraB     = "7F3A9E8D-7C6B-4A5F-9E4D-3C2B1A0F9E8D"
	near       = "7f3b1c2d-3e4f-4a5b-8c6d-7e8f9a0b1c2d"
	within     = "c61b7f3a-4e5d-4c6b-9a7f-8e9d0c1b2a3f"
	site       = "5e1d2c3b-4a5f-4e6d-8c7b-9a0f1e2d3c4b"
	tools      = "9c2e8a7b-6f5e-4d4c-9b3a-2f1e0d9c8b7a"
	oldLedger  = "e9a1b2c3-d4e5-4f6a-8b7c-9d0e1f2a3b4c"
)

// FindMoments are the moments the Log's find has to get right, on Thursday 1
// October 2026, going back through the days before: a directory's name
// matching sessions today, those running seen last first, then one ended,
// yesterday, and a week back, whatever its case; an id's start matching two,
// whatever its case, but neither one starting otherwise nor one holding it
// further on; one session on three days, found once, today, and another on
// two days before, found once, on its newest; and an ended session of
// another day, found with when its last request came, for the Log to pause
// at, beside one of today's that ended.
func FindMoments() []FindMoment {
	return []FindMoment{dirOnThreeDays(), idStartOfTwo(), oneOnThreeDays(), endedOfAnotherDay()}
}

func dirOnThreeDays() FindMoment {
	return FindMoment{
		Name: "a directory's name matching sessions today, yesterday and a week back",
		Lines: []ledger.Line{
			ranIn("~/work/rapid", requested("r1", rapid, Opus, work, status.ReasonNew, onDay(-6, 10, 0, 0))),
			ranIn("~/Code/API-docs", requested("r2", apiDocs, Opus, work, status.ReasonNew, onDay(-1, 9, 0, 0))),
			ranIn("~/Code/mobile", requested("r3", mobile, Opus, side, status.ReasonNew, onDay(-1, 11, 0, 0))),
			ranIn("~/Code/api", requested("r4", apiOld, Opus, side, status.ReasonNew, onDay(0, 15, 0, 0))),
			ranIn("~/Code/apiary", requested("r5", apiary, Opus, work, status.ReasonNew, At(10, 0, 0))),
			ranIn("~/Code/web", requested("r6", website, Opus, side, status.ReasonNew, At(13, 0, 0))),
			ranIn("~/Code/api-server", requested("r7", apiServer, Opus, side, status.ReasonNew, At(14, 30, 0))),
			ranIn("~/Code/api-gateway", requested("r8", apiGateway, Opus, side, status.ReasonNew, At(14, 41, 0))),
		},
		Sessions: []status.Session{
			session(apiServer, seat(Opus, side, At(14, 30, 10), "")),
			session(website, seat(Opus, side, At(13, 0, 10), "")),
			session(apiGateway, seat(Opus, side, At(14, 41, 10), "")),
		},
		Typed: "api",
		Now:   now,
		Want: []Found{
			{Session: apiGateway, Dir: "~/Code/api-gateway", Date: "2026-10-01"},
			{Session: apiServer, Dir: "~/Code/api-server", Date: "2026-10-01"},
			{Session: apiary, Dir: "~/Code/apiary", Date: "2026-10-01", Ended: true, Last: At(10, 0, 0)},
			{Session: apiOld, Dir: "~/Code/api", Date: "2026-09-30", Ended: true, Last: onDay(0, 15, 0, 0)},
			{Session: apiDocs, Dir: "~/Code/API-docs", Date: "2026-09-29", Ended: true, Last: onDay(-1, 9, 0, 0)},
			{Session: rapid, Dir: "~/work/rapid", Date: "2026-09-24", Ended: true, Last: onDay(-6, 10, 0, 0)},
		},
	}
}

func idStartOfTwo() FindMoment {
	return FindMoment{
		Name: "an id's start matching two",
		Lines: []ledger.Line{
			ranIn("~/Code/infra", requested("r1", infraB, Opus, work, status.ReasonNew, onDay(-4, 10, 0, 0))),
			ranIn("~/Code/infra", requested("r2", within, Opus, work, status.ReasonNew, onDay(-2, 10, 0, 0))),
			ranIn("~/Code/infra", requested("r3", near, Opus, side, status.ReasonNew, At(9, 0, 0))),
			ranIn("~/Code/infra", requested("r4", infraA, Opus, side, status.ReasonNew, At(14, 40, 0))),
		},
		Sessions: []status.Session{session(infraA, seat(Opus, side, At(14, 40, 10), ""))},
		Typed:    "7f3A",
		Now:      now,
		Want: []Found{
			{Session: infraA, Dir: "~/Code/infra", Date: "2026-10-01"},
			{Session: infraB, Dir: "~/Code/infra", Date: "2026-09-26", Ended: true, Last: onDay(-4, 10, 0, 0)},
		},
	}
}

func oneOnThreeDays() FindMoment {
	return FindMoment{
		Name: "one session on three days, found once, on its newest",
		Lines: []ledger.Line{
			ranIn("~/Code/switchboard-site", requested("r1", site, Opus, work, status.ReasonNew, onDay(-3, 10, 0, 0))),
			requested("r2", D28C, Opus, work, status.ReasonNew, onDay(-2, 10, 0, 0)),
			ranIn("~/Code/switchboard-site", requested("r3", site, Opus, work, status.ReasonSticky, onDay(-1, 10, 0, 0))),
			requested("r4", D28C, Opus, work, status.ReasonSticky, onDay(0, 10, 0, 0)),
			requested("r5", D28C, Opus, side, status.ReasonSticky, At(14, 41, 0)),
		},
		Sessions: []status.Session{session(D28C, seat(Opus, side, At(14, 41, 10), ""))},
		Typed:    "Switchboard",
		Now:      now,
		Want: []Found{
			{Session: D28C, Dir: dirs[D28C], Date: "2026-10-01"},
			{Session: site, Dir: "~/Code/switchboard-site", Date: "2026-09-29", Ended: true, Last: onDay(-1, 10, 0, 0)},
		},
	}
}

func endedOfAnotherDay() FindMoment {
	return FindMoment{
		Name: "an ended session of another day, and one of today's",
		Lines: []ledger.Line{
			ranIn("~/Code/ledger", requested("r1", oldLedger, Opus, work, status.ReasonNew, onDay(-2, 15, 0, 0))),
			ranIn("~/Code/ledger", requested("r2", oldLedger, Opus, work, status.ReasonSticky, onDay(-2, 16, 20, 0))),
			ranIn("~/Code/ledger-tools", requested("r3", tools, Opus, side, status.ReasonNew, At(10, 50, 0))),
			ranIn("~/Code/ledger-tools", requested("r4", tools, Opus, side, status.ReasonSticky, At(11, 21, 0))),
			requested("r5", C61B, Opus, side, status.ReasonNew, At(14, 35, 0)),
		},
		Sessions: []status.Session{session(C61B, seat(Opus, side, At(14, 35, 10), ""))},
		Typed:    "ledger",
		Now:      now,
		Want: []Found{
			{Session: tools, Dir: "~/Code/ledger-tools", Date: "2026-10-01", Ended: true, Last: At(11, 21, 0)},
			{Session: oldLedger, Dir: "~/Code/ledger", Date: "2026-09-28", Ended: true, Last: onDay(-2, 16, 20, 0)},
		},
	}
}

// ranIn returns l, a request of a session run in the directory given.
func ranIn(dir string, l ledger.Line) ledger.Line {
	l.Dir = dir
	return l
}
