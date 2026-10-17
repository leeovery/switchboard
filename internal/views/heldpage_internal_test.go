package views

import (
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
)

// Past midnight, before a feed of the store rolls its session on, a held
// page is built of its session rolled on to the new day as it's looked at,
// as the store rolls a look at a later day: the page the feed then gives,
// which rolls the session held, its prices alike on both days. A look of
// the day before, as a ledger yet to turn gives, never rolls it back.
func TestAHeldPagePastMidnightIsBuiltAsTheFeedThatRollsItGives(t *testing.T) {
	before := time.Date(2026, 10, 7, 23, 50, 0, 0, time.Local)
	after := before.Add(20 * time.Minute)
	lines := []ledger.Line{{
		Request: "r1", At: before, Kind: ledger.KindMessage, Session: "s1", Model: "claude-opus-5-5", Account: "work",
		Status: 200, Attempts: 1, TotalMS: 4000, Usage: []byte(`{"input_tokens":1000}`),
	}}
	h := HoldPage("s1", SessionsRead{day: Today(before), lines: map[string][]ledger.Line{"s1": lines}}, ledger.Pricing, 0, before)
	looked, _ := h.Build(PageLive{Now: after})
	fed, _ := h.Found().Fed(filedLedger{lines: lines, now: after}, Taken{}, SessionsWanted{}.Paging(h, Stream{}), after)
	rolled, _ := h.On(Today(after).Date)
	rolled, changed := rolled.Held(fed)
	if !changed || fed.Holding().Read > 0 {
		t.Fatalf("the feed past midnight changed the page %t, reading %d requests whole; want it changed, rolled on, none read", changed, fed.Holding().Read)
	}
	if got, _ := rolled.Build(PageLive{Now: after}); !reflect.DeepEqual(got, looked) {
		t.Errorf("past midnight, the page rolled by a feed is\n%+v\nwant it as looked at before the feed\n%+v", got, looked)
	}
	if lags, _ := rolled.On(Today(before).Date); lags.day != rolled.day {
		t.Errorf("a look of the day before, as a ledger yet to turn gives, held the page at %s, want it at %s", lags.day.Date, rolled.day.Date)
	}
}

// A page's story tells when its sealed requests went upstream as they were
// sealed, making only its open requests' times as it's built: built of 500
// requests sealed, its page allocates as often as of 50, and tells each.
func TestAPagesStoryMakesItsSealedRequestsTimesOnce(t *testing.T) {
	at := time.Date(2026, 10, 7, 9, 0, 0, 0, time.Local)
	built := func(n int) (BuiltPage, func()) {
		lines := make([]ledger.Line, n)
		for i := range lines {
			lines[i] = ledger.Line{
				Request: "r" + strconv.Itoa(i), At: at.Add(time.Duration(i) * 10 * time.Second), Kind: ledger.KindMessage, Session: "s1", Model: "claude-opus-5-5",
				Account: "work", Status: 200, Attempts: 1, TotalMS: 4000, Class: ledger.ClassMain, Prompt: "p1", Usage: []byte(`{"input_tokens":1000}`),
			}
		}
		now := lines[n-1].At.Add(3 * time.Hour)
		open := ledger.Line{Request: "open", At: now.Add(-time.Minute), Kind: ledger.KindMessage, Session: "s1", Model: "claude-opus-5-5", Account: "work",
			Status: 200, Attempts: 1, TotalMS: 4000, Class: ledger.ClassMain, Prompt: "p1", Usage: []byte(`{"input_tokens":1000}`)}
		page := ReadPage(filedLedger{lines: append(lines, open), now: now}, "s1", Stream{}, ledger.Pricing, 0, now)
		live := PageLive{Now: now}
		b, _ := page.Build(live)
		return b, func() { page.Build(live) }
	}
	few, buildFew := built(50)
	many, buildMany := built(500)
	if len(few.Story.Sent) != 51 || len(many.Story.Sent) != 501 {
		t.Fatalf("the stories tell %d and %d requests sent, want 51 and 501", len(few.Story.Sent), len(many.Story.Sent))
	}
	if fewer, got := testing.AllocsPerRun(10, buildFew), testing.AllocsPerRun(10, buildMany); got != fewer {
		t.Errorf("a page of 500 requests sealed allocates %.0f times as it's built, of 50 %.0f; want as many, whatever it sealed", got, fewer)
	}
}

// A page's points take another session's answers in working out only what
// isn't sealed: holding its session's every answer sealed, they let go of
// them all, and take another's in allocating as much whether they sealed 50
// or 500. Each rise is shared among the requests that ended before it, the
// sealed among them: another's answer on work between the session's two
// takes its share of work's 5-hour from 5 points to 3.
func TestAPagesPointsTakeAReadInWorkingOutOnlyWhatsOpen(t *testing.T) {
	at := time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local)
	day := Today(at)
	reset := strconv.FormatInt(at.Add(3*time.Hour).Unix(), 10)
	answerOf := func(request, session string, d time.Duration, used string) answered {
		l := ledger.Line{
			Request: request, At: at.Add(d), Kind: ledger.KindMessage, Session: session, Model: "claude-opus-5-5", Account: "work",
			Status: 200, Attempts: 1, TotalMS: 4000, Usage: []byte(`{"input_tokens":1000}`),
			Limits: map[string]string{"5h-utilization": used, "5h-reset": reset},
		}
		return answeredOf(&l, "s1")
	}
	pointed := func(n int, horizon instant) *pointing {
		answers := make([]answered, n)
		for i := range answers {
			used := strconv.FormatFloat(0.10+0.05*float64(i)/float64(n-1), 'f', 4, 64)
			answers[i] = answerOf("s"+strconv.Itoa(i), "s1", time.Duration(i-n)*time.Minute, used)
		}
		return pointingOf(map[string][]answered{day.Date: answers}, day).sealedAt(horizon, day)
	}
	sealedBy := instantOf(at.Add(-time.Second))
	theirs := []answered{answerOf("o1", "s2", time.Minute, "0.16")}
	few, many := pointed(50, sealedBy), pointed(500, sealedBy)
	if held := len(many.held[day.Date].all()); held != 0 || many.folded[day.Date] != 500 {
		t.Errorf("sealed behind its answers, the points hold %d of them, let go of %d; want none held, all 500 let go of", held, many.folded[day.Date])
	}
	taken := func(p *pointing) func() {
		return func() {
			took, _ := p.took(day.Date, theirs, false, day)
			took.sealedAt(sealedBy, day)
		}
	}
	fewer := testing.AllocsPerRun(10, taken(few))
	if got := testing.AllocsPerRun(10, taken(many)); got != fewer {
		t.Errorf("taking another's answer in allocates %.0f times having sealed 500 answers, %.0f having sealed 50; want as many, whatever it sealed", got, fewer)
	}

	open := pointed(2, unsealed)
	if got := open.of("work")["5h"]; got != 5 {
		t.Fatalf("s1 took %d points of work's 5-hour, want 5", got)
	}
	shared, ok := open.took(day.Date, []answered{answerOf("o3", "s2", -90*time.Second, "0.12")}, false, day)
	if got := shared.sealedAt(unsealed, day).of("work")["5h"]; !ok || got != 3 {
		t.Errorf("taking another's answer on work between its two, s1's points of work's 5-hour are %d, placed %t; want 3", got, ok)
	}
}
