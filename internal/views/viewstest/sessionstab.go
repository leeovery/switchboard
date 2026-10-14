package viewstest

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// The sessions of Sessions' moments beside D28C and C61B, as its frames name
// them.
const (
	ID41E0 = "41e0b6c2-8d3f-4a71-95e4-2c7b0f9a1d68"
	ID9C2E = "9c2e47b1-0d5a-4e83-b6f2-8a1c3e9d0f57"
	ID7F3A = "7f3a0c94-5d2e-4b18-a6f7-31c8e9d24b05"
	IDDB8A = "db8a71e3-9f04-4c6d-b2a8-5e1f7c39d860"
	IDF41A = "f41a6c08-2b7d-4e15-93a0-c4d8e2f7b916"
)

// The sessions of the 14:42:07 scene that ended, as the List's frame names
// them.
const (
	id5B19 = "5b19d3e7-6a2c-4f08-b1e4-9d7c3a5f2e60"
	id3D7A = "3d7a8e12-4c6b-4a95-8f3d-1b2e7c9a4d05"
	idE0C4 = "e0c4f5a9-1d3b-4e72-a8c6-5f2d9b0e7a14"
	idA8F1 = "a8f1b2c6-7e4d-4093-9a5f-3c8e1d6b2f70"
	id2B6D = "2b6d9f04-8a1e-4c37-b5d2-7e3a0c9f4b81"
	id5E7D = "5e7d0a3b-2f9c-4d61-8e7a-4b1c6d3f9e25"
	id4B0E = "4b0e6c1f-9d8a-4f23-a7b5-2e6c0d4a8f19"
	id8A2C = "8a2c5e7b-3f1d-4a86-9c0e-6d2b8f5a1c47"
)

// The directories the scene's sessions run in.
const (
	docsDir        = "~/Code/docs"
	websiteDir     = "~/Code/website"
	mobileDir      = "~/Code/mobile"
	switchboardDir = "~/Code/switchboard"
	apiDir         = "~/Code/api"
	infraDir       = "~/Code/infra"
)

// SessionsMoment is a scene Sessions has to get right, look by look, each
// after the last: the Switchboard keeping who sits where from one to the
// next, as views.Patch keeps it, and the List held as the ledger's lines are
// appended, as views.HeldList holds it.
type SessionsMoment struct {
	Name  string
	Looks []SessionsLook
}

// SessionsLook is a look at a scene, what it reads, and what Sessions holds
// at it.
type SessionsLook struct {
	// Doc is the router's document of the accounts, in the config's order,
	// or probing's, and Now when the look reads it all.
	Doc status.Document
	Now time.Time
	// Sessions are those the router lists, as GET /sessions gives them, or,
	// while it doesn't answer, as it last listed them: none where Unlisted
	// says no router lists them, as while the dashboard probes.
	Sessions []status.Session
	Unlisted bool
	// Stream is what the request stream told, as the watch heard it.
	Stream views.Stream
	// Lines are the ledger's lines by Now, oldest first: today's, and the
	// earlier days' of the sessions the List lists. Each look's are the last
	// look's and those appended since.
	Lines []ledger.Line
	// Board is each account's line, by its id, as the patch keeps it at the
	// look: its calls, top to bottom, each its session's shown id and its
	// jack, then the jack each stub hangs from, the first hung first, as
	// "9c2e@1 ╌0"; none for an account with neither.
	Board map[string]string
	// Calls are what each call says, by its session's shown id and its
	// account, as "41e0 personal": what it's doing, as Activity.Said says it,
	// and where it came from, as "↑ asking ▸ from personal"; its directory,
	// or "no dir"; and whether it works, its id bold and its cord lit, or
	// rests, " · " between, as "↑ asking ▸ from personal · ~/Code/docs ·
	// works".
	Calls map[string]string
	// List is the List's sessions in its order, each its shown id, its
	// state, as Activity.State says it, when it started, as prose.Past says
	// it, where it's known, and its requests today, " · " between, as "db8a
	// idle 50s · started yesterday 15:08 · 2 requests"; and Today the line
	// that sums them, as "14 sessions · 25 requests". Neither is wanted of a
	// look that leaves them out.
	List  []string
	Today string
}

// SessionsMoments are the moments Sessions has to get right: no sessions;
// probing, the List from the ledger; the 14:42:07 scene, six running and
// eight ended, d28c's two models on one row; a session on two accounts, then
// its other model moving; a refusal at personal's limit at 16:05:41, its
// move at 16:05:42 told by the stream before the router's document counts
// it, and the second session the limit moves at its next request; moves by
// pressure, a pin and by hand; throttled then answered, and a 401 then its
// move; an answer's end; midnight; the router not answering for ten minutes,
// then restarted; a router that gives no directory; 25 bent cords; fifty
// sessions; a new session joining, one ending, and asking again; a group
// starting below its line's first jack; one account with eight sessions; an
// Opus session's Haiku title after its main, before its next main request is
// heard and after; and an Opus main thread moved to work, waiting there over
// the IdleHour on its Haiku subagent there.
func SessionsMoments() []SessionsMoment {
	return []SessionsMoment{
		noSessions(), probingTheLedger(), theSceneAt1442(), onTwoAccounts(), refusedThenMovedByTheLimit(),
		movedWithNoLimit(), throttledAndRefused(), anAnswersEnd(), acrossTheNight(), theRouterSilent(),
		noDirFromTheRouter(), bentCords(), fiftySessions(), joiningAndEnding(), aGroupBelowItsLine(), oneAccountEight(),
		titledAfterItsMainOnTheBoard(), coldMainOnTheBoard(),
	}
}

// yesterdayAt is the local time on Wednesday 30 September 2026, the day
// before the moments', at the clock given.
func yesterdayAt(hour, minute, second int) time.Time {
	return At(hour, minute, second).AddDate(0, 0, -1)
}

// boardDoc is the router's document at the time given of the accounts with
// the given ids, in the config's order, personal the primary, new sessions
// going to side where it's among them, else to the first.
func boardDoc(at time.Time, accounts ...string) status.Document {
	doc := status.Document{Source: status.SourceRouter, GeneratedAt: at, Best: accounts[0]}
	for _, id := range accounts {
		doc.Accounts = append(doc.Accounts, status.Account{ID: id, Label: id, Primary: id == personal})
		if id == side {
			doc.Best = side
		}
	}
	return doc
}

// threeAt is the router's document of work, personal and side at the time
// given.
func threeAt(at time.Time) status.Document {
	return boardDoc(at, work, personal, side)
}

// heldBack is doc with the account given at its limit, reached at reached,
// lifting at until, its event counting the sessions it has moved.
func heldBack(doc status.Document, account string, reached, until time.Time, moved int) status.Document {
	i := slices.IndexFunc(doc.Accounts, func(a status.Account) bool { return a.ID == account })
	doc.Accounts = slices.Clone(doc.Accounts)
	doc.Accounts[i].Limit = status.Limit{ID: 1, Until: until}
	doc.Events = append(slices.Clone(doc.Events), status.Event{
		ID: 7, At: reached, Kind: status.EventLimit, Account: account, To: side, Until: until, Count: moved, Limit: 1,
	})
	return doc
}

// atItsLimit is the reason for a move off the account given as it hit its
// limit.
func atItsLimit(account string) string {
	return status.ReasonMovedOff + account + " " + status.WhyLimit
}

// listedOn is a session's model on an account, as the router lists it, in
// the directory given, put there at since for the reason given, last seen at
// seen, and doing what its request in flight does.
func listedOn(model, account, dir, reason string, since, seen time.Time, inFlight string) status.Assignment {
	family := "opus"
	if model == Haiku {
		family = "haiku"
	}
	return status.Assignment{
		Model: model, Family: family, Account: account, Dir: dir, Reason: reason,
		AssignedAt: since, LastSeen: seen, InFlight: inFlight,
	}
}

// called is the line of the request the router knows by the id given, of
// the session's own conversation, of the model given on the account given,
// in the directory given, arrived at at, its answer ending took later.
func called(id, session, model, account, dir string, at time.Time, took time.Duration) ledger.Line {
	l := withID(id, asMain(p1, "end_turn", line(session, model, account, at, took)))
	l.Dir = dir
	return l
}

// cameFrom returns l, a request that moved its session from the account
// given, for the reason given.
func cameFrom(from, reason string, l ledger.Line) ledger.Line {
	l.From, l.Reason = from, reason
	return l
}

// toldAsking is a request of the session's own conversation, of its model on
// an account, as the stream told of it: sent at sent, last told of at heard.
func toldAsking(request, session, model, account string, sent, heard time.Time) views.Told {
	return toldServing(p1, ledger.ClassMain, told(request, session, model, account, sent, heard))
}

// streaming returns t, its answer's first byte come at first, chars of it
// streamed so far.
func streaming(first time.Time, chars int, t views.Told) views.Told {
	t.First, t.Chars = first, chars
	return t
}

// verdict returns t, answered on its account as given, as the stream told.
func verdict(v views.Verdict, status int, t views.Told) views.Told {
	t.Verdict, t.Status = v, status
	return t
}

// movedTo is the move of the session's model from one account to another,
// for the reason given, at at, by the request given, as the stream told of
// it.
func movedTo(request, session, model, from, to, reason string, at time.Time) views.Moved {
	return views.Moved{Request: request, Session: session, Model: model, From: from, To: to, Reason: reason, At: at}
}

// sceneListed are the sessions the router lists at 14:42:07: on personal,
// 41e0, idle since 14:41:33, and 9c2e, since 14:33:07; on side, c61b
// answering, d28c asking, its haiku there too, 7f3a answering, and db8a,
// idle since 14:41:17, the last three moved from work as it reached its cap
// at 14:31.
func sceneListed() []status.Session {
	return []status.Session{
		session(ID41E0, listedOn(Opus, personal, docsDir, status.ReasonNew, At(12, 47, 12), At(14, 41, 33), "")),
		session(ID9C2E, listedOn(Opus, personal, websiteDir, status.ReasonNew, At(11, 55, 31), At(14, 33, 7), "")),
		session(C61B, listedOn(Opus, side, mobileDir, status.ReasonNew, At(14, 41, 11), At(14, 41, 50), status.Answering)),
		session(D28C,
			listedOn(Opus, side, switchboardDir, capped(work), At(14, 31, 13), At(14, 42, 5), status.Asking),
			listedOn(Haiku, side, switchboardDir, status.ReasonNew, At(14, 31, 44), At(14, 31, 44), ""),
		),
		session(ID7F3A, listedOn(Opus, side, apiDir, capped(work), At(14, 31, 20), At(14, 41, 58), status.Answering)),
		session(IDDB8A, listedOn(Opus, side, infraDir, capped(work), At(14, 31, 40), At(14, 41, 17), "")),
	}
}

// sceneStream is what the stream has told at 14:42:07: c61b's answer
// streaming since 14:41:52, d28c asking since 14:42:05, and 7f3a's answer
// streaming since 14:42:00.
func sceneStream() views.Stream {
	return live(
		streaming(At(14, 41, 52), 2000, toldAsking("c1", C61B, Opus, side, At(14, 41, 50), At(14, 42, 7))),
		toldAsking("d9", D28C, Opus, side, At(14, 42, 5), At(14, 42, 5)),
		streaming(At(14, 42, 0), 800, toldAsking("f4", ID7F3A, Opus, side, At(14, 41, 58), At(14, 42, 4))),
	)
}

// sceneLines are the ledger's lines by 14:42:07: the six running, db8a
// started yesterday, d28c, 7f3a and db8a moved from work at its cap at
// 14:31, d28c's title on side; and the eight ended today, 5e7d, 4b0e and
// 8a2c started yesterday, 8a2c moved from personal at its limit yesterday.
func sceneLines() []ledger.Line {
	const took = 10 * time.Second
	return []ledger.Line{
		called("y1", IDDB8A, Opus, work, infraDir, yesterdayAt(15, 8, 0), took),
		called("y2", id4B0E, Opus, work, infraDir, yesterdayAt(15, 52, 0), took),
		called("y3", id8A2C, Opus, personal, docsDir, yesterdayAt(16, 55, 0), took),
		cameFrom(personal, atItsLimit(personal), called("y4", id8A2C, Opus, side, docsDir, yesterdayAt(23, 30, 0), took)),
		called("y5", id5E7D, Opus, side, apiDir, yesterdayAt(23, 47, 0), took),
		called("t01", id8A2C, Opus, side, docsDir, At(0, 18, 0), took),
		called("t02", id4B0E, Opus, work, infraDir, At(0, 31, 0), took),
		called("t03", id5E7D, Opus, side, apiDir, At(0, 41, 0), took),
		called("t04", id2B6D, Opus, work, apiDir, At(7, 58, 0), took),
		called("t05", idA8F1, Opus, side, websiteDir, At(8, 31, 0), took),
		called("t06", id2B6D, Opus, work, apiDir, At(9, 5, 0), took),
		called("t07", idE0C4, Opus, work, infraDir, At(9, 12, 0), took),
		called("t08", idA8F1, Opus, side, websiteDir, At(9, 28, 0), took),
		called("t09", id3D7A, Opus, work, docsDir, At(9, 40, 0), took),
		called("t10", idE0C4, Opus, work, infraDir, At(10, 2, 0), took),
		called("t11", id5B19, Opus, personal, mobileDir, At(10, 2, 0), took),
		called("t12", id3D7A, Opus, work, docsDir, At(10, 15, 0), took),
		called("t13", id5B19, Opus, personal, mobileDir, At(11, 21, 0), took),
		called("t14", ID9C2E, Opus, personal, websiteDir, At(11, 55, 31), took),
		called("t15", D28C, Opus, work, switchboardDir, At(12, 2, 0), took),
		called("t16", ID41E0, Opus, personal, docsDir, At(12, 47, 12), took),
		called("t17", ID7F3A, Opus, work, apiDir, At(14, 12, 0), took),
		cameFrom(work, capped(work), called("t18", D28C, Opus, side, switchboardDir, At(14, 31, 13), took)),
		cameFrom(work, capped(work), called("t19", ID7F3A, Opus, side, apiDir, At(14, 31, 20), took)),
		cameFrom(work, capped(work), called("t20", IDDB8A, Opus, side, infraDir, At(14, 31, 40), took)),
		asTitle(p1, withID("t21", line(D28C, Haiku, side, At(14, 31, 44), 2*time.Second))),
		called("t22", ID9C2E, Opus, personal, websiteDir, At(14, 32, 57), took),
		called("t23", IDDB8A, Opus, side, infraDir, At(14, 41, 7), took),
		called("t24", C61B, Opus, side, mobileDir, At(14, 41, 11), took),
		called("t25", ID41E0, Opus, personal, docsDir, At(14, 41, 23), took),
	}
}

// sceneEnded are the List's sessions of the 14:42:07 scene that ended, as
// List says them then.
var sceneEnded = []string{
	"5b19 ended 11:21 · started 10:02 · 2 requests",
	"3d7a ended 10:15 · started 09:40 · 2 requests",
	"e0c4 ended 10:02 · started 09:12 · 2 requests",
	"a8f1 ended 09:28 · started 08:31 · 2 requests",
	"2b6d ended 09:05 · started 07:58 · 2 requests",
	"5e7d ended 00:41 · started yesterday 23:47 · 1 requests",
	"4b0e ended 00:31 · started yesterday 15:52 · 1 requests",
	"8a2c ended 00:18 · started yesterday 16:55 · 1 requests",
}

func noSessions() SessionsMoment {
	now := At(14, 42, 7)
	return SessionsMoment{Name: "no sessions", Looks: []SessionsLook{{
		Doc: threeAt(now), Now: now, Stream: live(), List: []string{}, Today: "0 sessions · 0 requests",
	}}}
}

// probingTheLedger is the dashboard probing at 15:00, no router listing the
// sessions: the Switchboard with no calls, and the List of today's from the
// ledger, 5b19 idle by its last line within the hour, 3d7a ended.
func probingTheLedger() SessionsMoment {
	now := At(15, 0, 0)
	doc := threeAt(now)
	doc.Source = status.SourceProbe
	return SessionsMoment{Name: "probing, the List from the ledger by the router's own hour", Looks: []SessionsLook{{
		Doc: doc, Now: now, Unlisted: true,
		Lines: []ledger.Line{
			called("p1", id3D7A, Opus, work, docsDir, At(12, 0, 0), 10*time.Second),
			called("p2", id5B19, Opus, personal, mobileDir, At(13, 0, 0), 10*time.Second),
			called("p3", id3D7A, Opus, work, docsDir, At(13, 40, 0), 10*time.Second),
			called("p4", id5B19, Opus, personal, mobileDir, At(14, 29, 50), 10*time.Second),
		},
		List:  []string{"5b19 idle 30m · started 13:00 · 2 requests", "3d7a ended 13:40 · started 12:00 · 2 requests"},
		Today: "2 sessions · 4 requests",
	}}}
}

// theSceneAt1442 is the 14:42:07 scene, the frames' first look: six running,
// eight ended, d28c's opus and haiku on side, one row.
func theSceneAt1442() SessionsMoment {
	now := At(14, 42, 7)
	return SessionsMoment{Name: "the 14:42:07 scene: six running, eight ended, d28c's two models on side one row", Looks: []SessionsLook{{
		Doc: threeAt(now), Now: now, Sessions: sceneListed(), Stream: sceneStream(), Lines: sceneLines(),
		Board: map[string]string{personal: "41e0@0 9c2e@1", side: "c61b@0 d28c@1 7f3a@2 db8a@3"},
		Calls: map[string]string{
			"41e0 personal": "idle 34s · ~/Code/docs · rests",
			"9c2e personal": "idle 9m · ~/Code/website · rests",
			"c61b side":     "↓ answering · ~/Code/mobile · works",
			"d28c side":     "↑ asking ▸ from work · ~/Code/switchboard · works",
			"7f3a side":     "↓ answering ▸ from work · ~/Code/api · works",
			"db8a side":     "idle 50s ▸ from work · ~/Code/infra · rests",
		},
		List: append([]string{
			"db8a idle 50s · started yesterday 15:08 · 2 requests",
			"9c2e idle 9m · started 11:55 · 2 requests",
			"d28c asking · started 12:02 · 3 requests",
			"41e0 idle 34s · started 12:47 · 2 requests",
			"7f3a answering · started 14:12 · 2 requests",
			"c61b answering · started 14:41 · 1 requests",
		}, sceneEnded...),
		Today: "14 sessions · 25 requests",
	}}}
}

// onTwoAccounts is d28c with its opus answering on work and its haiku on
// side, two calls and one List row; then its haiku moving to work by
// pressure, its row on side gone, side closing up, and no stub.
func onTwoAccounts() SessionsMoment {
	first, then := At(14, 42, 7), At(14, 42, 20)
	opusAnswering := streaming(At(14, 42, 1), 1200, toldAsking("r1", D28C, Opus, work, At(14, 42, 0), At(14, 42, 6)))
	title := done(At(14, 40, 2), 200, toldServing(p1, "auxiliary", told("r2", D28C, Haiku, side, At(14, 40, 0), At(14, 40, 2))))
	lines := []ledger.Line{
		called("l1", D28C, Opus, work, switchboardDir, At(14, 0, 0), 10*time.Second),
		called("l2", C61B, Opus, side, mobileDir, At(14, 30, 0), 10*time.Second),
		asTitle(p1, withID("r2", line(D28C, Haiku, side, At(14, 40, 0), 2*time.Second))),
	}
	listing := func(haikuOn, reason string, since time.Time) []status.Session {
		return []status.Session{
			session(D28C,
				listedOn(Opus, work, switchboardDir, status.ReasonNew, At(14, 0, 0), At(14, 42, 0), status.Answering),
				listedOn(Haiku, haikuOn, switchboardDir, reason, since, At(14, 40, 0), ""),
			),
			session(C61B, listedOn(Opus, side, mobileDir, status.ReasonNew, At(14, 30, 0), At(14, 41, 0), "")),
		}
	}
	moving := toldServing(p1, "auxiliary", told("r3", D28C, Haiku, work, At(14, 42, 14), At(14, 42, 15)))
	moved := live(opusAnswering, title, moving)
	moved.Moves = []views.Moved{movedTo("r3", D28C, Haiku, side, work, capped(side), At(14, 42, 15))}
	return SessionsMoment{Name: "a session on two accounts, two calls and one List row, then its haiku moving", Looks: []SessionsLook{
		{
			Doc: threeAt(first), Now: first, Sessions: listing(side, status.ReasonNew, At(14, 0, 0)), Stream: live(opusAnswering, title), Lines: lines,
			Board: map[string]string{work: "d28c@0", side: "c61b@0 d28c@1"},
			Calls: map[string]string{
				"d28c work": "↓ answering · ~/Code/switchboard · works",
				"d28c side": "idle 2m · ~/Code/switchboard · rests",
				"c61b side": "idle 1m · ~/Code/mobile · rests",
			},
			List:  []string{"d28c answering · started 14:00 · 2 requests", "c61b idle 1m · started 14:30 · 1 requests"},
			Today: "2 sessions · 3 requests",
		},
		{
			Doc: threeAt(then), Now: then, Sessions: listing(work, capped(side), At(14, 42, 15)), Stream: moved, Lines: lines,
			Board: map[string]string{work: "d28c@0", side: "c61b@0"},
			Calls: map[string]string{
				"d28c work": "↓ answering · ~/Code/switchboard · works",
				"c61b side": "idle 1m · ~/Code/mobile · rests",
			},
			List: []string{"d28c answering · started 14:00 · 2 requests", "c61b idle 1m · started 14:30 · 1 requests"},
		},
	}}
}

// at1605 are the sessions the router lists at 16:05: on personal, 41e0, seen
// at seen41E0 doing what doing41E0 says, and 9c2e, idle since 15:52:30; on
// side, c61b answering since 16:04:55, and d28c, 7f3a and db8a, moved from
// work at its cap, idle since the times given.
func at1605(seen41E0 time.Time, doing41E0 string, c61b, d28c, f3a, db8a status.Assignment) []status.Session {
	return []status.Session{
		session(ID41E0, listedOn(Opus, personal, docsDir, status.ReasonNew, At(12, 47, 12), seen41E0, doing41E0)),
		session(ID9C2E, listedOn(Opus, personal, websiteDir, status.ReasonNew, At(11, 55, 31), At(15, 52, 30), "")),
		session(C61B, c61b),
		session(D28C, d28c),
		session(ID7F3A, f3a),
		session(IDDB8A, db8a),
	}
}

// refusedThenMovedByTheLimit is personal reaching its limit as 41e0 asks at
// 16:05:41, refused, ✕ 429, its id bold; moved to side at 16:05:42, the
// stream telling of it before the router's document counts it, then counted,
// its stub hanging from the jack it left, 9c2e kept on its own; and 9c2e
// moved at its next request, at 16:07:09, the limit counting two.
func refusedThenMovedByTheLimit() SessionsMoment {
	c61bOn := listedOn(Opus, side, mobileDir, status.ReasonNew, At(14, 41, 11), At(16, 4, 55), status.Answering)
	d28cOn := listedOn(Opus, side, switchboardDir, capped(work), At(14, 31, 13), At(16, 4, 58), "")
	f3aOn := listedOn(Opus, side, apiDir, capped(work), At(14, 31, 20), At(16, 4, 30), "")
	db8aOn := listedOn(Opus, side, infraDir, capped(work), At(14, 31, 40), At(16, 3, 0), "")
	firstAnswer := streaming(At(16, 4, 57), 600, toldAsking("c2", C61B, Opus, side, At(16, 4, 55), At(16, 5, 0)))
	answered := done(At(16, 5, 20), 200, firstAnswer)
	c61bAgain := listedOn(Opus, side, mobileDir, status.ReasonNew, At(14, 41, 11), At(16, 5, 30), status.Answering)
	secondAnswer := streaming(At(16, 5, 31), 900, toldAsking("c3", C61B, Opus, side, At(16, 5, 30), At(16, 5, 41)))
	d28cSeen := listedOn(Opus, side, switchboardDir, capped(work), At(14, 31, 13), At(16, 5, 36), "")
	db8aSeen := listedOn(Opus, side, infraDir, capped(work), At(14, 31, 40), At(16, 4, 55), "")
	refusedAt := verdict(views.VerdictLimited, 429, toldAsking("r41", ID41E0, Opus, personal, At(16, 5, 40), At(16, 5, 41)))
	goneOut := toldAsking("r41", ID41E0, Opus, side, At(16, 5, 40), At(16, 5, 42))
	move41E0 := movedTo("r41", ID41E0, Opus, personal, side, atItsLimit(personal), At(16, 5, 42))

	moving := live(answered, streaming(At(16, 5, 31), 900, toldAsking("c3", C61B, Opus, side, At(16, 5, 30), At(16, 5, 42))), goneOut)
	moving.Moves = []views.Moved{move41E0}
	movedOn := listedOn(Opus, side, docsDir, atItsLimit(personal), At(16, 5, 42), At(16, 5, 42), status.Asking)
	listedMoved := at1605(At(16, 5, 40), status.Asking, c61bAgain, d28cSeen, f3aOn, db8aSeen)
	listedMoved[0] = session(ID41E0, movedOn)

	later := At(16, 7, 10)
	secondDone := done(At(16, 6, 50), 200, streaming(At(16, 5, 31), 900, toldAsking("c3", C61B, Opus, side, At(16, 5, 30), At(16, 6, 50))))
	nineGoneOut := toldAsking("r9", ID9C2E, Opus, side, At(16, 7, 8), At(16, 7, 9))
	both := live(answered, secondDone, done(At(16, 6, 20), 200, goneOut), nineGoneOut)
	both.Moves = []views.Moved{move41E0, movedTo("r9", ID9C2E, Opus, personal, side, atItsLimit(personal), At(16, 7, 9))}
	bothListed := []status.Session{
		session(ID41E0, listedOn(Opus, side, docsDir, atItsLimit(personal), At(16, 5, 42), At(16, 6, 20), "")),
		session(ID9C2E, listedOn(Opus, side, websiteDir, atItsLimit(personal), At(16, 7, 9), At(16, 7, 8), status.Asking)),
		session(C61B, listedOn(Opus, side, mobileDir, status.ReasonNew, At(14, 41, 11), At(16, 6, 50), "")),
		session(D28C, d28cSeen), session(ID7F3A, f3aOn), session(IDDB8A, db8aSeen),
	}
	linesThen := append(sceneLines(),
		called("c2", C61B, Opus, side, mobileDir, At(16, 4, 55), 25*time.Second),
		cameFrom(personal, atItsLimit(personal), called("r41", ID41E0, Opus, side, docsDir, At(16, 5, 40), 40*time.Second)),
		called("c3", C61B, Opus, side, mobileDir, At(16, 5, 30), 80*time.Second),
	)
	sixSide := "c61b@0 d28c@1 7f3a@2 db8a@3"
	return SessionsMoment{Name: "a refusal at personal's limit, then the move, told before it's counted, and the second session at its next request", Looks: []SessionsLook{
		{
			Doc: threeAt(At(16, 5, 0)), Now: At(16, 5, 0), Lines: sceneLines(),
			Sessions: at1605(At(16, 4, 50), "", c61bOn, d28cOn, f3aOn, db8aOn), Stream: live(firstAnswer),
			Board: map[string]string{personal: "41e0@0 9c2e@1", side: sixSide},
			Calls: map[string]string{
				"41e0 personal": "idle 10s · ~/Code/docs · rests",
				"9c2e personal": "idle 12m · ~/Code/website · rests",
				"c61b side":     "↓ answering · ~/Code/mobile · works",
				"d28c side":     "idle 2s ▸ from work · ~/Code/switchboard · rests",
				"7f3a side":     "idle 30s ▸ from work · ~/Code/api · rests",
				"db8a side":     "idle 2m ▸ from work · ~/Code/infra · rests",
			},
		},
		{
			Doc: heldBack(threeAt(At(16, 5, 41)), personal, At(16, 5, 41), At(16, 30, 0), 0), Now: At(16, 5, 41), Lines: sceneLines(),
			Sessions: at1605(At(16, 5, 40), status.Asking, c61bAgain, d28cSeen, f3aOn, db8aSeen), Stream: live(answered, secondAnswer, refusedAt),
			Board: map[string]string{personal: "41e0@0 9c2e@1", side: sixSide},
			Calls: map[string]string{
				"41e0 personal": "✕ 429 · ~/Code/docs · works",
				"9c2e personal": "idle 13m · ~/Code/website · rests",
				"c61b side":     "↓ answering · ~/Code/mobile · works",
				"d28c side":     "idle 5s ▸ from work · ~/Code/switchboard · rests",
				"7f3a side":     "idle 1m ▸ from work · ~/Code/api · rests",
				"db8a side":     "idle 46s ▸ from work · ~/Code/infra · rests",
			},
		},
		{
			Doc: heldBack(threeAt(At(16, 5, 41)), personal, At(16, 5, 41), At(16, 30, 0), 0), Now: At(16, 5, 42), Lines: sceneLines(),
			Sessions: at1605(At(16, 5, 40), status.Asking, c61bAgain, d28cSeen, f3aOn, db8aSeen), Stream: moving,
			Board: map[string]string{personal: "9c2e@1", side: sixSide + " 41e0@4"},
			Calls: map[string]string{
				"41e0 side":     "↑ asking ▸ from personal · ~/Code/docs · works",
				"9c2e personal": "idle 13m · ~/Code/website · rests",
				"c61b side":     "↓ answering · ~/Code/mobile · works",
				"d28c side":     "idle 6s ▸ from work · ~/Code/switchboard · rests",
				"7f3a side":     "idle 1m ▸ from work · ~/Code/api · rests",
				"db8a side":     "idle 47s ▸ from work · ~/Code/infra · rests",
			},
		},
		{
			Doc: heldBack(threeAt(At(16, 5, 43)), personal, At(16, 5, 41), At(16, 30, 0), 1), Now: At(16, 5, 43), Lines: sceneLines(),
			Sessions: listedMoved, Stream: moving,
			Board: map[string]string{personal: "9c2e@1 ╌0", side: sixSide + " 41e0@4"},
			Calls: map[string]string{
				"41e0 side":     "↑ asking ▸ from personal · ~/Code/docs · works",
				"9c2e personal": "idle 13m · ~/Code/website · rests",
				"c61b side":     "↓ answering · ~/Code/mobile · works",
				"d28c side":     "idle 7s ▸ from work · ~/Code/switchboard · rests",
				"7f3a side":     "idle 1m ▸ from work · ~/Code/api · rests",
				"db8a side":     "idle 48s ▸ from work · ~/Code/infra · rests",
			},
		},
		{
			Doc: heldBack(threeAt(later), personal, At(16, 5, 41), At(16, 30, 0), 2), Now: later, Lines: linesThen,
			Sessions: bothListed, Stream: both,
			Board: map[string]string{personal: "╌0 ╌1", side: sixSide + " 41e0@4 9c2e@5"},
			Calls: map[string]string{
				"41e0 side": "idle 50s ▸ from personal · ~/Code/docs · rests",
				"9c2e side": "↑ asking ▸ from personal · ~/Code/website · works",
				"c61b side": "idle 20s · ~/Code/mobile · rests",
				"d28c side": "idle 1m ▸ from work · ~/Code/switchboard · rests",
				"7f3a side": "idle 2m ▸ from work · ~/Code/api · rests",
				"db8a side": "idle 2m ▸ from work · ~/Code/infra · rests",
			},
			List: append([]string{
				"db8a idle 2m · started yesterday 15:08 · 2 requests",
				"9c2e asking · started 11:55 · 2 requests",
				"d28c idle 1m · started 12:02 · 3 requests",
				"41e0 idle 50s · started 12:47 · 3 requests",
				"7f3a idle 2m · started 14:12 · 2 requests",
				"c61b idle 20s · started 14:41 · 3 requests",
			}, sceneEnded...),
			Today: "14 sessions · 28 requests",
		},
	}}
}

// movedWithNoLimit is d28c moved from work by pressure, 7f3a by the global
// pin, and c61b by hand, with no request since: each joining its new group's
// foot, saying where it came from, and hanging no stub.
func movedWithNoLimit() SessionsMoment {
	seen := func(id, account string, at time.Time) status.Session {
		return session(id, listedOn(Opus, account, apiDir, status.ReasonNew, At(14, 0, 0), at, ""))
	}
	listing := []status.Session{
		seen(D28C, work, At(14, 59, 50)), seen(ID7F3A, work, At(14, 59, 40)), seen(C61B, work, At(14, 59, 30)), seen(IDDB8A, side, At(14, 59, 0)),
	}
	lines := []ledger.Line{
		called("m1", IDDB8A, Opus, side, apiDir, At(14, 0, 0), 10*time.Second),
		called("m2", D28C, Opus, work, apiDir, At(14, 10, 0), 10*time.Second),
		called("m3", ID7F3A, Opus, work, apiDir, At(14, 20, 0), 10*time.Second),
		called("m4", C61B, Opus, work, apiDir, At(14, 30, 0), 10*time.Second),
	}
	pressure := movedTo("p1", D28C, Opus, work, side, capped(work), At(15, 0, 9))
	pinned := movedTo("p2", ID7F3A, Opus, work, side, status.ReasonMovedByPin, At(15, 0, 19))
	byHand := movedTo("", C61B, Opus, work, personal, status.ReasonPinned, At(15, 0, 29))
	d28cAsks := toldAsking("p1", D28C, Opus, side, At(15, 0, 8), At(15, 0, 9))
	f3aAsks := toldAsking("p2", ID7F3A, Opus, side, At(15, 0, 18), At(15, 0, 19))
	stream := func(told []views.Told, moves ...views.Moved) views.Stream {
		s := live(told...)
		s.Moves = moves
		return s
	}
	return SessionsMoment{Name: "moves by pressure, by the global pin and by hand, with no stub", Looks: []SessionsLook{
		{
			Doc: threeAt(At(15, 0, 0)), Now: At(15, 0, 0), Sessions: listing, Stream: live(), Lines: lines,
			Board: map[string]string{work: "d28c@0 7f3a@1 c61b@2", side: "db8a@0"},
			Calls: map[string]string{
				"d28c work": "idle 10s · ~/Code/api · rests", "7f3a work": "idle 20s · ~/Code/api · rests",
				"c61b work": "idle 30s · ~/Code/api · rests", "db8a side": "idle 1m · ~/Code/api · rests",
			},
		},
		{
			Doc: threeAt(At(15, 0, 10)), Now: At(15, 0, 10), Sessions: listing, Stream: stream([]views.Told{d28cAsks}, pressure), Lines: lines,
			Board: map[string]string{work: "7f3a@1 c61b@2", side: "db8a@0 d28c@1"},
			Calls: map[string]string{
				"d28c side": "↑ asking ▸ from work · ~/Code/api · works", "7f3a work": "idle 30s · ~/Code/api · rests",
				"c61b work": "idle 40s · ~/Code/api · rests", "db8a side": "idle 1m · ~/Code/api · rests",
			},
		},
		{
			Doc: threeAt(At(15, 0, 20)), Now: At(15, 0, 20), Sessions: listing, Stream: stream([]views.Told{d28cAsks, f3aAsks}, pressure, pinned), Lines: lines,
			Board: map[string]string{work: "c61b@2", side: "db8a@0 d28c@1 7f3a@2"},
			Calls: map[string]string{
				"d28c side": "↑ asking ▸ from work · ~/Code/api · works", "7f3a side": "↑ asking ▸ from work · ~/Code/api · works",
				"c61b work": "idle 50s · ~/Code/api · rests", "db8a side": "idle 1m · ~/Code/api · rests",
			},
		},
		{
			Doc: threeAt(At(15, 0, 30)), Now: At(15, 0, 30), Sessions: listing, Stream: stream([]views.Told{d28cAsks, f3aAsks}, pressure, pinned, byHand), Lines: lines,
			Board: map[string]string{personal: "c61b@0", side: "db8a@0 d28c@1 7f3a@2"},
			Calls: map[string]string{
				"d28c side": "↑ asking ▸ from work · ~/Code/api · works", "7f3a side": "↑ asking ▸ from work · ~/Code/api · works",
				"c61b personal": "idle 1m ▸ from work · ~/Code/api · rests", "db8a side": "idle 1m · ~/Code/api · rests",
			},
		},
	}}
}

// throttledAndRefused is d28c throttled on work, … 429, then answered; and
// c61b refused on side by a 401, ✕ 401, then moved to work: no stub, as no
// limit moved it.
func throttledAndRefused() SessionsMoment {
	listing := []status.Session{
		session(D28C, listedOn(Opus, work, switchboardDir, status.ReasonNew, At(14, 0, 0), At(14, 59, 58), status.Asking)),
		session(C61B, listedOn(Opus, side, mobileDir, status.ReasonNew, At(14, 0, 0), At(14, 59, 57), status.Asking)),
	}
	lines := []ledger.Line{
		called("w1", D28C, Opus, work, switchboardDir, At(14, 0, 0), 10*time.Second),
		called("w2", C61B, Opus, side, mobileDir, At(14, 10, 0), 10*time.Second),
	}
	throttledOnWork := verdict(views.VerdictThrottled, 429, toldAsking("t1", D28C, Opus, work, At(14, 59, 58), At(14, 59, 59)))
	answeredOnWork := streaming(At(15, 0, 3), 300, toldAsking("t1", D28C, Opus, work, At(14, 59, 58), At(15, 0, 4)))
	refusedOnSide := verdict(views.VerdictRefused, 401, toldAsking("t2", C61B, Opus, side, At(14, 59, 57), At(14, 59, 59)))
	movedOnWork := toldAsking("t2", C61B, Opus, work, At(14, 59, 57), At(15, 0, 4))
	moved := live(answeredOnWork, movedOnWork)
	moved.Moves = []views.Moved{movedTo("t2", C61B, Opus, side, work, status.ReasonMovedOff+side+" "+status.WhyRefused, At(15, 0, 4))}
	return SessionsMoment{Name: "throttled, then answered; refused by a 401, then moved, with no stub", Looks: []SessionsLook{
		{
			Doc: threeAt(At(15, 0, 0)), Now: At(15, 0, 0), Sessions: listing, Stream: live(throttledOnWork, refusedOnSide), Lines: lines,
			Board: map[string]string{work: "d28c@0", side: "c61b@0"},
			Calls: map[string]string{"d28c work": "… 429 · ~/Code/switchboard · works", "c61b side": "✕ 401 · ~/Code/mobile · works"},
		},
		{
			Doc: threeAt(At(15, 0, 5)), Now: At(15, 0, 5), Sessions: listing, Stream: moved, Lines: lines,
			Board: map[string]string{work: "d28c@0 c61b@1"},
			Calls: map[string]string{"d28c work": "↓ answering · ~/Code/switchboard · works", "c61b work": "↑ asking ▸ from side · ~/Code/mobile · works"},
		},
	}}
}

// anAnswersEnd is d28c's answer streaming, then ending at 15:00:04: idle 0s
// at once, its cord at rest.
func anAnswersEnd() SessionsMoment {
	sent := At(14, 59, 50)
	listing := []status.Session{session(D28C, listedOn(Opus, work, switchboardDir, status.ReasonNew, At(14, 0, 0), sent, status.Answering))}
	earlier := called("e1", D28C, Opus, work, switchboardDir, At(14, 0, 0), 10*time.Second)
	answering := streaming(At(14, 59, 52), 4000, toldAsking("e2", D28C, Opus, work, sent, At(15, 0, 0)))
	return SessionsMoment{Name: "an answer's end, idle 0s at once", Looks: []SessionsLook{
		{
			Doc: threeAt(At(15, 0, 0)), Now: At(15, 0, 0), Sessions: listing, Stream: live(answering), Lines: []ledger.Line{earlier},
			Board: map[string]string{work: "d28c@0"},
			Calls: map[string]string{"d28c work": "↓ answering · ~/Code/switchboard · works"},
			List:  []string{"d28c answering · started 14:00 · 1 requests"},
		},
		{
			Doc: threeAt(At(15, 0, 4)), Now: At(15, 0, 4), Sessions: listing, Stream: live(done(At(15, 0, 4), 200, answering)),
			Lines: []ledger.Line{earlier, called("e2", D28C, Opus, work, switchboardDir, sent, 14*time.Second)},
			Board: map[string]string{work: "d28c@0"},
			Calls: map[string]string{"d28c work": "idle 0s · ~/Code/switchboard · rests"},
			List:  []string{"d28c idle 0s · started 14:00 · 2 requests"},
		},
	}}
}

// acrossTheNight is midnight: 4b0e idle since 23:50 yesterday, moved there
// from personal at 23:30, its ▸ from going at midnight, its requests none
// today; 5e7d, started at 23:47 yesterday, asking again at 00:41, then ended
// an hour after; and 3d7a, ended yesterday, dropped at midnight.
func acrossTheNight() SessionsMoment {
	const took = 10 * time.Second
	night := []ledger.Line{
		called("n1", id4B0E, Opus, personal, infraDir, yesterdayAt(15, 52, 0), took),
		called("n2", id3D7A, Opus, work, docsDir, yesterdayAt(20, 0, 0), took),
		called("n3", id3D7A, Opus, work, docsDir, yesterdayAt(21, 0, 0), took),
		cameFrom(personal, atItsLimit(personal), called("n4", id4B0E, Opus, work, infraDir, yesterdayAt(23, 30, 0), took)),
		called("n5", id5E7D, Opus, side, apiDir, yesterdayAt(23, 47, 0), took),
		called("n6", id4B0E, Opus, work, infraDir, yesterdayAt(23, 49, 50), took),
		called("n7", id5E7D, Opus, side, apiDir, yesterdayAt(23, 56, 50), took),
	}
	listing := func(seen5E7D time.Time) []status.Session {
		return []status.Session{
			session(id4B0E, listedOn(Opus, work, infraDir, atItsLimit(personal), yesterdayAt(23, 30, 0), yesterdayAt(23, 50, 0), "")),
			session(id5E7D, listedOn(Opus, side, apiDir, status.ReasonNew, yesterdayAt(23, 47, 0), seen5E7D, "")),
		}
	}
	asksAgain := append(slices.Clone(night), called("n8", id5E7D, Opus, side, apiDir, At(0, 41, 0), took))
	return SessionsMoment{Name: "midnight: one idle since 23:50 yesterday, one ended at 00:41, one ended yesterday", Looks: []SessionsLook{
		{
			Doc: threeAt(yesterdayAt(23, 59, 30)), Now: yesterdayAt(23, 59, 30), Sessions: listing(yesterdayAt(23, 57, 0)), Stream: live(), Lines: night,
			Board: map[string]string{work: "4b0e@0", side: "5e7d@0"},
			Calls: map[string]string{
				"4b0e work": "idle 9m ▸ from personal · ~/Code/infra · rests",
				"5e7d side": "idle 2m · ~/Code/api · rests",
			},
			List: []string{
				"4b0e idle 9m · started 15:52 · 3 requests",
				"5e7d idle 2m · started 23:47 · 2 requests",
				"3d7a ended 21:00 · started 20:00 · 2 requests",
			},
			Today: "3 sessions · 7 requests",
		},
		{
			Doc: threeAt(At(0, 0, 30)), Now: At(0, 0, 30), Sessions: listing(yesterdayAt(23, 57, 0)), Stream: live(), Lines: night,
			Board: map[string]string{work: "4b0e@0", side: "5e7d@0"},
			Calls: map[string]string{
				"4b0e work": "idle 10m · ~/Code/infra · rests",
				"5e7d side": "idle 3m · ~/Code/api · rests",
			},
			List: []string{
				"4b0e idle 10m · started yesterday 15:52 · 0 requests",
				"5e7d idle 3m · started yesterday 23:47 · 0 requests",
			},
			Today: "2 sessions · 0 requests",
		},
		{
			Doc: threeAt(At(0, 41, 30)), Now: At(0, 41, 30), Sessions: listing(At(0, 41, 0)), Stream: live(), Lines: asksAgain,
			Board: map[string]string{work: "4b0e@0", side: "5e7d@0"},
			Calls: map[string]string{
				"4b0e work": "idle 51m · ~/Code/infra · rests",
				"5e7d side": "idle 30s · ~/Code/api · rests",
			},
			List: []string{
				"4b0e idle 51m · started yesterday 15:52 · 0 requests",
				"5e7d idle 20s · started yesterday 23:47 · 1 requests",
			},
			Today: "2 sessions · 1 requests",
		},
		{
			Doc: threeAt(At(1, 42, 0)), Now: At(1, 42, 0), Stream: live(), Lines: asksAgain,
			List:  []string{"5e7d ended 00:41 · started yesterday 23:47 · 1 requests"},
			Today: "1 sessions · 1 requests",
		},
	}}
}

// theRouterSilent is the 14:42:07 scene's running sessions, then the router
// not answering for ten minutes, each drawn from its last listing, idle
// counting on; then restarted, the stream it told forgotten, each where it
// sat, d28c's haiku listed on work, cold since 12:40; then not answering
// again, 9c2e past its hour, ended by its lines, d28c's opus warm still, its
// cold haiku listed with it, as the router lists a session; and past every
// session's hour, when its last listing runs none.
func theRouterSilent() SessionsMoment {
	first, silent, back := At(14, 42, 7), At(14, 52, 7), At(14, 52, 30)
	again, past := At(15, 40, 0), back.Add(70*time.Minute)
	listing := sceneListed()[:4]
	lines := slices.DeleteFunc(sceneLines(), func(l ledger.Line) bool {
		return !slices.Contains([]string{ID41E0, ID9C2E, C61B, D28C}, l.Session)
	})
	afterOutage := append(slices.Clone(lines),
		called("c1", C61B, Opus, side, mobileDir, At(14, 41, 50), 3*time.Minute),
		called("d9", D28C, Opus, side, switchboardDir, At(14, 42, 5), 115*time.Second),
	)
	heard := sceneStream()
	delete(heard.Requests, "f4")
	stale := heard
	stale.Live = false
	unheard := live()
	unheard.Live = false
	relisted := []status.Session{
		listing[0], listing[1],
		session(C61B, listedOn(Opus, side, mobileDir, status.ReasonNew, At(14, 41, 11), At(14, 44, 50), "")),
		session(D28C,
			listedOn(Opus, side, switchboardDir, capped(work), At(14, 31, 13), At(14, 44, 0), ""),
			listedOn(Haiku, work, switchboardDir, status.ReasonNew, At(12, 40, 0), At(12, 40, 0), ""),
		),
	}
	board := map[string]string{personal: "41e0@0 9c2e@1", side: "c61b@0 d28c@1"}
	return SessionsMoment{Name: "the router not answering for ten minutes, then restarted, the stream forgotten, then not answering past the hour", Looks: []SessionsLook{
		{
			Doc: threeAt(first), Now: first, Sessions: listing, Stream: heard, Lines: lines, Board: board,
			Calls: map[string]string{
				"41e0 personal": "idle 34s · ~/Code/docs · rests",
				"9c2e personal": "idle 9m · ~/Code/website · rests",
				"c61b side":     "↓ answering · ~/Code/mobile · works",
				"d28c side":     "↑ asking ▸ from work · ~/Code/switchboard · works",
			},
		},
		{
			Doc: threeAt(first), Now: silent, Sessions: listing, Unlisted: true, Stream: stale, Lines: lines, Board: board,
			Calls: map[string]string{
				"41e0 personal": "idle 10m · ~/Code/docs · rests",
				"9c2e personal": "idle 19m · ~/Code/website · rests",
				"c61b side":     "↓ answering · ~/Code/mobile · works",
				"d28c side":     "↑ asking ▸ from work · ~/Code/switchboard · works",
			},
		},
		{
			Doc: threeAt(back), Now: back, Sessions: relisted, Stream: live(), Lines: afterOutage, Board: board,
			Calls: map[string]string{
				"41e0 personal": "idle 10m · ~/Code/docs · rests",
				"9c2e personal": "idle 19m · ~/Code/website · rests",
				"c61b side":     "idle 7m · ~/Code/mobile · rests",
				"d28c side":     "idle 8m ▸ from work · ~/Code/switchboard · rests",
			},
		},
		{
			Doc: threeAt(back), Now: again, Sessions: relisted, Unlisted: true, Stream: unheard, Lines: afterOutage,
			Board: map[string]string{personal: "41e0@0", side: "c61b@0 d28c@1"},
			Calls: map[string]string{
				"41e0 personal": "idle 58m · ~/Code/docs · rests",
				"c61b side":     "idle 55m · ~/Code/mobile · rests",
				"d28c side":     "idle 56m ▸ from work · ~/Code/switchboard · rests",
			},
			List: []string{
				"d28c idle 56m · started 12:02 · 4 requests",
				"41e0 idle 58m · started 12:47 · 2 requests",
				"c61b idle 55m · started 14:41 · 2 requests",
				"9c2e ended 14:32 · started 11:55 · 2 requests",
			},
			Today: "4 sessions · 10 requests",
		},
		{
			Doc: threeAt(back), Now: past, Sessions: relisted, Unlisted: true, Stream: unheard, Lines: afterOutage, Board: map[string]string{},
			Calls: map[string]string{},
			List: []string{
				"d28c ended 14:42 · started 12:02 · 4 requests",
				"c61b ended 14:41 · started 14:41 · 2 requests",
				"41e0 ended 14:41 · started 12:47 · 2 requests",
				"9c2e ended 14:32 · started 11:55 · 2 requests",
			},
			Today: "4 sessions · 10 requests",
		},
	}}
}

// noDirFromTheRouter is a router that gives no directory: d28c's from its
// lines, 7f3a's lines naming none, and c61b, its first request in flight,
// with no lines yet.
func noDirFromTheRouter() SessionsMoment {
	now := At(14, 42, 7)
	listing := []status.Session{
		session(C61B, listedOn(Opus, side, "", status.ReasonNew, At(14, 42, 0), At(14, 42, 0), status.Asking)),
		session(D28C, listedOn(Opus, side, "", status.ReasonNew, At(14, 0, 0), At(14, 41, 0), "")),
		session(ID7F3A, listedOn(Opus, side, "", status.ReasonNew, At(14, 10, 0), At(14, 40, 0), "")),
	}
	return SessionsMoment{Name: "a router with no directory: the lines', else the id alone", Looks: []SessionsLook{{
		Doc: threeAt(now), Now: now, Sessions: listing, Stream: live(toldAsking("a1", C61B, Opus, side, At(14, 42, 0), At(14, 42, 0))),
		Lines: []ledger.Line{
			called("d1", D28C, Opus, side, switchboardDir, At(14, 0, 0), 10*time.Second),
			called("f1", ID7F3A, Opus, side, "", At(14, 10, 0), 10*time.Second),
		},
		Board: map[string]string{side: "c61b@0 d28c@1 7f3a@2"},
		Calls: map[string]string{
			"c61b side": "↑ asking · no dir · works",
			"d28c side": "idle 1m · ~/Code/switchboard · rests",
			"7f3a side": "idle 2m · no dir · rests",
		},
	}}}
}

// many are n sessions' ids, each shown as the letter given and its number,
// as "b007".
func many(letter byte, n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("%c%03d5e17-3a9b-4f60-8c21-7e4d9b0a6f13", letter, i)
	}
	return ids
}

// crowd is a look at 15:00 of the sessions with the given ids running on the
// accounts given in turn, as many on each as counts gives, none asking, each
// seen a second before the one before it, so each group lists them in
// order; and what it holds: each line's calls in turn on its first jacks,
// and each call idle.
func crowd(name string, accounts []string, counts []int, ids []string) SessionsMoment {
	now := At(15, 0, 0)
	look := SessionsLook{Doc: boardDoc(now, accounts...), Now: now, Stream: live(), Board: map[string]string{}, Calls: map[string]string{}}
	next := 0
	for i, account := range accounts {
		var line []string
		for jack := range counts[i] {
			id := ids[next]
			seen := now.Add(-time.Duration(next+1) * time.Second)
			look.Sessions = append(look.Sessions, session(id, listedOn(Opus, account, "", status.ReasonNew, At(14, 0, 0), seen, "")))
			shown := status.ShownID(id)
			line = append(line, shown+"@"+strconv.Itoa(jack))
			look.Calls[shown+" "+account] = "idle " + strconv.Itoa(next+1) + "s · no dir · rests"
			next++
		}
		if len(line) > 0 {
			look.Board[account] = strings.Join(line, " ")
		}
	}
	return SessionsMoment{Name: name, Looks: []SessionsLook{look}}
}

// bentCords is 25 sessions on side, the third account, each cord bent, one
// more than the bends' room holds at 160 columns.
func bentCords() SessionsMoment {
	return crowd("25 bent cords at 160 columns, one more than the room holds", []string{work, personal, side}, []int{0, 0, 25}, many('b', 25))
}

// fiftySessions is fifty sessions over three accounts: twenty on work, ten
// on personal and twenty on side.
func fiftySessions() SessionsMoment {
	return crowd("fifty sessions over three accounts", []string{work, personal, side}, []int{20, 10, 20}, many('f', 50))
}

// aGroupBelowItsLine is eight sessions on work and one on personal: its
// group starting below its line's first jack, the line moving down to meet
// it.
func aGroupBelowItsLine() SessionsMoment {
	return crowd("a group starting below its line's first jack", []string{work, personal, side}, []int{8, 1, 0}, many('g', 9))
}

// oneAccountEight is one account with eight sessions: its line growing two
// rows, a jack each.
func oneAccountEight() SessionsMoment {
	return crowd("one account with eight sessions, its line growing two rows", []string{work}, []int{8}, many('h', 8))
}

// joiningAndEnding is three on side; f41a joining at the foot; d28c ending
// an hour after its last request, side closing up, its jack freed; 5b19
// joining on that jack, at the foot; and d28c asking again, at the foot, and
// back in its place by its start among the List's running.
func joiningAndEnding() SessionsMoment {
	const took = 10 * time.Second
	seen := func(id string, at time.Time, inFlight string) status.Session {
		return session(id, listedOn(Opus, side, apiDir, status.ReasonNew, At(14, 0, 0), at, inFlight))
	}
	lines := []ledger.Line{
		called("j1", C61B, Opus, side, apiDir, At(14, 0, 0), took),
		called("j2", D28C, Opus, side, apiDir, At(14, 10, 0), took),
		called("j3", ID7F3A, Opus, side, apiDir, At(14, 20, 0), took),
	}
	ended := append(slices.Clone(lines),
		called("j4", IDF41A, Opus, side, apiDir, At(15, 0, 25), took),
		called("j5", D28C, Opus, side, apiDir, At(15, 5, 0), took),
	)
	return SessionsMoment{Name: "a new session at its group's foot; one ending, its group closing up, then asking again", Looks: []SessionsLook{
		{
			Doc: threeAt(At(15, 0, 0)), Now: At(15, 0, 0), Stream: live(), Lines: lines,
			Sessions: []status.Session{seen(C61B, At(14, 59, 50), ""), seen(D28C, At(14, 59, 40), ""), seen(ID7F3A, At(14, 59, 30), "")},
			Board:    map[string]string{side: "c61b@0 d28c@1 7f3a@2"},
			Calls: map[string]string{
				"c61b side": "idle 10s · ~/Code/api · rests", "d28c side": "idle 20s · ~/Code/api · rests", "7f3a side": "idle 30s · ~/Code/api · rests",
			},
		},
		{
			Doc: threeAt(At(15, 0, 30)), Now: At(15, 0, 30), Stream: live(toldAsking("j4", IDF41A, Opus, side, At(15, 0, 25), At(15, 0, 25))), Lines: lines,
			Sessions: []status.Session{
				seen(IDF41A, At(15, 0, 25), status.Asking), seen(C61B, At(14, 59, 50), ""), seen(D28C, At(14, 59, 40), ""), seen(ID7F3A, At(14, 59, 30), ""),
			},
			Board: map[string]string{side: "c61b@0 d28c@1 7f3a@2 f41a@3"},
			Calls: map[string]string{
				"c61b side": "idle 40s · ~/Code/api · rests", "d28c side": "idle 50s · ~/Code/api · rests", "7f3a side": "idle 1m · ~/Code/api · rests",
				"f41a side": "↑ asking · ~/Code/api · works",
			},
			List: []string{
				"c61b idle 40s · started 14:00 · 1 requests", "d28c idle 50s · started 14:10 · 1 requests",
				"7f3a idle 1m · started 14:20 · 1 requests", "f41a asking · 0 requests",
			},
		},
		{
			Doc: threeAt(At(16, 10, 0)), Now: At(16, 10, 0), Stream: live(), Lines: ended,
			Sessions: []status.Session{seen(IDF41A, At(16, 9, 30), ""), seen(C61B, At(16, 9, 0), ""), seen(ID7F3A, At(16, 8, 0), "")},
			Board:    map[string]string{side: "c61b@0 7f3a@2 f41a@3"},
			Calls: map[string]string{
				"c61b side": "idle 1m · ~/Code/api · rests", "7f3a side": "idle 2m · ~/Code/api · rests", "f41a side": "idle 30s · ~/Code/api · rests",
			},
			List: []string{
				"c61b idle 1m · started 14:00 · 1 requests", "7f3a idle 2m · started 14:20 · 1 requests",
				"f41a idle 30s · started 15:00 · 1 requests", "d28c ended 15:05 · started 14:10 · 2 requests",
			},
		},
		{
			Doc: threeAt(At(16, 10, 30)), Now: At(16, 10, 30), Stream: live(toldAsking("k1", id5B19, Opus, side, At(16, 10, 25), At(16, 10, 25))), Lines: ended,
			Sessions: []status.Session{
				seen(id5B19, At(16, 10, 25), status.Asking), seen(IDF41A, At(16, 9, 30), ""), seen(C61B, At(16, 9, 0), ""), seen(ID7F3A, At(16, 8, 0), ""),
			},
			Board: map[string]string{side: "c61b@0 7f3a@2 f41a@3 5b19@1"},
			Calls: map[string]string{
				"c61b side": "idle 1m · ~/Code/api · rests", "7f3a side": "idle 2m · ~/Code/api · rests", "f41a side": "idle 1m · ~/Code/api · rests",
				"5b19 side": "↑ asking · ~/Code/api · works",
			},
		},
		{
			Doc: threeAt(At(16, 11, 0)), Now: At(16, 11, 0), Lines: ended,
			Stream: live(toldAsking("k1", id5B19, Opus, side, At(16, 10, 25), At(16, 10, 25)), toldAsking("k2", D28C, Opus, side, At(16, 10, 58), At(16, 10, 58))),
			Sessions: []status.Session{
				seen(D28C, At(16, 10, 58), status.Asking), seen(id5B19, At(16, 10, 25), status.Asking), seen(IDF41A, At(16, 9, 30), ""),
				seen(C61B, At(16, 9, 0), ""), seen(ID7F3A, At(16, 8, 0), ""),
			},
			Board: map[string]string{side: "c61b@0 7f3a@2 f41a@3 5b19@1 d28c@4"},
			Calls: map[string]string{
				"c61b side": "idle 2m · ~/Code/api · rests", "7f3a side": "idle 3m · ~/Code/api · rests", "f41a side": "idle 1m · ~/Code/api · rests",
				"5b19 side": "↑ asking · ~/Code/api · works", "d28c side": "↑ asking · ~/Code/api · works",
			},
			List: []string{
				"c61b idle 2m · started 14:00 · 1 requests", "d28c asking · started 14:10 · 2 requests",
				"7f3a idle 3m · started 14:20 · 1 requests", "f41a idle 1m · started 15:00 · 1 requests", "5b19 asking · 0 requests",
			},
		},
	}}
}

// titledAfterItsMainOnTheBoard is d28c, whose main thread is Claude Opus 5.5
// on work, its title by Claude Haiku 4.5 on work too, after its main
// request: its lines name it Opus as the stream, just opened, tells of
// neither; then as its next main request is heard.
func titledAfterItsMainOnTheBoard() SessionsMoment {
	mainAt, titledAt, sent := At(14, 30, 0), At(14, 31, 30), At(14, 42, 20)
	title := withID("t2", asTitle(p1, line(D28C, Haiku, work, titledAt, 2*time.Second)))
	title.Dir = switchboardDir
	lines := []ledger.Line{called("t1", D28C, Opus, work, switchboardDir, mainAt, time.Minute), title}
	listing := func(opusSeen time.Time, inFlight string) []status.Session {
		return []status.Session{session(D28C,
			listedOn(Opus, work, switchboardDir, status.ReasonNew, mainAt, opusSeen, inFlight),
			listedOn(Haiku, work, switchboardDir, status.ReasonNew, titledAt, titledAt, ""),
		)}
	}
	return SessionsMoment{Name: "an Opus session's Haiku title after its main, named Opus by its lines, before its next main request is heard and after", Looks: []SessionsLook{
		{
			Doc: threeAt(At(14, 42, 7)), Now: At(14, 42, 7), Sessions: listing(mainAt, ""), Stream: live(), Lines: lines,
			Board: map[string]string{work: "d28c@0"},
			Calls: map[string]string{"d28c work": "idle 10m · ~/Code/switchboard · rests"},
			List:  []string{"d28c idle 10m · started 14:30 · 2 requests"},
		},
		{
			Doc: threeAt(At(14, 42, 30)), Now: At(14, 42, 30), Sessions: listing(sent, status.Asking), Lines: lines,
			Stream: live(toldAsking("t3", D28C, Opus, work, sent, sent)),
			Board:  map[string]string{work: "d28c@0"},
			Calls:  map[string]string{"d28c work": "↑ asking · ~/Code/switchboard · works"},
			List:   []string{"d28c asking · started 14:30 · 2 requests"},
		},
	}}
}

// coldMainOnTheBoard is d28c, whose main thread, Claude Opus 5.5, moved to
// work from personal at its limit at 13:30 and has waited since on its
// subagent, of Claude Haiku 4.5, on work too, seen at 14:40: its call there
// named by its main thread, cold as it is, and come from personal, as the
// List has it.
func coldMainOnTheBoard() SessionsMoment {
	subagent := func(id string, at time.Time) ledger.Line {
		l := withID(id, asSubagent("a1", p2, line(D28C, Haiku, work, at, 5*time.Second)))
		l.Dir = switchboardDir
		return l
	}
	moved := cameFrom(personal, atItsLimit(personal), called("c2", D28C, Opus, work, switchboardDir, At(13, 30, 0), 10*time.Second))
	moved.Prompt, moved.Answer.Stop = p2, "tool_use"
	lines := []ledger.Line{
		called("c1", D28C, Opus, personal, switchboardDir, At(13, 0, 0), 10*time.Second), moved,
		subagent("c3", At(13, 31, 0)), subagent("c4", At(14, 10, 0)), subagent("c5", At(14, 40, 0)),
	}
	listing := []status.Session{session(D28C,
		listedOn(Opus, work, switchboardDir, atItsLimit(personal), At(13, 30, 0), At(13, 30, 10), ""),
		listedOn(Haiku, work, switchboardDir, status.ReasonNew, At(13, 31, 0), At(14, 40, 0), ""),
	)}
	return SessionsMoment{Name: "an Opus main thread moved to work, waiting there over the IdleHour on its Haiku subagent there", Looks: []SessionsLook{{
		Doc: threeAt(At(14, 42, 7)), Now: At(14, 42, 7), Sessions: listing, Stream: live(), Lines: lines,
		Board: map[string]string{work: "d28c@0"},
		Calls: map[string]string{"d28c work": "idle 2m ▸ from personal · ~/Code/switchboard · rests"},
		List:  []string{"d28c idle 2m · started 13:00 · 5 requests"},
	}}}
}
