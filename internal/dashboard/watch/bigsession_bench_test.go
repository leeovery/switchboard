package watch

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"os"
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

// The big session's size, as the owner's longest: one session running every
// day for forty days, 300,000 requests among them, each turn's prompt served
// by its own conversation's requests and by three subagents', most of them
// the subagents'.
const (
	bigDays     = 40
	bigRequests = 300_000 / bigDays
	// bigTurn is how many requests a turn takes, bigMain of every bigStride
	// its own conversation's, the rest bigAgents subagents' in turn.
	bigTurn   = 125
	bigMain   = 7
	bigStride = 25
	bigAgents = 3
)

// bigID is the big session's id.
const bigID = "b16b16b1-0000-4000-8000-000000000001"

// bigWorld is a state directory of the benchmark's own holding the big
// session's ledger, its last day the one now falls on, to now, the days
// before the last two compressed, each day that has ended summarised and
// its digest built, as the router leaves them.
type bigWorld struct {
	busyWorld
}

// newBigWorld writes a big world whose today ends at now.
func newBigWorld(tb testing.TB, now time.Time) bigWorld {
	tb.Helper()
	w := bigWorld{busyWorld{state: tb.TempDir(), now: now}}
	dir := ledger.Dir(w.state)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		tb.Fatal(err)
	}
	for d := range bigDays {
		day := dayfile.DayStart(now, d-bigDays+1)
		writeDay(tb, dir, "requests-"+day.Format(time.DateOnly)+".jsonl", bigLines(day, d, now), d < bigDays-3)
	}
	l := w.ledger(ledger.Digesting(views.Digests{}))
	l.SummariseEnded(now)
	l.DigestEnded(tb.Context(), now)
	return w
}

// bigLine is the format of a line of the big session: when it arrived, its
// request's id, timings, prompt, class and subagent, size, answer's id, and
// usage given.
const bigLine = `{"at":"%s","request":"%016x","kind":"message","session":"` + bigID + `","dir":"~/Code/project-big","model":"claude-opus-5-5",` +
	`"account":"work","reason":"sticky","status":200,"attempts":1,"first_ms":%d,"total_ms":%d,"agent":"claude-cli/2.1.300 (external, cli)",` +
	`"prompt":"%s","class":"%s"%s,"shape":{"bytes":%d,"messages":%d,"system":3,"tools":31,"max_tokens":32000,"stream":true},` +
	`"answer":{"id":"req_011C%016x","model":"claude-opus-5-5","stop":"tool_use","blocks":{"text":1,"tool_use":2},"tools":["Bash","Read","Edit"]},` +
	`"usage":{"input_tokens":%d,"cache_creation_input_tokens":%d,"cache_read_input_tokens":%d,"output_tokens":%d,"service_tier":"standard"}}` + "\n"

// bigLines are the big session's lines of the world's dth day, which starts
// at start, that arrived by now: bigRequests through the day, a turn every
// bigTurn of them.
func bigLines(start time.Time, d int, now time.Time) []byte {
	r := rand.New(rand.NewPCG(uint64(d), 3))
	var day bytes.Buffer
	for i := range bigRequests {
		at := start.Add(time.Duration(i) * 24 * time.Hour / bigRequests)
		if at.After(now) {
			break
		}
		turn, k := i/bigTurn, i%bigTurn
		class, agent := ledger.ClassMain, ""
		if k%bigStride >= bigMain {
			class, agent = "subagent", fmt.Sprintf(`,"agent_id":"a%02d%03d%d"`, d, turn, k%bigAgents)
		}
		fmt.Fprintf(&day, bigLine, at.UTC().Format("2006-01-02T15:04:05.000Z"), r.Uint64(), r.IntN(5000), r.IntN(90000),
			bigPrompt(d, turn), class, agent, r.IntN(900000), r.IntN(400), r.Uint64(),
			r.IntN(100), r.IntN(20000), r.IntN(400000), r.IntN(8000))
	}
	return day.Bytes()
}

// bigPrompt is the prompt of the big world's dth day's nth turn.
func bigPrompt(d, n int) string {
	return fmt.Sprintf("big-%02d-%03d", d, n)
}

// running is the router's listing of the big session, running on work.
func (w bigWorld) running() []status.Session {
	return []status.Session{{ID: bigID, Assignments: []status.Assignment{{Dir: "~/Code/project-big", Model: "claude-opus-5-5", Family: "opus",
		Account: "work", Reason: status.ReasonSticky, AssignedAt: w.now.Add(-time.Hour).UTC(), LastSeen: w.now.UTC()}}}}
}

// onProjects is the big world's harness on Projects, its page read, the
// router listing the big session.
func (w bigWorld) onProjects(tb testing.TB) *harness {
	tb.Helper()
	h := w.harnessListing(tb, views.PoolRead{}, w.running())
	h.firstShown(dashboard.Projects, func(m Model) bool { return m.projects.shown.Page != nil })
	return h
}

// onPage is the big world's harness with the big session's page open over
// Projects, as the owner opened it, a request of its newest turn streaming.
func (w bigWorld) onPage(tb testing.TB) *harness {
	tb.Helper()
	h := w.onProjects(tb)
	h.openPage(bigID)
	h.hear(w.told(router.StreamSent, "streaming"), w.told(router.StreamFirst, "streaming"))
	h.settle()
	return h
}

// told is the request stream's event of the kind given of the request with
// the given id, of the big session's newest turn, at the harness's now.
func (w bigWorld) told(kind, request string) router.StreamEvent {
	return router.StreamEvent{At: w.now, Kind: kind, Request: request, Attempt: 1, Session: bigID, Model: "claude-opus-5-5", Account: "work",
		Prompt: bigPrompt(bigDays-1, 0), Class: ledger.ClassMain}
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

// BenchmarkTheBigSessionsPage times the page of a session as big as the
// owner's longest, as newBigWorld writes it, opened over Projects while it
// runs: opening it, till its lines show; a turn of the wheel over its
// turns, and over the requests of the turn before its newest, opened; a tick of the
// clock, a second on; and a request of it sent. Each is drawn after, as
// Bubble Tea draws the screen after every message.
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
	h := w.onPage(b)
	b.Run("a notch over its turns", func(b *testing.B) {
		h.notched(b)
	})
	b.Run("a tick", func(b *testing.B) {
		for b.Loop() {
			h.clock.now = h.clock.now.Add(time.Second)
			h.timers = nil
			h.deliver(tickMsg{chain: h.model.chain})
			_ = h.model.View()
		}
	})
	b.Run("a request of it sent", func(b *testing.B) {
		n := 0
		for b.Loop() {
			n++
			h.clock.now = h.clock.now.Add(time.Second / 10)
			h.timers = nil
			e := w.told(router.StreamSent, fmt.Sprintf("sent-%06d", n))
			e.At = h.clock.now
			h.hear(e)
			_ = h.model.View()
		}
	})
	turn := w.onPage(b)
	turn.keys(downKey, enterKey)
	turn.settle()
	if len(turn.shownPage().Requests) <= bigTurn/2 {
		b.Fatalf("the newest turn opened %d requests, want more than a screen of them", len(turn.shownPage().Requests))
	}
	b.Run("a notch over a turn's requests", func(b *testing.B) {
		turn.notched(b)
	})
}
