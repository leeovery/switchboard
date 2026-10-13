package viewstest

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// LogMoment is the request ledger and the request stream at a moment the
// Log's rows have to get right, and the table wanted of them then.
type LogMoment struct {
	Name string
	// Lines are the ledger's lines, every day's, oldest first, each filed
	// under the local day its request arrived on: those of the days the table
	// spans are its own, and every one of a session's tells its story.
	Lines []ledger.Line
	// Sessions are those the router lists, as GET /sessions gives them: none
	// where none does.
	Sessions []status.Session
	// Stream is what the request stream told, as the watch heard it.
	Stream views.Stream
	// Totals are the days' figures, as TotalsByDay gives them.
	Totals []views.DayTotals
	// Filter is what the table shows: the zero Filter every line.
	Filter views.LogFilter
	// Oldest is the oldest day the Log reads back to: none known for the
	// zero OldestDay.
	Oldest views.OldestDay
	// The table spans the local days from From's to Now's, read at Now.
	From, Now time.Time
	// Want are the table's lines wanted, oldest first, and Counts what each
	// day's lines count, by the day's date: a day not named counts none.
	Want   []LogLine
	Counts map[string]Count
	// Then, where it's set, is the ledger and the stream as they were as the
	// table paused, the line at Chosen among its table's chosen: the line
	// chosen is at ChosenNow among the table's lines wanted, and New of its
	// rows have come since.
	Then              *LogMoment
	Chosen, ChosenNow int
	New               int
}

// Count is what a day's lines count at a moment, of what the Log's rules
// decide: those the table shows; the day's lines of each kind, and those
// that failed; and the kinds the heading names as hiding those hidden.
type Count struct {
	Shown, Requests, Checks, Counts, Failed int
	HiddenBy                                []views.RowKind
}

// LogLine is a line of the Log's table as a moment wants it: a request's
// row; where Rule is set, a day's rule, as "2026-09-30 → 2026-10-01", the
// dates of the days it falls between; or, where Oldest is set, the oldest
// day's line, as "2025-08-27, 400 days", its date and how long the ledger
// keeps its lines, or "2026-09-29, for good".
type LogLine struct {
	Row          Row
	Rule, Oldest string
}

// Row is what a request's row says at a moment of what the Log's rules
// decide: its id; its session's id and directory; its time, to the second,
// as the Log shows it; its mark; how far it has come; whether it failed, and
// whether it wrote its session's context again; its first and took, as
// prose.Seconds says them, "" where they aren't known; its stopped, wide
// and on a phone, and its routing, each run together; and its move's cost,
// none for 0.
type Row struct {
	Request, Session, Dir   string
	At                      string
	Mark                    views.RowMark
	Stage                   views.RowStage
	Failed, Rewrote         bool
	First, Took             string
	Stopped, Phone, Routing string
	Cost                    ledger.Picodollars
}

// LogMoments are the moments the Log's rows have to get right, on and
// around Thursday 1 October 2026, and as London's clocks go back an hour on
// Sunday 25 October: a request across midnight, in flight and once its line
// came, the next day's first line since; the stream ending with requests in
// flight, joined again with none; a reader dropped 256 events behind; a line
// arriving for a request shown in flight, its arrival earlier than its
// sending, beside one the router answered itself; a request sent again after
// a limit, a 429 every account refused, one cut off and one given up, and
// several tools; no session, and no directory; a session's first request,
// and its second model's; a side request's move, then its conversation's
// next request, and a move its cache would have run out before anyway; no
// lines today; a day with none between two that have them; the table
// reaching back past the oldest day kept; the router not answering while
// its ledger grows; two requests at 01:30 as the clocks go back; checks and
// counts, hidden and shown; a request answering as the stream closed, drawn
// while it stays closed, its line come meanwhile, and the stream open again;
// one session's lines alone, across midnight; failed requests alone; checks
// shown and counts hidden; a request answering, its details open, then its
// line come, paused; the ledger keeping its lines for good, and 400 days;
// paused through a filter, across midnight, and at the second 01:30 as the
// clocks go back; and the router gone, its ledger last written by a check.
func LogMoments() []LogMoment {
	return []LogMoment{
		inFlightAtMidnight(), acrossMidnightLined(), streamEndedInFlight(), droppedBehind(), linedAfterSent(), sentAgainAndFailed(),
		unnamed(), firstRequests(), movesAndTheirCost(), noLinesToday(), daysBetween(), pastTheOldestDay(), routerNotAnswering(),
		fallingBackRows(), checksAndCounts(false), checksAndCounts(true),
		streamClosed(closedInFlight), streamClosed(closedLined), streamClosed(reopenedInFlight),
		sessionAcrossMidnight(), failedOnly(), checksShownCountsHidden(), inFlightDetails(false), inFlightDetails(true),
		keptForGood(), keptFourHundredDays(), pausedThroughAFilter(), pausedAcrossMidnight(), pausedAtTheSecondOneThirty(), notLive(),
	}
}

// The directories the Log's sessions run in, by their ids.
var dirs = map[string]string{D28C: "~/Code/switchboard", C61B: "~/Code/mobile"}

// Usages the Log's requests' answers give: a turn's, reading its context
// from the cache, worth a little; and rewriteUsage, writing a million
// tokens to it for an hour, worth $8 of Claude Opus 5.5, as a move's cost
// is where its request wrote it.
const (
	turnUsage    = `{"input_tokens":3,"cache_read_input_tokens":180000,"cache_creation_input_tokens":2000,"cache_creation":{"ephemeral_1h_input_tokens":2000},"output_tokens":900}`
	rewriteUsage = `{"input_tokens":3,"cache_creation_input_tokens":1000000,"cache_creation":{"ephemeral_1h_input_tokens":1000000},"output_tokens":1200}`
	rewriteCost  = 8 * dollar
)

// onDay is the local time on the given day of October 2026, the day before
// the first the 30th of September, at the clock given.
func onDay(day, hour, minute, second int) time.Time {
	return time.Date(2026, 10, day, hour, minute, second, 0, time.Local)
}

// requested is a request of the session's own conversation, of the model
// given, as its line gives it: its id given; in the directory its session
// runs in; Claude Code's, asking for two betas, in a turn's shape; routed to
// account for reason; arriving at at, its answer's first byte passing on
// 0.8s later and its end 10s after it arrived; its answer, msg_ and its id,
// thinking and ending its turn, giving turnUsage, and the account's windows
// as turnLimits gives them.
func requested(id, session, model, account, reason string, at time.Time) ledger.Line {
	return ledger.Line{
		At: at, Request: id, Kind: ledger.KindMessage, Session: session, Dir: dirs[session], Model: model, Account: account, Reason: reason,
		Status: 200, Attempts: 1, FirstMS: new(int64(800)), TotalMS: 10_000, Class: ledger.ClassMain,
		Agent: "claude-cli/2.1.0 (external, cli)", Betas: []string{"context-1m-2025-08-07", "interleaved-thinking-2025-05-14"},
		Shape: ledger.Shape{
			Bytes: 482_000, Messages: 12, System: 3, Tools: 9, MaxTokens: new(int64(32_000)), Stream: new(true),
			Thinking: ledger.Thinking{Type: "enabled", BudgetTokens: new(int64(16_000))}, OutputConfig: ledger.OutputConfig{Effort: "high"},
		},
		Answer: ledger.Answer{ID: "msg_" + id, Model: model, Stop: "end_turn", Blocks: map[string]int{"text": 1, "thinking": 2}},
		Usage:  json.RawMessage(turnUsage), Limits: turnLimits(),
	}
}

// turnLimits are an answer's usage headers, as a line keeps them: its
// 5-hour window 42% used, resetting at 17:10, and its week 18%, each
// allowed, as the answer is.
func turnLimits() map[string]string {
	return map[string]string{
		"5h-utilization": "0.42", "5h-reset": strconv.FormatInt(At(17, 10, 0).Unix(), 10), "5h-status": "allowed",
		"7d-utilization": "0.18", "7d-reset": strconv.FormatInt(onDay(5, 9, 0, 0).Unix(), 10), "7d-status": "allowed",
		"status": "allowed",
	}
}

// sentAgain returns l, its request sent again after each account tried
// couldn't serve it.
func sentAgain(l ledger.Line, tried ...ledger.Tried) ledger.Line {
	l.Tried, l.Attempts = tried, len(tried)+1
	return l
}

// failing returns l, its request answered with the status given, its
// answer telling nothing of itself, nor giving usage or windows.
func failing(status int, l ledger.Line) ledger.Line {
	l.Status, l.Answer, l.Usage, l.Limits = status, ledger.Answer{}, nil, nil
	return l
}

// calling returns l, its answer stopping to call the tools given.
func calling(l ledger.Line, tools ...string) ledger.Line {
	l.Answer.Stop, l.Answer.Tools = "tool_use", tools
	return l
}

// rewriting returns l, its answer giving rewriteUsage.
func rewriting(l ledger.Line) ledger.Line {
	l.Usage = json.RawMessage(rewriteUsage)
	return l
}

// classed returns l, a request of the class given.
func classed(class string, l ledger.Line) ledger.Line {
	l.Class = class
	return l
}

// kinded returns l, a request of the kind given.
func kinded(kind string, l ledger.Line) ledger.Line {
	l.Kind = kind
	return l
}

// clock is t as the Log shows a time, to the second, in its own time zone.
func clock(t time.Time) string {
	return prose.ToTheSecond(t, t)
}

// seconds is n milliseconds as prose.Seconds says them.
func seconds(n int64) string {
	return prose.Seconds(time.Duration(n) * time.Millisecond)
}

// rowOf is the row wanted of the request l, as its line alone tells it: no
// mark, nothing moved, its stopped its answer's.
func rowOf(l ledger.Line) Row {
	r := Row{Request: l.Request, Session: l.Session, Dir: l.Dir, At: clock(l.At), Took: seconds(l.TotalMS), Stopped: l.Answer.Stop, Phone: l.Answer.Stop}
	if l.FirstMS != nil {
		r.First = seconds(*l.FirstMS)
	}
	return r
}

// first is the row wanted of a session's first request, l: ▲, new session.
func first(l ledger.Line) Row {
	r := rowOf(l)
	r.Mark, r.Phone, r.Routing = views.MarkNew, "new session", "new session"
	return r
}

// rows are the table's lines wanted of rows.
func rows(r ...Row) []LogLine {
	lines := make([]LogLine, len(r))
	for i, row := range r {
		lines[i] = LogLine{Row: row}
	}
	return lines
}

// rule is the rule wanted between the days with the given dates.
func rule(ended, began string) LogLine {
	return LogLine{Rule: ended + " → " + began}
}

// shown counts a day's n requests, every one shown, none failed.
func shown(n int) Count {
	return Count{Shown: n, Requests: n}
}

// shownFailed counts a day's n requests, every one shown, those given
// failed.
func shownFailed(n, failed int) Count {
	return Count{Shown: n, Requests: n, Failed: failed}
}

// listed is the session with the given id as the router lists it, its
// Claude Opus 5.5 on side, last seen at seen, doing what its request in
// flight does.
func listed(id string, seen time.Time, inFlight string) status.Session {
	return session(id, status.Assignment{Model: Opus, Account: side, Reason: status.ReasonSticky, LastSeen: seen, InFlight: inFlight})
}

func inFlightAtMidnight() LogMoment {
	r1 := requested("r1", D28C, Opus, side, status.ReasonNew, onDay(0, 23, 40, 0))
	sent := onDay(0, 23, 59, 50)
	r2 := told("r2", D28C, Opus, side, sent, onDay(1, 0, 0, 4))
	r2.Dir, r2.First, r2.Chars = dirs[D28C], sent.Add(time.Second), 1200
	answering := Row{
		Request: "r2", Session: D28C, Dir: dirs[D28C], At: "23:59:50", Mark: views.MarkAnswering, Stage: views.RowAnswering,
		First: "1.0s", Took: "15.0s", Stopped: "answering · 1.2k characters", Phone: "answering",
	}
	return LogMoment{
		Name:  "a request arriving 23:59:50, in flight at midnight: on the day it was sent, no rule till the next day's first line",
		Lines: []ledger.Line{r1}, Sessions: []status.Session{listed(D28C, sent, status.Answering)}, Stream: live(r2),
		From: onDay(0, 0, 0, 0), Now: onDay(1, 0, 0, 5),
		Want:   rows(first(r1), answering),
		Counts: map[string]Count{"2026-09-30": shown(2)},
	}
}

func acrossMidnightLined() LogMoment {
	r1 := requested("r1", D28C, Opus, side, status.ReasonNew, onDay(0, 23, 40, 0))
	sent := onDay(0, 23, 59, 50)
	r2 := requested("r2", D28C, Opus, side, status.ReasonSticky, sent.Add(-100*time.Millisecond))
	r2.TotalMS = 22_100
	ended := told("r2", D28C, Opus, side, sent, onDay(1, 0, 0, 12))
	ended.Ended, ended.Status = onDay(1, 0, 0, 12), 200
	r3 := requested("r3", C61B, Opus, side, status.ReasonNew, onDay(1, 0, 1, 0))
	return LogMoment{
		Name:  "a request arriving 23:59:50 and ending 00:00:12, its line come, then the next day's first: above the rule, counted on the day it arrived",
		Lines: []ledger.Line{r1, r2, r3}, Stream: live(ended),
		Totals: []views.DayTotals{{Date: "2026-09-30", Requests: 2, Sessions: 1}},
		From:   onDay(0, 0, 0, 0), Now: onDay(1, 0, 1, 30),
		Want:   []LogLine{{Row: first(r1)}, {Row: rowOf(r2)}, rule("2026-09-30", "2026-10-01"), {Row: first(r3)}},
		Counts: map[string]Count{"2026-09-30": shown(2), "2026-10-01": shown(1)},
	}
}

func streamEndedInFlight() LogMoment {
	r0 := requested("r0", D28C, Opus, side, status.ReasonNew, At(14, 0, 0))
	lost := told("r2", D28C, Opus, side, At(14, 41, 50), At(14, 41, 55))
	lost.First, lost.Chars, lost.Lost = At(14, 41, 51), 2000, true
	cut := told("r3", D28C, Opus, side, At(14, 41, 52), At(14, 41, 55))
	cut.Lost = true
	cutOff := failing(0, requested("r3", D28C, Opus, side, status.ReasonSticky, At(14, 41, 52).Add(-100*time.Millisecond)))
	cutOff.CutOff, cutOff.FirstMS, cutOff.TotalMS = true, nil, 3000
	asking := told("r4", D28C, Opus, side, At(14, 42, 2), At(14, 42, 2))
	cutRow := rowOf(cutOff)
	cutRow.Mark, cutRow.Failed = views.MarkFailed, true
	return LogMoment{
		Name:  "the stream ending with requests in flight and no done, joined again with none in flight: one lost, failed, one filled in by its line",
		Lines: []ledger.Line{r0, cutOff}, Sessions: []status.Session{listed(D28C, At(14, 42, 2), status.Asking)}, Stream: live(lost, cut, asking),
		From: now, Now: now,
		Want: rows(first(r0),
			Row{Request: "r2", Session: D28C, At: "14:41:50", Mark: views.MarkFailed, Stage: views.RowLost, Failed: true,
				Stopped: lostWords, Phone: lostWords},
			cutRow,
			Row{Request: "r4", Session: D28C, At: "14:42:02", Mark: views.MarkAsking, Stage: views.RowAsking, Took: "5.0s", Stopped: "asking", Phone: "asking"},
		),
		Counts: map[string]Count{"2026-10-01": shownFailed(4, 2)},
	}
}

func droppedBehind() LogMoment {
	r1 := requested("r1", D28C, Opus, side, status.ReasonNew, At(14, 40, 0).Add(-100*time.Millisecond))
	toldR1 := told("r1", D28C, Opus, side, At(14, 40, 0), At(14, 40, 10))
	toldR1.Ended, toldR1.Status = At(14, 40, 10), 200
	// r2 and r3 came and went while the reader was dropped, untold.
	r2 := requested("r2", C61B, Opus, side, status.ReasonNew, At(14, 40, 20))
	r3 := requested("r3", D28C, Opus, side, status.ReasonSticky, At(14, 40, 30))
	// r5 was in flight as the reader was dropped, and ended before it joined
	// again: lost, as the stream's in flight no longer listed it, its line
	// filling it in.
	r5 := requested("r5", C61B, Opus, side, status.ReasonSticky, At(14, 40, 40).Add(-100*time.Millisecond))
	toldR5 := told("r5", C61B, Opus, side, At(14, 40, 40), At(14, 40, 41))
	toldR5.Lost = true
	asking := told("r4", D28C, Opus, side, At(14, 42, 0), At(14, 42, 0))
	asking.Dir = dirs[D28C]
	return LogMoment{
		Name:  "a reader dropped 256 events behind, joined again: the gap filled from the ledger, each request once",
		Lines: []ledger.Line{r1, r2, r3, r5}, Sessions: []status.Session{listed(D28C, At(14, 42, 0), status.Asking)}, Stream: live(toldR1, toldR5, asking),
		From: now, Now: now,
		Want: rows(first(r1), first(r2), rowOf(r3), rowOf(r5),
			Row{Request: "r4", Session: D28C, Dir: dirs[D28C], At: "14:42:00", Mark: views.MarkAsking, Stage: views.RowAsking, Took: "7.0s", Stopped: "asking", Phone: "asking"},
		),
		Counts: map[string]Count{"2026-10-01": shown(5)},
	}
}

func linedAfterSent() LogMoment {
	r0 := requested("r0", D28C, Opus, side, status.ReasonNew, At(14, 30, 0))
	sent := At(14, 42, 0).Add(100 * time.Millisecond)
	r1 := requested("r1", D28C, Opus, side, status.ReasonSticky, sent.Add(-200*time.Millisecond))
	toldR1 := told("r1", D28C, Opus, side, sent, At(14, 42, 5))
	toldR1.Ended, toldR1.Status = At(14, 42, 5), 200
	// The router answered r2 itself, no account having room: never on the
	// stream, and with no first byte.
	r2 := failing(429, requested("r2", D28C, Opus, "", "no account has room", At(14, 42, 0)))
	r2.Attempts, r2.FirstMS, r2.TotalMS = 0, nil, 2
	unsent := rowOf(r2)
	unsent.Mark, unsent.Failed = views.MarkFailed, true
	then := LogMoment{Lines: []ledger.Line{r0, r2}, Stream: live(told("r1", D28C, Opus, side, sent, At(14, 42, 1))), From: now, Now: At(14, 42, 1)}
	return LogMoment{
		Name: "a line come for a request shown in flight, its arrival earlier than its sending, paused at it: where it was sent, " +
			"after one the router answered itself, chosen still, not new",
		Lines: []ledger.Line{r0, r1, r2}, Stream: live(toldR1),
		From: now, Now: now,
		Want:   rows(first(r0), unsent, rowOf(r1)),
		Counts: map[string]Count{"2026-10-01": shownFailed(3, 1)},
		Then:   &then, Chosen: 2, ChosenNow: 2,
	}
}

func sentAgainAndFailed() LogMoment {
	atLimit := func(account string) ledger.Tried { return ledger.Tried{Account: account, Why: status.WhyLimit} }
	r0 := requested("r0", D28C, Opus, work, status.ReasonNew, At(14, 30, 0))
	r1 := calling(rewriting(sentAgain(movedOff(work, requested("r1", D28C, Opus, side, status.ReasonMovedOff+"work "+status.WhyLimit, At(14, 35, 0))),
		atLimit(work))), "Bash", "Read")
	c0 := requested("c0", C61B, Opus, side, status.ReasonNew, At(14, 20, 0))
	c1 := sentAgain(failing(429, requested("c1", C61B, Opus, side, status.ReasonSticky, At(14, 36, 0))), atLimit(side), atLimit(work))
	c2 := failing(0, requested("c2", C61B, Opus, side, status.ReasonSticky, At(14, 37, 0)))
	c2.CutOff, c2.FirstMS = true, nil
	c3 := failing(0, requested("c3", C61B, Opus, side, status.ReasonSticky, At(14, 38, 0)))
	c3.Canceled = true
	retried := rowOf(r1)
	retried.Mark, retried.Rewrote, retried.Cost = views.MarkMoved, true, rewriteCost
	retried.Stopped, retried.Phone, retried.Routing = "tool_use · Bash +1", "from work", "after work's limit"
	refused, cutOff, givenUp := rowOf(c1), rowOf(c2), rowOf(c3)
	refused.Routing, refused.Phone = "after side's limit, work's limit", "after side's limit, work's limit"
	for _, r := range []*Row{&refused, &cutOff, &givenUp} {
		r.Mark, r.Failed = views.MarkFailed, true
	}
	return LogMoment{
		Name:  "a request sent again after work's limit, calling several tools; a 429 every account refused, passed on; one cut off; one given up",
		Lines: []ledger.Line{c0, r0, c1, c2, c3, r1}, Sessions: []status.Session{listed(D28C, At(14, 35, 10), "")},
		From: now, Now: now,
		Want:   rows(first(c0), first(r0), retried, refused, cutOff, givenUp),
		Counts: map[string]Count{"2026-10-01": shownFailed(6, 3)},
	}
}

func unnamed() LogMoment {
	noSession := requested("r1", "", Opus, work, "unsessioned", At(14, 40, 0))
	noDir := requested("r2", C61B, Opus, side, status.ReasonNew, At(14, 41, 0))
	noDir.Dir = ""
	return LogMoment{
		Name:  "a request of no session, and one of a session that names no directory",
		Lines: []ledger.Line{noSession, noDir},
		From:  now, Now: now,
		Want:   rows(rowOf(noSession), first(noDir)),
		Counts: map[string]Count{"2026-10-01": shown(2)},
	}
}

func firstRequests() LogMoment {
	c0 := calling(requested("c0", C61B, Opus, side, status.ReasonNew, At(14, 41, 11)), "Bash")
	// The router routes a session's models apart, so its haiku's first is
	// new to it, but no session's first.
	c1 := classed("auxiliary", requested("c1", C61B, Haiku, side, status.ReasonNew, At(14, 41, 38)))
	opening := first(c0)
	opening.Stopped = "tool_use · Bash"
	return LogMoment{
		Name:  "a session's first request, and its second model's first",
		Lines: []ledger.Line{c0, c1},
		From:  now, Now: now,
		Want:   rows(opening, rowOf(c1)),
		Counts: map[string]Count{"2026-10-01": shown(2)},
	}
}

func movesAndTheirCost() LogMoment {
	d0 := requested("d0", D28C, Opus, work, status.ReasonNew, At(14, 0, 0))
	// A side request of the conversation's model moves it; the
	// conversation's next request, writing its context again on side,
	// settles what that cost.
	d1 := classed("auxiliary", movedOff(work, requested("d1", D28C, Opus, side, status.ReasonMovedOff+"work "+status.WhyReserve, At(14, 30, 0))))
	d2 := rewriting(requested("d2", D28C, Opus, side, status.ReasonSticky, At(14, 31, 0)))
	// c61b moves two hours after its last request, its cache cold an hour
	// since.
	c0 := requested("c0", C61B, Opus, work, status.ReasonNew, At(12, 0, 0))
	c1 := rewriting(movedOff(work, requested("c1", C61B, Opus, side, status.ReasonMovedOff+"work "+status.WhyReserve, At(14, 0, 0))))
	sideMove, rewrote, coldMove := rowOf(d1), rowOf(d2), rowOf(c1)
	sideMove.Mark, sideMove.Phone, sideMove.Routing, sideMove.Cost = views.MarkMoved, "from work", "from work, at its cap", rewriteCost
	rewrote.Rewrote = true
	coldMove.Mark, coldMove.Phone, coldMove.Routing = views.MarkMoved, "from work", "from work, at its cap"
	return LogMoment{
		Name:     "a side request's move, then its conversation's next request; and a move after its cache ran out",
		Lines:    []ledger.Line{c0, d0, c1, d1, d2},
		Sessions: []status.Session{listed(D28C, At(14, 31, 10), ""), listed(C61B, At(14, 0, 10), "")},
		From:     now, Now: now,
		Want:   rows(first(c0), first(d0), coldMove, sideMove, rewrote),
		Counts: map[string]Count{"2026-10-01": shown(5)},
	}
}

func noLinesToday() LogMoment {
	return LogMoment{Name: "no lines today", From: now, Now: now}
}

func daysBetween() LogMoment {
	r1 := requested("r1", D28C, Opus, work, status.ReasonNew, onDay(-1, 18, 0, 0))
	r2 := requested("r2", D28C, Opus, work, status.ReasonSticky, At(9, 0, 0))
	return LogMoment{
		Name:  "a day with no lines between two that have them: one rule across it, of the day that ended",
		Lines: []ledger.Line{r1, r2}, Totals: []views.DayTotals{{Date: "2026-09-29", Requests: 1, Sessions: 1}},
		From: onDay(-1, 0, 0, 0), Now: now,
		Want:   []LogLine{{Row: first(r1)}, rule("2026-09-29", "2026-10-01"), {Row: rowOf(r2)}},
		Counts: map[string]Count{"2026-09-29": shown(1), "2026-10-01": shown(1)},
	}
}

func pastTheOldestDay() LogMoment {
	r1 := requested("r1", D28C, Opus, work, status.ReasonNew, onDay(0, 9, 0, 0))
	r2 := requested("r2", D28C, Opus, work, status.ReasonSticky, At(9, 0, 0))
	return LogMoment{
		Name:  "the table reaching back past the oldest day the ledger keeps: no rule above its first line",
		Lines: []ledger.Line{r1, r2}, Totals: []views.DayTotals{{Date: "2026-09-30", Requests: 1, Sessions: 1}},
		From: onDay(-5, 0, 0, 0), Now: now,
		Want:   []LogLine{{Row: first(r1)}, rule("2026-09-30", "2026-10-01"), {Row: rowOf(r2)}},
		Counts: map[string]Count{"2026-09-30": shown(1), "2026-10-01": shown(1)},
	}
}

func routerNotAnswering() LogMoment {
	r0 := requested("r0", D28C, Opus, side, status.ReasonNew, At(14, 0, 0))
	// r1 and r2 were in flight as the stream ended; r1's line has come since.
	r1 := requested("r1", D28C, Opus, side, status.ReasonSticky, At(14, 41, 0).Add(-100*time.Millisecond))
	toldR1 := told("r1", D28C, Opus, side, At(14, 41, 0), At(14, 41, 2))
	toldR1.Lost = true
	toldR2 := told("r2", C61B, Opus, side, At(14, 41, 5), At(14, 41, 6))
	toldR2.Lost = true
	// r3 came after the stream ended, untold.
	r3 := requested("r3", D28C, Opus, side, status.ReasonSticky, At(14, 41, 30))
	stream := live(toldR1, toldR2)
	stream.Live = false
	return LogMoment{
		Name:  "the router not answering while its ledger grows: its lines fill the table, nothing in flight",
		Lines: []ledger.Line{r0, r1, r3}, Stream: stream,
		From: now, Now: now,
		Want: rows(first(r0), rowOf(r1),
			Row{Request: "r2", Session: C61B, At: "14:41:05", Mark: views.MarkFailed, Stage: views.RowLost, Failed: true,
				Stopped: lostWords, Phone: lostWords},
			rowOf(r3)),
		Counts: map[string]Count{"2026-10-01": shownFailed(4, 1)},
	}
}

func fallingBackRows() LogMoment {
	early := time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC).Local()
	late := early.Add(time.Hour)
	r1 := requested("r1", D28C, Opus, side, status.ReasonNew, early)
	r2 := requested("r2", D28C, Opus, side, status.ReasonSticky, late)
	return LogMoment{
		Name:  "two requests at 01:30 as the clocks go back: one day, in the order they came",
		Lines: []ledger.Line{r2, r1},
		From:  early, Now: late.Add(time.Hour),
		Want:   rows(first(r1), rowOf(r2)),
		Counts: map[string]Count{dayfile.DateOf(early): shown(2)},
	}
}

func checksAndCounts(show bool) LogMoment {
	r1 := requested("r1", D28C, Opus, side, status.ReasonNew, At(14, 40, 0))
	k1 := kinded(ledger.KindCheck, requested("k1", D28C, Haiku, side, status.ReasonNew, At(14, 40, 10)))
	n1 := kinded(ledger.KindCount, requested("n1", D28C, Opus, side, status.ReasonSticky, At(14, 40, 20)))
	m := LogMoment{
		Name:  "Claude Code's quota checks and its counts of tokens, hidden until shown",
		Lines: []ledger.Line{r1, k1, n1}, Filter: views.LogOpening(),
		From: now, Now: now,
		Want: rows(first(r1)),
		Counts: map[string]Count{"2026-10-01": {
			Shown: 1, Requests: 1, Checks: 1, Counts: 1, HiddenBy: []views.RowKind{views.RowCheck, views.RowCount},
		}},
	}
	if !show {
		return m
	}
	m.Name, m.Filter = "Claude Code's quota checks and its counts of tokens, shown", views.LogFilter{}
	m.Want, m.Counts = rows(first(r1), hiddenKind(k1), hiddenKind(n1)), map[string]Count{"2026-10-01": {Shown: 3, Requests: 1, Checks: 1, Counts: 1}}
	return m
}

// hiddenKind is the row wanted of l, a check or a count, shown: ·, stopped
// blank.
func hiddenKind(l ledger.Line) Row {
	r := rowOf(l)
	r.Mark, r.Stopped, r.Phone = views.MarkHidden, "", ""
	return r
}

func sessionAcrossMidnight() LogMoment {
	r1 := requested("r1", D28C, Opus, side, status.ReasonNew, onDay(0, 23, 50, 0))
	c1 := requested("c1", C61B, Opus, side, status.ReasonNew, onDay(0, 23, 55, 0))
	c2 := requested("c2", C61B, Opus, side, status.ReasonSticky, onDay(1, 0, 5, 0))
	r2 := requested("r2", D28C, Opus, side, status.ReasonSticky, onDay(1, 0, 10, 0))
	return LogMoment{
		Name:  "d28c's lines alone, across midnight: both days' rows, the rule, today's counting today's alone",
		Lines: []ledger.Line{r1, c1, c2, r2}, Totals: []views.DayTotals{{Date: "2026-09-30", Requests: 2, Sessions: 2}},
		Filter: views.LogFilter{Sessions: []string{D28C}},
		From:   onDay(0, 0, 0, 0), Now: onDay(1, 0, 20, 0),
		Want:   []LogLine{{Row: first(r1)}, rule("2026-09-30", "2026-10-01"), {Row: rowOf(r2)}},
		Counts: map[string]Count{"2026-09-30": {Shown: 1, Requests: 2}, "2026-10-01": {Shown: 1, Requests: 2}},
	}
}

func failedOnly() LogMoment {
	atLimit := func(account string) ledger.Tried { return ledger.Tried{Account: account, Why: status.WhyLimit} }
	r0 := requested("r0", D28C, Opus, side, status.ReasonNew, At(14, 30, 0))
	c0 := requested("c0", C61B, Opus, side, status.ReasonNew, At(14, 31, 0))
	refused := sentAgain(failing(429, requested("c1", C61B, Opus, side, status.ReasonSticky, At(14, 36, 0))), atLimit(side), atLimit(work))
	cutOff := failing(0, requested("c2", C61B, Opus, side, status.ReasonSticky, At(14, 37, 0)))
	cutOff.CutOff, cutOff.FirstMS = true, nil
	givenUp := failing(0, requested("c3", C61B, Opus, side, status.ReasonSticky, At(14, 38, 0)))
	givenUp.Canceled = true
	broke := failing(500, requested("c4", C61B, Opus, side, status.ReasonSticky, At(14, 39, 0)))
	r5 := requested("r5", D28C, Opus, side, status.ReasonSticky, At(14, 40, 0))
	lost := told("r9", D28C, Opus, side, At(14, 41, 50), At(14, 41, 55))
	lost.Lost = true
	wanted := []Row{rowOf(refused), rowOf(cutOff), rowOf(givenUp), rowOf(broke)}
	wanted[0].Routing, wanted[0].Phone = "after side's limit, work's limit", "after side's limit, work's limit"
	for i := range wanted {
		wanted[i].Mark, wanted[i].Failed = views.MarkFailed, true
	}
	wanted = append(wanted, Row{Request: "r9", Session: D28C, At: "14:41:50", Mark: views.MarkFailed, Stage: views.RowLost, Failed: true,
		Stopped: lostWords, Phone: lostWords})
	return LogMoment{
		Name:  "failed only: a 429 every account refused, passed on; one cut off; one given up; a 500; one lost",
		Lines: []ledger.Line{r0, c0, refused, cutOff, givenUp, broke, r5}, Stream: live(lost),
		Filter: views.LogFilter{Kinds: views.LogOpening().Kinds, Failed: true},
		From:   now, Now: now,
		Want:   rows(wanted...),
		Counts: map[string]Count{"2026-10-01": {Shown: 5, Requests: 8, Failed: 5}},
	}
}

// lostWords say a request was lost, as its row's stopped says it.
const lostWords = "no answer: the router went away"

func checksShownCountsHidden() LogMoment {
	r1 := requested("r1", D28C, Opus, side, status.ReasonNew, At(14, 40, 0))
	k1 := kinded(ledger.KindCheck, requested("k1", D28C, Haiku, side, status.ReasonNew, At(14, 40, 10)))
	lines := []ledger.Line{r1, k1}
	for i, id := range []string{"n1", "n2", "n3", "n4"} {
		lines = append(lines, kinded(ledger.KindCount, requested(id, D28C, Opus, side, status.ReasonSticky, At(14, 40, 20+i))))
	}
	return LogMoment{
		Name:   "checks shown, counts hidden: four hidden, by counts",
		Lines:  lines,
		Filter: views.LogFilter{Kinds: []views.RowKind{views.RowRequest, views.RowCheck}},
		From:   now, Now: now,
		Want:   rows(first(r1), hiddenKind(k1)),
		Counts: map[string]Count{"2026-10-01": {Shown: 2, Requests: 1, Checks: 1, Counts: 4, HiddenBy: []views.RowKind{views.RowCount}}},
	}
}

// closing is how far a request answering as the stream closed has come.
type closing int

const (
	// closedInFlight is the stream closed still, the request's line to come.
	closedInFlight closing = iota
	// closedLined is the stream closed still, the request's line come.
	closedLined
	// reopenedInFlight is the stream open again, telling of the request in
	// flight still.
	reopenedInFlight
)

// streamClosed is a request of d28c's answering as the stream closed, as
// the watch closes it while no page it moves shows, keeping what it told,
// and the Log drawn as far as c has come. While the stream's closed, what of
// the request is in flight is unknown: the table holds nothing in flight, as
// it does without the router, and the request's line, come, sits where it
// was sent. Open again, the stream tells of it in flight still.
func streamClosed(c closing) LogMoment {
	r0 := requested("r0", D28C, Opus, side, status.ReasonNew, At(14, 30, 0))
	sent := At(14, 41, 30)
	// c0 arrived after r1 did, but before r1 went out.
	c0 := requested("c0", C61B, Opus, side, status.ReasonNew, sent.Add(-50*time.Millisecond))
	r1 := told("r1", D28C, Opus, side, sent, At(14, 41, 40))
	r1.Dir, r1.First, r1.Chars = dirs[D28C], sent.Add(time.Second), 1200
	stream := live(r1)
	stream.Live = false
	m := LogMoment{
		Name:  "a request answering as the stream closed, drawn while it stays closed: nothing in flight, as without the router",
		Lines: []ledger.Line{r0, c0}, Sessions: []status.Session{listed(D28C, At(14, 41, 40), status.Answering)}, Stream: stream,
		From: now, Now: now,
		Want:   rows(first(r0), first(c0)),
		Counts: map[string]Count{"2026-10-01": shown(2)},
	}
	switch c {
	case closedInFlight:
	case closedLined:
		lined := requested("r1", D28C, Opus, side, status.ReasonSticky, sent.Add(-100*time.Millisecond))
		lined.TotalMS = 20_000
		m.Name = "a request answering as the stream closed, its line come while it stays closed: where it was sent, after one arriving after it"
		m.Lines = append(m.Lines, lined)
		m.Want, m.Counts = rows(first(r0), first(c0), rowOf(lined)), map[string]Count{"2026-10-01": shown(3)}
	case reopenedInFlight:
		r1.Chars, r1.Heard = 3400, At(14, 42, 5)
		m.Name = "a request answering as the stream closed, the stream open again telling of it in flight still: answering, where it was sent"
		m.Stream = live(r1)
		m.Want = rows(first(r0), first(c0), Row{
			Request: "r1", Session: D28C, Dir: dirs[D28C], At: "14:41:30", Mark: views.MarkAnswering, Stage: views.RowAnswering,
			First: "1.0s", Took: "37.0s", Stopped: "answering · 3.4k characters", Phone: "answering",
		})
		m.Counts = map[string]Count{"2026-10-01": shown(3)}
	}
	return m
}

// inFlightDetails is a request of d28c's answering, its details open, and,
// where lined, its line come while the stream's open: in flight, its details
// hold what the stream told alone; lined, the rest, its row where it sat.
func inFlightDetails(lined bool) LogMoment {
	r0 := requested("r0", D28C, Opus, side, status.ReasonNew, At(14, 30, 0))
	sent := At(14, 41, 50)
	r1 := told("r1", D28C, Opus, side, sent, At(14, 42, 5))
	r1.Dir, r1.First, r1.Chars = dirs[D28C], sent.Add(time.Second), 5100
	m := LogMoment{
		Name:  "a request answering, its details open: what the stream told alone",
		Lines: []ledger.Line{r0}, Sessions: []status.Session{listed(D28C, At(14, 42, 5), status.Answering)}, Stream: live(r1),
		From: now, Now: now,
		Want: rows(first(r0), Row{
			Request: "r1", Session: D28C, Dir: dirs[D28C], At: "14:41:50", Mark: views.MarkAnswering, Stage: views.RowAnswering,
			First: "1.0s", Took: "17.0s", Stopped: "answering · 5.1k characters", Phone: "answering",
		}),
		Counts: map[string]Count{"2026-10-01": shown(2)},
	}
	if !lined {
		return m
	}
	then := m
	l := requested("r1", D28C, Opus, side, status.ReasonSticky, sent.Add(-100*time.Millisecond))
	l.TotalMS = 16_000
	ended := sent.Add(16 * time.Second)
	m.Name = "a request answering, its details open, paused, its line come while the stream's open: the rest of them, its row where it sat, not new"
	m.Lines, m.Sessions, m.Stream = append(m.Lines, l), []status.Session{listed(D28C, ended, "")}, live(done(ended, 200, r1))
	m.Want = rows(first(r0), rowOf(l))
	m.Then, m.Chosen, m.ChosenNow = &then, 1, 1
	return m
}

// oldest is the oldest day's line wanted, of the day with the given date,
// the ledger keeping its lines as kept says.
func oldest(date, kept string) LogLine {
	return LogLine{Oldest: date + ", " + kept}
}

func keptForGood() LogMoment {
	r1 := requested("r1", D28C, Opus, work, status.ReasonNew, onDay(-1, 18, 0, 0))
	r2 := requested("r2", D28C, Opus, work, status.ReasonSticky, At(9, 0, 0))
	return LogMoment{
		Name:  "the ledger keeping its lines for good, the table reaching back past its first day: that day's line atop it",
		Lines: []ledger.Line{r1, r2}, Totals: []views.DayTotals{{Date: "2026-09-29", Requests: 1, Sessions: 1}},
		Oldest: views.OldestKept(r1.At, dayfile.Forever, now),
		From:   onDay(-3, 0, 0, 0), Now: now,
		Want:   []LogLine{oldest("2026-09-29", "for good"), {Row: first(r1)}, rule("2026-09-29", "2026-10-01"), {Row: rowOf(r2)}},
		Counts: map[string]Count{"2026-09-29": shown(1), "2026-10-01": shown(1)},
	}
}

func keptFourHundredDays() LogMoment {
	// The ledger began before the oldest day it keeps, and the day before
	// that is yet to be pruned.
	began := time.Date(2025, 3, 1, 9, 0, 0, 0, time.Local)
	unkept := requested("r1", D28C, Opus, work, status.ReasonNew, time.Date(2025, 8, 26, 23, 0, 0, 0, time.Local))
	r2 := requested("r2", C61B, Opus, work, status.ReasonNew, time.Date(2025, 8, 27, 9, 0, 0, 0, time.Local))
	r3 := requested("r3", C61B, Opus, work, status.ReasonSticky, At(9, 0, 0))
	return LogMoment{
		Name:  "the ledger keeping its lines 400 days, the table reaching back past the oldest day it keeps: that day's line atop it, the day before never read",
		Lines: []ledger.Line{unkept, r2, r3}, Oldest: views.OldestKept(began, 400*24*time.Hour, now),
		From: time.Date(2025, 8, 20, 0, 0, 0, 0, time.Local), Now: now,
		Want:   []LogLine{oldest("2025-08-27", "400 days"), {Row: first(r2)}, rule("2025-08-27", "2026-10-01"), {Row: rowOf(r3)}},
		Counts: map[string]Count{"2025-08-27": shown(1), "2026-10-01": shown(1)},
	}
}

func pausedThroughAFilter() LogMoment {
	r0 := requested("r0", D28C, Opus, side, status.ReasonNew, At(14, 30, 0))
	c0 := requested("c0", C61B, Opus, side, status.ReasonNew, At(14, 35, 0))
	d28c := views.LogFilter{Kinds: views.LogOpening().Kinds, Sessions: []string{D28C}}
	then := LogMoment{Lines: []ledger.Line{r0, c0}, Filter: d28c, From: At(14, 40, 0), Now: At(14, 40, 0)}
	r1 := requested("r1", D28C, Opus, side, status.ReasonSticky, At(14, 41, 0))
	c1 := requested("c1", C61B, Opus, side, status.ReasonSticky, At(14, 41, 10))
	k1 := kinded(ledger.KindCheck, requested("k1", D28C, Haiku, side, status.ReasonNew, At(14, 41, 20)))
	r2 := requested("r2", D28C, Opus, side, status.ReasonSticky, At(14, 41, 30))
	return LogMoment{
		Name:  "paused, d28c's requests alone shown: its new requests counted, not another session's, nor its check",
		Lines: []ledger.Line{r0, c0, r1, c1, k1, r2}, Filter: d28c,
		From: now, Now: now,
		Want:   rows(first(r0), rowOf(r1), rowOf(r2)),
		Counts: map[string]Count{"2026-10-01": {Shown: 3, Requests: 5, Checks: 1}},
		Then:   &then, New: 2,
	}
}

func pausedAcrossMidnight() LogMoment {
	r1 := requested("r1", D28C, Opus, side, status.ReasonNew, onDay(0, 23, 40, 0))
	sent := onDay(0, 23, 59, 20)
	r2 := told("r2", D28C, Opus, side, sent, onDay(0, 23, 59, 30))
	r2.Dir, r2.First, r2.Chars = dirs[D28C], sent.Add(time.Second), 800
	then := LogMoment{Lines: []ledger.Line{r1}, Stream: live(r2), From: onDay(0, 0, 0, 0), Now: onDay(0, 23, 59, 30)}
	l2 := requested("r2", D28C, Opus, side, status.ReasonSticky, sent.Add(-100*time.Millisecond))
	l2.TotalMS = 50_000
	c1 := requested("c1", C61B, Opus, side, status.ReasonNew, onDay(1, 0, 0, 20))
	return LogMoment{
		Name:  "paused at 23:59:30 at a request answering, midnight passing: its line fills it in above the rule, not new, the next day's first new",
		Lines: []ledger.Line{r1, l2, c1}, Stream: live(done(sent.Add(50*time.Second), 200, r2)),
		Totals: []views.DayTotals{{Date: "2026-09-30", Requests: 2, Sessions: 1}},
		From:   onDay(0, 0, 0, 0), Now: onDay(1, 0, 0, 40),
		Want:   []LogLine{{Row: first(r1)}, {Row: rowOf(l2)}, rule("2026-09-30", "2026-10-01"), {Row: first(c1)}},
		Counts: map[string]Count{"2026-09-30": shown(2), "2026-10-01": shown(1)},
		Then:   &then, Chosen: 1, ChosenNow: 1, New: 1,
	}
}

func pausedAtTheSecondOneThirty() LogMoment {
	early := time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC).Local()
	late := early.Add(time.Hour)
	// The router started again between them, its ids starting again with it.
	r1 := requested("r1", D28C, Opus, side, status.ReasonNew, early)
	again := requested("r1", C61B, Opus, side, status.ReasonNew, late)
	then := LogMoment{Lines: []ledger.Line{r1, again}, From: early, Now: late.Add(30 * time.Second)}
	r2 := requested("r2", C61B, Opus, side, status.ReasonSticky, late.Add(10*time.Minute))
	return LogMoment{
		Name:  "paused at the second 01:30 as the clocks go back, at a request of an id the router gave again: it stays chosen, a later one new",
		Lines: []ledger.Line{r1, again, r2},
		From:  early, Now: late.Add(time.Hour),
		Want:   rows(first(r1), first(again), rowOf(r2)),
		Counts: map[string]Count{dayfile.DateOf(early): shown(3)},
		Then:   &then, Chosen: 1, ChosenNow: 1, New: 1,
	}
}

func notLive() LogMoment {
	r0 := requested("r0", D28C, Opus, side, status.ReasonNew, At(14, 40, 0))
	r0.TotalMS = 40_000
	r1 := requested("r1", C61B, Opus, side, status.ReasonNew, At(14, 40, 30))
	r1.TotalMS = 5_000
	k1 := kinded(ledger.KindCheck, requested("k1", C61B, Haiku, side, status.ReasonNew, At(14, 40, 51)))
	k1.TotalMS = 1_000
	return LogMoment{
		Name:  "the router gone, its ledger last written at 14:40:52: the latest end, a check's, hidden, not the latest arrival",
		Lines: []ledger.Line{r0, r1, k1}, Filter: views.LogOpening(),
		From: now, Now: now,
		Want:   rows(first(r0), first(r1)),
		Counts: map[string]Count{"2026-10-01": {Shown: 2, Requests: 2, Checks: 1, HiddenBy: []views.RowKind{views.RowCheck}}},
	}
}
