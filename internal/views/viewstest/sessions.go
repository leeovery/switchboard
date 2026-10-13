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

// SessionMoment is the router's sessions, what the request stream told of
// their requests, and their lines, at a moment the views have to get right,
// and what each session is doing then.
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
	// Lines are the sessions' lines, today's, oldest first.
	Lines []ledger.Line
	// Want is what each session is doing, by its id.
	Want map[string]Doing
}

// Doing is what a session is doing at a moment: as a whole, as the List
// says it; and on each account a model of its runs on, by the account's id,
// as SESSIONS says it, none where it runs on none.
type Doing struct {
	Whole views.Activity
	On    map[string]views.Activity
}

// SessionMoments are the moments the views have to tell what each session
// is doing at: asking, as the stream tells, or as the router listed it, the
// stream joined yet to tell; answering; refused at a limit, or by a 403;
// throttled; idle, its answer's end as the stream told it, or, without the
// stream, as its line gives it; the stream ending with a request in flight,
// joined again with none; the router restarted; the router gone, or
// probing; a session ended, then resumed; a move while it runs; two models
// on two accounts; and idle 59 minutes after an answer that called a tool.
func SessionMoments() []SessionMoment {
	return []SessionMoment{
		asking(), askingAsListed(), answering(), refused(), throttled(), idleAsTheStreamTold(), idleAsItsLineGives(),
		lostAsTheStreamEnded(), routerRestarted(), routerGone(), ended(), resumed(), movedAsItRuns(),
		twoModelsOnTwoAccounts(), idleAfterAToolCall(),
	}
}

// At is the local time on Thursday 1 October 2026, at the clock given, when
// the moments are: the ledger files its lines under local days.
func At(hour, minute, second int) time.Time {
	return time.Date(2026, 10, 1, hour, minute, second, 0, time.Local)
}

// now is when the moments are read: 14:42:07.
var now = At(14, 42, 7)

// doc is the router's document of work and side, new sessions going to side.
func doc() status.Document {
	return status.Document{
		Source: status.SourceRouter, GeneratedAt: now, Best: "side",
		Accounts: []status.Account{{ID: "work"}, {ID: "side"}},
	}
}

// moment is a moment, named, at now, of doc and the sessions given, the
// router listing them.
func moment(name string, sessions ...status.Session) SessionMoment {
	return SessionMoment{Name: name, Doc: doc(), Now: now, Sessions: sessions}
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

// on is what a session does on one account.
func on(account string, a views.Activity) map[string]views.Activity {
	return map[string]views.Activity{account: a}
}

// alike is a session doing a on its one account, and so as a whole.
func alike(account string, a views.Activity) Doing {
	return Doing{Whole: a, On: on(account, a)}
}

func asking() SessionMoment {
	sent := At(14, 42, 5)
	m := moment("asking", session(D28C, seat(Opus, "work", sent, status.Asking)))
	m.Stream = live(told("r1", D28C, Opus, "work", sent, sent))
	m.Want = map[string]Doing{D28C: alike("work", views.Activity{Doing: views.Asking, Since: sent, LastSeen: sent})}
	return m
}

func askingAsListed() SessionMoment {
	sent := At(14, 42, 5)
	m := moment("asking, as the router listed it, the stream joined yet to tell of it", session(D28C, seat(Opus, "work", sent, status.Asking)))
	m.Stream = live()
	m.Want = map[string]Doing{D28C: alike("work", views.Activity{Doing: views.Asking, Since: sent, LastSeen: sent})}
	return m
}

func answering() SessionMoment {
	sent, progress := At(14, 42, 0), At(14, 42, 6)
	r := told("r1", D28C, Opus, "work", sent, progress)
	r.First, r.Chars = At(14, 42, 1), 4800
	m := moment("answering", session(D28C, seat(Opus, "work", sent, status.Answering)))
	m.Stream = live(r)
	m.Want = map[string]Doing{D28C: alike("work", views.Activity{Doing: views.Answering, Chars: 4800, Since: sent, LastSeen: progress})}
	return m
}

func refused() SessionMoment {
	sent, limited, refusedAt := At(14, 42, 5), At(14, 42, 6), At(14, 42, 4)
	atLimit := told("r1", D28C, Opus, "work", sent, limited)
	atLimit.Verdict, atLimit.Status = views.VerdictLimited, 429
	forbidden := told("r2", C61B, Opus, "side", At(14, 42, 3), refusedAt)
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
	return m
}

func throttled() SessionMoment {
	sent, throttledAt := At(14, 42, 0), At(14, 42, 3)
	r := told("r1", D28C, Opus, "work", sent, throttledAt)
	r.Verdict, r.Status = views.VerdictThrottled, 429
	m := moment("throttled", session(D28C, seat(Opus, "work", sent, status.Asking)))
	m.Stream = live(r)
	m.Want = map[string]Doing{D28C: alike("work", views.Activity{Doing: views.Throttled, Status: 429, Since: sent, LastSeen: throttledAt})}
	return m
}

func idleAsTheStreamTold() SessionMoment {
	sent, done := At(14, 41, 20), At(14, 41, 33)
	r := told("r1", D28C, Opus, "work", sent, done)
	r.First, r.Chars, r.Ended, r.Status = At(14, 41, 21), 900, done, 200
	m := moment("idle, its answer's end as the stream told it, the router listing it answering since", session(D28C, seat(Opus, "work", sent, status.Answering)))
	m.Stream = live(r)
	m.Lines = []ledger.Line{line(D28C, Opus, "work", sent, 13*time.Second)}
	m.Want = map[string]Doing{D28C: alike("work", views.Activity{Since: done, LastSeen: done})}
	return m
}

func idleAsItsLineGives() SessionMoment {
	sent, answered := At(14, 41, 20), At(14, 41, 33)
	m := moment("idle, without the stream: since its line's end where it's read, else the router's last_seen", session(D28C, seat(Opus, "work", sent, "")))
	m.Lines = []ledger.Line{line(D28C, Opus, "work", sent, 13*time.Second)}
	m.Want = map[string]Doing{D28C: {
		Whole: views.Activity{Since: answered, LastSeen: sent},
		On:    on("work", views.Activity{Since: sent, LastSeen: sent}),
	}}
	return m
}

func lostAsTheStreamEnded() SessionMoment {
	sent, progress := At(14, 41, 50), At(14, 41, 55)
	r := told("r1", D28C, Opus, "work", sent, progress)
	r.First, r.Chars, r.Lost = At(14, 41, 51), 2000, true
	m := moment("the stream ending with a request in flight, joined again with none in flight",
		session(D28C, seat(Opus, "work", sent, status.Answering)))
	m.Stream = live(r)
	m.Want = map[string]Doing{D28C: alike("work", views.Activity{Since: progress, LastSeen: progress})}
	return m
}

func routerRestarted() SessionMoment {
	lost := told("r1", D28C, Opus, "work", At(14, 41, 50), At(14, 41, 55))
	lost.Lost = true
	gone := told("r2", C61B, Opus, "side", At(14, 41, 52), At(14, 41, 56))
	gone.Lost = true
	again := told("9000", C61B, Opus, "side", At(14, 42, 4), At(14, 42, 4))
	m := moment("the router restarted, the last one's stream forgotten",
		session(D28C, seat(Opus, "work", At(14, 41, 50), "")),
		session(C61B, seat(Opus, "side", At(14, 42, 4), status.Asking)),
	)
	m.Stream = live(lost, gone, again)
	m.Want = map[string]Doing{
		D28C: alike("work", views.Activity{Since: At(14, 41, 55), LastSeen: At(14, 41, 55)}),
		C61B: alike("side", views.Activity{Doing: views.Asking, Since: At(14, 42, 4), LastSeen: At(14, 42, 4)}),
	}
	return m
}

func routerGone() SessionMoment {
	m := moment("the router gone, or probing: drawn from its lines, by the router's own hour")
	m.Unlisted = true
	m.Lines = []ledger.Line{
		line(C61B, Opus, "side", At(13, 30, 0), 20*time.Second),
		line(D28C, Opus, "work", At(14, 20, 0), 15*time.Second),
	}
	m.Want = map[string]Doing{
		D28C: {Whole: views.Activity{Since: At(14, 20, 15), LastSeen: At(14, 20, 0)}},
		C61B: {Whole: views.Activity{Doing: views.Ended, Since: At(13, 30, 0), LastSeen: At(13, 30, 0)}},
	}
	return m
}

func ended() SessionMoment {
	m := moment("ended, an hour after its last request", session(C61B, seat(Opus, "side", At(14, 40, 0), "")))
	m.Lines = []ledger.Line{line(D28C, Opus, "work", At(13, 30, 0), 20*time.Second)}
	m.Want = map[string]Doing{
		D28C: {Whole: views.Activity{Doing: views.Ended, Since: At(13, 30, 0), LastSeen: At(13, 30, 0)}},
		C61B: alike("side", views.Activity{Since: At(14, 40, 0), LastSeen: At(14, 40, 0)}),
	}
	return m
}

func resumed() SessionMoment {
	sent := At(14, 42, 0)
	back := seat(Opus, "side", sent, status.Asking)
	back.Reason, back.AssignedAt = status.Rescored("1h 12m"), sent
	m := moment("resumed, asking again after it ended", session(D28C, back))
	m.Stream = live(told("r1", D28C, Opus, "side", sent, sent))
	m.Lines = []ledger.Line{line(D28C, Opus, "work", At(13, 30, 0), 20*time.Second)}
	m.Want = map[string]Doing{D28C: alike("side", views.Activity{Doing: views.Asking, Since: sent, LastSeen: sent})}
	return m
}

func movedAsItRuns() SessionMoment {
	sent, limited := At(14, 42, 4), At(14, 42, 5)
	r := told("r1", D28C, Opus, "side", sent, limited)
	m := moment("moved to side as it runs, the router listing it on work still",
		session(D28C, seat(Opus, "work", sent, status.Asking)),
		session(C61B, seat(Opus, "work", At(14, 30, 0), "")),
	)
	m.Stream = live(r)
	m.Stream.Moves = []views.Moved{{Request: "r1", Session: D28C, Model: Opus, From: "work", To: "side", Reason: status.ReasonMovedOff + "work " + status.WhyLimit, At: limited}}
	m.Want = map[string]Doing{
		D28C: alike("side", views.Activity{Doing: views.Asking, Since: sent, LastSeen: limited}),
		C61B: alike("work", views.Activity{Since: At(14, 30, 0), LastSeen: At(14, 30, 0)}),
	}
	return m
}

func twoModelsOnTwoAccounts() SessionMoment {
	sent, progress, done := At(14, 42, 0), At(14, 42, 6), At(14, 40, 0)
	opus := told("r1", D28C, Opus, "work", sent, progress)
	opus.First, opus.Chars = At(14, 42, 2), 1200
	haiku := told("r2", D28C, Haiku, "side", At(14, 39, 58), done)
	haiku.Ended, haiku.Status = done, 200
	m := moment("two models on two accounts", session(D28C,
		seat(Opus, "work", sent, status.Answering),
		seat(Haiku, "side", At(14, 39, 58), ""),
	))
	m.Stream = live(opus, haiku)
	answeringOnWork := views.Activity{Doing: views.Answering, Chars: 1200, Since: sent, LastSeen: progress}
	m.Want = map[string]Doing{D28C: {
		Whole: answeringOnWork,
		On:    map[string]views.Activity{"work": answeringOnWork, "side": {Since: done, LastSeen: done}},
	}}
	return m
}

func idleAfterAToolCall() SessionMoment {
	sent := At(13, 42, 50)
	called := line(D28C, Opus, "work", sent, 17*time.Second)
	called.Answer = ledger.Answer{Stop: "tool_use", Tools: []string{"Bash"}}
	m := moment("idle 59 minutes after an answer that called a tool", session(D28C, seat(Opus, "work", sent, "")))
	m.Stream = live()
	m.Lines = []ledger.Line{called}
	m.Want = map[string]Doing{D28C: {
		Whole: views.Activity{Since: At(13, 43, 7), LastSeen: sent},
		On:    on("work", views.Activity{Since: sent, LastSeen: sent}),
	}}
	return m
}
