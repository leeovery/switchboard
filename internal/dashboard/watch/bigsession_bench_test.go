package watch

import (
	"bytes"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// The big session's shape, as the owner's longest: one session running every
// day for 37 days, some 305,000 requests in 3,000 turns, heavy-tailed, the
// median turn of 7 requests and the largest of near 2,000, its newest the
// largest of all, and some 20,000 requests naming no prompt, which a page
// counts in its totals and its story and in no turn. Of those naming one,
// 6% its own conversation's, a fifth auxiliary, one in 512 a compaction, and
// the rest its subagents', a subagent to every bigAgent of a turn's requests.
const (
	bigDays     = 37
	bigTurns    = 3000
	bigNoPrompt = 20_272
	bigAgent    = 40
	bigBand     = 233
)

// bigLargest are the sizes of the big session's largest turns, the largest
// first, as the owner's.
var bigLargest = []int{1917, 1841, 1685, 1628, 1520}

// bigSizes are how many requests each of the big session's turns takes, in
// the order they came, its largest the newest: the largest five as
// bigLargest has them; the next bigBand from 1,500 down to 460; and the
// rest falling away from 100 to one, the median turn of 7, the others in an
// order of their own.
func bigSizes() []int {
	sizes := slices.Clone(bigLargest[1:])
	for r := range bigBand {
		sizes = append(sizes, 1500-r*1040/(bigBand-1))
	}
	tail := bigTurns - len(bigLargest) - bigBand
	fall := math.Log(100.0/7) / float64(bigTurns/2-len(bigLargest)-bigBand)
	for r := range tail {
		sizes = append(sizes, max(1, int(math.Round(100*math.Exp(-fall*float64(r))))))
	}
	rand.New(rand.NewPCG(5, 8)).Shuffle(len(sizes), func(i, j int) { sizes[i], sizes[j] = sizes[j], sizes[i] })
	return append(sizes, bigLargest[0])
}

// bigID is the big session's id.
const bigID = "b16b16b1-0000-4000-8000-000000000001"

// bigWorld is a state directory of the benchmark's own holding the big
// session's ledger, its last request arriving at now, the days before the
// last three compressed, each day that has ended summarised and its digest
// built, as the router leaves them; and the number of its newest turn, and
// how many requests it has.
type bigWorld struct {
	busyWorld
	newest   int
	requests int
}

// bigRequest is one of the big session's requests: its id, when it arrived,
// its turn, numbered from 1, 0 where it names no prompt, its class, and its
// subagent, "" for none.
type bigRequest struct {
	id    uint64
	at    time.Time
	turn  int
	class string
	agent string
}

// newBigWorld writes a big world whose last request arrives at now.
func newBigWorld(tb testing.TB, now time.Time) bigWorld {
	tb.Helper()
	var w bigWorld
	w.state, w.now = tb.TempDir(), now
	requests := bigRequests(now)
	w.newest, w.requests = bigTurns, len(requests)
	dir := ledger.Dir(w.state)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		tb.Fatal(err)
	}
	days := map[string]*bytes.Buffer{}
	r := rand.New(rand.NewPCG(1, 3))
	for _, q := range requests {
		date := q.at.Format(time.DateOnly)
		if days[date] == nil {
			days[date] = new(bytes.Buffer)
		}
		days[date].WriteString(q.line(r))
	}
	recent := dayfile.DayStart(now, -2).Format(time.DateOnly)
	for date, lines := range days {
		writeDay(tb, dir, "requests-"+date+".jsonl", lines.Bytes(), date < recent)
	}
	l := w.ledger(ledger.Digesting(views.Digests{}))
	l.SummariseEnded(now)
	l.DigestEnded(tb.Context(), now)
	return w
}

// bigRequests are the big session's requests, oldest first, evenly through
// the times it works in, as bigWorking has them, to now: its turns' in turn,
// as bigSizes has them, and those naming no prompt spread evenly among them.
func bigRequests(now time.Time) []bigRequest {
	var requests []bigRequest
	sizes := bigSizes()
	prompted := 0
	for _, s := range sizes {
		prompted += s
	}
	every := prompted / bigNoPrompt
	g := 0
	for t, size := range sizes {
		for k := range size {
			requests = append(requests, bigRequest{turn: t + 1, class: bigClass(g, k), agent: bigAgentOf(t+1, k, bigClass(g, k))})
			g++
			if g%every == 0 && len(requests) < prompted+bigNoPrompt {
				requests = append(requests, bigRequest{class: "auxiliary"})
			}
		}
	}
	working := bigWorking(now)
	n, w := len(requests), len(working)
	for k, span := range working {
		from, to := k*n/w, (k+1)*n/w
		step := span.To.Sub(span.From) / time.Duration(to-from)
		for i := from; i < to; i++ {
			requests[i].id = uint64(i + 1)
			requests[i].at = span.From.Add(step * time.Duration(i-from+1))
		}
	}
	return requests
}

// bigWorking are the times the big session works in, oldest first, the
// newest ending at now: three stretches of four hours a day, an hour and a
// half apart, and nine hours of night between one day's and the next's, so
// its story folds three times a day.
func bigWorking(now time.Time) []ledger.Span {
	gaps := []time.Duration{90 * time.Minute, 90 * time.Minute, 9 * time.Hour}
	var spans []ledger.Span
	to := now
	for i := range bigDays * len(gaps) {
		from := to.Add(-4 * time.Hour)
		spans = append(spans, ledger.Span{From: from, To: to})
		to = from.Add(-gaps[i%len(gaps)])
	}
	slices.Reverse(spans)
	return spans
}

// bigClass is the class of the big session's gth request naming a prompt,
// the kth of its turn: its own conversation's, as a turn's first is, 3 of
// every 50; auxiliary, 10 of every 50; a compaction, one in 512; else a
// subagent's.
func bigClass(g, k int) string {
	switch {
	case k == 0 || g%50 < 3:
		return ledger.ClassMain
	case g%512 == 0:
		return "compaction"
	case g%50 < 13:
		return "auxiliary"
	}
	return "subagent"
}

// bigAgentOf is the subagent that sent the kth request of the big session's
// turn numbered turn, of the class given: none but a subagent's.
func bigAgentOf(turn, k int, class string) string {
	if class != "subagent" {
		return ""
	}
	return fmt.Sprintf("a%04d-%02d", turn, k/bigAgent)
}

// bigLine is the format of a line of the big session: when it arrived, its
// request's id, timings, prompt, class and subagent, size, answer's id,
// usage given, and its limits, each window's use rising through it.
const bigLine = `{"at":"%s","request":"%s","kind":"message","session":"` + bigID + `","dir":"~/Code/project-big","model":"claude-opus-5-5",` +
	`"account":"work","reason":"sticky","status":200,"attempts":1,"first_ms":%d,"total_ms":%d,"agent":"claude-cli/2.1.300 (external, cli)",` +
	`"prompt":"%s","class":"%s"%s,"shape":{"bytes":%d,"messages":%d,"system":3,"tools":31,"max_tokens":32000,"stream":true},` +
	`"answer":{"id":"req_011C%016x","model":"claude-opus-5-5","stop":"tool_use","blocks":{"text":1,"tool_use":2},"tools":["Bash","Read","Edit"]},` +
	`"usage":{"input_tokens":%d,"cache_creation_input_tokens":%d,"cache_read_input_tokens":%d,"output_tokens":%d,"service_tier":"standard"},` +
	`"limits":{"status":"allowed","5h-utilization":"%.2f","5h-reset":"%d","5h-status":"allowed","7d-utilization":"%.2f","7d-reset":"%d","7d-status":"allowed"}}` + "\n"

// line is q's line in the ledger, its figures drawn from r.
func (q bigRequest) line(r *rand.Rand) string {
	agent := ""
	if q.agent != "" {
		agent = fmt.Sprintf(`,"agent_id":%q`, q.agent)
	}
	session, week := q.at.Truncate(5*time.Hour), q.at.Truncate(7*24*time.Hour)
	return fmt.Sprintf(bigLine, q.at.UTC().Format("2006-01-02T15:04:05.000Z"), q.request(), r.IntN(5000), r.IntN(20000),
		q.prompt(), q.class, agent, r.IntN(900000), r.IntN(400), r.Uint64(),
		r.IntN(100), r.IntN(20000), r.IntN(400000), r.IntN(8000),
		float64(q.at.Sub(session))/float64(5*time.Hour), session.Add(5*time.Hour).Unix(),
		float64(q.at.Sub(week))/float64(7*24*time.Hour), week.Add(7*24*time.Hour).Unix())
}

// request is q's id, as its line and the stream give it.
func (q bigRequest) request() string {
	return fmt.Sprintf("%016x", q.id)
}

// prompt is the prompt q serves: "" where it names none.
func (q bigRequest) prompt() string {
	if q.turn == 0 {
		return ""
	}
	return fmt.Sprintf("big-%04d", q.turn)
}

// running is the router's listing of the big session at now, running on
// work.
func (w bigWorld) running(now time.Time) []status.Session {
	return []status.Session{{ID: bigID, Assignments: []status.Assignment{{Dir: "~/Code/project-big", Model: "claude-opus-5-5", Family: "opus",
		Account: "work", Reason: status.ReasonSticky, AssignedAt: w.now.Add(-time.Hour).UTC(), LastSeen: now.UTC()}}}}
}

// onProjects is the big world's harness on Projects, its page read, the
// router listing the big session.
func (w bigWorld) onProjects(tb testing.TB) *harness {
	tb.Helper()
	h := w.harnessListing(tb, views.PoolRead{}, w.running(w.now))
	h.firstShown(dashboard.Projects, func(m Model) bool { return m.projects.shown.Page != nil })
	return h
}

// onPage is the big world's harness with the big session's page open over
// Projects, as the owner opened it, its newest turn chosen.
func (w bigWorld) onPage(tb testing.TB) *harness {
	tb.Helper()
	h := w.onProjects(tb)
	h.openPage(bigID)
	h.settle()
	return h
}

// onLargest is the big world's harness with the big session's page open,
// its newest turn, its largest, opened, its requests read.
func (w bigWorld) onLargest(tb testing.TB) *harness {
	tb.Helper()
	h := w.onPage(tb)
	h.keys(enterKey)
	h.settle()
	if got := len(h.shownPage().Requests); got < bigLargest[0] {
		tb.Fatalf("the largest turn opened %d requests, want %d", got, bigLargest[0])
	}
	return h
}

// wheelOver is a turn of the wheel the way given over the session's page's
// turns, as the big world's harness draws them, 160 by 37.
func wheelOver(button tea.MouseButton) tea.MouseWheelMsg {
	return tea.MouseWheelMsg{X: 10, Y: 30, Button: button}
}

// notched turns the wheel over the page's turns, twenty notches down then
// twenty up, as many times as b loops, the screen drawn after each, as
// Bubble Tea draws it after every message.
func (h *harness) notched(b *testing.B) {
	n := 0
	for b.Loop() {
		button := tea.MouseWheelDown
		if n%40 >= 20 {
			button = tea.MouseWheelUp
		}
		n++
		h.timers = nil
		h.deliver(h.update(wheelOver(button))...)
		_ = h.model.View()
	}
}

// busy is the big session running on as its newest turn's busiest, the
// ledger and the stream telling of its requests: 1.6 a second, a subagent's
// each, every one done 20 seconds after it's sent, its line filed as it's
// done.
type busy struct {
	w      bigWorld
	h      *harness
	sent   []bigRequest
	next   uint64
	second int
}

// busyOn is the big session busy on h, its requests numbered on from the
// world's.
func (w bigWorld) busyOn(h *harness) *busy {
	return &busy{w: w, h: h, next: uint64(w.requests) + 1}
}

// told is the stream's event of the kind given of q, at the harness's now.
func (b *busy) told(kind string, q bigRequest) router.StreamEvent {
	e := router.StreamEvent{At: b.h.clock.now, Kind: kind, Request: q.request(), Attempt: 1, Session: bigID, Model: "claude-opus-5-5",
		Account: "work", Prompt: q.prompt(), Class: q.class, AgentID: q.agent}
	if kind == router.StreamDone {
		e.Status, e.Chars = 200, 900
	}
	return e
}

// send has the stream tell of a request of the newest turn sent.
func (b *busy) send() {
	q := bigRequest{id: b.next, turn: b.w.newest, class: "subagent", agent: fmt.Sprintf("a%04d-live-%d", b.w.newest, b.next%4)}
	b.next++
	q.at = b.h.clock.now
	b.sent = append(b.sent, q)
	b.h.hear(b.told(router.StreamSent, q), b.told(router.StreamFirst, q))
}

// on runs a second on: the clock ticks; the requests sent 20 seconds before
// are done, their lines filed and the stream telling of it; one or two are
// sent, 1.6 a second; every fifth second the router is looked at, its look
// reading the ledger's tail; and each frame the page asks for in the second
// is drawn, as Bubble Tea draws 30 a second while anything on screen moves.
// The screen is drawn after each message.
func (b *busy) on(tb testing.TB) {
	h := b.h
	second := h.clock.now
	b.second++
	h.deliver(tickMsg{chain: h.model.chain})
	_ = h.model.View()
	var done []bigRequest
	for len(b.sent) > 0 && !b.sent[0].at.After(h.clock.now.Add(-20*time.Second)) {
		done, b.sent = append(done, b.sent[0]), b.sent[1:]
	}
	if len(done) > 0 {
		b.filed(tb, done)
		events := make([]router.StreamEvent, len(done))
		for i, q := range done {
			events[i] = b.told(router.StreamDone, q)
		}
		h.hear(events...)
		_ = h.model.View()
	}
	sends := 1
	if b.second%5 < 3 {
		sends = 2
	}
	for range sends {
		b.send()
		_ = h.model.View()
	}
	if b.second%5 == 0 {
		h.deliver(fetchedMsg{doc: busyDocument(), sessions: b.w.running(h.clock.now), listed: true})
		_ = h.model.View()
	}
	for tm, ok := h.pendingFrame(); ok && tm.due.Before(second.Add(time.Second)); tm, ok = h.pendingFrame() {
		h.fire(tm)
		_ = h.model.View()
	}
	h.timers = slices.DeleteFunc(h.timers, func(tm *timer) bool {
		_, frame := tm.msg.(frameMsg)
		return tm.fired || !frame
	})
	h.clock.now = second.Add(time.Second)
}

// warm runs the session busy for a minute, so as many of its requests are in
// flight as stay so while it's busy.
func (b *busy) warm(tb testing.TB) {
	for range 60 {
		b.on(tb)
	}
}

// filed appends the lines of the requests given to today's file, each
// arriving now.
func (b *busy) filed(tb testing.TB, done []bigRequest) {
	tb.Helper()
	now := b.h.clock.now
	f, err := os.OpenFile(filepath.Join(ledger.Dir(b.w.state), "requests-"+now.Format(time.DateOnly)+".jsonl"), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		tb.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	r := rand.New(rand.NewPCG(b.next, 9))
	for _, q := range done {
		q.at = now
		if _, err := f.WriteString(q.line(r)); err != nil {
			tb.Fatal(err)
		}
	}
}

// BenchmarkTheBigSessionsPage times the page of a session of the shape of
// the owner's longest, as newBigWorld writes it, opened over Projects while
// it runs: opening it, till its lines show; a turn of the wheel over its
// turns; opening its largest turn, its newest, till its requests show; a
// turn of the wheel over that turn's requests; and, while it's busy at its
// peak of 1.6 requests a second, a tick of the clock, a second on, a frame,
// a request of it sent, and a whole second of it, with no turn open and
// with its largest open. Each is drawn after, as Bubble Tea draws the screen
// after every message. The harness draws without colour, so no number
// brightens and no second asks for frames: a frame is timed alone, and runs
// 30 a second while anything on the page moves.
func BenchmarkTheBigSessionsPage(b *testing.B) {
	w := newBigWorld(b, start)
	b.Run("opening the page", func(b *testing.B) {
		h := w.onProjects(b)
		on := h.model
		for b.Loop() {
			h.model, h.timers, h.waits = on, nil, nil
			h.openPage(bigID)
			_ = h.model.View()
		}
	})
	b.Run("a notch over its turns", func(b *testing.B) {
		w.onPage(b).notched(b)
	})
	b.Run("opening its largest turn", func(b *testing.B) {
		h := w.onPage(b)
		on := h.model
		for b.Loop() {
			h.model, h.timers = on, nil
			h.keys(enterKey)
			_ = h.model.View()
		}
		if got := len(h.shownPage().Requests); got < bigLargest[0] {
			b.Fatalf("the largest turn opened %d requests, want %d", got, bigLargest[0])
		}
	})
	b.Run("a notch over its largest turn's requests", func(b *testing.B) {
		w.onLargest(b).notched(b)
	})
	b.Run("a tick, busy", func(b *testing.B) {
		busy := w.busyOn(w.onLargest(b))
		busy.warm(b)
		h := busy.h
		for b.Loop() {
			h.clock.now = h.clock.now.Add(time.Second)
			h.timers = nil
			h.deliver(tickMsg{chain: h.model.chain})
			_ = h.model.View()
		}
	})
	b.Run("a frame, busy", func(b *testing.B) {
		busy := w.busyOn(w.onLargest(b))
		busy.warm(b)
		h := busy.h
		for b.Loop() {
			h.clock.now = h.clock.now.Add(frameEvery)
			h.timers = nil
			h.deliver(frameMsg{gen: h.model.frames})
			_ = h.model.View()
		}
	})
	b.Run("a request of it sent, busy", func(b *testing.B) {
		busy := w.busyOn(w.onLargest(b))
		busy.warm(b)
		h := busy.h
		for b.Loop() {
			h.clock.now = h.clock.now.Add(time.Second / 10)
			h.timers = nil
			busy.send()
			_ = h.model.View()
		}
	})
	for _, on := range []struct {
		name   string
		harned func(testing.TB) *harness
	}{{"with no turn open", w.onPage}, {"with its largest turn open", w.onLargest}} {
		b.Run("a second of it busy "+on.name, func(b *testing.B) {
			busy := w.busyOn(on.harned(b))
			busy.warm(b)
			for b.Loop() {
				busy.on(b)
			}
		})
	}
}
