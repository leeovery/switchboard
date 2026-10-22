package views_test

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// wordsSaid runs words together, each account named in brackets, so a test
// sees both what they say and what they name.
func wordsSaid(w status.Words) string {
	var b strings.Builder
	for _, p := range w {
		if p.Kind == status.PartAccount {
			b.WriteString("[" + p.Text + "]")
			continue
		}
		b.WriteString(p.Text)
	}
	return b.String()
}

// Where a session runs is its named model's account, since when and why: a
// move of its own that brought it there, said as a move, a forced move's why
// short on a phone; a reason of the router's that moved nothing, since it
// started; any other, as a move when the router put it there; and once it's
// over, where it ran.
func TestWhereASessionRunsIsItsNamedModelsAccountAndHowItCameThere(t *testing.T) {
	now := time.Date(2026, 10, 7, 16, 30, 0, 0, time.UTC)
	const opus, haiku = "claude-opus-5-5", "claude-haiku-4-5"
	at := func(hour, minute int) time.Time { return time.Date(2026, 10, 7, hour, minute, 0, 0, time.UTC) }
	onto := func(reason string) views.SessionMove {
		return views.SessionMove{At: at(14, 31), Model: opus, From: "work", To: "side", Reason: reason}
	}
	moved := onto("moved: work hit its limit")
	running, ended := views.Activity{Doing: views.Idle}, views.Activity{Doing: views.Ended}
	movedOnto := func(account string, m views.SessionMove) views.SessionPage {
		return views.SessionPage{Account: account, Model: opus, Moves: []views.SessionMove{m}, Activity: running}
	}
	goesThere := func(account, reason string, since time.Time) views.SessionPage {
		return views.SessionPage{Account: account, Model: opus, Activity: running, Goes: []views.SessionModel{{Model: opus, Account: account, Reason: reason, Since: since}}}
	}
	tests := []struct {
		name                string
		page                views.SessionPage
		onSince, why, short string
	}{
		{name: "moved off an account at its cap", page: movedOnto("side", onto("moved: work is at its reserve")),
			onSince: "on [side] since 14:31", why: ", moved from [work]: [work] reached its cap", short: "moved from [work]: its cap"},
		{name: "moved off an account at its limit", page: movedOnto("side", moved),
			onSince: "on [side] since 14:31", why: ", moved from [work]: [work] reached its limit", short: "moved from [work]: its limit"},
		{name: "moved off an account that refused it", page: movedOnto("side", onto("moved: work was refused")),
			onSince: "on [side] since 14:31", why: ", moved from [work]: [work] refused the request", short: "moved from [work]: refused"},
		{name: "rescored after an idle hour",
			page:    movedOnto("side", views.SessionMove{At: at(9, 12), Model: opus, From: "work", To: "side", Reason: "rescored after 15h 7m idle"}),
			onSince: "on [side] since 09:12", why: ", rescored after 15h 7m idle: [side] had the most room", short: "rescored after 15h 7m idle"},
		{name: "pinned here, its own pin moving it", page: movedOnto("side", onto(status.ReasonPinned)),
			onSince: "on [side] since 14:31", why: ", pinned here", short: "pinned here"},
		{name: "the global pin's move", page: movedOnto("side", onto(status.ReasonMovedByPin)),
			onSince: "on [side] since 14:31", why: ", pinned to [side]", short: "pinned to [side]"},
		{name: "its pin yielding, as it went elsewhere than its pin sends it", page: movedOnto("side", onto(status.ReasonPinYields+"work has no room")),
			onSince: "on [side] since 14:31", why: ": its pin to [work] yields: [work] has no room", short: "its pin to [work] yields: [work] has no room"},
		{name: "a move the stream told of, its line yet to come, as the router puts it there",
			page:    goesThere("side", "moved: work hit its limit", at(16, 29)),
			onSince: "on [side] since 16:29", why: ", moved from [work]: [work] reached its limit", short: "moved from [work]: its limit"},
		{name: "pinned here where it ran, as the router lists its pin", page: goesThere("side", status.ReasonPinned, at(14, 39)),
			onSince: "on [side] since 14:39", why: ", pinned here", short: "pinned here"},
		{name: "rescored where it ran, its model's last move taking it elsewhere",
			page:    views.SessionPage{Account: "personal", Model: opus, Moves: []views.SessionMove{moved}, Activity: running, Goes: []views.SessionModel{{Model: opus, Account: "personal", Reason: "rescored after 1h idle", Since: at(16, 0)}}},
			onSince: "on [personal] since 16:00", why: ", rescored after 1h idle: [personal] had the most room", short: "rescored after 1h idle"},
		{name: "a reason the router gives, when it put it there not known", page: goesThere("side", status.ReasonPinned, time.Time{}),
			onSince: "on [side]", why: ": pinned here", short: "pinned here"},
		{name: "sticky, since it started", page: goesThere("work", status.ReasonSticky, at(12, 0)),
			onSince: "on [work] since it started", why: ": sticky", short: "sticky"},
		{name: "a new session's, the choice passing an account over for pressure", page: goesThere("work", status.PassingOver(status.ReasonNew, "personal"), at(12, 0)),
			onSince: "on [work] since it started", why: ": [personal] came under pressure", short: "[personal] came under pressure"},
		{name: "another model's move onto it telling nothing of how it came there",
			page: views.SessionPage{Account: "side", Model: opus, Activity: running,
				Moves: []views.SessionMove{{At: at(14, 40), Model: haiku, From: "work", To: "side", Reason: "moved: work hit its limit"}},
				Goes:  []views.SessionModel{{Model: opus, Account: "side", Reason: status.ReasonNew, Since: at(12, 0)}}},
			onSince: "on [side] since it started"},
		{name: "since it started, where nothing gives why", page: views.SessionPage{Account: "work", Model: opus, Activity: running},
			onSince: "on [work] since it started"},
		{name: "once ended, where it ran, its named model's moves alone",
			page: views.SessionPage{Account: "side", Model: opus, Started: at(12, 0), Activity: ended,
				Moves: []views.SessionMove{moved, {At: at(14, 40), Model: haiku, From: "work", To: "personal", Reason: "moved: work hit its limit"}}},
			why: "ran on [work], then [side] from 14:31: [work] reached its limit", short: "ran on [work], then [side] from 14:31: its limit"},
		{name: "once ended, wholly yesterday, its move's time alone, as its start names the day",
			page: views.SessionPage{Account: "side", Model: opus, Started: at(13, 20).AddDate(0, 0, -1), Activity: ended,
				Moves: []views.SessionMove{{At: at(16, 2).AddDate(0, 0, -1), Model: opus, From: "work", To: "side", Reason: "moved: work is at its reserve"}}},
			why: "ran on [work], then [side] from 16:02: [work] reached its cap", short: "ran on [work], then [side] from 16:02: its cap"},
		{name: "once ended, a move on a day after its start's, with its day",
			page: views.SessionPage{Account: "side", Model: opus, Started: at(23, 30).AddDate(0, 0, -1), Activity: ended,
				Moves: []views.SessionMove{{At: at(0, 40), Model: opus, From: "work", To: "side", Reason: "moved: work is at its reserve"}}},
			why: "ran on [work], then [side] from today 00:40: [work] reached its cap", short: "ran on [work], then [side] from today 00:40: its cap"},
		{name: "once ended, on the one account it never left", page: views.SessionPage{Account: "work", Model: opus, Activity: ended},
			why: "ran on [work]", short: "ran on [work]"},
		{name: "none, named on no account", page: views.SessionPage{Model: opus, Activity: running}},
		{name: "none, ended on no account", page: views.SessionPage{Model: opus, Activity: ended}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := tt.page.Where(now)
			if got := wordsSaid(w.OnSince); got != tt.onSince {
				t.Errorf("Where's OnSince = %q, want %q", got, tt.onSince)
			}
			if got := wordsSaid(w.Why); got != tt.why {
				t.Errorf("Where's Why = %q, want %q", got, tt.why)
			}
			if got := wordsSaid(w.Short); got != tt.short {
				t.Errorf("Where's Short = %q, want %q", got, tt.short)
			}
			if got, want := wordsSaid(w.Words()), tt.onSince+tt.why; got != want {
				t.Errorf("Where's Words = %q, want %q", got, want)
			}
		})
	}
}

// WhereWidest says what Where says, each time it names in the widest form
// a day gives it, "yesterday 14:31", but a move on an ended session's
// start's day, which is said by its time alone at any clock: so it says the
// same whether the clock is on the time's day, the next, or a week or a
// year on.
func TestWhereWidestSaysWhereAtItsWidestWhateverTheDay(t *testing.T) {
	at := time.Date(2026, 10, 7, 14, 31, 0, 0, time.UTC)
	running := views.SessionPage{Account: "side", Model: "claude-opus-5-5", Activity: views.Activity{Doing: views.Idle},
		Moves: []views.SessionMove{{At: at, Model: "claude-opus-5-5", From: "work", To: "side", Reason: "moved: work hit its limit"}}}
	ended := running
	ended.Started, ended.Activity = at.Add(-2*time.Hour), views.Activity{Doing: views.Ended}
	overnight := ended
	overnight.Started = at.AddDate(0, 0, -1)
	tests := []struct {
		name string
		page views.SessionPage
		want string
	}{
		{name: "running, since its move", page: running,
			want: "on [side] since yesterday 14:31, moved from [work]: [work] reached its limit"},
		{name: "ended, from its move on its start's day", page: ended,
			want: "ran on [work], then [side] from 14:31: [work] reached its limit"},
		{name: "ended, from its move on a day after its start's", page: overnight,
			want: "ran on [work], then [side] from yesterday 14:31: [work] reached its limit"},
	}
	for _, tt := range tests {
		for _, days := range []int{0, 1, 7, 400} {
			now := at.Add(time.Hour).AddDate(0, 0, days)
			if got := wordsSaid(tt.page.WhereWidest(now).Words()); got != tt.want {
				t.Errorf("%s, %d days on, WhereWidest says %q, want %q", tt.name, days, got, tt.want)
			}
		}
	}
}

// What a session's doing and how it has run are said at their widest for
// its own kind, the same whatever it's doing now and wherever the clock is:
// a running session's doings, each at its widest, an ended one's end the
// day before; and how it has run to a minute short of a day, or resumed the
// day before.
func TestASessionsDoingAndRunAtTheirWidestFollowItsKind(t *testing.T) {
	now := time.Date(2026, 10, 7, 9, 5, 0, 0, time.UTC)
	started := now.Add(-2 * time.Hour)
	running := views.SessionPage{Started: started, Activity: views.Activity{Doing: views.Asking}}
	resumed := running
	resumed.Resumed = now.Add(-time.Hour)
	ended := views.SessionPage{Started: started, Activity: views.Activity{Doing: views.Ended, Since: now.Add(-time.Hour)}}
	tests := []struct {
		name  string
		page  views.SessionPage
		doing []string
		ran   string
	}{
		{name: "running", page: running, doing: []string{"↑ asking", "↓ answering", "… 429", "✕ 403", "idle 59m"}, ran: "23h 59m"},
		{name: "resumed", page: resumed, doing: []string{"↑ asking", "↓ answering", "… 429", "✕ 403", "idle 59m"}, ran: "resumed yesterday 09:05"},
		{name: "ended", page: ended, doing: []string{"ended yesterday 09:05"}, ran: "ran 23h 59m"},
		{name: "its start not known", page: views.SessionPage{Activity: views.Activity{Doing: views.Idle}}, doing: []string{"↑ asking", "↓ answering", "… 429", "✕ 403", "idle 59m"}},
	}
	for _, tt := range tests {
		if got := tt.page.DoingSaidWidest(now); !slices.Equal(got, tt.doing) {
			t.Errorf("%s, DoingSaidWidest = %q, want %q", tt.name, got, tt.doing)
		}
		if got := tt.page.RanSaidWidest(now); got != tt.ran {
			t.Errorf("%s, RanSaidWidest = %q, want %q", tt.name, got, tt.ran)
		}
		for _, days := range []int{1, 7, 365} {
			later := now.AddDate(0, 0, days)
			if got := tt.page.RanSaidWidest(later); len(got) != len(tt.ran) {
				t.Errorf("%s, %d days on, RanSaidWidest = %q, want it as wide as %q", tt.name, days, got, tt.ran)
			}
		}
	}
}

// A move's label on the story says what moved it and where to, and where it
// wrote its context again, what that wrote and cost, wide and short.
func TestAMovesLabelSaysWhatMovedItAndWhatItCost(t *testing.T) {
	const opus = "claude-opus-5-5"
	worth := ledger.Picodollars(1_820_000_000_000)
	move := func(reason string) views.SessionMove {
		return views.SessionMove{Model: opus, From: "work", To: "side", Reason: reason, Written: 182000}
	}
	cached := func(m views.SessionMove, cost *ledger.Picodollars) views.SessionMove {
		m.Rewrote, m.Cost = true, cost
		return m
	}
	tests := []struct {
		name        string
		move        views.SessionMove
		wide, short string
	}{
		{name: "capped, its context written again", move: cached(move("moved: work is at its reserve"), &worth),
			wide: "[work] reached its cap → [side] · {182k} cached again, {$1.82}", short: "its cap → [side], {$1.82}"},
		{name: "limited, written again on a model the table doesn't price", move: cached(move("moved: work hit its limit"), nil),
			wide: "[work] reached its limit → [side] · {182k} cached again, unpriced", short: "its limit → [side], unpriced"},
		{name: "refused, its cache run out anyway", move: move("moved: work was refused"),
			wide: "[work] refused the request → [side]", short: "refused → [side]"},
		{name: "rescored after an idle gap", move: move("rescored after 15h 7m idle"),
			wide: "rescored after 15h 7m idle → [side]", short: "rescored after 15h 7m idle → [side]"},
		{name: "pinned here", move: move(status.ReasonPinned), wide: "pinned here → [side]", short: "pinned here → [side]"},
		{name: "the global pin's move, its words naming where it went", move: move(status.ReasonMovedByPin),
			wide: "pinned to [side]", short: "pinned to [side]"},
		{name: "a pin yielding", move: views.SessionMove{Model: opus, From: "side", To: "work", Reason: status.ReasonPinYields + "side has no room"},
			wide: "its pin to [side] yields: [side] has no room → [work]", short: "its pin to [side] yields: [side] has no room → [work]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := figured(tt.move.Said(views.MoveWide)); got != tt.wide {
				t.Errorf("Said(MoveWide) = %q, want %q", got, tt.wide)
			}
			if got := figured(tt.move.Said(views.MoveShort)); got != tt.short {
				t.Errorf("Said(MoveShort) = %q, want %q", got, tt.short)
			}
		})
	}
}

// figured runs words together as wordsSaid does, each figure in braces.
func figured(w status.Words) string {
	var b strings.Builder
	for _, p := range w {
		switch p.Kind {
		case status.PartAccount:
			b.WriteString("[" + p.Text + "]")
		case status.PartFigure:
			b.WriteString("{" + p.Text + "}")
		default:
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

func TestASessionsMovesAreItsNamedModelsAlone(t *testing.T) {
	const opus, haiku = "claude-opus-5-5", "claude-haiku-4-5"
	onto := views.SessionMove{Model: opus, From: "work", To: "side"}
	back := views.SessionMove{Model: opus, From: "side", To: "work"}
	other := views.SessionMove{Model: haiku, From: "work", To: "personal"}
	p := views.SessionPage{Account: "work", Model: opus, Moves: []views.SessionMove{onto, other, back}}
	if m, ok := p.MovedOnto(); !ok || m != back {
		t.Errorf("MovedOnto = %+v, %v; want its model's last move, which brought it back onto work", m, ok)
	}
	if first, moves := p.RanOn(); first != "work" || !reflect.DeepEqual(moves, []views.SessionMove{onto, back}) {
		t.Errorf("RanOn = %q, %+v; want work first, then its model's two moves", first, moves)
	}
	p.Account = "personal"
	if m, ok := p.MovedOnto(); ok {
		t.Errorf("MovedOnto on personal = %+v, want none, its model's last move taking it to work, another's to personal", m)
	}
}

// Row 6's right says when the session started, then how long it has run,
// its minutes two digits past an hour, when it came back after a fold, or
// once it's over, how long it ran; a phone's row 8 the second part alone.
func TestASessionsStartAndRunAreSaidAsRow6SaysThem(t *testing.T) {
	now := local(14, 42, 7)
	running, ended := views.Activity{Since: local(14, 33, 0)}, views.Activity{Doing: views.Ended, Since: local(18, 47, 0).AddDate(0, 0, -1)}
	tests := []struct {
		name         string
		page         views.SessionPage
		started, ran string
	}{
		{name: "running", page: views.SessionPage{Started: local(12, 2, 0), Activity: running}, started: "12:02", ran: "2h 40m"},
		{name: "running two hours and five minutes", page: views.SessionPage{Started: local(12, 37, 0), Activity: running},
			started: "12:37", ran: "2h 05m"},
		{name: "resumed after a fold", page: views.SessionPage{Started: local(16, 40, 0).AddDate(0, 0, -1), Resumed: local(9, 12, 0), Activity: running},
			started: "yesterday 16:40", ran: "resumed 09:12"},
		{name: "ended, from its first request to its last", page: views.SessionPage{Started: local(13, 20, 0).AddDate(0, 0, -1),
			Resumed: local(15, 0, 0).AddDate(0, 0, -1), Activity: ended}, started: "yesterday 13:20", ran: "ran 5h 27m"},
		{name: "across midnight", page: views.SessionPage{Started: local(23, 50, 0).AddDate(0, 0, -1), Activity: running},
			started: "yesterday 23:50", ran: "14h 52m"},
		{name: "its start not known", page: views.SessionPage{Activity: running}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.page.StartedSaid(now); got != tt.started {
				t.Errorf("StartedSaid = %q, want %q", got, tt.started)
			}
			if got := tt.page.RanSaid(now); got != tt.ran {
				t.Errorf("RanSaid = %q, want %q", got, tt.ran)
			}
		})
	}
}

// Each account the session ran on has its time there, to now on the one its
// latest request went to while it runs, each end with its day where the
// session began before today.
func TestASessionsTimeOnEachAccountIsItsStay(t *testing.T) {
	now := local(14, 42, 7)
	yesterday := func(hour, minute int) time.Time { return local(hour, minute, 0).AddDate(0, 0, -1) }
	running, ended := views.Activity{Since: local(14, 40, 0)}, views.Activity{Doing: views.Ended, Since: local(13, 20, 0)}
	tests := []struct {
		name     string
		accounts []views.SessionAccount
		a        views.Activity
		want     []string
	}{
		{name: "moved, to now on the account its latest request went to",
			accounts: []views.SessionAccount{{Account: "work", From: local(12, 2, 0), To: local(14, 31, 0)}, {Account: "side", From: local(14, 31, 13), To: local(14, 40, 0)}},
			a:        running, want: []string{"12:02 – 14:31", "14:31 – now"}},
		{name: "the last account while running, back on one it left",
			accounts: []views.SessionAccount{{Account: "work", From: local(12, 2, 0), To: local(14, 40, 0)}, {Account: "side", From: local(13, 0, 0), To: local(13, 30, 0)}},
			a:        running, want: []string{"12:02 – now", "13:00 – 13:30"}},
		{name: "resumed today, dated",
			accounts: []views.SessionAccount{{Account: "side", From: yesterday(16, 40), To: yesterday(18, 5)}, {Account: "personal", From: local(9, 12, 0), To: local(14, 40, 0)}},
			a:        running, want: []string{"yesterday 16:40 – 18:05", "today 09:12 – now"}},
		{name: "running since yesterday on one account, dated as its time reaches today",
			accounts: []views.SessionAccount{{Account: "work", From: yesterday(23, 50), To: yesterday(23, 58)}},
			a:        running, want: []string{"yesterday 23:50 – now"}},
		{name: "ended, each to its last request",
			accounts: []views.SessionAccount{{Account: "work", From: local(12, 0, 0), To: local(12, 40, 0)}, {Account: "side", From: local(12, 41, 0), To: local(13, 20, 0)}},
			a:        ended, want: []string{"12:00 – 12:40", "12:41 – 13:20"}},
		{name: "ended wholly yesterday, dated, as an undated time reads as today's",
			accounts: []views.SessionAccount{{Account: "work", From: yesterday(13, 20), To: yesterday(16, 2)}, {Account: "side", From: yesterday(16, 2), To: yesterday(18, 47)}},
			a:        views.Activity{Doing: views.Ended, Since: yesterday(18, 47)}, want: []string{"yesterday 13:20 – 16:02", "yesterday 16:02 – 18:47"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, s := range (views.SessionPage{Accounts: tt.accounts, Activity: tt.a}).Stays(now) {
				got = append(got, s.Said)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Stays = %q, want %q", got, tt.want)
			}
		})
	}
}

// A session's rows say their worth to the cent, each part of TURNS' total,
// their rounding reconciled so they add up to it, cent for cent: each the
// cents the running sum through it comes to, less those before it, so a row
// can say a cent off what it'd say alone, and more than nothing that comes
// to none reads <$0.01. What's unpriced reads as it would alone.
func TestASessionsRowsWorthAddsUpToTURNSTotal(t *testing.T) {
	// worth is a worth of the millionths of a dollar given, unpriced as
	// given, and of none priced where they're fewer than none.
	worth := func(millionths int64, unpriced ...string) views.Worth {
		w := views.Worth{Unpriced: unpriced}
		if millionths >= 0 {
			w.Amount = new(ledger.Picodollars(millionths * 1_000_000))
		}
		return w
	}
	tests := []struct {
		name   string
		worths []views.Worth
		total  views.Worth
		want   []string
	}{
		{name: "two half cents, a cent", worths: []views.Worth{worth(5000), worth(5000)}, total: worth(10_000), want: []string{"$0.01", "<$0.01"}},
		{name: "three under half a cent, a cent", worths: []views.Worth{worth(3500), worth(3500), worth(3500)}, total: worth(10_500),
			want: []string{"<$0.01", "$0.01", "<$0.01"}},
		{name: "one a cent short of what it'd say alone", worths: []views.Worth{worth(1_006_000, "claude-opus-9"), worth(2_006_000)},
			total: worth(3_012_000, "claude-opus-9"), want: []string{"$1.01+", "$2.00"}},
		{name: "one wholly unpriced", worths: []views.Worth{worth(1_234_000), worth(-1, "claude-opus-9")}, total: worth(1_234_000, "claude-opus-9"),
			want: []string{"$1.23", "unpriced"}},
		{name: "each as it'd say alone", worths: []views.Worth{worth(41_060_000), worth(3_200_000)}, total: worth(44_260_000), want: []string{"$41.06", "$3.20"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := views.SessionPage{Activity: views.Activity{Doing: views.Ended}, Totals: views.SessionTotals{Worth: tt.total}}
			for i, w := range tt.worths {
				p.Accounts = append(p.Accounts, views.SessionAccount{Account: fmt.Sprint("account-", i), Worth: w})
			}
			var got []string
			for _, s := range p.Stays(now) {
				got = append(got, s.Cents())
			}
			if !slices.Equal(got, tt.want) || centsOf(got) != centsOf([]string{p.Totals.Cents()}) {
				t.Errorf("the rows say %q, TURNS' total %s; want %q, adding up to it", got, p.Totals.Cents(), tt.want)
			}
		})
	}
}

// centsOf adds up worths said to the cent, as "$1.23", "$1.23+", "<$0.01" or
// "unpriced", the last two none, in cents.
func centsOf(said []string) int {
	sum := 0
	for _, s := range said {
		var dollars, cents int
		if _, err := fmt.Sscanf(strings.TrimSuffix(s, views.PartUnpriced), "$%d.%d", &dollars, &cents); err == nil {
			sum += 100*dollars + cents
		}
	}
	return sum
}

// A turn took the minutes from the one it started in to the one it ended
// in, both counted, as the frames count them; one going, to the one now is
// in.
func TestATurnTookTheMinutesItSpansBothEndsCounted(t *testing.T) {
	now := local(14, 42, 7)
	tests := []struct {
		name  string
		turn  views.Turn
		want  string
		going bool
	}{
		{name: "14:29 to 14:37", turn: views.Turn{Started: local(14, 29, 4), Ended: local(14, 37, 51)}, want: "9m"},
		{name: "within one minute", turn: views.Turn{Started: local(13, 6, 2), Ended: local(13, 6, 58)}, want: "1m"},
		{name: "a minute into the next", turn: views.Turn{Started: local(13, 6, 50), Ended: local(13, 7, 3)}, want: "2m"},
		{name: "going, from 14:40 to now", turn: views.Turn{Started: local(14, 40, 0)}, want: "3m", going: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, going := tt.turn.Took(now); got != tt.want || going != tt.going {
				t.Errorf("Took = %q, going %t; want %q, going %t", got, going, tt.want, tt.going)
			}
		})
	}
}

func TestATurnsToolsAreTheThreeItCalledMost(t *testing.T) {
	tests := []struct {
		tools []views.ToolCalls
		want  string
	}{
		{tools: []views.ToolCalls{{Tool: "Bash", Times: 14}, {Tool: "Edit", Times: 9}, {Tool: "Read", Times: 8}, {Tool: "Grep", Times: 2}}, want: "Bash ×14, Edit ×9, Read ×8"},
		{tools: []views.ToolCalls{{Tool: "Read\x1b[2J", Times: 1}}, want: "Read [2J ×1"},
		{},
	}
	for _, tt := range tests {
		if got := (views.Turn{Tools: tt.tools}).ToolsSaid(); got != tt.want {
			t.Errorf("ToolsSaid of %+v = %q, want %q", tt.tools, got, tt.want)
		}
	}
}

// A turn's tools at their widest are the three it names, each Claude Code's
// widest tool, called 999 times; and its worth at its widest, $999.99 part
// unpriced, or unpriced.
func TestATurnsToolsAndWorthAtTheirWidest(t *testing.T) {
	if got, want := views.ToolsSaidWidest(), "AskUserQuestion ×999, AskUserQuestion ×999, AskUserQuestion ×999"; got != want {
		t.Errorf("ToolsSaidWidest = %q, want %q", got, want)
	}
	if got, want := views.WorthSaidWidest(), []string{"$999.99+", "unpriced"}; !slices.Equal(got, want) {
		t.Errorf("WorthSaidWidest = %q, want %q", got, want)
	}
	tools := views.Turn{Tools: []views.ToolCalls{{Tool: "Bash", Times: 14}, {Tool: "Edit", Times: 9}}}
	if got, want := tools.ToolsEach(), []string{"Bash ×14", "Edit ×9"}; !slices.Equal(got, want) {
		t.Errorf("ToolsEach = %q, want %q", got, want)
	}
}

func TestTheWindowsASessionTookPointsOfAreNamedTheWeekFirst(t *testing.T) {
	tests := []struct {
		points map[string]int
		want   string
	}{
		{points: map[string]int{"7d": 6, "5h": 41}, want: "≈ 6 points of its week · 41 of its 5-hour"},
		{points: map[string]int{"5h": 1, "7d": 1, "7d_oi": 2}, want: "≈ 1 point of its week · 1 of its 5-hour · 2 of its Fable week"},
		{points: map[string]int{"5h": 0, "7d": 3}, want: "≈ 3 points of its week"},
		{points: map[string]int{"5h": 0}},
		{},
	}
	for _, tt := range tests {
		if got := (views.SessionAccount{Points: tt.points}).PointsSaid(); got != tt.want {
			t.Errorf("PointsSaid of %v = %q, want %q", tt.points, got, tt.want)
		}
	}
}

// Each window's points are words apart, the first leading with ≈ and each
// after it with ·, their points a figure, so a row short of room leaves
// whole windows off its end; a phone says them short. Each says the window
// whose key PointsKeys gives in its place, none that rounds to none.
func TestTheWindowsASessionTookPointsOfAreWordsApartWideAndShort(t *testing.T) {
	points := map[string]int{"7d": 6, "5h": 1, "7d_oi": 2, "7d_sonnet": 0}
	if got, want := (views.SessionAccount{Points: points}).PointsKeys(), []string{"7d", "5h", "7d_oi"}; !slices.Equal(got, want) {
		t.Errorf("PointsKeys = %q, want %q", got, want)
	}
	tests := []struct {
		form views.PointsForm
		want []status.Words
	}{
		{form: views.PointsWide, want: []status.Words{
			{status.Plain("≈ "), status.Figure("6"), status.Plain(" points"), status.Plain(" of its week")},
			{status.Plain(" · "), status.Figure("1"), status.Plain(" of its 5-hour")},
			{status.Plain(" · "), status.Figure("2"), status.Plain(" of its Fable week")},
		}},
		{form: views.PointsShort, want: []status.Words{
			{status.Plain("≈ "), status.Figure("6"), status.Plain(" of week")},
			{status.Plain(" · "), status.Figure("1"), status.Plain(" of 5-hour")},
			{status.Plain(" · "), status.Figure("2"), status.Plain(" of Fable week")},
		}},
	}
	for _, tt := range tests {
		if got := (views.SessionAccount{Points: points}).PointsWords(tt.form); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("PointsWords(%d) = %q, want %q", tt.form, got, tt.want)
		}
	}
}
