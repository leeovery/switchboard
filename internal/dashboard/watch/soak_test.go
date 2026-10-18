package watch

import (
	"cmp"
	"flag"
	"fmt"
	"maps"
	"os"
	"reflect"
	"runtime"
	"runtime/pprof"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/views"
)

// The soak's flags: it runs only as -soak asks, for hours of the simulated
// clock, which take minutes of the real one.
var (
	soakFor     = flag.Duration("soak", 0, "run the dashboard soak for this long on the simulated clock, as 12h: none skips it")
	soakOn      = flag.String("soak.on", "rounds", "what the soak shows: overview, accounts, history, sessions, runway, log, events (the Log's, paged back), session (a session's page), rounds, each in turn, turning (the Overview, a phone turned every ten seconds) or stepping (a tab on every half minute)")
	soakDays    = flag.Int("soak.days", 9, "how many days the soak's world has run as it starts, its long session in each")
	soakEvery   = flag.Duration("soak.every", 20*time.Minute, "how long the rounds stay on each station")
	soakFrames  = flag.Bool("soak.frames", true, "fire the frames the model arms: false lets them lapse, for a faster soak")
	soakProfile = flag.String("soak.profile", "", "where the soak writes its heap profile as it ends: none writes none")
)

// soakStart is when the soak starts: the evening of a day, so it crosses
// midnight three hours in.
var soakStart = soakAt(21, 0)

// soakAt is the time of day given on the date the tests' clock starts on, in
// the machine's zone, as the ledger's days are: so the soak's midnight is the
// one its world's days turn at, whatever zone the tests run in.
func soakAt(hour, minute int) time.Time {
	return time.Date(start.Year(), start.Month(), start.Day(), hour, minute, 0, 0, time.Local)
}

// TestTheDashboardsLiveHeapOverASoak runs the dashboard over the soak's
// world for the simulated hours -soak asks, at the owner's rate, on what
// -soak.on says, and reports its live heap after each hour, the collector
// having run, beside what the model holds, and what its Updates and its
// commands took; then writes the heap profile where -soak.profile says.
func TestTheDashboardsLiveHeapOverASoak(t *testing.T) {
	if *soakFor <= 0 {
		t.Skip("the soak runs only as -soak asks")
	}
	began := time.Now()
	s := newSoak(t, soakSetup{start: soakStart, days: *soakDays, pace: ownersPace, frames: *soakFrames})
	s.plan(*soakOn)
	t.Logf("soak on %s from %s for %s, %d days at %d requests a day behind it, written in %.1fs", *soakOn, soakStart.Format(time.DateTime), *soakFor,
		*soakDays, ownersPace.rate, time.Since(began).Seconds())
	s.report(0, time.Now())
	for hour := 1; time.Duration(hour-1)*time.Hour < *soakFor; hour++ {
		began := time.Now()
		s.runUntil(soakStart.Add(min(time.Duration(hour)*time.Hour, *soakFor)))
		s.report(hour, began)
	}
	if *soakProfile != "" {
		s.profile(*soakProfile)
	}
}

// soak runs the model over the soak's world on the harness's clock: the
// timers it arms fired as they fall due, in turn, its frames let lapse
// unless frames says, and the world run between them a step of its pace at
// a time, what the router's stream tells of it told while the model
// listens, what's shown changed as the schedule says; each Update and each
// command the model returns timed, by the message each takes in or gives.
// follower is the world's ledger, and guard, where it's set, what holds the
// soak to what it reads and holds, as soakGuard has it.
type soak struct {
	t        *testing.T
	w        *soakWorld
	h        *harness
	frames   bool
	follower *ledger.Follower
	guard    soakGuard
	timers   []*timer
	waits    []soakWait
	// closed counts the streams opened whose ends have been heard, and told
	// the streams the requests in flight have been told to, as each opens;
	// heard counts the stream's events told.
	closed, told, heard int
	schedule            []soakAction
	updates, commands   timings
}

// soakSetup is how a soak runs: from start, over a world run for days to
// it at pace, its frames fired or let lapse, held by guard where it's set.
type soakSetup struct {
	start  time.Time
	days   int
	pace   soakPace
	frames bool
	guard  soakGuard
}

// soakWait is a wait on the request stream the model armed, and the stream
// it waits on, by the order it was opened in.
type soakWait struct {
	wait   tea.Cmd
	stream int
}

// soakAction is something the schedule has done at its time.
type soakAction struct {
	at time.Time
	do func()
}

// newSoak is the soak setup says, its model started on the Overview.
func newSoak(t *testing.T, setup soakSetup) *soak {
	t.Helper()
	w := newSoakWorld(t, setup.start, setup.days, setup.pace)
	s := &soak{t: t, w: w, frames: setup.frames, guard: setup.guard, updates: make(timings), commands: make(timings)}
	h := routedHarness(t, w.document(setup.start))
	h.clock.now = setup.start
	h.source.health = router.Health{OK: true, StartedAt: soakRun}
	follower, history, filed := w.readers(h.clock.Now)
	var read views.Ledger = follower
	if s.guard != nil {
		read = s.guard.reads(s, follower)
	}
	configured := make(config.Accounts, len(busyAccounts))
	for i, a := range busyAccounts {
		configured[i] = config.Account{ID: a}
	}
	cfg := h.model.cfg
	cfg.Ledger, cfg.Readings, cfg.Events, cfg.Accounts = read, history, filed, configured
	cfg.After, cfg.Await = s.arm, s.await
	h.model = New(t.Context(), cfg)
	s.h, s.follower = h, follower
	s.stands()
	s.resized(tea.WindowSizeMsg{Width: 160, Height: 37})
	s.deliver(s.run(h.model.Init())...)
	return s
}

// arm holds a timer the model arms, to fire once it falls due.
func (s *soak) arm(d time.Duration, msg tea.Msg) tea.Cmd {
	s.timers = append(s.timers, &timer{delay: d, due: s.h.clock.now.Add(d), msg: msg})
	return nil
}

// await holds a wait on the request stream, the one opened last, which the
// model listens to, till the world tells it something or it ends.
func (s *soak) await(wait tea.Cmd) tea.Cmd {
	s.waits = append(s.waits, soakWait{wait: wait, stream: len(s.h.source.streams) - 1})
	return nil
}

// deliver has the model take in msgs, and what they send back, till all are
// taken in, each Update timed by the message it takes, and told the guard
// of, where it's set.
func (s *soak) deliver(msgs ...tea.Msg) {
	for len(msgs) > 0 {
		was, began := s.h.model, time.Now()
		next, cmd := s.h.model.Update(msgs[0])
		s.updates.add(msgs[0], time.Since(began))
		s.h.model = next.(Model)
		if s.guard != nil {
			s.guard.took(msgs[0], was)
		}
		msgs = append(msgs[1:], s.run(cmd)...)
	}
}

// run runs cmd, and every command it batches, returning what they send
// back, each timed by the message it gives.
func (s *soak) run(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	began := time.Now()
	switch msg := cmd().(type) {
	case nil:
		return nil
	case tea.BatchMsg:
		var msgs []tea.Msg
		for _, c := range msg {
			msgs = append(msgs, s.run(c)...)
		}
		return msgs
	default:
		s.commands.add(msg, time.Since(began))
		return []tea.Msg{msg}
	}
}

// runUntil runs the soak till until: each timer due fired in turn, and the
// world run on between them.
func (s *soak) runUntil(until time.Time) {
	for {
		next := s.w.at.Add(s.w.pace.step)
		tm := s.due()
		switch {
		case tm != nil && !tm.due.After(next) && !tm.due.After(until):
			s.fire(tm)
		case !next.After(until):
			s.step(next)
		default:
			return
		}
	}
}

// due is the timer due soonest, nil where none is armed.
func (s *soak) due() *timer {
	var soonest *timer
	for _, tm := range s.timers {
		if soonest == nil || tm.due.Before(soonest.due) {
			soonest = tm
		}
	}
	return soonest
}

// fire moves the clock to when tm falls due, lets it go, and delivers it:
// a frame lapses unless frames says.
func (s *soak) fire(tm *timer) {
	s.timers = slices.DeleteFunc(s.timers, func(o *timer) bool { return o == tm })
	if _, frame := tm.msg.(frameMsg); frame && !s.frames {
		return
	}
	s.h.clock.now = later(tm.due, s.h.clock.now)
	s.deliver(tm.msg)
}

// later is the later of a and b, so the clock never goes back.
func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// step runs the world on to at: what the schedule has done, then the world
// itself, as advance has it, its files written, the router's document and
// sessions standing as they do at at, and what its stream tells told,
// while the model listens.
func (s *soak) step(at time.Time) {
	was := s.w.at
	s.h.clock.now = later(at, s.h.clock.now)
	for len(s.schedule) > 0 && !s.schedule[0].at.After(at) {
		s.schedule[0].do()
		s.schedule = s.schedule[1:]
	}
	told := s.w.advance(was, at)
	s.w.flush()
	s.stands()
	s.ends()
	s.tell(told)
}

// stands has the router's document and its sessions stand as they do at the
// clock's time.
func (s *soak) stands() {
	doc := s.w.document(s.h.clock.now)
	s.h.source.router, s.h.source.sessions = &doc, s.w.listed(s.h.clock.now)
}

// ends hears the end of each stream the model closed, as the router's
// client closes it once its context ends, and has the model take in what
// the wait on it gives, which it drops.
func (s *soak) ends() {
	streams := s.h.source.streams
	for ; s.closed < len(streams)-1; s.closed++ {
		close(streams[s.closed])
	}
	if last := len(streams) - 1; last >= s.closed && s.h.source.contexts[last].Err() != nil {
		close(streams[last])
		s.closed++
	}
	var ended []soakWait
	s.waits = slices.DeleteFunc(s.waits, func(w soakWait) bool {
		if w.stream < s.closed {
			ended = append(ended, w)
			return true
		}
		return false
	})
	for _, w := range ended {
		s.deliver(s.run(w.wait)...)
	}
}

// tell has the stream the model listens to tell the events, as many at a
// time as its channel holds: first, where it has just opened, of each
// request in flight, as the router's stream opens. What the model doesn't
// listen to, it never hears.
func (s *soak) tell(events []router.StreamEvent) {
	last := len(s.h.source.streams) - 1
	if last < s.closed {
		return
	}
	if s.told <= last {
		s.told = last + 1
		events = append(s.w.inFlight(s.h.clock.now), events...)
	}
	for chunk := range slices.Chunk(events, cap(s.h.source.streams[last])) {
		i := slices.IndexFunc(s.waits, func(w soakWait) bool { return w.stream == last })
		if i < 0 {
			return
		}
		wait := s.waits[i]
		s.waits = slices.Delete(s.waits, i, i+1)
		for _, e := range chunk {
			s.h.source.streams[last] <- e
		}
		s.heard += len(chunk)
		s.deliver(s.run(wait.wait)...)
	}
}

// The stations the soak can keep the dashboard on, and rounds, the order
// it goes round them in.
var (
	soakTabs = map[string]dashboard.View{
		"overview": dashboard.Overview, "accounts": dashboard.Accounts, "history": dashboard.History,
		"sessions": dashboard.Sessions, "runway": dashboard.Runway, "log": dashboard.Log, "events": dashboard.Log,
	}
	soakRounds = []string{"overview", "log", "runway", "sessions", "session", "accounts", "history", "events"}
)

// The sizes the soak draws at: its terminal's, and a phone's, turned on its
// side and upright, in Blink through Mosh and tmux.
var (
	soakWide    = tea.WindowSizeMsg{Width: 160, Height: 37}
	soakSide    = tea.WindowSizeMsg{Width: 134, Height: 31}
	soakUpright = tea.WindowSizeMsg{Width: 58, Height: 44}
)

// plan schedules what's shown as on says: the Overview, a phone turned
// every ten seconds, from upright to its side to the terminal's size; a tab
// on every half minute, round them all; one station throughout, the Log's
// Events paged back each ten minutes; or the rounds, a station each
// -soak.every, and at every third, a quarter of the way on, a phone turned
// eight times, a couple of seconds apart, and half way on, the tabs stepped
// through, a second apart, and back.
func (s *soak) plan(on string) {
	end := soakStart.Add(*soakFor)
	switch on {
	case "turning":
		s.show("overview")
		for i, at := 0, soakStart.Add(10*time.Second); at.Before(end); i, at = i+1, at.Add(10*time.Second) {
			size := []tea.WindowSizeMsg{soakUpright, soakSide, soakWide}[i%3]
			s.schedule = append(s.schedule, soakAction{at: at, do: func() { s.resized(size) }})
		}
		return
	case "stepping":
		for at := soakStart.Add(30 * time.Second); at.Before(end); at = at.Add(30 * time.Second) {
			s.schedule = append(s.schedule, soakAction{at: at, do: func() { s.keyed(tea.KeyPressMsg{Code: tea.KeyTab}) }})
		}
		return
	}
	if on != "rounds" {
		s.show(on)
		for at := soakStart.Add(10 * time.Minute); on == "events" && at.Before(end); at = at.Add(10 * time.Minute) {
			s.schedule = append(s.schedule, soakAction{at: at, do: s.pagedBack})
		}
		return
	}
	for i, at := 0, soakStart; at.Before(end); i, at = i+1, at.Add(*soakEvery) {
		station := soakRounds[i%len(soakRounds)]
		s.schedule = append(s.schedule, soakAction{at: at, do: func() { s.show(station) }})
		if i%3 != 2 {
			continue
		}
		turning, stepping := at.Add(*soakEvery/4), at.Add(*soakEvery/2)
		for turn := range 8 {
			size := []tea.WindowSizeMsg{soakUpright, soakSide}[turn%2]
			s.schedule = append(s.schedule, soakAction{at: turning.Add(time.Duration(turn) * 2 * time.Second), do: func() { s.resized(size) }})
		}
		s.schedule = append(s.schedule, soakAction{at: turning.Add(20 * time.Second), do: func() { s.resized(soakWide) }})
		for step := range len(dashboard.Views()) {
			s.schedule = append(s.schedule, soakAction{at: stepping.Add(time.Duration(step) * time.Second), do: func() { s.keyed(tea.KeyPressMsg{Code: tea.KeyTab}) }})
		}
	}
	slices.SortStableFunc(s.schedule, func(a, b soakAction) int { return a.at.Compare(b.at) })
	s.schedule[0].do()
	s.schedule = s.schedule[1:]
}

// show puts the station given on screen: a tab, from wherever, its session's
// page closed, as tab steps to it; the Log's Events, paged back; or the page
// of the session that has run every day of the world's, over the tab shown.
// The Overview shows GRAPHS and SESSIONS beside its first sections, by
// account.
func (s *soak) show(station string) {
	if station == "session" {
		next, cmd := s.h.model.openSession(s.w.longest())
		s.h.model = next
		s.deliver(s.run(cmd)...)
		return
	}
	tab, ok := soakTabs[station]
	if !ok {
		s.t.Fatalf("no station %q to soak on", station)
	}
	if s.h.model.session != nil {
		s.keyed(tea.KeyPressMsg{Code: tea.KeyEscape})
	}
	s.stepTo(tab)
	switch {
	case tab == dashboard.Overview && !s.h.model.sections.Has(dashboard.SectionGraphs):
		s.typed("g", "s", "b")
	case station == "events":
		if s.h.model.log.page.Shown != dashboard.LogEvents {
			s.typed("]")
		}
		s.pagedBack()
	case station == "log" && s.h.model.log.page.Shown == dashboard.LogEvents:
		s.typed("[")
	}
}

// stepTo steps from the tab shown to the one given, by tab or shift-tab,
// whichever passes fewer tabs on the way, as each tab passed reads what it
// shows.
func (s *soak) stepTo(tab dashboard.View) {
	tabs := dashboard.Views()
	ahead := (slices.Index(tabs, tab) - slices.Index(tabs, s.h.model.view) + len(tabs)) % len(tabs)
	key := tea.KeyPressMsg{Code: tea.KeyTab}
	if ahead > len(tabs)/2 {
		key, ahead = tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}, len(tabs)-ahead
	}
	for range ahead {
		s.keyed(key)
	}
}

// pagedBack pages the Log's table shown back, as far as twenty pages go,
// for it to read back the days before.
func (s *soak) pagedBack() {
	for range 20 {
		s.keyed(tea.KeyPressMsg{Code: tea.KeyPgUp})
	}
}

// typed has the model take in each key given, and what it sends back.
func (s *soak) typed(keys ...string) {
	for _, k := range keys {
		s.keyed(tea.KeyPressMsg{Code: []rune(k)[0], Text: k})
	}
}

// keyed has the model take in a key, and what it sends back.
func (s *soak) keyed(key tea.KeyPressMsg) {
	s.deliver(key)
}

// resized has the model take in the terminal's size, and what it sends back.
func (s *soak) resized(size tea.WindowSizeMsg) {
	s.deliver(size)
}

// report logs the live heap after the hour given, which took the real time
// since began, the collector having run; what the model holds; and what its
// Updates and commands took over the hour.
func (s *soak) report(hour int, began time.Time) {
	runtime.GC()
	runtime.GC()
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	m := s.h.model
	s.t.Logf("hour %2d %s on %-8s took %6.1fs: heap %7.1f MB, in use %7.1f MB, %9d objects, %d goroutines; sent %d (%d the long session's), filed %d, told %d",
		hour, s.h.clock.now.Format("Jan 2 15:04"), m.shownView(), time.Since(began).Seconds(), mb(mem.HeapAlloc), mb(mem.HeapInuse), mem.HeapObjects,
		runtime.NumGoroutine(), s.w.sent, s.w.longSent(), s.w.filed, s.heard)
	s.t.Logf("        traffic: %d requests, %d lines, %d seats, %d calls, %d moves; log: %d lines read, %d days, from %s, %d days' totals",
		len(m.traffic.requests), len(m.traffic.lines), len(m.traffic.seen), len(m.traffic.calls), len(m.traffic.moves),
		m.log.read.Len(), held(m.log.read, "days"), m.log.from.Date, len(m.log.totals))
	store := m.held.Holding()
	open := 0
	for _, n := range store.Open {
		open += n
	}
	s.t.Logf("        store: %d sessions on %s, %d requests open, %d keys; its last feed gained %d, folded %d, read %d whole",
		len(store.Open), store.Day, open, store.Keys, store.Gained, store.Folded, store.Read)
	s.t.Logf("        held: %s", heldOf(m))
	s.t.Logf("        updates: %s", s.updates)
	s.t.Logf("        commands: %s", s.commands)
	s.updates, s.commands = make(timings), make(timings)
	s.h.source.asked = nil
}

// held is how many the field of v with the given name holds, a map or a
// slice, unexported or not.
func held(v any, field string) int {
	return reflect.ValueOf(v).FieldByName(field).Len()
}

// profile writes the heap profile to path, the collector having run.
func (s *soak) profile(path string) {
	runtime.GC()
	file, err := os.Create(path)
	if err != nil {
		s.t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	if err := pprof.WriteHeapProfile(file); err != nil {
		s.t.Fatal(err)
	}
	s.t.Logf("heap profile written to %s", path)
}

// mb is n bytes in megabytes.
func mb(n uint64) float64 {
	return float64(n) / (1 << 20)
}

// timings are how many times a thing was timed, and what it took in all and
// at most, by the kind of message it took or gave.
type timings map[string]*timing

type timing struct {
	n          int
	total, top time.Duration
}

// add times a thing of msg's kind as taking d.
func (t timings) add(msg tea.Msg, d time.Duration) {
	kind := strings.TrimPrefix(fmt.Sprintf("%T", msg), "watch.")
	if t[kind] == nil {
		t[kind] = &timing{}
	}
	k := t[kind]
	k.n, k.total, k.top = k.n+1, k.total+d, max(k.top, d)
}

// String names the kinds that took longest in all, eight of them, each's
// count, total, mean and most.
func (t timings) String() string {
	kinds := slices.SortedFunc(maps.Keys(t), func(a, b string) int { return cmp.Compare(t[b].total, t[a].total) })
	var words []string
	for _, kind := range kinds[:min(8, len(kinds))] {
		k := t[kind]
		words = append(words, fmt.Sprintf("%s %d×, %.1fs, mean %.1fms, most %.0fms", kind, k.n, k.total.Seconds(), float64(k.total)/float64(k.n)/1e6,
			float64(k.top)/1e6))
	}
	return strings.Join(words, "; ")
}
