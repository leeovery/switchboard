package views_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// withID returns l, its request given the id given.
func withID(id string, l ledger.Line) ledger.Line {
	l.Request = id
	return l
}

// rowWords runs a row's words together, each part's kind marked, so a test
// sees both what they say and how each is drawn: what's named in brackets,
// what stands between in braces, and what a request in flight is doing in
// angle brackets.
func rowWords(w views.RowWords) string {
	var b strings.Builder
	for _, p := range w {
		switch p.Kind {
		case views.RowNamed:
			b.WriteString("[" + p.Text + "]")
		case views.RowBetween:
			b.WriteString("{" + p.Text + "}")
		case views.RowDoing:
			b.WriteString("<" + p.Text + ">")
		default:
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

// routingWords runs a row's routing together, as status's tests mark them,
// and its cost after it, as a row says it.
func routingWords(r views.RoutingSaid) string {
	var b strings.Builder
	for _, p := range r.Words {
		if p.Kind == status.PartAccount {
			b.WriteString("[" + p.Text + "]")
			continue
		}
		b.WriteString(p.Text)
	}
	if r.Cost != nil {
		b.WriteString("  " + r.Cost.Cents())
	}
	return b.String()
}

func TestARequestsRowHoldsItsLinesFacts(t *testing.T) {
	l := withID("r1", in("~/Code/api", stopped("tool_use", headersAfter(800*time.Millisecond, took(11800*time.Millisecond,
		line(mover, opus, "work", "sticky", october(7, 13, 0), hourUsage))), "Bash", "Read", "Edit")))
	l.Canceled = true
	got := views.RowOfLine(l, []ledger.Line{l}, ledger.Pricing, now)
	want := views.RequestRow{
		Request: "r1", At: l.At, Session: mover, Dir: "~/Code/api", Model: opus, Account: "work", Status: 200, Canceled: true,
		First: new(800 * time.Millisecond), Took: new(11800 * time.Millisecond),
		Read: 100000, Written: 1000, Out: 500, Metered: true, Worth: priced(hourWorth), Stop: "tool_use", Tool: "Bash", More: 2,
		NewSession: true, Routing: views.RoutingSaid{Words: status.RoutingWords(status.ReasonNew, status.Facts{})}, Mark: views.MarkNew,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the row is\n%+v\nwant\n%+v", got, want)
	}
}

func TestARowKnowsNoMoreThanItsLineSays(t *testing.T) {
	unsent := withID("r1", line(mover, opus, "", "no account has room", october(7, 13, 0), ""))
	unsent.Status, unsent.Attempts, unsent.TotalMS = 429, 0, 2
	got := views.RowOfLine(unsent, []ledger.Line{unsent}, ledger.Pricing, now)
	if got.First != nil || got.Metered || got.Read+got.Written+got.Out != 0 || got.Account != "" || !got.Failed || *got.Took != 2*time.Millisecond {
		t.Errorf("the row of a request the router answered itself is %+v, want no first byte, no usage, no account, failed, its took its line's", got)
	}
}

func TestARowsMarkIsTheFirstThatApplies(t *testing.T) {
	sent := october(7, 13, 11)
	told := func(change func(*views.Told)) views.Told {
		r := views.Told{Request: "r9", Session: mover, Model: opus, Account: "side", Sent: sent, Heard: sent}
		change(&r)
		return r
	}
	kind := func(k string, l ledger.Line) ledger.Line {
		l.Kind = k
		return l
	}
	answered := func(status int, l ledger.Line) ledger.Line {
		l.Status = status
		return l
	}
	before := withID("r0", line(mover, opus, "work", "new", october(7, 12, 59), hourUsage))
	stayed := withID("r1", line(mover, opus, "work", "sticky", october(7, 13, 0), hourUsage))
	moved := movedFrom("work", withID("r1", line(mover, opus, "side", "moved: work is at its reserve", october(7, 13, 0), hourUsage)))
	lineTests := []struct {
		name string
		l    ledger.Line
		want views.RowMark
	}{
		{name: "a request that stayed and succeeded", l: stayed, want: views.NoMark},
		{name: "a session's first", l: before, want: views.MarkNew},
		{name: "a move", l: moved, want: views.MarkMoved},
		{name: "a failed move", l: answered(500, moved), want: views.MarkFailed},
		{name: "a failed first", l: answered(429, before), want: views.MarkFailed},
		{name: "cut off", l: answered(0, stayed), want: views.MarkFailed},
		{name: "a quota check", l: kind(ledger.KindCheck, before), want: views.MarkHidden},
		{name: "a failed count", l: answered(500, kind(ledger.KindCount, stayed)), want: views.MarkHidden},
	}
	for _, tt := range lineTests {
		if got := views.RowOfLine(tt.l, []ledger.Line{before, tt.l}, ledger.Pricing, now).Mark; got != tt.want {
			t.Errorf("%s: its row's mark is %q, want %q", tt.name, got, tt.want)
		}
	}
	toldTests := []struct {
		name string
		t    views.Told
		want views.RowMark
	}{
		{name: "asking", t: told(func(*views.Told) {}), want: views.MarkAsking},
		{name: "answering", t: told(func(r *views.Told) { r.First = sent.Add(time.Second) }), want: views.MarkAnswering},
		{name: "throttled, to go out again", t: told(func(r *views.Told) { r.Verdict, r.Status = views.VerdictThrottled, 429 }), want: views.MarkAsking},
		{name: "refused at a limit, to go out again", t: told(func(r *views.Told) { r.Verdict, r.Status = views.VerdictLimited, 429 }), want: views.MarkAsking},
		{name: "ended, its line to come", t: told(func(r *views.Told) { r.Ended, r.Status = sent.Add(time.Second), 200 }), want: views.NoMark},
		{name: "ended failing", t: told(func(r *views.Told) { r.Ended, r.Status = sent.Add(time.Second), 529 }), want: views.MarkFailed},
		{name: "ended, its client gone first", t: told(func(r *views.Told) { r.Ended = sent.Add(time.Second) }), want: views.MarkFailed},
		{name: "lost", t: told(func(r *views.Told) { r.First, r.Lost = sent.Add(time.Second), true }), want: views.MarkFailed},
		{name: "a quota check in flight", t: told(func(r *views.Told) { r.Check = true }), want: views.MarkHidden},
		{name: "a count ended failing", t: told(func(r *views.Told) { r.Count, r.Ended, r.Status = true, sent, 500 }), want: views.MarkHidden},
	}
	for _, tt := range toldTests {
		if got := views.RowOfTold(tt.t, now).Mark; got != tt.want {
			t.Errorf("%s: its row's mark is %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestARowInFlightIsWhatTheStreamTold(t *testing.T) {
	sent := october(7, 13, 11)
	base := views.Told{Request: "r9", Session: mover, Dir: "~/Code/cli", Model: opus, Account: "side", Sent: sent, Heard: sent}
	answering, ended, lost := base, base, base
	answering.First, answering.Chars = sent.Add(700*time.Millisecond), 5100
	ended.First, ended.Ended, ended.Status = sent.Add(time.Second), sent.Add(14*time.Second), 200
	lost.First, lost.Lost = sent.Add(time.Second), true
	from := views.RequestRow{Request: "r9", At: sent, Session: mover, Dir: "~/Code/cli", Model: opus, Account: "side"}
	tests := []struct {
		name string
		t    views.Told
		want func(r *views.RequestRow)
	}{
		{name: "asking, its took counting to now", t: base, want: func(r *views.RequestRow) {
			r.Stage, r.Took, r.Mark = views.RowAsking, new(now.Sub(sent)), views.MarkAsking
		}},
		{name: "answering, its first byte come", t: answering, want: func(r *views.RequestRow) {
			r.Stage, r.First, r.Took, r.Chars, r.Mark = views.RowAnswering, new(700*time.Millisecond), new(now.Sub(sent)), 5100, views.MarkAnswering
		}},
		{name: "ended, its line to come", t: ended, want: func(r *views.RequestRow) {
			r.Stage, r.Status, r.First, r.Took = views.RowEnded, 200, new(time.Second), new(14*time.Second)
		}},
		{name: "lost, nothing of its answer known", t: lost, want: func(r *views.RequestRow) {
			r.Stage, r.Failed, r.Mark = views.RowLost, true, views.MarkFailed
		}},
	}
	for _, tt := range tests {
		want := from
		tt.want(&want)
		if got := views.RowOfTold(tt.t, now); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: the row is\n%+v\nwant\n%+v", tt.name, got, want)
		}
	}
}

func TestStoppedSaysWhyItsAnswerStoppedOrWhatItsDoing(t *testing.T) {
	sent := october(7, 13, 11)
	first := withID("r0", line(mover, opus, "work", "new", october(7, 12, 0), hourUsage))
	lined := func(l ledger.Line) views.RequestRow {
		return views.RowOfLine(l, []ledger.Line{first, l}, ledger.Pricing, now)
	}
	stop := func(why string, tools ...string) ledger.Line {
		return stopped(why, withID("r1", line(mover, opus, "work", "sticky", october(7, 13, 0), hourUsage)), tools...)
	}
	moved := movedFrom("work", withID("r1", line(mover, opus, "side", "moved: work is at its reserve", october(7, 13, 0), hourUsage)))
	toldDoing := func(first time.Time, chars int, lost bool) views.RequestRow {
		return views.RowOfTold(views.Told{Request: "r9", Session: mover, Model: opus, Account: "side", Sent: sent, First: first, Chars: chars, Lost: lost}, now)
	}
	check := stop("end_turn")
	check.Kind = ledger.KindCheck
	tests := []struct {
		name        string
		row         views.RequestRow
		wide, phone string
	}{
		{name: "its answer ended", row: lined(stop("end_turn")), wide: "end_turn", phone: "end_turn"},
		{name: "to call a tool", row: lined(stop("tool_use", "Bash")), wide: "tool_use{ · }[Bash]", phone: "[Bash]"},
		{name: "to call several tools", row: lined(stop("tool_use", "TodoWrite", "Bash")), wide: "tool_use{ · }[TodoWrite] +1", phone: "[TodoWrite] +1"},
		{name: "its answer telling no why", row: lined(stop("")), wide: "", phone: ""},
		{name: "its why and tool from elsewhere, cleaned", row: lined(stop("tool\x07use", "Ba\nsh")), wide: "tool use{ · }[Ba sh]", phone: "[Ba sh]"},
		{name: "a session's first, on a phone", row: lined(stopped("tool_use", first, "Bash")), wide: "tool_use{ · }[Bash]", phone: "new session"},
		{name: "a move, on a phone", row: lined(stopped("end_turn", moved)), wide: "end_turn", phone: "from [work]"},
		{name: "asking", row: toldDoing(time.Time{}, 0, false), wide: "<asking>", phone: "<asking>"},
		{name: "answering, no characters counted yet", row: toldDoing(sent.Add(time.Second), 0, false), wide: "<answering>", phone: "<answering>"},
		{name: "answering", row: toldDoing(sent.Add(time.Second), 5100, false), wide: "<answering> · 5.1k characters", phone: "<answering>"},
		{name: "answering at length", row: toldDoing(sent.Add(time.Second), 958_200, false), wide: "<answering> · 958.2k characters", phone: "<answering>"},
		{name: "lost", row: toldDoing(sent.Add(time.Second), 2000, true), wide: lostAnswer, phone: lostAnswer},
		{name: "a quota check", row: lined(check), wide: "", phone: ""},
	}
	for _, tt := range tests {
		if got := rowWords(tt.row.Stopped()); got != tt.wide {
			t.Errorf("%s: Stopped() = %q, want %q", tt.name, got, tt.wide)
		}
		if got := rowWords(tt.row.PhoneStopped()); got != tt.phone {
			t.Errorf("%s: PhoneStopped() = %q, want %q", tt.name, got, tt.phone)
		}
	}
}

// lostAnswer is what a lost request's row says of how it stopped.
const lostAnswer = "no answer: the router went away"

func TestARowsRoutingSaysHowTheRouterChoseItsAccount(t *testing.T) {
	first := withID("r0", line(mover, opus, "work", "new", october(7, 12, 0), hourUsage))
	tried := func(l ledger.Line, tried ...ledger.Tried) ledger.Line {
		l.Tried = tried
		return l
	}
	answered := func(status int, l ledger.Line) ledger.Line {
		l.Status = status
		return l
	}
	atLimit := ledger.Tried{Account: "work", Why: status.WhyLimit}
	tests := []struct {
		name  string
		lines []ledger.Line
		want  string
		mark  views.RowMark
	}{
		{name: "stayed", lines: []ledger.Line{first, withID("r1", line(mover, opus, "work", "sticky", october(7, 12, 10), hourUsage))}},
		{name: "its session's first", lines: []ledger.Line{first}, want: "new session", mark: views.MarkNew},
		{
			name:  "its session's first, sent by the global pin",
			lines: []ledger.Line{withID("r0", line(mover, opus, "work", status.ReasonGlobalPin, october(7, 12, 0), hourUsage))},
			want:  "new session", mark: views.MarkNew,
		},
		{
			name:  "its session's second model's first, the router's new not its session's",
			lines: []ledger.Line{first, withID("r1", line(mover, haiku, "side", "new", october(7, 12, 10), smallUsage))},
		},
		{
			name:  "a move, its cost its own write to the cache",
			lines: []ledger.Line{first, movedFrom("work", withID("r1", of(ledger.ClassMain, line(mover, opus, "side", "moved: work is at its reserve", october(7, 12, 10), hourUsage))))},
			want:  "from [work], at its cap  " + cost(hourWrite).Cents(), mark: views.MarkMoved,
		},
		{
			name:  "sent again after a limit, its tried in place of its reason",
			lines: []ledger.Line{first, tried(movedFrom("work", withID("r1", line(mover, opus, "side", "moved: work hit its limit", october(7, 12, 10), hourUsage))), atLimit)},
			want:  "after [work]'s limit  " + cost(hourWrite).Cents(), mark: views.MarkMoved,
		},
		{
			name: "a 429 every account refused, passed on, its session back where it was",
			lines: []ledger.Line{first, tried(answered(429, withID("r1", line(mover, opus, "work", "sticky", october(7, 12, 10), ""))),
				atLimit, ledger.Tried{Account: "side", Why: status.WhyLimit})},
			want: "after [work]'s limit, [side]'s limit", mark: views.MarkFailed,
		},
		{name: "of no session", lines: []ledger.Line{withID("r1", line("", opus, "work", "unsessioned", october(7, 12, 10), hourUsage))}},
	}
	for _, tt := range tests {
		l := tt.lines[len(tt.lines)-1]
		got := views.RowOfLine(l, tt.lines, ledger.Pricing, now)
		if routing := routingWords(got.Routing); routing != tt.want || got.Mark != tt.mark {
			t.Errorf("%s: its routing is %q, marked %q, want %q, marked %q", tt.name, routing, got.Mark, tt.want, tt.mark)
		}
	}
}

func TestAMovesCostIsOnTheRequestThatMovedAsTheStorySettlesIt(t *testing.T) {
	conversation := func(id, account, reason string, at time.Time) ledger.Line {
		return withID(id, serving(p1, ledger.ClassMain, line(paged, opus, account, reason, at, hourUsage)))
	}
	side := func(id string, at time.Time) ledger.Line {
		return movedFrom("work", withID(id, serving(p1, "auxiliary", line(paged, opus, "side", "moved: work is at its reserve", at, smallUsage))))
	}
	tests := []struct {
		name  string
		lines []ledger.Line
		// want is what each row's routing and cost say, and whether it
		// wrote the session's context again, by its id.
		want map[string]string
	}{
		{
			name:  "a main request that moved, warm: its own write",
			lines: []ledger.Line{conversation("r1", "work", "new", local(10, 0, 0)), movedFrom("work", conversation("r2", "side", "moved: work is at its reserve", local(10, 30, 0)))},
			want:  map[string]string{"r1": "new session", "r2": "from [work], at its cap  " + cost(hourWrite).Cents() + " rewrote"},
		},
		{
			name:  "a side request that moved, then the next main request: the move's cost on the side request, the write on the main",
			lines: []ledger.Line{conversation("r1", "work", "new", local(10, 0, 0)), side("r2", local(10, 30, 0)), conversation("r3", "side", "sticky", local(10, 31, 0))},
			want:  map[string]string{"r1": "new session", "r2": "from [work], at its cap  " + cost(hourWrite).Cents(), "r3": " rewrote"},
		},
		{
			name:  "a side request that moved, no main request since: no cost known",
			lines: []ledger.Line{conversation("r1", "work", "new", local(10, 0, 0)), side("r2", local(10, 30, 0))},
			want:  map[string]string{"r1": "new session", "r2": "from [work], at its cap"},
		},
		{
			name:  "a side request that moved, the next main request elsewhere: none",
			lines: []ledger.Line{conversation("r1", "work", "new", local(10, 0, 0)), side("r2", local(10, 30, 0)), conversation("r3", "personal", "sticky", local(10, 31, 0))},
			want:  map[string]string{"r1": "new session", "r2": "from [work], at its cap", "r3": ""},
		},
		{
			name:  "a move where the cache would have run out anyway: no cost, nothing written that staying wouldn't have",
			lines: []ledger.Line{conversation("r1", "work", "new", local(9, 0, 0)), movedFrom("work", conversation("r2", "side", "moved: work is at its reserve", local(10, 30, 0)))},
			want:  map[string]string{"r1": "new session", "r2": "from [work], at its cap"},
		},
	}
	for _, tt := range tests {
		got := make(map[string]string)
		for _, l := range tt.lines {
			r := views.RowOfLine(l, tt.lines, ledger.Pricing, now)
			got[r.Request] = routingWords(r.Routing)
			if r.Rewrote {
				got[r.Request] += " rewrote"
			}
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: the rows say %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestTheListsMarkAndRoutingAreAsTheLogsRowSaysThem(t *testing.T) {
	seen := october(7, 13, 10)
	started := withID("r0", of(ledger.ClassMain, line(mover, opus, "work", "new", october(7, 12, 30), hourUsage)))
	moved := movedFrom("work", withID("r1", of(ledger.ClassMain, line(mover, opus, "side", "moved: work hit its limit", october(7, 12, 40), hourUsage))))
	retried := moved
	retried.Tried = []ledger.Tried{{Account: "work", Why: status.WhyLimit}}
	onSide := func(reason, inFlight string) []status.Session {
		return []status.Session{{ID: mover, Assignments: []status.Assignment{{Model: opus, Account: "side", Reason: reason, InFlight: inFlight, LastSeen: seen}}}}
	}
	verdict := func(v views.Verdict) views.Stream {
		r := views.Told{Request: "r2", Session: mover, Model: opus, Account: "side", Sent: seen, Heard: seen, Verdict: v, Status: 429}
		return views.Stream{Live: true, Requests: map[string]views.Told{"r2": r}}
	}
	movedCost := "  " + cost(hourWrite).Cents()
	tests := []struct {
		name    string
		lines   []ledger.Line
		running []status.Session
		stream  views.Stream
		mark    views.RowMark
		routing string
	}{
		{name: "idle where it started", lines: []ledger.Line{started}, running: onSide("sticky", "")},
		{name: "asking, moved there", lines: []ledger.Line{started, moved}, running: onSide("sticky", status.Asking), mark: views.MarkAsking,
			routing: "from [work], at its limit" + movedCost},
		{name: "answering", lines: []ledger.Line{started}, running: onSide("sticky", status.Answering), mark: views.MarkAnswering},
		{name: "refused at a limit, to go out again", lines: []ledger.Line{started}, running: onSide("sticky", status.Asking),
			stream: verdict(views.VerdictLimited), mark: views.MarkAsking},
		{name: "throttled, to go out again", lines: []ledger.Line{started}, running: onSide("sticky", status.Asking),
			stream: verdict(views.VerdictThrottled), mark: views.MarkAsking},
		{name: "idle, moved there", lines: []ledger.Line{started, moved}, running: onSide("sticky", ""), mark: views.MarkMoved,
			routing: "from [work], at its limit" + movedCost},
		{name: "idle, moved there by a request sent again after a limit", lines: []ledger.Line{started, retried}, running: onSide("sticky", ""),
			mark: views.MarkMoved, routing: "after [work]'s limit" + movedCost},
		{name: "pinned there on its own", lines: []ledger.Line{started}, running: onSide(status.ReasonPinned, ""), routing: "pinned here"},
		{name: "ended, moved there", lines: []ledger.Line{started, moved}, mark: views.MarkMoved, routing: "from [work], at its limit" + movedCost},
	}
	for _, tt := range tests {
		list := views.ListSessions(views.SessionSources{
			Running: tt.running, Stream: tt.stream, Ledger: fakeLedger{now: now, lines: tt.lines}, Prices: ledger.Pricing, Now: now,
		})
		if len(list.Sessions) != 1 {
			t.Fatalf("%s: the List holds %+v, want one session", tt.name, list.Sessions)
		}
		if s := list.Sessions[0]; s.Mark != tt.mark || routingWords(s.Routing) != tt.routing {
			t.Errorf("%s: the List marks it %q, routed %q, want %q, routed %q", tt.name, s.Mark, routingWords(s.Routing), tt.mark, tt.routing)
		}
	}
}
