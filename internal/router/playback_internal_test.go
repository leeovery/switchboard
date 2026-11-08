package router

import (
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/forecast"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens/tokenstest"
)

// windowAt is the window with the given key read at utilization, resetting
// at resets.
func windowAt(key string, utilization float64, resets time.Time) quota.Window {
	return quota.Window{Key: key, Utilization: utilization, ResetsAt: resets}
}

// played is what a document gives of an account that a playback is to give
// as the router does: its windows as last read, but for their labels, which
// the readings history doesn't keep; when it was last read; the rates of its
// windows; and the pressure that puts it under.
type played struct {
	windows  []quota.Window
	readAt   time.Time
	rates    []status.Rate
	pressure status.Pressure
}

// playedOf is what doc gives of each of its accounts, as played has it, by
// the account's id.
func playedOf(doc status.Document) map[string]played {
	of := make(map[string]played, len(doc.Accounts))
	for _, a := range doc.Accounts {
		windows := make([]quota.Window, len(a.Windows))
		for i, w := range a.Windows {
			windows[i] = quota.Window{Key: w.Key, Utilization: w.Utilization, ResetsAt: w.ResetsAt, Status: w.Status, RestartedAt: w.RestartedAt}
		}
		of[a.ID] = played{windows: windows, readAt: a.FetchedAt, rates: a.Rates, pressure: a.Pressure}
	}
	return of
}

func TestAPlaybackOfTheReadingsHistoryGivesTheRatesAndDemandTheRouterHad(t *testing.T) {
	// Work, on Max, keeps a tenth back; personal is on Pro and side on Ultra,
	// all three with tokens. Work's week reaches its reserve two hours in,
	// and side's its limit an hour later, which leaves the pool no room till
	// side's week resets six hours in, unread till seven; work's session
	// resets by its time, then by hand; personal is read from nine hours in.
	configured := planned()
	configured[0].Reserve = 0.1
	clock := &testClock{now: start}
	live := newState(resolve(configured, tokenstest.Files{"work": workToken, "personal": personalToken, "side": sideToken}.Read, unrecorded),
		testPolicy, claude.Provider{}.Family, clock.read, unkept, unkept)
	var heard []readings.Reading
	live.history = func(read []readings.Reading) { heard = append(heard, read...) }
	p := NewPlayback(configured, testPolicy)
	read := func(after time.Duration, id string, windows ...quota.Window) {
		clock.now = start.Add(after)
		live.record(id, windows, live.mark())
	}
	compare := func(after time.Duration) {
		t.Helper()
		clock.now = start.Add(after)
		for _, r := range heard {
			p.Read(r)
		}
		heard = nil
		want, got := live.document(), p.Document(clock.now)
		if !sameDemand(got.Demand, want.Demand) || got.Demand != nil && !reflect.DeepEqual(got.Demand.Hold, want.Demand.Hold) {
			t.Errorf("%v in, the playback's demand is %+v, want the router's, %+v", after, got.Demand, want.Demand)
		}
		if got, want := playedOf(got), playedOf(want); !reflect.DeepEqual(got, want) {
			t.Errorf("%v in, the playback's accounts are\n%+v\nwant the router's\n%+v", after, got, want)
		}
	}
	weekResets, sideResets := start.Add(5*24*time.Hour), start.Add(6*time.Hour)
	sessionResets := start.Add(5 * time.Hour)

	read(0, "work", windowAt("5h", 0.1, sessionResets), windowAt("7d", 0.4, weekResets))
	read(0, "side", windowAt("7d", 0.2, sideResets))
	read(10*time.Minute, "work", windowAt("5h", 0.2, sessionResets), windowAt("7d", 0.42, weekResets))
	read(time.Hour, "side", windowAt("7d", 0.25, sideResets))
	read(2*time.Hour, "work", windowAt("7d", 0.9, weekResets))
	compare(2*time.Hour + 30*time.Minute)
	read(3*time.Hour, "side", quota.Window{Key: "7d", Utilization: 1, ResetsAt: sideResets, Status: quota.StatusRejected})
	compare(5 * time.Hour)
	read(5*time.Hour+10*time.Minute, "work", windowAt("5h", 0.05, start.Add(10*time.Hour+10*time.Minute)))
	compare(6*time.Hour + 30*time.Minute)
	read(7*time.Hour, "side", windowAt("7d", 0.01, sideResets.Add(7*24*time.Hour)))
	read(7*time.Hour+30*time.Minute, "work", windowAt("5h", 0.3, start.Add(10*time.Hour+10*time.Minute)))
	read(8*time.Hour, "work", windowAt("5h", 0.05, start.Add(10*time.Hour+10*time.Minute)))
	compare(8*time.Hour + 30*time.Minute)
	read(9*time.Hour, "personal", windowAt("7d", 0.3, start.Add(3*24*time.Hour)))
	read(10*time.Hour, "personal", windowAt("7d", 0.35, start.Add(3*24*time.Hour)))
	compare(10*time.Hour + 30*time.Minute)
	compare(30 * time.Hour)
}

func TestAPlaybackPassesOverAReadingOfAnAccountNotConfigured(t *testing.T) {
	p := NewPlayback(testConfigured, testPolicy)
	p.Read(readings.Reading{At: start, Account: "gone", Key: "7d", Utilization: 0.5, ResetsAt: week.ResetsAt, Source: readings.FromProbe})

	doc := p.Document(start.Add(time.Hour))
	if doc.Demand != nil {
		t.Errorf("the demand is %+v, want none, as no account configured has been read", doc.Demand)
	}
	if _, ok := doc.Account("gone"); ok {
		t.Error("the document has the account read but not configured, want it left out")
	}
}

// thousandIn is an answer's usage of a thousand tokens in, which weighs 1000.
var thousandIn = json.RawMessage(`{"input_tokens":1000}`)

// answerLine is the line of a message of session, on opus, that arrived at at
// and was answered on account, its usage given, in total ms.
func answerLine(at time.Time, session, account string, usage json.RawMessage, totalMS int64) ledger.Line {
	return ledger.Line{
		At: at, Request: "r-" + at.Format(time.RFC3339Nano) + session + account, Kind: ledger.KindMessage, Session: session, Model: opus,
		Account: account, Status: http.StatusOK, Attempts: 1, TotalMS: totalMS, Usage: usage,
	}
}

func TestAPlaybackWeighsEachAnswerByItsSessionsTimeOnItsAccountAtItsEnd(t *testing.T) {
	// Session one comes to work, stays there, and moves to side, which a
	// quota check of it doesn't do, then comes back to side after more than
	// a week unused; a request without a session, a count of tokens, and an
	// answer that gave no usage come between.
	minute := int64(time.Minute / time.Millisecond)
	lines := []ledger.Line{
		answerLine(start, "one", "work", thousandIn, minute),
		answerLine(start.Add(2*time.Hour), "one", "work", thousandIn, 0),
		edited(answerLine(start.Add(3*time.Hour), "one", "side", thousandIn, 0), func(l *ledger.Line) { l.Kind = ledger.KindCheck }),
		answerLine(start.Add(3*time.Hour+30*time.Minute), "one", "work", thousandIn, 0),
		answerLine(start.Add(4*time.Hour), "one", "side", thousandIn, 0),
		edited(answerLine(start.Add(5*time.Hour), "one", "side", thousandIn, 0), func(l *ledger.Line) { l.Kind = ledger.KindCount }),
		answerLine(start.Add(5*time.Hour), "one", "side", nil, 0),
		answerLine(start.Add(10*time.Hour), "one", "side", thousandIn, 0),
		answerLine(start.Add(10*time.Hour), "", "work", thousandIn, 0),
		answerLine(start.Add(10*time.Hour+8*24*time.Hour), "one", "side", thousandIn, 0),
	}
	p := NewPlayback(testConfigured, testPolicy)
	for _, l := range lines {
		p.Line(l)
	}
	p.Document(start.Add(9 * 24 * time.Hour))

	// Each answer weighs a thousand, at its end, from a session on its
	// account so long: none, then two hours, kept on work; none for the
	// check, never remembered on side; three and a half hours still on work;
	// none, as it came to side; six hours there; none without a session; and
	// none, as it came back to side afresh, a week unused.
	answers := []struct {
		after, onAccount time.Duration
	}{
		{time.Minute, time.Minute},
		{2 * time.Hour, 2 * time.Hour},
		{3 * time.Hour, 0},
		{3*time.Hour + 30*time.Minute, 3*time.Hour + 30*time.Minute},
		{4 * time.Hour, 0},
		{10 * time.Hour, 6 * time.Hour},
		{10 * time.Hour, 0},
		{10*time.Hour + 8*24*time.Hour, 0},
	}
	var want forecast.Hold
	for _, a := range answers {
		want = want.Answered(1000, a.onAccount, start.Add(a.after))
	}
	if got := p.state.hold; got != want {
		t.Errorf("the hold curve is %+v, want %+v", got, want)
	}
}

func TestAPlaybacksDocumentHoldsTheAnswersEndedByItsTime(t *testing.T) {
	// Session one's request on work arrives at the start, and its answer ends
	// two hours on.
	p := NewPlayback(testConfigured, testPolicy)
	p.Line(answerLine(start, "one", "work", thousandIn, int64(2*time.Hour/time.Millisecond)))

	p.Document(start.Add(time.Hour))
	if got := p.state.hold; got != (forecast.Hold{}) {
		t.Errorf("an hour on, the hold curve is %+v, want none taken in: the answer is yet to end", got)
	}
	p.Document(start.Add(2 * time.Hour))
	if got, want := p.state.hold, (forecast.Hold{}).Answered(1000, 2*time.Hour, start.Add(2*time.Hour)); got != want {
		t.Errorf("two hours on, the hold curve is %+v, want %+v: the answer, from a session two hours on work", got, want)
	}
}

func TestAPlaybackOfTheLedgerHoldsTheAnswersAsTheRouterDid(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Work and side have room, and side's quota needs using sooner. Over a
		// day and more, two sessions start, one is moved off side at a limit
		// and chosen afresh later, and a request without a session, a count
		// of tokens and a quota check come between.
		clock := &setClock{now: start}
		r := newTestRouter(t, clock.read, &stubProber{readings: map[string]quota.Probe{
			workToken: probed(nil, session, laterWeek), sideToken: probed(nil, session, soonWeek),
		}})
		r.state.record("work", []quota.Window{session, laterWeek}, r.state.mark())
		r.state.record("side", []quota.Window{session, soonWeek}, r.state.mark())
		r.proxy.transport = &scriptedUpstream{answers: map[string][]answer{workToken: {messageWhole}, sideToken: {messageWhole}}}
		lines := keepingLedger(t, r)
		routeAt := func(after time.Duration, path, session, body string) {
			clock.set(start.Add(after))
			if rec := routeAs(t.Context(), r, path, session, body); rec.Code != http.StatusOK {
				t.Fatalf("%v in, a request of session %q answered %d, want 200", after, session, rec.Code)
			}
		}
		routeAt(0, "/v1/messages", "one", opusAsked)
		routeAt(2*time.Hour, "/v1/messages", "one", opusAsked)
		routeAt(3*time.Hour, "/v1/messages", "two", opusAsked)
		clock.set(start.Add(3 * time.Hour))
		r.state.limit("side", nil, start.Add(4*time.Hour), r.state.mark())
		routeAt(3*time.Hour, "/v1/messages", "one", opusAsked)
		routeAt(5*time.Hour, "/v1/messages", "one", opusAsked)
		routeAt(26*time.Hour, "/v1/messages", "one", opusAsked)
		routeAt(26*time.Hour, "/v1/messages", "", opusAsked)
		routeAt(26*time.Hour, "/v1/messages/count_tokens", "two", opusAsked)
		routeAt(26*time.Hour, "/v1/messages", "two", `{"model":"`+haiku+`","max_tokens":1,"messages":[{"role":"user","content":"quota"}]}`)
		routeAt(27*time.Hour, "/v1/messages", "two", opusAsked)
		want := r.state.hold

		held := lines()
		if accounts := answeredOn(held); !slices.Contains(accounts, "work") || !slices.Contains(accounts, "side") {
			t.Fatalf("the requests went to %v, want both work and side, to hold sessions moving between them", accounts)
		}
		p := NewPlayback(testConfigured, testPolicy)
		for _, l := range held {
			p.Line(l)
		}
		p.Document(clock.read())
		if got := p.state.hold; got != want {
			t.Errorf("played back, the hold curve is %+v, want the router's, %+v", got, want)
		}
	})
}

// answeredOn are the accounts the lines' requests were answered on, each
// once, in the order first answered.
func answeredOn(lines []ledger.Line) []string {
	var accounts []string
	for _, l := range lines {
		if !slices.Contains(accounts, l.Account) {
			accounts = append(accounts, l.Account)
		}
	}
	return accounts
}
