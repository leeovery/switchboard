// Package viewstest builds what the views read of the router's sessions, the
// request stream and the ledger's lines, for tests, at the moments the views
// have to get right, as statustest builds documents.
package viewstest

import (
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// The sessions of the moments, and the models their requests ask for.
const (
	D28C  = "d28c5e17-3a9b-4f60-8c21-7e4d9b0a6f13"
	C61B  = "c61b0f2e-7d45-4a19-9e3c-5b8a2d71f604"
	Opus  = "claude-opus-5-5"
	Haiku = "claude-haiku-4-5"
)

// The prompts the moments' requests serve, as Claude Code names them.
const (
	p1 = "prompt-1"
	p2 = "prompt-2"
)

// SessionMoment is the router's sessions, what the request stream told of
// their requests, and their lines, at a moment the views have to get right,
// and what each session is doing then, its turns, and what names it.
type SessionMoment struct {
	Name string
	// Doc is the router's document of work and side, new sessions going to
	// side, and Now when the views read it all.
	Doc status.Document
	Now time.Time
	// Sessions are those the router lists, as GET /sessions gives them: none
	// where none does, as Unlisted says.
	Sessions []status.Session
	// Unlisted is set where no router lists its sessions: it's gone, or the
	// dashboard probes.
	Unlisted bool
	// Stream is what the request stream told, as the watch heard it.
	Stream views.Stream
	// Lines are the sessions' lines, oldest first: today's, and yesterday's
	// of one that ran across midnight.
	Lines []ledger.Line
	// Want is what each session is doing, by its id.
	Want map[string]Doing
	// Turns are each session's turns, newest first, as its page tells them,
	// by its id: none where its requests name no prompt.
	Turns map[string][]Turn
	// Named is what names each session, by its id.
	Named map[string]Named
	// Page is what each session's page tells of it, by its id.
	Page map[string]PageFacts
}

// PageFacts are what a session's page tells of it at a moment: whether it
// runs, as its page draws it; where its models go, as row 7 has them; whether
// its newest turn goes on; whether a request of it waits for its answer; the
// accounts its lines went to, its rows', in the order they first did; and,
// of its story, the accounts its ribbon names, in turn, and how many moves
// it marks.
type PageFacts struct {
	Runs     bool
	Goes     []Named
	Going    bool
	Waiting  bool
	Accounts []string
	Ribbon   []string
	Marks    int
}

// Named is the account and model naming a session, following its main
// thread.
type Named struct {
	Account, Model string
}

// Doing is what a session is doing at a moment: as a whole, as the List
// says it; and on each account a model of its runs on, by the account's id,
// as SESSIONS says it, none where it runs on none.
type Doing struct {
	Whole views.Activity
	On    map[string]views.Activity
}

// Turn is what the moments hold of a session's turn, as its page tells it:
// its number; when it started, and ended, zero while it's going; its
// requests; and the accounts its own conversation's requests went to.
type Turn struct {
	Turn           int
	Started, Ended time.Time
	Requests       int
	Accounts       []string
}

// SessionMoments are the moments the views have to tell what each session
// is doing at, and its turns: asking, as the stream tells, or as the router
// listed it, the stream joined yet to tell; answering; refused at a limit,
// or by a 403; throttled; idle, its answer's end as the stream told it, or,
// without the stream, as its line gives it; the stream ending with a request
// in flight, joined again with none; the router restarted; the router gone,
// or probing; a session ended, then resumed; a move while it runs; two
// models on two accounts; idle 59 minutes after an answer that called a
// tool; a turn of side requests alone, its main in flight, as the stream
// tells and as the router lists it with no stream; its main failed; a
// request of the newest turn in flight, its answer ended with its line to
// come, then its line come; a turn that moved twice; a turn across midnight;
// lines that name no prompt, nor class; a move by hand with no request
// since; a new session, its first line read before the router's listing at
// the next look; a session idle with its page open, then ended an hour after
// its last line; probing within the hour; a subagent's model moved, its main
// thread staying; a request in flight naming no prompt after named turns;
// refused on one account, then throttled on another; a request the router
// answered itself, no account having room; and an Opus session's Haiku title
// on its account after its main, the stream just opened telling of neither,
// then its next main request heard.
func SessionMoments() []SessionMoment {
	return []SessionMoment{
		asking(), askingAsListed(), answering(), refused(), throttled(), idleAsTheStreamTold(), idleAsItsLineGives(),
		lostAsTheStreamEnded(), routerRestarted(), routerGone(), ended(), resumed(), movedAsItRuns(),
		twoModelsOnTwoAccounts(), idleAfterAToolCall(), sideRequestsAloneItsMainInFlight(), sideRequestsAloneAsListed(),
		itsMainFailed(), inFlightInTheNewestTurn(), endedItsLineToCome(), itsLineCome(), movedTwice(), turnAcrossMidnight(),
		noPrompts(), movedByHand(), newBeforeTheNextLook(), idleItsPageOpen(), endedItsPageOpen(), probingWithinTheHour(),
		subagentMoved(), inFlightNamingNoPrompt(), refusedThenThrottled(), answeredByTheRouter(),
		titledAfterItsMainUnheard(), titledAfterItsMainThenItsMainHeard(),
	}
}

// At is the local time on Thursday 1 October 2026, at the clock given, when
// the moments are: the ledger files its lines under local days.
func At(hour, minute, second int) time.Time {
	return time.Date(2026, 10, 1, hour, minute, second, 0, time.Local)
}

// now is when the moments are read: 14:42:07.
var now = At(14, 42, 7)

// docAt is the router's document of work and side at the time given, new
// sessions going to side.
func docAt(at time.Time) status.Document {
	return status.Document{
		Source: status.SourceRouter, GeneratedAt: at, Best: "side",
		Accounts: []status.Account{{ID: "work"}, {ID: "side"}},
	}
}

// moment is a moment, named, at now, of the router's document then and the
// sessions given, the router listing them.
func moment(name string, sessions ...status.Session) SessionMoment {
	return momentAt(now, name, sessions...)
}

// momentAt is a moment, named, at the time given, of the router's document
// then and the sessions given, the router listing them.
func momentAt(at time.Time, name string, sessions ...status.Session) SessionMoment {
	return SessionMoment{Name: name, Doc: docAt(at), Now: at, Sessions: sessions}
}

// session is the session with the given id as the router lists it, its
// models as given.
func session(id string, models ...status.Assignment) status.Session {
	return status.Session{ID: id, Assignments: models}
}

// seat is a session's model on an account, as the router lists it, put
// there at 14:00 as the session started, last seen at seen, and doing what
// its request in flight does.
func seat(model, account string, seen time.Time, inFlight string) status.Assignment {
	family := "opus"
	if model == Haiku {
		family = "haiku"
	}
	return status.Assignment{
		Model: model, Family: family, Account: account, Reason: status.ReasonNew,
		AssignedAt: At(14, 0, 0), LastSeen: seen, InFlight: inFlight,
	}
}

// told is a request of the session's model on an account, as the stream
// told of it, sent at sent, and last told of at heard.
func told(request, session, model, account string, sent, heard time.Time) views.Told {
	return views.Told{Request: request, Session: session, Model: model, Account: account, Sent: sent, Heard: heard}
}

// toldServing returns t, a request of the class given, serving the prompt
// given, as the stream told.
func toldServing(prompt, class string, t views.Told) views.Told {
	t.Prompt, t.Class = prompt, class
	return t
}

// done returns t, ended at at, answered as given, as the stream told.
func done(at time.Time, status int, t views.Told) views.Told {
	t.Ended, t.Heard, t.Status = at, at, status
	return t
}

// live is the stream, live, telling of the requests given.
func live(requests ...views.Told) views.Stream {
	s := views.Stream{Live: true, Requests: make(map[string]views.Told, len(requests))}
	for _, r := range requests {
		s.Requests[r.Request] = r
	}
	return s
}

// line is a request of the session's model on an account, as its line
// gives it: arrived at at, and answered, its answer ending took later.
func line(session, model, account string, at time.Time, took time.Duration) ledger.Line {
	return ledger.Line{
		At: at, Kind: ledger.KindMessage, Session: session, Model: model, Account: account,
		Status: 200, Attempts: 1, TotalMS: took.Milliseconds(),
	}
}

// withID returns l, the line of the request the router knows by the id
// given.
func withID(request string, l ledger.Line) ledger.Line {
	l.Request = request
	return l
}

// asMain returns l, a request of its session's own conversation serving the
// prompt given, its answer stopping as stop says.
func asMain(prompt, stop string, l ledger.Line) ledger.Line {
	l.Prompt, l.Class, l.Answer.Stop = prompt, ledger.ClassMain, stop
	return l
}

// asTitle returns l, a side request, a title's, serving the prompt given.
func asTitle(prompt string, l ledger.Line) ledger.Line {
	l.Prompt, l.Class, l.Answer.Stop = prompt, "auxiliary", "end_turn"
	return l
}

// movedOff returns l, a request that moved its session from the account
// given.
func movedOff(from string, l ledger.Line) ledger.Line {
	l.From = from
	return l
}

// on is what a session does on one account.
func on(account string, a views.Activity) map[string]views.Activity {
	return map[string]views.Activity{account: a}
}

// alike is a session doing a on its one account, and so as a whole.
func alike(account string, a views.Activity) Doing {
	return Doing{Whole: a, On: on(account, a)}
}

// going is a turn, numbered n, started at started, still going, of the
// requests given, its own conversation's on the accounts given.
func going(n int, started time.Time, requests int, accounts ...string) Turn {
	return Turn{Turn: n, Started: started, Requests: requests, Accounts: accounts}
}

// over is a turn, numbered n, started at started and ended at ended, of the
// requests given, its own conversation's on the accounts given.
func over(n int, started, ended time.Time, requests int, accounts ...string) Turn {
	return Turn{Turn: n, Started: started, Ended: ended, Requests: requests, Accounts: accounts}
}

// opusOn is a session's one model, Claude Opus 5.5, going to the account
// given.
func opusOn(account string) []Named {
	return []Named{{account, Opus}}
}

// runsOn are the page facts of a session running its one model, Claude Opus
// 5.5, on the account given, its story's ribbon on that account alone; its
// newest turn going or not, and a request of it waiting for its answer or
// not; its rows those of the accounts its lines went to, given.
func runsOn(account string, going, waiting bool, rows ...string) PageFacts {
	return PageFacts{Runs: true, Goes: opusOn(account), Going: going, Waiting: waiting, Accounts: rows, Ribbon: []string{account}}
}

func asking() SessionMoment {
	sent := At(14, 42, 5)
	m := moment("asking", session(D28C, seat(Opus, "work", sent, status.Asking)))
	m.Stream = live(toldServing(p1, ledger.ClassMain, told("r1", D28C, Opus, "work", sent, sent)))
	m.Want = map[string]Doing{D28C: alike("work", views.Activity{Doing: views.Asking, Since: sent, LastSeen: sent})}
	m.Turns = map[string][]Turn{D28C: {going(1, sent, 1, "work")}}
	m.Named = map[string]Named{D28C: {"work", Opus}}
	m.Page = map[string]PageFacts{D28C: runsOn("work", true, true)}
	return m
}

func askingAsListed() SessionMoment {
	sent := At(14, 42, 5)
	m := moment("asking, as the router listed it, the stream joined yet to tell of it", session(D28C, seat(Opus, "work", sent, status.Asking)))
	m.Stream = live()
	m.Want = map[string]Doing{D28C: alike("work", views.Activity{Doing: views.Asking, Since: sent, LastSeen: sent})}
	m.Named = map[string]Named{D28C: {"work", Opus}}
	m.Page = map[string]PageFacts{D28C: {Runs: true, Goes: opusOn("work"), Waiting: true}}
	return m
}

func answering() SessionMoment {
	sent, progress := At(14, 42, 0), At(14, 42, 6)
	r := toldServing(p1, ledger.ClassMain, told("r1", D28C, Opus, "work", sent, progress))
	r.First, r.Chars = At(14, 42, 1), 4800
	m := moment("answering", session(D28C, seat(Opus, "work", sent, status.Answering)))
	m.Stream = live(r)
	m.Want = map[string]Doing{D28C: alike("work", views.Activity{Doing: views.Answering, Chars: 4800, Since: sent, LastSeen: progress})}
	m.Turns = map[string][]Turn{D28C: {going(1, sent, 1, "work")}}
	m.Named = map[string]Named{D28C: {"work", Opus}}
	m.Page = map[string]PageFacts{D28C: runsOn("work", true, false)}
	return m
}

func refused() SessionMoment {
	sent, limited, refusedAt := At(14, 42, 5), At(14, 42, 6), At(14, 42, 4)
	atLimit := toldServing(p1, ledger.ClassMain, told("r1", D28C, Opus, "work", sent, limited))
	atLimit.Verdict, atLimit.Status = views.VerdictLimited, 429
	forbidden := toldServing(p1, ledger.ClassMain, told("r2", C61B, Opus, "side", At(14, 42, 3), refusedAt))
	forbidden.Verdict, forbidden.Status = views.VerdictRefused, 403
	m := moment("refused: a 429 at work's limit, and a 403",
		session(D28C, seat(Opus, "work", sent, status.Asking)),
		session(C61B, seat(Opus, "side", At(14, 42, 3), status.Asking)),
	)
	m.Stream = live(atLimit, forbidden)
	m.Want = map[string]Doing{
		D28C: alike("work", views.Activity{Doing: views.Refused, Status: 429, Since: sent, LastSeen: limited}),
		C61B: alike("side", views.Activity{Doing: views.Refused, Status: 403, Since: At(14, 42, 3), LastSeen: refusedAt}),
	}
	m.Turns = map[string][]Turn{D28C: {going(1, sent, 1, "work")}, C61B: {going(1, At(14, 42, 3), 1, "side")}}
	m.Named = map[string]Named{D28C: {"work", Opus}, C61B: {"side", Opus}}
	m.Page = map[string]PageFacts{D28C: runsOn("work", true, true), C61B: runsOn("side", true, true)}
	return m
}

func throttled() SessionMoment {
	sent, throttledAt := At(14, 42, 0), At(14, 42, 3)
	r := toldServing(p1, ledger.ClassMain, told("r1", D28C, Opus, "work", sent, throttledAt))
	r.Verdict, r.Status = views.VerdictThrottled, 429
	m := moment("throttled", session(D28C, seat(Opus, "work", sent, status.Asking)))
	m.Stream = live(r)
	m.Want = map[string]Doing{D28C: alike("work", views.Activity{Doing: views.Throttled, Status: 429, Since: sent, LastSeen: throttledAt})}
	m.Turns = map[string][]Turn{D28C: {going(1, sent, 1, "work")}}
	m.Named = map[string]Named{D28C: {"work", Opus}}
	m.Page = map[string]PageFacts{D28C: runsOn("work", true, true)}
	return m
}

func idleAsTheStreamTold() SessionMoment {
	sent, answered := At(14, 41, 20), At(14, 41, 33)
	r := done(answered, 200, toldServing(p1, ledger.ClassMain, told("r1", D28C, Opus, "work", sent, answered)))
	r.First, r.Chars = At(14, 41, 21), 900
	m := moment("idle, its answer's end as the stream told it, the router listing it answering since", session(D28C, seat(Opus, "work", sent, status.Answering)))
	m.Stream = live(r)
	m.Lines = []ledger.Line{withID("r1", asMain(p1, "end_turn", line(D28C, Opus, "work", sent, 13*time.Second)))}
	m.Want = map[string]Doing{D28C: alike("work", views.Activity{Since: answered, LastSeen: answered})}
	m.Turns = map[string][]Turn{D28C: {over(1, sent, answered, 1, "work")}}
	m.Named = map[string]Named{D28C: {"work", Opus}}
	m.Page = map[string]PageFacts{D28C: runsOn("work", false, false, "work")}
	return m
}

func idleAsItsLineGives() SessionMoment {
	sent, answered := At(14, 41, 20), At(14, 41, 33)
	m := moment("idle, without the stream: since its line's end where it's read, else the router's last_seen, of its one request",
		session(D28C, seat(Opus, "work", sent, "")))
	m.Lines = []ledger.Line{asMain(p1, "end_turn", line(D28C, Opus, "work", sent, 13*time.Second))}
	m.Want = map[string]Doing{D28C: {
		Whole: views.Activity{Since: answered, LastSeen: sent},
		On:    on("work", views.Activity{Since: sent, LastSeen: sent}),
	}}
	m.Turns = map[string][]Turn{D28C: {over(1, sent, answered, 1, "work")}}
	m.Named = map[string]Named{D28C: {"work", Opus}}
	m.Page = map[string]PageFacts{D28C: runsOn("work", false, false, "work")}
	return m
}

func lostAsTheStreamEnded() SessionMoment {
	sent, progress := At(14, 41, 50), At(14, 41, 55)
	r := toldServing(p1, ledger.ClassMain, told("r1", D28C, Opus, "work", sent, progress))
	r.First, r.Chars, r.Lost = At(14, 41, 51), 2000, true
	m := moment("the stream ending with a request in flight, joined again with none in flight, its line never come",
		session(D28C, seat(Opus, "work", sent, status.Answering)))
	m.Stream = live(r)
	m.Want = map[string]Doing{D28C: alike("work", views.Activity{Since: progress, LastSeen: progress})}
	m.Turns = map[string][]Turn{D28C: {over(1, sent, progress, 1, "work")}}
	m.Named = map[string]Named{D28C: {"work", Opus}}
	m.Page = map[string]PageFacts{D28C: runsOn("work", false, false)}
	return m
}

func routerRestarted() SessionMoment {
	lost := toldServing(p1, ledger.ClassMain, told("r1", D28C, Opus, "work", At(14, 41, 50), At(14, 41, 55)))
	lost.Lost = true
	gone := toldServing(p1, ledger.ClassMain, told("r2", C61B, Opus, "side", At(14, 41, 52), At(14, 41, 56)))
	gone.Lost = true
	again := toldServing(p1, ledger.ClassMain, told("9000", C61B, Opus, "side", At(14, 42, 4), At(14, 42, 4)))
	m := moment("the router restarted, the last one's stream forgotten",
		session(D28C, seat(Opus, "work", At(14, 41, 50), "")),
		session(C61B, seat(Opus, "side", At(14, 42, 4), status.Asking)),
	)
	m.Stream = live(lost, gone, again)
	m.Want = map[string]Doing{
		D28C: alike("work", views.Activity{Since: At(14, 41, 55), LastSeen: At(14, 41, 55)}),
		C61B: alike("side", views.Activity{Doing: views.Asking, Since: At(14, 42, 4), LastSeen: At(14, 42, 4)}),
	}
	m.Turns = map[string][]Turn{
		D28C: {over(1, At(14, 41, 50), At(14, 41, 55), 1, "work")},
		C61B: {going(1, At(14, 41, 52), 2, "side")},
	}
	m.Named = map[string]Named{D28C: {"work", Opus}, C61B: {"side", Opus}}
	m.Page = map[string]PageFacts{D28C: runsOn("work", false, false), C61B: runsOn("side", true, true)}
	return m
}

func routerGone() SessionMoment {
	m := moment("the router gone, or probing: drawn from its lines, by the router's own hour, each last answer calling a tool")
	m.Unlisted = true
	m.Lines = []ledger.Line{
		asMain(p1, "tool_use", line(C61B, Opus, "side", At(13, 30, 0), 20*time.Second)),
		asMain(p1, "tool_use", line(D28C, Opus, "work", At(14, 20, 0), 15*time.Second)),
	}
	m.Want = map[string]Doing{
		D28C: {Whole: views.Activity{Since: At(14, 20, 15), LastSeen: At(14, 20, 0)}},
		C61B: {Whole: views.Activity{Doing: views.Ended, Since: At(13, 30, 0), LastSeen: At(13, 30, 0)}},
	}
	m.Turns = map[string][]Turn{
		D28C: {going(1, At(14, 20, 0), 1, "work")},
		C61B: {over(1, At(13, 30, 0), At(13, 30, 20), 1, "side")},
	}
	m.Named = map[string]Named{D28C: {"work", Opus}, C61B: {"side", Opus}}
	m.Page = map[string]PageFacts{
		D28C: runsOn("work", true, false, "work"),
		C61B: {Goes: opusOn("side"), Accounts: []string{"side"}, Ribbon: []string{"side"}},
	}
	return m
}

func ended() SessionMoment {
	m := moment("ended, an hour after its last request, its last answer calling a tool", session(C61B, seat(Opus, "side", At(14, 40, 0), "")))
	m.Lines = []ledger.Line{asMain(p1, "tool_use", line(D28C, Opus, "work", At(13, 30, 0), 20*time.Second))}
	m.Want = map[string]Doing{
		D28C: {Whole: views.Activity{Doing: views.Ended, Since: At(13, 30, 0), LastSeen: At(13, 30, 0)}},
		C61B: alike("side", views.Activity{Since: At(14, 40, 0), LastSeen: At(14, 40, 0)}),
	}
	m.Turns = map[string][]Turn{D28C: {over(1, At(13, 30, 0), At(13, 30, 20), 1, "work")}}
	m.Named = map[string]Named{D28C: {"work", Opus}, C61B: {"side", Opus}}
	m.Page = map[string]PageFacts{
		D28C: {Goes: opusOn("work"), Accounts: []string{"work"}, Ribbon: []string{"work"}},
		C61B: {Runs: true, Goes: opusOn("side")},
	}
	return m
}

func resumed() SessionMoment {
	sent := At(14, 42, 0)
	back := seat(Opus, "side", sent, status.Asking)
	back.Reason, back.AssignedAt = status.Rescored(time.Hour+12*time.Minute), sent
	m := moment("resumed, asking again after it ended", session(D28C, back))
	m.Stream = live(toldServing(p2, ledger.ClassMain, told("r1", D28C, Opus, "side", sent, sent)))
	m.Lines = []ledger.Line{asMain(p1, "end_turn", line(D28C, Opus, "work", At(13, 30, 0), 20*time.Second))}
	m.Want = map[string]Doing{D28C: alike("side", views.Activity{Doing: views.Asking, Since: sent, LastSeen: sent})}
	m.Turns = map[string][]Turn{D28C: {going(2, sent, 1, "side"), over(1, At(13, 30, 0), At(13, 30, 20), 1, "work")}}
	m.Named = map[string]Named{D28C: {"side", Opus}}
	m.Page = map[string]PageFacts{D28C: {Runs: true, Goes: opusOn("side"), Going: true, Waiting: true, Accounts: []string{"work"}, Ribbon: []string{"work", "side"}}}
	return m
}

func movedAsItRuns() SessionMoment {
	sent, limited := At(14, 42, 4), At(14, 42, 5)
	r := toldServing(p1, ledger.ClassMain, told("r1", D28C, Opus, "side", sent, limited))
	m := moment("moved to side as it runs, the router listing it on work still",
		session(D28C, seat(Opus, "work", sent, status.Asking)),
		session(C61B, seat(Opus, "work", At(14, 30, 0), "")),
	)
	m.Stream = live(r)
	m.Stream.Moves = []views.Moved{{Request: "r1", Session: D28C, Model: Opus, From: "work", To: "side", Reason: status.ReasonMovedOff + "work " + status.WhyLimit, At: limited}}
	m.Lines = []ledger.Line{asMain(p1, "tool_use", line(D28C, Opus, "work", At(14, 41, 0), 10*time.Second))}
	m.Want = map[string]Doing{
		D28C: alike("side", views.Activity{Doing: views.Asking, Since: sent, LastSeen: limited}),
		C61B: alike("work", views.Activity{Since: At(14, 30, 0), LastSeen: At(14, 30, 0)}),
	}
	m.Turns = map[string][]Turn{D28C: {going(1, At(14, 41, 0), 2, "work", "side")}}
	m.Named = map[string]Named{D28C: {"side", Opus}, C61B: {"work", Opus}}
	m.Page = map[string]PageFacts{
		D28C: {Runs: true, Goes: opusOn("side"), Going: true, Waiting: true, Accounts: []string{"work"}, Ribbon: []string{"work", "side"}, Marks: 1},
		C61B: {Runs: true, Goes: opusOn("work")},
	}
	return m
}

func twoModelsOnTwoAccounts() SessionMoment {
	sent, progress, titled := At(14, 42, 0), At(14, 42, 6), At(14, 40, 0)
	opus := toldServing(p1, ledger.ClassMain, told("r1", D28C, Opus, "work", sent, progress))
	opus.First, opus.Chars = At(14, 42, 2), 1200
	haiku := done(titled, 200, toldServing(p1, "auxiliary", told("r2", D28C, Haiku, "side", At(14, 39, 58), titled)))
	m := moment("two models on two accounts, its title on side", session(D28C,
		seat(Opus, "work", sent, status.Answering),
		seat(Haiku, "side", At(14, 39, 58), ""),
	))
	m.Stream = live(opus, haiku)
	m.Lines = []ledger.Line{withID("r2", asTitle(p1, line(D28C, Haiku, "side", At(14, 39, 58), 2*time.Second)))}
	answeringOnWork := views.Activity{Doing: views.Answering, Chars: 1200, Since: sent, LastSeen: progress}
	m.Want = map[string]Doing{D28C: {
		Whole: answeringOnWork,
		On:    map[string]views.Activity{"work": answeringOnWork, "side": {Since: titled, LastSeen: titled}},
	}}
	m.Turns = map[string][]Turn{D28C: {going(1, At(14, 39, 58), 2, "work")}}
	m.Named = map[string]Named{D28C: {"work", Opus}}
	m.Page = map[string]PageFacts{D28C: {Runs: true, Goes: []Named{{"work", Opus}, {"side", Haiku}}, Going: true, Accounts: []string{"side"}, Ribbon: []string{"work"}}}
	return m
}

func idleAfterAToolCall() SessionMoment {
	sent := At(13, 42, 50)
	called := asMain(p1, "tool_use", line(D28C, Opus, "work", sent, 17*time.Second))
	called.Answer.Tools = []string{"Bash"}
	m := moment("idle 59 minutes after an answer that called a tool", session(D28C, seat(Opus, "work", sent, "")))
	m.Stream = live()
	m.Lines = []ledger.Line{called}
	m.Want = map[string]Doing{D28C: {
		Whole: views.Activity{Since: At(13, 43, 7), LastSeen: sent},
		On:    on("work", views.Activity{Since: sent, LastSeen: sent}),
	}}
	m.Turns = map[string][]Turn{D28C: {going(1, sent, 1, "work")}}
	m.Named = map[string]Named{D28C: {"work", Opus}}
	m.Page = map[string]PageFacts{D28C: runsOn("work", true, false, "work")}
	return m
}

func sideRequestsAloneItsMainInFlight() SessionMoment {
	sent, titled, answered := At(14, 42, 0), At(14, 42, 1), At(14, 42, 3)
	m := moment("the newest turn of side requests alone, its main in flight, its title on side",
		session(D28C, seat(Opus, "work", sent, status.Asking), seat(Haiku, "side", titled, "")))
	m.Stream = live(
		toldServing(p2, ledger.ClassMain, told("r2", D28C, Opus, "work", sent, sent)),
		done(answered, 200, toldServing(p2, "auxiliary", told("r3", D28C, Haiku, "side", titled, titled))),
	)
	m.Lines = []ledger.Line{
		withID("r1", asMain(p1, "end_turn", line(D28C, Opus, "work", At(14, 30, 0), time.Minute))),
		withID("r3", asTitle(p2, line(D28C, Haiku, "side", titled, 2*time.Second))),
	}
	m.Want = map[string]Doing{D28C: {
		Whole: views.Activity{Doing: views.Asking, Since: sent, LastSeen: answered},
		On: map[string]views.Activity{
			"work": {Doing: views.Asking, Since: sent, LastSeen: sent},
			"side": {Since: answered, LastSeen: answered},
		},
	}}
	m.Turns = map[string][]Turn{D28C: {going(2, sent, 2, "work"), over(1, At(14, 30, 0), At(14, 31, 0), 1, "work")}}
	m.Named = map[string]Named{D28C: {"work", Opus}}
	m.Page = map[string]PageFacts{D28C: titledOnSide}
	return m
}

// titledOnSide are the page facts of a session running Claude Opus 5.5 on
// work and its titles, by Claude Haiku 4.5, on side, its newest turn going
// as a request of its own conversation waits for its answer.
var titledOnSide = PageFacts{
	Runs: true, Goes: []Named{{"work", Opus}, {"side", Haiku}}, Going: true, Waiting: true, Accounts: []string{"work", "side"}, Ribbon: []string{"work"},
}

func itsMainFailed() SessionMoment {
	sent, titled, failed, answered := At(14, 42, 0), At(14, 42, 1), At(14, 42, 3), At(14, 42, 5)
	overloaded := withID("r2", asMain(p2, "", line(D28C, Opus, "work", sent, 3*time.Second)))
	overloaded.Status = 529
	m := moment("its main failed, its title answered after, on its account",
		session(D28C, seat(Opus, "work", sent, ""), seat(Haiku, "work", titled, "")))
	m.Stream = live(
		done(failed, 529, toldServing(p2, ledger.ClassMain, told("r2", D28C, Opus, "work", sent, sent))),
		done(answered, 200, toldServing(p2, "auxiliary", told("r3", D28C, Haiku, "work", titled, titled))),
	)
	m.Lines = []ledger.Line{
		withID("r1", asMain(p1, "end_turn", line(D28C, Opus, "work", At(14, 30, 0), time.Minute))),
		overloaded,
		withID("r3", asTitle(p2, line(D28C, Haiku, "work", titled, 4*time.Second))),
	}
	m.Want = map[string]Doing{D28C: alike("work", views.Activity{Since: answered, LastSeen: answered})}
	m.Turns = map[string][]Turn{D28C: {over(2, sent, failed, 2, "work"), over(1, At(14, 30, 0), At(14, 31, 0), 1, "work")}}
	m.Named = map[string]Named{D28C: {"work", Opus}}
	m.Page = map[string]PageFacts{D28C: {Runs: true, Goes: []Named{{"work", Opus}, {"work", Haiku}}, Accounts: []string{"work"}, Ribbon: []string{"work"}}}
	return m
}

// newestTurn is the router listing D28C on work, the request with the given
// id of its newest turn, and its lines: its answer before it, which called a
// tool, sent at 14:40, and, where it has come, the line given.
func newestTurn(name, inFlight string, r views.Told, newest ...ledger.Line) SessionMoment {
	m := moment(name, session(D28C, seat(Opus, "work", r.Sent, inFlight)))
	called := withID("r1", asMain(p1, "tool_use", line(D28C, Opus, "work", At(14, 40, 0), 50*time.Second)))
	m.Stream = live(done(At(14, 40, 50), 200, toldServing(p1, ledger.ClassMain, told("r1", D28C, Opus, "work", called.At, called.At))), r)
	m.Lines = append([]ledger.Line{called}, newest...)
	return m
}

func inFlightInTheNewestTurn() SessionMoment {
	sent := At(14, 41, 0)
	m := newestTurn("a request of the newest turn in flight", status.Asking, toldServing(p1, ledger.ClassMain, told("r2", D28C, Opus, "work", sent, sent)))
	m.Want = map[string]Doing{D28C: alike("work", views.Activity{Doing: views.Asking, Since: sent, LastSeen: sent})}
	m.Turns = map[string][]Turn{D28C: {going(1, At(14, 40, 0), 2, "work")}}
	m.Named = map[string]Named{D28C: {"work", Opus}}
	m.Page = map[string]PageFacts{D28C: runsOn("work", true, true, "work")}
	return m
}

func endedItsLineToCome() SessionMoment {
	sent, answered := At(14, 41, 0), At(14, 41, 30)
	m := newestTurn("its answer ended, its line yet to come", "", done(answered, 200, toldServing(p1, ledger.ClassMain, told("r2", D28C, Opus, "work", sent, sent))))
	m.Want = map[string]Doing{D28C: alike("work", views.Activity{Since: answered, LastSeen: answered})}
	m.Turns = map[string][]Turn{D28C: {going(1, At(14, 40, 0), 2, "work")}}
	m.Named = map[string]Named{D28C: {"work", Opus}}
	m.Page = map[string]PageFacts{D28C: runsOn("work", true, false, "work")}
	return m
}

func itsLineCome() SessionMoment {
	sent, answered := At(14, 41, 0), At(14, 41, 30)
	m := newestTurn("its line come, its turn ended with its answer", "", done(answered, 200, toldServing(p1, ledger.ClassMain, told("r2", D28C, Opus, "work", sent, sent))),
		withID("r2", asMain(p1, "end_turn", line(D28C, Opus, "work", sent, 30*time.Second))))
	m.Want = map[string]Doing{D28C: alike("work", views.Activity{Since: answered, LastSeen: answered})}
	m.Turns = map[string][]Turn{D28C: {over(1, At(14, 40, 0), answered, 2, "work")}}
	m.Named = map[string]Named{D28C: {"work", Opus}}
	m.Page = map[string]PageFacts{D28C: runsOn("work", false, false, "work")}
	return m
}

func movedTwice() SessionMoment {
	last := At(14, 42, 0)
	m := moment("a turn that moved twice, to side and back", session(D28C, seat(Opus, "work", last, "")))
	m.Lines = []ledger.Line{
		asMain(p1, "tool_use", line(D28C, Opus, "work", At(14, 40, 0), 20*time.Second)),
		movedOff("work", asMain(p1, "tool_use", line(D28C, Opus, "side", At(14, 41, 0), 20*time.Second))),
		movedOff("side", asMain(p1, "end_turn", line(D28C, Opus, "work", last, 5*time.Second))),
	}
	m.Want = map[string]Doing{D28C: {
		Whole: views.Activity{Since: At(14, 42, 5), LastSeen: last},
		On:    on("work", views.Activity{Since: last, LastSeen: last}),
	}}
	m.Turns = map[string][]Turn{D28C: {over(1, At(14, 40, 0), At(14, 42, 5), 3, "work", "side", "work")}}
	m.Named = map[string]Named{D28C: {"work", Opus}}
	m.Page = map[string]PageFacts{D28C: {Runs: true, Goes: opusOn("work"), Accounts: []string{"work", "side"}, Ribbon: []string{"work", "side", "work"}, Marks: 2}}
	return m
}

func turnAcrossMidnight() SessionMoment {
	at := func(day, hour, minute int) time.Time { return time.Date(2026, 10, day, hour, minute, 0, 0, time.Local) }
	last := at(2, 0, 2)
	m := momentAt(at(2, 0, 3).Add(7*time.Second), "a turn across midnight", session(D28C, seat(Opus, "work", last, "")))
	m.Lines = []ledger.Line{
		asMain(p1, "tool_use", line(D28C, Opus, "work", at(1, 23, 58), 30*time.Second)),
		asMain(p1, "end_turn", line(D28C, Opus, "work", last, 10*time.Second)),
	}
	m.Want = map[string]Doing{D28C: {
		Whole: views.Activity{Since: last.Add(10 * time.Second), LastSeen: last},
		On:    on("work", views.Activity{Since: last, LastSeen: last}),
	}}
	m.Turns = map[string][]Turn{D28C: {over(1, at(1, 23, 58), last.Add(10*time.Second), 2, "work")}}
	m.Named = map[string]Named{D28C: {"work", Opus}}
	m.Page = map[string]PageFacts{D28C: runsOn("work", false, false, "work")}
	return m
}

func noPrompts() SessionMoment {
	first, last := At(14, 20, 0), At(14, 30, 0)
	m := moment("its lines from before requests named their prompt and class, its two models on two accounts",
		session(D28C, seat(Haiku, "side", last, ""), seat(Opus, "work", first, "")))
	m.Lines = []ledger.Line{
		line(D28C, Opus, "work", first, 10*time.Second),
		line(D28C, Haiku, "side", last, 10*time.Second),
	}
	m.Want = map[string]Doing{D28C: {
		Whole: views.Activity{Since: At(14, 30, 10), LastSeen: last},
		On:    map[string]views.Activity{"work": {Since: first, LastSeen: first}, "side": {Since: last, LastSeen: last}},
	}}
	m.Named = map[string]Named{D28C: {"side", Haiku}}
	m.Page = map[string]PageFacts{D28C: {Runs: true, Goes: []Named{{"side", Haiku}, {"work", Opus}}, Accounts: []string{"work", "side"}, Ribbon: []string{"work", "side"}}}
	return m
}

func newBeforeTheNextLook() SessionMoment {
	sent, titled, answered := At(14, 42, 0), At(14, 42, 1), At(14, 42, 3)
	m := moment("a new session, its first line read before the next look, the router listing another",
		session(C61B, seat(Opus, "side", At(14, 30, 0), "")))
	m.Stream = live(
		toldServing(p1, ledger.ClassMain, told("r1", D28C, Opus, "work", sent, sent)),
		done(answered, 200, toldServing(p1, "auxiliary", told("r2", D28C, Haiku, "work", titled, titled))),
	)
	m.Lines = []ledger.Line{withID("r2", asTitle(p1, line(D28C, Haiku, "work", titled, 2*time.Second)))}
	m.Want = map[string]Doing{
		D28C: {Whole: views.Activity{Doing: views.Asking, Since: sent, LastSeen: answered}},
		C61B: alike("side", views.Activity{Since: At(14, 30, 0), LastSeen: At(14, 30, 0)}),
	}
	m.Turns = map[string][]Turn{D28C: {going(1, sent, 2, "work")}}
	m.Named = map[string]Named{D28C: {"work", Opus}, C61B: {"side", Opus}}
	m.Page = map[string]PageFacts{
		D28C: {Runs: true, Goes: []Named{{"work", Opus}, {"work", Haiku}}, Going: true, Waiting: true, Accounts: []string{"work"}, Ribbon: []string{"work"}},
		C61B: {Runs: true, Goes: opusOn("side")},
	}
	return m
}

func movedByHand() SessionMoment {
	last := At(14, 30, 0)
	moved := seat(Opus, "side", last, "")
	moved.Reason, moved.AssignedAt = status.ReasonMovedByPin, At(14, 40, 0)
	m := moment("moved by hand, with no request since", session(D28C, moved))
	m.Lines = []ledger.Line{asMain(p1, "end_turn", line(D28C, Opus, "work", last, 10*time.Second))}
	m.Want = map[string]Doing{D28C: {
		Whole: views.Activity{Since: At(14, 30, 10), LastSeen: last},
		On:    on("side", views.Activity{Since: last, LastSeen: last}),
	}}
	m.Turns = map[string][]Turn{D28C: {over(1, last, At(14, 30, 10), 1, "work")}}
	m.Named = map[string]Named{D28C: {"side", Opus}}
	m.Page = map[string]PageFacts{D28C: {Runs: true, Goes: opusOn("side"), Accounts: []string{"work"}, Ribbon: []string{"work"}}}
	return m
}

func sideRequestsAloneAsListed() SessionMoment {
	sent, titled := At(14, 42, 0), At(14, 42, 1)
	m := moment("the newest turn of side requests alone, its main in flight as the router lists it, with no stream",
		session(D28C, seat(Opus, "work", sent, status.Asking), seat(Haiku, "side", titled, "")))
	m.Lines = []ledger.Line{
		withID("r1", asMain(p1, "end_turn", line(D28C, Opus, "work", At(14, 30, 0), time.Minute))),
		withID("r3", asTitle(p2, line(D28C, Haiku, "side", titled, 2*time.Second))),
	}
	m.Want = map[string]Doing{D28C: {
		Whole: views.Activity{Doing: views.Asking, Since: sent, LastSeen: titled},
		On: map[string]views.Activity{
			"work": {Doing: views.Asking, Since: sent, LastSeen: sent},
			"side": {Since: titled, LastSeen: titled},
		},
	}}
	m.Turns = map[string][]Turn{D28C: {going(2, titled, 1, "side"), over(1, At(14, 30, 0), At(14, 31, 0), 1, "work")}}
	m.Named = map[string]Named{D28C: {"work", Opus}}
	m.Page = map[string]PageFacts{D28C: titledOnSide}
	return m
}

// quietLines are D28C's two requests of its own conversation on work, at
// 14:20 and 14:33, each answer calling a tool.
func quietLines() []ledger.Line {
	return []ledger.Line{
		asMain(p1, "tool_use", line(D28C, Opus, "work", At(14, 20, 0), 10*time.Second)),
		asMain(p1, "tool_use", line(D28C, Opus, "work", At(14, 33, 0), 10*time.Second)),
	}
}

func idleItsPageOpen() SessionMoment {
	last := At(14, 33, 0)
	m := moment("idle 9 minutes, its page open, its last answer calling a tool", session(D28C, seat(Opus, "work", last, "")))
	m.Lines = quietLines()
	m.Want = map[string]Doing{D28C: {
		Whole: views.Activity{Since: last.Add(10 * time.Second), LastSeen: last},
		On:    on("work", views.Activity{Since: last, LastSeen: last}),
	}}
	m.Turns = map[string][]Turn{D28C: {going(1, At(14, 20, 0), 2, "work")}}
	m.Named = map[string]Named{D28C: {"work", Opus}}
	m.Page = map[string]PageFacts{D28C: runsOn("work", true, false, "work")}
	return m
}

func endedItsPageOpen() SessionMoment {
	last := At(14, 33, 0)
	m := momentAt(last.Add(views.IdleHour+time.Second), "ended an hour after its last line, its page open, the router listing it no longer")
	m.Unlisted = true
	m.Lines = quietLines()
	m.Want = map[string]Doing{D28C: {Whole: views.Activity{Doing: views.Ended, Since: last, LastSeen: last}}}
	m.Turns = map[string][]Turn{D28C: {over(1, At(14, 20, 0), last.Add(10*time.Second), 2, "work")}}
	m.Named = map[string]Named{D28C: {"work", Opus}}
	m.Page = map[string]PageFacts{D28C: {Goes: opusOn("work"), Accounts: []string{"work"}, Ribbon: []string{"work"}}}
	return m
}

func probingWithinTheHour() SessionMoment {
	m := moment("probing within the hour: idle, its models where its lines last sent them")
	m.Unlisted = true
	m.Lines = []ledger.Line{
		asMain(p1, "end_turn", line(D28C, Opus, "work", At(14, 20, 0), 15*time.Second)),
		asTitle(p1, line(D28C, Haiku, "side", At(14, 20, 5), 2*time.Second)),
	}
	m.Want = map[string]Doing{D28C: {Whole: views.Activity{Since: At(14, 20, 7), LastSeen: At(14, 20, 5)}}}
	m.Turns = map[string][]Turn{D28C: {over(1, At(14, 20, 0), At(14, 20, 15), 2, "work")}}
	m.Named = map[string]Named{D28C: {"work", Opus}}
	m.Page = map[string]PageFacts{D28C: {Runs: true, Goes: []Named{{"work", Opus}, {"side", Haiku}}, Accounts: []string{"work", "side"}, Ribbon: []string{"work"}}}
	return m
}

// asSubagent returns l, a request of the subagent with the given id, serving
// the prompt given.
func asSubagent(agent, prompt string, l ledger.Line) ledger.Line {
	l.Prompt, l.Class, l.AgentID, l.Answer.Stop = prompt, "subagent", agent, "end_turn"
	return l
}

func subagentMoved() SessionMoment {
	moved := seat(Haiku, "side", At(14, 35, 0), "")
	moved.Reason, moved.AssignedAt = status.ReasonMovedOff+"work "+status.WhyReserve, At(14, 35, 0)
	m := moment("a subagent's model moved to side, its main thread staying on work",
		session(D28C, seat(Opus, "work", At(14, 40, 0), ""), moved))
	m.Lines = []ledger.Line{
		asMain(p1, "tool_use", line(D28C, Opus, "work", At(14, 30, 0), 10*time.Second)),
		asSubagent("a1", p1, line(D28C, Haiku, "work", At(14, 31, 0), 5*time.Second)),
		movedOff("work", asSubagent("a1", p1, line(D28C, Haiku, "side", At(14, 35, 0), 5*time.Second))),
		asMain(p1, "end_turn", line(D28C, Opus, "work", At(14, 40, 0), 10*time.Second)),
	}
	m.Want = map[string]Doing{D28C: {
		Whole: views.Activity{Since: At(14, 40, 10), LastSeen: At(14, 40, 0)},
		On:    map[string]views.Activity{"work": {Since: At(14, 40, 0), LastSeen: At(14, 40, 0)}, "side": {Since: At(14, 35, 0), LastSeen: At(14, 35, 0)}},
	}}
	m.Turns = map[string][]Turn{D28C: {over(1, At(14, 30, 0), At(14, 40, 10), 4, "work")}}
	m.Named = map[string]Named{D28C: {"work", Opus}}
	m.Page = map[string]PageFacts{D28C: {Runs: true, Goes: []Named{{"work", Opus}, {"side", Haiku}}, Accounts: []string{"work", "side"}, Ribbon: []string{"work"}}}
	return m
}

func inFlightNamingNoPrompt() SessionMoment {
	sent := At(14, 42, 0)
	m := moment("a request in flight naming no prompt, after a named turn", session(D28C, seat(Opus, "work", sent, status.Asking)))
	m.Stream = live(told("r2", D28C, Opus, "work", sent, sent))
	m.Lines = []ledger.Line{withID("r1", asMain(p1, "end_turn", line(D28C, Opus, "work", At(14, 30, 0), time.Minute)))}
	m.Want = map[string]Doing{D28C: alike("work", views.Activity{Doing: views.Asking, Since: sent, LastSeen: sent})}
	m.Turns = map[string][]Turn{D28C: {going(1, At(14, 30, 0), 1, "work")}}
	m.Named = map[string]Named{D28C: {"work", Opus}}
	m.Page = map[string]PageFacts{D28C: runsOn("work", true, true, "work")}
	return m
}

func refusedThenThrottled() SessionMoment {
	sent, refusedAt, throttledAt := At(14, 42, 0), At(14, 42, 2), At(14, 42, 4)
	r := toldServing(p1, ledger.ClassMain, told("r1", D28C, Opus, "side", sent, throttledAt))
	r.Verdict, r.Status = views.VerdictThrottled, 429
	reason := status.ReasonMovedOff + "work " + status.WhyRefused
	moved := seat(Opus, "side", sent, status.Asking)
	moved.Reason, moved.AssignedAt = reason, refusedAt
	m := moment("refused on work, then throttled on side", session(D28C, moved))
	m.Stream = live(r)
	m.Stream.Moves = []views.Moved{{Request: "r1", Session: D28C, Model: Opus, From: "work", To: "side", Reason: reason, At: refusedAt}}
	m.Want = map[string]Doing{D28C: alike("side", views.Activity{Doing: views.Throttled, Status: 429, Since: sent, LastSeen: throttledAt})}
	m.Turns = map[string][]Turn{D28C: {going(1, sent, 1, "side")}}
	m.Named = map[string]Named{D28C: {"side", Opus}}
	m.Page = map[string]PageFacts{D28C: {Runs: true, Goes: opusOn("side"), Going: true, Waiting: true, Ribbon: []string{"side"}, Marks: 1}}
	return m
}

func answeredByTheRouter() SessionMoment {
	last := At(14, 36, 0)
	m := moment("a request of its own conversation the router answered itself, no account having room, then sent again on work",
		session(D28C, seat(Opus, "work", last, "")))
	m.Lines = []ledger.Line{
		asMain(p1, "tool_use", line(D28C, Opus, "work", At(14, 30, 0), 10*time.Second)),
		asMain(p1, "", unsent("r2", D28C, At(14, 35, 0))),
		asMain(p1, "end_turn", line(D28C, Opus, "work", last, 10*time.Second)),
	}
	m.Want = map[string]Doing{D28C: {
		Whole: views.Activity{Since: last.Add(10 * time.Second), LastSeen: last},
		On:    on("work", views.Activity{Since: last, LastSeen: last}),
	}}
	m.Turns = map[string][]Turn{D28C: {over(1, At(14, 30, 0), last.Add(10*time.Second), 3, "work")}}
	m.Named = map[string]Named{D28C: {"work", Opus}}
	m.Page = map[string]PageFacts{D28C: runsOn("work", false, false, "work")}
	return m
}

// titledAfterItsMain is a moment, named, of a session whose main thread is
// Claude Opus 5.5 on work, and its title by Claude Haiku 4.5 on work too,
// after its main request, each put there as it first came, its Opus last
// seen at opusSeen, doing what inFlight says; the stream telling what's
// given. Its lines name it Opus, whatever the stream tells.
func titledAfterItsMain(name string, stream views.Stream, opusSeen time.Time, inFlight string) SessionMoment {
	mainAt, titledAt := At(14, 30, 0), At(14, 31, 30)
	opus := seat(Opus, "work", opusSeen, inFlight)
	opus.AssignedAt = mainAt
	haiku := seat(Haiku, "work", titledAt, "")
	haiku.AssignedAt = titledAt
	m := moment(name, session(D28C, opus, haiku))
	m.Stream = stream
	m.Lines = []ledger.Line{
		withID("r1", asMain(p1, "end_turn", line(D28C, Opus, "work", mainAt, time.Minute))),
		withID("r2", asTitle(p1, line(D28C, Haiku, "work", titledAt, 2*time.Second))),
	}
	m.Named = map[string]Named{D28C: {"work", Opus}}
	return m
}

func titledAfterItsMainUnheard() SessionMoment {
	titledAt := At(14, 31, 30)
	m := titledAfterItsMain("an Opus session's Haiku title on its account after its main, the stream just opened, telling of neither",
		live(), At(14, 30, 0), "")
	m.Want = map[string]Doing{D28C: {
		Whole: views.Activity{Since: titledAt.Add(2 * time.Second), LastSeen: titledAt},
		On:    on("work", views.Activity{Since: titledAt, LastSeen: titledAt}),
	}}
	m.Turns = map[string][]Turn{D28C: {over(1, At(14, 30, 0), At(14, 31, 0), 2, "work")}}
	m.Page = map[string]PageFacts{D28C: {Runs: true, Goes: []Named{{"work", Opus}, {"work", Haiku}}, Accounts: []string{"work"}, Ribbon: []string{"work"}}}
	return m
}

func titledAfterItsMainThenItsMainHeard() SessionMoment {
	sent := At(14, 42, 0)
	m := titledAfterItsMain("an Opus session's Haiku title on its account after its main, then its next main request heard",
		live(toldServing(p2, ledger.ClassMain, told("r3", D28C, Opus, "work", sent, sent))), sent, status.Asking)
	m.Want = map[string]Doing{D28C: alike("work", views.Activity{Doing: views.Asking, Since: sent, LastSeen: sent})}
	m.Turns = map[string][]Turn{D28C: {going(2, sent, 1, "work"), over(1, At(14, 30, 0), At(14, 31, 0), 2, "work")}}
	m.Page = map[string]PageFacts{D28C: {Runs: true, Goes: []Named{{"work", Opus}, {"work", Haiku}}, Going: true, Waiting: true, Accounts: []string{"work"}, Ribbon: []string{"work"}}}
	return m
}
