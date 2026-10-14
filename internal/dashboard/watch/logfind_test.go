package watch

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// The sessions the find's tests find: api and api-docs, which "api" finds,
// and web, which it doesn't.
const (
	apiSession  = "a9100000-0000-4000-8000-000000000001"
	docsSession = "d0c50000-0000-4000-8000-000000000002"
	webSession  = "0eb00000-0000-4000-8000-000000000003"
)

// ranIn is the ledger's line of a message of the session given, run in the
// directory given, arrived d past when the clock starts.
func ranIn(request, session, dir string, d time.Duration) ledger.Held {
	l := arrived(request, "work", d)
	l.Session, l.Dir = session, dir
	return l
}

// foundIDs are the ids of the sessions the Log's find finds, in order, each
// with its directory where it's known.
func foundIDs(h *harness) []string {
	var ids []string
	for _, s := range h.model.log.page.Finding.Found.Sessions {
		ids = append(ids, strings.TrimSpace(s.ID+" "+s.Dir))
	}
	return ids
}

// Escape and Enter, as the keyboard sends them.
var (
	escape = tea.KeyPressMsg{Code: tea.KeyEscape}
	enter  = tea.KeyPressMsg{Code: tea.KeyEnter}
)

// The find walks back a day at a time, newest first, reading only the days
// whose summaries name a session, and only while it's open, a read under
// way as it closes landing without another; each day once a run, so it
// finds again what it found without reading. While it's open, the key line
// lists its keys alone.
func TestTheFindWalksBackADayAtATimeWhileOpenOnceARun(t *testing.T) {
	state := t.TempDir()
	writeLines(t, state,
		ranIn("0a000001", apiSession, "~/Code/api", -3*day),
		ranIn("0a000002", docsSession, "~/Code/api-docs", -day),
		ranIn("0a000003", webSession, "~/Code/web", -time.Minute),
	)
	h := newHarness(t, probedWithoutTheRouter())
	l := onTheLog(h, state)
	h.start()
	opened := len(l.days)

	summed := h.press("/")
	if typed := h.press("w"); slices.ContainsFunc(typed, func(msg tea.Msg) bool { _, ok := msg.(findReadMsg); return ok }) {
		t.Error("typed while a read for the find was under way, the find read again, want one read at a time")
	}
	h.keys(escape)
	h.deliver(summed...)
	if got := l.days[opened:]; len(got) > 0 {
		t.Errorf("closed as it began, the find read the days %v, want none", got)
	}

	h.typed("/", "a", "p", "i")
	walked := slices.Clone(l.days[opened:])
	if want := []string{dayfile.DateOf(past(-day)), dayfile.DateOf(past(-3 * day))}; !slices.Equal(walked, want) {
		t.Errorf("the find read the days %v, want %v: newest first, none whose summary names no session", walked, want)
	}
	want := []string{docsSession + " ~/Code/api-docs", apiSession + " ~/Code/api"}
	if got := foundIDs(h); !slices.Equal(got, want) {
		t.Errorf("api finds %q, want %q", got, want)
	}
	if got := h.keyLine(); got != "↑↓ pick · enter filter · esc cancel" {
		t.Errorf("while the find is open, the key line reads %q, want its keys alone", got)
	}

	h.keys(escape)
	read := len(l.days)
	h.read(probedWithoutTheRouter())
	h.typed("/", "a", "p", "i")
	if len(l.days) != read {
		t.Errorf("closed, then open again, the find read the days %v, want none read again", l.days[read:])
	}
	if got := foundIDs(h); !slices.Equal(got, want) {
		t.Errorf("open again, api finds %q, want %q, as the days read were held", got, want)
	}
}

// The find reads the days' summaries afresh each time it opens: a session
// of a day that has turned since it was last open, whose summary then named
// none, is found.
func TestTheFindReadsTheDaysSummariesAfreshAsItOpens(t *testing.T) {
	state := t.TempDir()
	h := newHarness(t, probedWithoutTheRouter())
	onTheLog(h, state)
	h.clock.now = onTheDay(13, 0, 0)
	h.start()
	h.typed("/")
	h.keys(escape)

	writeLines(t, state, arrivedAt("0d000001", onTheDay(23, 0, 0)))
	h.clock.now = onTheDay(24, 1, 0)
	h.read(probedWithoutTheRouter())
	h.typed("/")
	if got, want := foundIDs(h), []string{idD28C}; !slices.Equal(got, want) {
		t.Errorf("past midnight, the find finds %q, want %q, of the day before, its summary read afresh", got, want)
	}
}

// A read of the days' summaries under way as the find closes, landing once
// it has opened again, is of the find as it last opened: it's dropped, and
// they're read afresh, so a session of a day that has turned between is
// found.
func TestTheFindDropsASummariesReadOfAnEarlierOpening(t *testing.T) {
	state := t.TempDir()
	h := newHarness(t, probedWithoutTheRouter())
	onTheLog(h, state)
	h.clock.now = onTheDay(13, 0, 0)
	h.start()
	stale := h.press("/")
	h.keys(escape)

	writeLines(t, state, arrivedAt("0d000001", onTheDay(23, 0, 0)))
	h.clock.now = onTheDay(24, 1, 0)
	h.read(probedWithoutTheRouter())
	h.typed("/")
	h.deliver(stale...)
	if got, want := foundIDs(h), []string{idD28C}; !slices.Equal(got, want) {
		t.Errorf("open again as the read of its last opening came, the find finds %q, want %q, of the day before, its summary read afresh", got, want)
	}
}

// enter on a session of an earlier day filters both tables to it, the
// tables reading back to its day, how far its lines reach read from the
// days' summaries first, then pauses at its newest line, chosen, the panel
// listing it among today's sessions, ticked; but with the filter let go on
// the way, they read back no further, and nothing pauses.
func TestEnterOnAnEarlierDaysSessionShowsItsLinesAlonePausedAtItsNewest(t *testing.T) {
	state := t.TempDir()
	var today []ledger.Held
	for i := range 45 {
		today = append(today, ranIn(fmt.Sprintf("0c%06d", i), webSession, "~/Code/web", -time.Duration(50-i)*time.Minute))
	}
	writeLines(t, state, today...)
	writeLines(t, state, ranIn("0b000001", apiSession, "~/Code/api", -3*day), ranIn("0b000002", apiSession, "~/Code/api", -3*day+time.Minute))
	h := newHarness(t, probedWithoutTheRouter())
	onTheLog(h, state)
	h.start()
	if h.model.log.from.Date != dayfile.DateOf(past(0)) {
		t.Fatalf("the Log spans from %s as it opens, want today alone, its view filled", h.model.log.from.Date)
	}

	h.typed("/", "a", "p", "i")
	var reading []tea.Msg
	for _, reach := range h.update(enter) {
		reading = append(reading, h.update(reach)...)
	}
	h.keys(escape)
	h.deliver(reading...)
	if got, want := h.model.log.from.Date, dayfile.DateOf(past(-day)); got != want || h.model.log.page.Paused() {
		t.Errorf("the filter let go as the tables read back, they span from %s, paused %t; want %s, the read under way, and following",
			got, h.model.log.page.Paused(), want)
	}

	h.typed("/", "a", "p", "i")
	h.keys(enter)
	p := h.model.log.page
	if !slices.Equal(p.Filter.Sessions, []string{apiSession}) || !slices.Equal(p.Shows.Sessions, []string{apiSession}) {
		t.Errorf("the tables keep the sessions %v and %v, want api's alone in both", p.Filter.Sessions, p.Shows.Sessions)
	}
	if got, want := h.model.log.from.Date, dayfile.DateOf(past(-3*day)); got != want {
		t.Errorf("the tables span from %s, want %s, api's day", got, want)
	}
	if row, _, ok := p.Chosen(); !ok || !p.Paused() || row.Request != "0b000002" {
		t.Errorf("the table chose %q, paused %t; want it paused at api's newest line, 0b000002", row.Request, p.Paused())
	}
	if got := shownRequests(h); !slices.Equal(got, []string{"0b000001", "0b000002"}) {
		t.Errorf("the Requests show %v, want api's lines alone", got)
	}

	h.typed("f")
	panel := h.model.log.panel
	listed := slices.IndexFunc(panel.Sessions, func(s views.LogSession) bool { return s.ID == apiSession })
	if listed < 0 || panel.Sessions[listed].Dir != "~/Code/api" || !slices.Contains(panel.Filter.Sessions, apiSession) {
		t.Errorf("the panel lists %+v, ticking %v; want api among them, in ~/Code/api, ticked", panel.Sessions, panel.Filter.Sessions)
	}
}

// enter on a session running today that also worked yesterday filters the
// tables to its few lines, which read back as on opening to fill the view,
// yesterday's among them, though the table filled its view before.
func TestEnterOnARunningSessionReadsBackItsEarlierLinesToFillTheView(t *testing.T) {
	state := t.TempDir()
	var today []ledger.Held
	for i := range 60 {
		today = append(today, ranIn(fmt.Sprintf("0c%06d", i), webSession, "~/Code/web", -time.Duration(70-i)*time.Minute))
	}
	writeLines(t, state, today...)
	writeLines(t, state,
		ranIn("0b000001", apiSession, "~/Code/api", -day),
		ranIn("0b000002", apiSession, "~/Code/api", -5*time.Minute), ranIn("0b000003", apiSession, "~/Code/api", -4*time.Minute),
	)
	h := newHarness(t, probedWithoutTheRouter())
	onTheLog(h, state)
	h.start()
	if h.model.log.from.Date != dayfile.DateOf(past(0)) {
		t.Fatalf("the Log spans from %s as it opens, want today alone, its view filled", h.model.log.from.Date)
	}

	h.typed("/", "a", "p", "i")
	h.keys(enter)
	if h.model.log.page.Paused() {
		t.Fatal("found as running, api's lines are paused at, want them followed")
	}
	if got, want := shownRequests(h), []string{"0b000001", "0b000002", "0b000003"}; !slices.Equal(got, want) {
		t.Errorf("filtered to api, the Requests show %v, want %v, yesterday's read back to fill the view", got, want)
	}
}

// A session found, its lines fewer than the view, reads back no further
// than the days' summaries say its lines reach: to its first day, every day
// after it read, as the tables hold every day from the first they span; not
// to the oldest day the ledger keeps.
func TestAFoundSessionReadsBackNoFurtherThanItsFirstDay(t *testing.T) {
	tests := []struct {
		name  string
		ran   []int
		reach int
	}{
		{name: "api ran the two days before today", ran: []int{1, 2}, reach: 2},
		{name: "api ran first six days before today", ran: []int{2, 6}, reach: 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := t.TempDir()
			var lines []ledger.Held
			for i := range 60 {
				lines = append(lines, ranIn(fmt.Sprintf("0c%06d", i), webSession, "~/Code/web", -time.Duration(70-i)*time.Minute))
			}
			for d := 1; d <= 30; d++ {
				for i := range 20 {
					lines = append(lines, ranIn(fmt.Sprintf("0d%02d%04d", d, i), webSession, "~/Code/web", -time.Duration(d)*day+time.Duration(i)*time.Minute))
				}
			}
			for _, d := range tt.ran {
				lines = append(lines, ranIn(fmt.Sprintf("0b%06d", d), apiSession, "~/Code/api", -time.Duration(d)*day+time.Hour))
			}
			lines = append(lines, ranIn("0b100001", apiSession, "~/Code/api", -5*time.Minute), ranIn("0b100002", apiSession, "~/Code/api", -4*time.Minute))
			writeLines(t, state, lines...)
			h := newHarness(t, probedWithoutTheRouter())
			l := onTheLog(h, state)
			h.start()
			h.typed("/", "a", "p", "i")
			found := len(l.days)

			h.keys(enter)
			var want []string
			for d := 1; d <= tt.reach; d++ {
				want = append(want, dayfile.DateOf(past(-time.Duration(d)*day)))
			}
			if got := l.days[found:]; !slices.Equal(got, want) {
				t.Errorf("found api, the Log read back the days %v, want %v: to api's first day, and no further", got, want)
			}
		})
	}
}

// esc on the Log, the newest first: closes the find; follows again where
// the table is paused; then lets the filter go, as the find set it.
func TestEscOnTheLogClosesTheFindThenFollowsThenLetsTheFilterGo(t *testing.T) {
	state := t.TempDir()
	writeLines(t, state,
		ranIn("0a000001", apiSession, "~/Code/api", -5*time.Minute), ranIn("0a000002", apiSession, "~/Code/api", -4*time.Minute),
		ranIn("0a000003", webSession, "~/Code/web", -3*time.Minute),
	)
	h := newHarness(t, probedWithoutTheRouter())
	onTheLog(h, state)
	h.start()
	h.typed("/", "a", "p", "i")
	h.keys(enter, tea.KeyPressMsg{Code: tea.KeyUp})
	h.typed("/")

	steps := []struct {
		name                     string
		finding, paused, filters bool
	}{
		{name: "the find closed", paused: true, filters: true},
		{name: "following again", filters: true},
		{name: "the filter let go"},
		{name: "nothing more"},
	}
	for _, step := range steps {
		h.keys(escape)
		p := h.model.log.page
		if p.Finding.Open != step.finding || p.Paused() != step.paused || p.Filtered() != step.filters {
			t.Errorf("after esc, %s: finding %t, paused %t, filtered %t; want %t, %t, %t",
				step.name, p.Finding.Open, p.Paused(), p.Filtered(), step.finding, step.paused, step.filters)
		}
	}
	if got := shownRequests(h); !slices.Equal(got, []string{"0a000001", "0a000002", "0a000003"}) {
		t.Errorf("the filter let go, the Requests show %v, want every session's", got)
	}
}

// / before the Log's first read lands opens the find, its line drawn in the
// state line's row as the design has / open it, what's typed shown as it's
// typed, finding none of today's sessions till the read counts them; and it
// stays open as the read lands, finding the session it reads.
func TestTheFindDrawsItsLineBeforeTheLogsFirstReadLands(t *testing.T) {
	state := t.TempDir()
	writeLines(t, state, ranIn("0a000001", idD28C, "~/Code/web", -time.Minute))
	h := newHarness(t, probedWithoutTheRouter())
	onTheLog(h, state)
	var unread []tea.Msg
	for msgs := h.init(); len(msgs) > 0; msgs = msgs[1:] {
		if _, ok := msgs[0].(logReadMsg); ok {
			unread = append(unread, msgs[0])
			continue
		}
		msgs = append(msgs, h.update(msgs[0])...)
	}
	if len(unread) == 0 || h.model.log.page.Read {
		t.Fatal("the Log's first read landed, want it held")
	}

	h.typed("/", "d")
	stateRow := func() string {
		rows := strings.Split(h.view(), "\n")
		return strings.TrimSpace(rows[len(rows)-2])
	}
	if got := stateRow(); !strings.HasPrefix(got, "/ d▏") || !strings.HasSuffix(got, "no sessions") {
		t.Errorf("/ then d before the Log's first read lands, the state line's row reads %q, want the find's line, finding none yet", got)
	}
	h.deliver(unread...)
	if got := stateRow(); !h.model.log.page.Read || !strings.HasPrefix(got, "/ d▏") || !strings.Contains(got, "d28c web  today") {
		t.Errorf("the Log's first read landed with the find open, the state line's row reads %q, want the find's line still, d28c found today", got)
	}
}

// The panel's /, as it offers, closes it and opens the find.
func TestThePanelsSlashOpensTheFind(t *testing.T) {
	h := newHarness(t, probedWithoutTheRouter())
	onTheLog(h, t.TempDir())
	h.start()
	h.typed("f", "/")
	if h.model.open != nothingOpen || !h.model.finding() {
		t.Errorf("/ on the panel left %d open, finding %t; want the panel closed, the find open", h.model.open, h.model.finding())
	}
}
