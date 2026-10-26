package watch

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// The busy install's size, as the owner's: four accounts, a dozen sessions
// running at a time, one of them resumed every day for the four weeks of
// the ledger and the rest each for three days, and 5,000 requests a day.
const (
	busyDays     = 28
	busyRequests = 5000
	busyRunning  = 12
	busyLasts    = 3
	// busyStarting is how many sessions start a day, and busyLong the one
	// resumed every day.
	busyStarting = (busyRunning - 1) / busyLasts
	busyLong     = -1
)

// busyAccounts are the busy install's accounts.
var busyAccounts = []string{"work", "personal", "side", "client"}

// busyWorld is a state directory of the benchmark's own, as busy as the
// owner's install, its last day the one now falls on, to now: the ledger's
// day files, the days before the last two compressed, each day that has
// ended summarised, as the router leaves them; the readings history; and
// the router's events.
type busyWorld struct {
	state string
	now   time.Time
}

// newBusyWorld writes a busy world whose today ends at now.
func newBusyWorld(tb testing.TB, now time.Time) busyWorld {
	tb.Helper()
	w := busyWorld{state: tb.TempDir(), now: now}
	dir, history := ledger.Dir(w.state), readings.Dir(w.state)
	for _, d := range []string{dir, history} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			tb.Fatal(err)
		}
	}
	for d := range busyDays {
		day := w.day(d)
		date := day.Format(time.DateOnly)
		compressed := d < busyDays-3
		writeDay(tb, dir, "requests-"+date+".jsonl", busyLines(day, d, now), compressed)
		writeDay(tb, history, "readings-"+date+".jsonl", busyReadings(tb, day, now), compressed)
		writeDay(tb, dir, "events-"+date+".jsonl", busyEvents(tb, day, d, now), compressed)
	}
	quiet := slog.New(slog.DiscardHandler)
	h := ledger.HistoryIn(readings.Files(history, quiet))
	ledger.Open(dir, 400*24*time.Hour, w.clock, h, w.caps(), quiet).SummariseEnded(now)
	return w
}

// day is the start of the world's dth day, the last being today.
func (w busyWorld) day(d int) time.Time {
	return dayfile.DayStart(w.now, d-busyDays+1)
}

func (w busyWorld) clock() time.Time {
	return w.now
}

func (w busyWorld) caps() ledger.Caps {
	return ledger.Caps{Shared: policy.Shared}
}

// readers are the world's readers, as the dashboard's are, by clock.
func (w busyWorld) readers(clock func() time.Time) (*ledger.Follower, *readings.Reader, *events.Reader) {
	quiet := slog.New(slog.DiscardHandler)
	return ledger.NewFollower(w.state, clock, w.caps(), views.DigestVersion, quiet), readings.NewReader(w.state, quiet), events.NewReader(w.state, clock, quiet)
}

// busySeat is the session of the world's dth day's jth seat of those
// running: the long one, at the first; else the gth of those that run three
// days, from its first day, as busyFirst has it.
func busySeat(d, j int) int {
	if j == 0 {
		return busyLong
	}
	return busyStarting*d + j - 1
}

// busyFirst is the first day of the world's gth session.
func busyFirst(g int) int {
	if g == busyLong {
		return 0
	}
	return g/busyStarting - busyLasts + 1
}

// busySession is the id of the world's gth session.
func busySession(g int) string {
	r := rand.New(rand.NewPCG(uint64(g), 7))
	return fmt.Sprintf("%08x-%04x-4%03x-8%03x-%012x", r.Uint32(), r.Uint32()&0xffff, r.Uint32()&0xfff, r.Uint32()&0xfff, r.Uint64()&0xffffffffffff)
}

// busyOn is the account the world's gth session runs on on its dth day: a
// day on, another, as it's rescored after a night idle.
func busyOn(g, d int) string {
	n := len(busyAccounts)
	return busyAccounts[((g+d)%n+n)%n]
}

// running are the sessions the router lists at the world's now: today's,
// each on its account.
func (w busyWorld) running() []status.Session {
	d := busyDays - 1
	sessions := make([]status.Session, busyRunning)
	for j := range sessions {
		g := busySeat(d, j)
		sessions[j] = status.Session{ID: busySession(g), Assignments: []status.Assignment{{Dir: fmt.Sprintf("~/Code/project-%d", g),
			Model: "claude-opus-5-5", Family: "opus", Account: busyOn(g, d), Reason: status.ReasonSticky,
			AssignedAt: w.day(d).UTC(), LastSeen: w.now.Add(-time.Duration(j) * time.Second).UTC(),
		}}}
	}
	return sessions
}

// longest is the session running at the world's now that has run longest,
// every day of the world's; and lately one that has run three days.
func (w busyWorld) longest() string {
	return busySession(busyLong)
}

func (w busyWorld) lately() string {
	return busySession(busySeat(busyDays-1, 1))
}

// busyLine is the format of a line of the busy world: when it arrived, its
// request's id, session, directory, account, reason and from, timings,
// prompt, size, answer's id, usage given, and its limits.
const busyLine = `{"at":"%s","request":"%016x","kind":"message","session":"%s","dir":"~/Code/project-%d","model":"claude-opus-5-5",` +
	`"account":"%s","reason":"%s"%s,"status":200,"attempts":1,"first_ms":%d,"total_ms":%d,"agent":"claude-cli/2.1.300 (external, cli)",` +
	`"betas":["claude-code-20250219","context-1m-2025-08-07","interleaved-thinking-2025-05-14"],"prompt":"%s","class":"main",` +
	`"shape":{"bytes":%d,"messages":%d,"system":3,"tools":31,"max_tokens":32000,"thinking":{"type":"enabled","budget_tokens":31999},` +
	`"stream":true,"output_config":{"effort":"high"}},"answer":{"id":"req_011C%016x","model":"claude-opus-5-5","stop":"tool_use",` +
	`"blocks":{"thinking":1,"text":1,"tool_use":2},"tools":["Bash","Read","Edit"]},"usage":{"input_tokens":%d,` +
	`"cache_creation_input_tokens":%d,"cache_read_input_tokens":%d,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":%d},` +
	`"output_tokens":%d,"service_tier":"standard"},"limits":{"status":"allowed","5h-utilization":"%.2f","5h-reset":"%d",` +
	`"5h-status":"allowed","7d-utilization":"%.2f","7d-reset":"%d","7d-status":"allowed"}}` + "\n"

// busyLines are the lines of the world's dth day, which starts at start,
// that arrived by now: busyRequests through the day, the dozen sessions
// running taking turns, thirty requests a prompt.
func busyLines(start time.Time, d int, now time.Time) []byte {
	r := rand.New(rand.NewPCG(uint64(d), 1))
	var day bytes.Buffer
	for i := range busyRequests {
		at := start.Add(time.Duration(i) * 24 * time.Hour / busyRequests)
		if at.After(now) {
			break
		}
		g := busySeat(d, i%busyRunning)
		account, reason, from := busyOn(g, d), status.ReasonSticky, ""
		switch {
		case i < busyRunning && busyFirst(g) == d:
			reason = status.ReasonNew
		case i < busyRunning:
			reason, from = "rescored after 9h idle", fmt.Sprintf(`,"from":%q`, busyOn(g, d-1))
		}
		fifth := float64(at.Hour()%5*60+at.Minute()) / 300
		fmt.Fprintf(&day, busyLine, at.UTC().Format("2006-01-02T15:04:05.000Z"), r.Uint64(), busySession(g), g, account, reason, from,
			r.IntN(5000), r.IntN(90000), fmt.Sprintf("%s-%d-%d", busySession(g), d, i/busyRunning/30), r.IntN(900000), r.IntN(400), r.Uint64(),
			r.IntN(100), r.IntN(20000), r.IntN(400000), r.IntN(20000), r.IntN(8000),
			fifth, at.Truncate(5*time.Hour).Add(5*time.Hour).Unix(), float64(d%7)/7+fifth/7, start.AddDate(0, 0, 7-d%7).Unix())
	}
	return day.Bytes()
}

// busyReadings are the readings history's lines of the day that starts at
// start, to now: each account's 5-hour window and week read every minute.
func busyReadings(tb testing.TB, start, now time.Time) []byte {
	tb.Helper()
	var day bytes.Buffer
	for at := start; at.Before(start.Add(24*time.Hour)) && !at.After(now); at = at.Add(time.Minute) {
		for k, account := range busyAccounts {
			for _, key := range []string{"5h", "7d"} {
				use := float64((at.Minute()+k*7)%60) / 60
				r := readings.Reading{At: at.UTC(), Account: account, Key: key, Utilization: use, ResetsAt: at.Truncate(5 * time.Hour).Add(5 * time.Hour).UTC(),
					Status: quota.StatusAllowed, Source: readings.FromAnswer}
				data, err := json.Marshal(r)
				if err != nil {
					tb.Fatal(err)
				}
				day.Write(append(data, '\n'))
			}
		}
	}
	return day.Bytes()
}

// busyEvents are the router's events' lines of the world's dth day, which
// starts at start, to now: each session's move as the day begins, and an
// account reaching its cap and coming back every few hours.
func busyEvents(tb testing.TB, start time.Time, d int, now time.Time) []byte {
	tb.Helper()
	var day bytes.Buffer
	run := start.UTC()
	note := func(e status.Event) {
		if e.At.After(now) {
			return
		}
		data, err := events.Marshal(events.Line{Event: e, Run: run})
		if err != nil {
			tb.Fatal(err)
		}
		day.Write(append(data, '\n'))
	}
	id := 0
	for j := range busyRunning {
		g := busySeat(d, j)
		id++
		note(status.Event{ID: id, At: start.Add(time.Duration(j) * 17 * time.Second).UTC(), Kind: status.EventMoved, Session: busySession(g),
			From: busyOn(g, d-1), To: busyOn(g, d), Reason: "rescored after 9h idle"})
	}
	for hour := 0; hour < 24; hour += 3 {
		account := busyAccounts[hour/3%len(busyAccounts)]
		id++
		note(status.Event{ID: id, At: start.Add(time.Duration(hour) * time.Hour).UTC(), Kind: status.EventCap, Account: account, Windows: []string{"5h"}})
		id++
		note(status.Event{ID: id, At: start.Add(time.Duration(hour)*time.Hour + 40*time.Minute).UTC(), Kind: status.EventRoom, Account: account})
	}
	return day.Bytes()
}

// writeDay writes data to the file of the given name in dir, compressed as
// the files of a day two days past are.
func writeDay(tb testing.TB, dir, name string, data []byte, compressed bool) {
	tb.Helper()
	if compressed {
		var gz bytes.Buffer
		w, err := gzip.NewWriterLevel(&gz, gzip.BestSpeed)
		if err != nil {
			tb.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			tb.Fatal(err)
		}
		if err := w.Close(); err != nil {
			tb.Fatal(err)
		}
		name, data = name+".gz", gz.Bytes()
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		tb.Fatal(err)
	}
}

// BenchmarkTheBusyDashboard times the dashboard on a busy world, as
// newBusyWorld writes it, as its user meets it: enter on a session in
// Accounts' Detail till its page shows its lines, of the session that ran
// every day of the four weeks, and of one that ran the last three; the Log
// shown the first time, till its first read lands; a look at the router's
// document with that page open, and with the Log shown; a batch of the
// request stream's events, an answer of the session's streaming on, with the
// page open and on the Log, the stream having told of the hour's requests
// before; a look with the Log's panel open; on the Overview by account,
// GRAPHS charting each account and the pool beside USAGE, a look, a look as
// the pool's 5-hour chart moves a column on, twelve minutes, its readings
// history having gained a reading of each account before it, the pool's
// charts judged in full, and a resize; Runway built afresh, and a look at
// it, its readings history having gained a reading of each account since
// the last; Graphs alike, its lanes of each window, and with POOL or GLANCE
// shown too, and POOL's stack built afresh; a request sent, with the
// Log shown; and the Events, filled through every kind, refiltered to a kind
// none of the four weeks holds.
func BenchmarkTheBusyDashboard(b *testing.B) {
	w := newBusyWorld(b, start)
	for _, s := range []struct{ name, id string }{{"of four weeks", w.longest()}, {"of three days", w.lately()}} {
		b.Run("opening the page "+s.name, func(b *testing.B) {
			h := w.onDetail(b, s.id)
			on := h.model
			for b.Loop() {
				h.model, h.timers, h.waits = on, nil, nil
				h.entered(s.id)
			}
		})
	}
	b.Run("opening the Log", func(b *testing.B) {
		h := w.harness(b)
		on := h.model
		for b.Loop() {
			h.model, h.timers, h.waits = on, nil, nil
			h.model.view = dashboard.Log
			h.deliver(h.update(nudgeMsg{})...)
			if !h.model.log.opened {
				b.Fatal("the Log didn't open")
			}
		}
	})
	page := w.onDetail(b, w.longest())
	page.entered(w.longest())
	page.hearHour(w)
	log := w.harness(b)
	log.model.view = dashboard.Log
	log.deliver(log.update(nudgeMsg{})...)
	log.hearHour(w)
	for _, on := range []struct {
		name string
		h    *harness
	}{{"on the page", page}, {"on the Log", log}} {
		b.Run("a look "+on.name, func(b *testing.B) {
			for b.Loop() {
				on.h.look(w)
			}
		})
		b.Run("an answer streaming on "+on.name, func(b *testing.B) {
			for b.Loop() {
				on.h.streamingOn(w)
			}
		})
	}
	b.Run("a look on the Log's panel", func(b *testing.B) {
		h := w.harness(b)
		h.model.view = dashboard.Log
		h.deliver(h.update(nudgeMsg{})...)
		h.hearHour(w)
		h.typed("f")
		for b.Loop() {
			h.look(w)
		}
	})
	b.Run("a look on the Overview by account", func(b *testing.B) {
		h := w.byAccount(b)
		for b.Loop() {
			h.look(w)
		}
	})
	b.Run("the pool's charts judged in full", func(b *testing.B) {
		h := w.byAccount(b)
		f := h.model.frame(h.model.now())
		for b.Loop() {
			h.model.pool.rooms = nil
			h.model = h.model.poolBuilt(f)
		}
	})
	b.Run("a resize by account", func(b *testing.B) {
		h := w.byAccount(b)
		widths := []int{170, 160}
		n := 0
		for b.Loop() {
			n++
			h.deliver(h.update(tea.WindowSizeMsg{Width: widths[n%2], Height: 37})...)
		}
	})
	b.Run("Runway built afresh", func(b *testing.B) {
		h := w.onRunway(b)
		for b.Loop() {
			h.model.runwayTab.lanes, h.model.runwayTab.builtAt = views.Lanes{}, nil
			h.model = h.model.runwayBuilt()
		}
	})
	b.Run("a look on the Overview by account, a column on", func(b *testing.B) {
		h := w.byAccount(b)
		const column = 5 * time.Hour / (25 - 1)
		for b.Loop() {
			h.clock.now = h.clock.now.Add(column)
			w.gained(b, h.clock.now.Truncate(column).Add(-time.Minute))
			h.look(w)
		}
	})
	b.Run("a look on Runway, a reading gained", func(b *testing.B) {
		h := w.onRunway(b)
		for b.Loop() {
			h.clock.now = h.clock.now.Add(30 * time.Second)
			w.gained(b, h.clock.now.Add(-10*time.Second))
			h.look(w)
		}
	})
	b.Run("Graphs built afresh", func(b *testing.B) {
		h := w.onGraphs(b)
		for b.Loop() {
			h.model.graphsTab.lanes, h.model.graphsTab.builtAt = nil, nil
			h.model = h.model.graphsBuilt()
		}
	})
	for _, on := range []struct {
		name  string
		shown []string
	}{
		{"a look on Graphs, a reading gained", nil},
		{"a look on Graphs with POOL, a reading gained", []string{"week", "5-hour", "pool"}},
		{"a look on Graphs with GLANCE, a reading gained", []string{"week", "5-hour", "glance"}},
	} {
		b.Run(on.name, func(b *testing.B) {
			h := w.onGraphs(b, on.shown...)
			for b.Loop() {
				h.clock.now = h.clock.now.Add(30 * time.Second)
				w.gained(b, h.clock.now.Add(-10*time.Second))
				h.look(w)
			}
		})
	}
	b.Run("POOL's stack built afresh", func(b *testing.B) {
		h := w.onGraphs(b, "pool")
		for b.Loop() {
			h.model.graphsTab.pool, h.model.graphsTab.builtAt = views.PoolStack{}, nil
			h.model = h.model.graphsBuilt()
		}
	})
	b.Run("a request sent on the Log", func(b *testing.B) {
		n := 0
		for b.Loop() {
			n++
			log.sent(w, fmt.Sprintf("sent-%06d", n))
		}
	})
	b.Run("the Events refiltered to a kind none was filed of", func(b *testing.B) {
		h := w.harness(b)
		h.model.view = dashboard.Log
		h.deliver(h.update(nudgeMsg{})...)
		h.typed("]")
		h.settled()
		on := h.model
		for b.Loop() {
			h.model, h.timers, h.waits = on, nil, nil
			h.refiltered(views.EventFilter{Kinds: []string{status.EventLimit}})
		}
	})
}

// busyDocument is the router's document of the busy world's accounts.
func busyDocument() status.Document {
	accounts := make([]status.Account, len(busyAccounts))
	for i, id := range busyAccounts {
		accounts[i] = account(id, strings.ToUpper(id[:1])+id[1:], session(0.1*float64(i+1), time.Duration(i+1)*time.Hour), week(0.2*float64(i+1)))
	}
	return routerDocument(accounts...)
}

// harness is a model of the router of the busy world, reading its files,
// in a terminal 160 columns wide and 37 tall, started.
func (w busyWorld) harness(tb testing.TB) *harness {
	tb.Helper()
	h := routedHarness(tb, busyDocument())
	h.clock.now = w.now
	h.source.sessions = w.running()
	follower, history, filed := w.readers(h.clock.Now)
	h.model.cfg.Ledger, h.model.cfg.Readings, h.model.cfg.Events = follower, history, filed
	h.update(tea.WindowSizeMsg{Width: 160, Height: 37})
	h.start()
	return h
}

// byAccount is the busy world's harness on the Overview by account, GRAPHS
// shown, so each account's charts and the pool's sit beside USAGE, its
// first look taken.
func (w busyWorld) byAccount(tb testing.TB) *harness {
	tb.Helper()
	h := w.harness(tb)
	h.model.by, h.model.sections = dashboard.ByAccount, dashboard.Sections{}.Toggled(dashboard.SectionGraphs)
	h.look(w)
	return h
}

// onRunway is the busy world's harness on Runway's live week view, the view
// it opens on, its first look taken.
func (w busyWorld) onRunway(tb testing.TB) *harness {
	tb.Helper()
	h := w.harness(tb)
	h.model.view = dashboard.Runway
	h.look(w)
	return h
}

// onGraphs is the busy world's harness on Graphs, showing the sections
// named, where any are, else those it shows at first, what they draw built,
// its lanes over Runway's live view of each window among them, its first
// look taken.
func (w busyWorld) onGraphs(tb testing.TB, shown ...string) *harness {
	tb.Helper()
	h := w.harness(tb)
	h.model.view = dashboard.Graphs
	if len(shown) > 0 {
		h.model.graphs = dashboard.GraphsSectionsNamed(shown)
	}
	h.look(w)
	return h
}

// gained has the world's readings history gain a reading of each account's
// 5-hour window at at, as the router files them, on the day's file.
func (w busyWorld) gained(tb testing.TB, at time.Time) {
	tb.Helper()
	at = at.In(w.now.Location())
	file, err := os.OpenFile(filepath.Join(readings.Dir(w.state), "readings-"+at.Format(time.DateOnly)+".jsonl"), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		tb.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	for k, account := range busyAccounts {
		r := readings.Reading{At: at.UTC(), Account: account, Key: "5h", Utilization: float64((at.Minute()+k*7)%60) / 60,
			ResetsAt: at.Truncate(5 * time.Hour).Add(5 * time.Hour).UTC(), Status: quota.StatusAllowed, Source: readings.FromAnswer}
		data, err := json.Marshal(r)
		if err != nil {
			tb.Fatal(err)
		}
		if _, err := file.Write(append(data, '\n')); err != nil {
			tb.Fatal(err)
		}
	}
}

// onDetail is the busy world's harness on Accounts' Detail of the account
// the session with the given id runs on, its SESSIONS choosing it.
func (w busyWorld) onDetail(tb testing.TB, id string) *harness {
	tb.Helper()
	h := w.harness(tb)
	on := views.Routed(w.running(), id).Assignments[0].Account
	configured := make(config.Accounts, len(busyAccounts))
	for i, a := range busyAccounts {
		configured[i] = config.Account{ID: a}
	}
	h.model.view = dashboard.Accounts
	h.model.accounts.shown = dashboard.AccountsShown{Accounts: configured, Shown: dashboard.AccountsDetail, Chosen: on, InSessions: true, Selected: id}
	h.deliver(h.update(nudgeMsg{})...)
	return h
}

// entered presses enter, and runs what it starts, and what that starts in
// turn, till the page of the session with the given id shows its lines:
// what the update that shows it starts runs after it's on screen.
func (h *harness) entered(id string) {
	h.t.Helper()
	cmds, d := []tea.Cmd{h.updated(tea.KeyPressMsg{Code: tea.KeyEnter})}, delivery{h: h}
	for ; len(cmds) > 0 && !h.pageShows(id); cmds = cmds[1:] {
		for _, msg := range h.run(cmds[0]) {
			if !h.pageShows(id) {
				d.took(msg)
				cmds = append(cmds, h.updated(msg))
			}
		}
	}
	if !h.pageShows(id) {
		h.t.Fatalf("enter didn't open %s's page", id)
	}
}

// updated gives the model msg, and returns the command it returns, unrun.
func (h *harness) updated(msg tea.Msg) tea.Cmd {
	next, cmd := h.model.Update(msg)
	h.model = next.(Model)
	return cmd
}

// pageShows reports whether the page open shows the lines of the session
// with the given id.
func (h *harness) pageShows(id string) bool {
	return h.model.session != nil && !h.model.session.shown.Loading && h.model.session.shown.Page.Session == id
}

// hearHour has the request stream tell of the busy world's last hour of
// requests, each sent and done, and a request of its longest session's
// sent, its answer streaming.
func (h *harness) hearHour(w busyWorld) {
	h.t.Helper()
	var events []router.StreamEvent
	for i := range busyRequests / 24 {
		at := w.now.Add(-time.Hour + time.Duration(i)*24*time.Hour/busyRequests)
		g := busySeat(busyDays-1, i%busyRunning)
		e := router.StreamEvent{At: at, Kind: router.StreamSent, Request: fmt.Sprintf("hour-%04d", i), Attempt: 1, Session: busySession(g),
			Model: "claude-opus-5-5", Account: busyOn(g, busyDays-1)}
		done := e
		done.At, done.Kind, done.Status, done.Chars = at.Add(20*time.Second), router.StreamDone, 200, 900
		events = append(events, e, done)
	}
	streaming := router.StreamEvent{At: w.now, Kind: router.StreamSent, Request: "streaming", Attempt: 1, Session: w.longest(), Model: "claude-opus-5-5",
		Account: views.Routed(w.running(), w.longest()).Assignments[0].Account}
	first := streaming
	first.Kind = router.StreamFirst
	for chunk := range slices.Chunk(append(events, streaming, first), 64) {
		h.hear(chunk...)
	}
}

// look has the model take in a look at the router's document, the
// sessions it lists listed, and what it sends back.
func (h *harness) look(w busyWorld) {
	h.t.Helper()
	h.deliver(fetchedMsg{doc: busyDocument(), sessions: w.running(), listed: true})
}

// sent has the request stream tell of a request of the busy world's
// longest session sent, with the id given, a tenth of a second on.
func (h *harness) sent(w busyWorld, id string) {
	h.t.Helper()
	h.clock.now = h.clock.now.Add(time.Second / 10)
	h.hear(router.StreamEvent{At: h.clock.now, Kind: router.StreamSent, Request: id, Attempt: 1, Session: w.longest(), Model: "claude-opus-5-5",
		Account: views.Routed(w.running(), w.longest()).Assignments[0].Account})
}

// settled has the model take in frames till every read for the Log it
// wants, a day back each, has landed.
func (h *harness) settled() {
	h.t.Helper()
	for range busyDays + 2 {
		h.deliver(h.update(frameMsg{gen: h.model.frames})...)
	}
}

// refiltered applies the Log's panel with its Events' filter as given, and
// takes in what it has the Log read.
func (h *harness) refiltered(shows views.EventFilter) {
	h.t.Helper()
	h.keyed("f")
	h.model.log.panel.Shows = shows
	h.keys(tea.KeyPressMsg{Code: tea.KeyEnter})
	h.settled()
}

// streamingOn has the request stream tell of the answer of the busy
// world's longest session streaming on, a tenth of a second on, a batch of
// four of its progress.
func (h *harness) streamingOn(w busyWorld) {
	h.t.Helper()
	h.clock.now = h.clock.now.Add(time.Second / 10)
	progress := router.StreamEvent{At: h.clock.now, Kind: router.StreamProgress, Request: "streaming", Attempt: 1, Session: w.longest(), Model: "claude-opus-5-5",
		Account: views.Routed(w.running(), w.longest()).Assignments[0].Account}
	batch := make([]router.StreamEvent, 4)
	for i := range batch {
		batch[i] = progress
		batch[i].Chars = int(h.clock.now.Sub(w.now)/time.Millisecond) + i
	}
	h.hear(batch...)
}
