package watch

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

// loadingSaid is what a page says while it loads.
const loadingSaid = "loading…"

// askedPage asks for the page of the session with the given id, as a click
// on its row does, and returns its read, held back undelivered as one still
// under way, and the timer armed for its page to say it's loading.
func (h *harness) askedPage(id string) ([]tea.Msg, *timer) {
	h.t.Helper()
	next, cmd := h.model.openSession(id)
	h.model = next
	read := h.run(cmd)
	tm, ok := h.pending(pageLoadingMsg{asked: h.model.opening, id: id})
	if len(read) == 0 || !ok {
		h.t.Fatalf("asked for %s's page, its read sent back %d messages, and its loading was armed %t; want its read, and its loading armed", id, len(read), ok)
	}
	return read, tm
}

// row is the screen's row y as drawn, without its escapes, its blanks at
// either end trimmed.
func (h *harness) row(y int) string {
	return strings.TrimSpace(strings.Split(h.view(), "\n")[y])
}

// A session's page whose read lands within LoadingAfter of being asked for
// opens as it lands, saying nothing of loading: the tab it's opened over
// shows, and acts, as it is till then, and the page stays as it opened once
// LoadingAfter has passed.
func TestASessionsPageLandingSoonSaysNothingOfLoading(t *testing.T) {
	h, _ := pageHarness(t, promptedLines(2, start.Add(-30*time.Minute)))
	read, loading := h.askedPage(idD28C)

	h.clock.now = h.clock.now.Add(LoadingAfter - time.Millisecond)
	h.redraw()
	if h.model.session != nil {
		t.Fatalf("its read under way short of %s, a page is open, want the tab as it was", LoadingAfter)
	}
	h.deliver(read...)
	h.fire(loading)
	if s := h.model.session; s == nil || s.shown.Loading || len(s.shown.Page.Turns) == 0 || strings.Contains(h.view(), loadingSaid) {
		t.Errorf("its read landed short of %s, the page is %+v; want it open as it landed, saying nothing of loading", LoadingAfter, s)
	}
}

// A session's page whose read takes LoadingAfter or longer opens as it's
// asked for, loading: its heading alone, naming the session as the router
// lists it, then loading…, in place of the tab, and esc back its only key,
// those of its turns and p doing nothing; its read landing builds it in
// place.
func TestASessionsPageSaysItsLoadingWhileItsReadTakesAWhile(t *testing.T) {
	h, _ := pageHarness(t, promptedLines(2, start.Add(-30*time.Minute)))
	h.source.sessions[0].Assignments[0].Dir = "~/Code/switchboard"
	h.tickUntilAsked()
	read, loading := h.askedPage(idD28C)
	h.fire(loading)

	heading := h.row(4)
	if s := h.model.session; s == nil || !s.shown.Loading || !strings.HasPrefix(heading, "SESSION   d28c   switchboard  ~/Code/switchboard   loading…") ||
		!strings.HasSuffix(heading, "esc back") {
		t.Fatalf("its read under way %s on, the heading reads %q; want the page loading, naming d28c, switchboard and its directory, then loading…", LoadingAfter, heading)
	}
	for y := 6; y < 37; y++ {
		if got := h.row(y); got != "" {
			t.Errorf("loading, row %d reads %q, want it blank", y, got)
		}
	}
	if got := h.listed(); !slices.Equal(got, []string{"esc", "?", "q"}) {
		t.Errorf("loading, the key line lists %q, want esc of its own, then ? and q", got)
	}
	h.keyed("down enter p")
	if s := h.model.session; s == nil || !s.shown.Loading || h.model.open != nothingOpen {
		t.Errorf("loading, ↓ enter p left the page %+v, %d open; want it loading still, nothing open", s, h.model.open)
	}

	h.deliver(read...)
	if s := h.model.session; s == nil || s.shown.Loading || len(s.shown.Page.Turns) != 2 || strings.Contains(h.row(4), loadingSaid) {
		t.Errorf("its read landed, the page is %+v, its heading %q; want it built, two turns, saying nothing of loading", s, h.row(4))
	}
}

// A session's page closed, asked for, its read under way, opens nothing
// once the read lands: esc back while it loads, tab while it loads, and esc
// before it says it's loading, on the tab it was asked from.
func TestASessionsPageClosedWhileItsReadIsUnderWayOpensNothing(t *testing.T) {
	tests := []struct {
		name    string
		loading bool
		key     tea.KeyPressMsg
		view    dashboard.View
	}{
		{name: "esc while it loads", loading: true, key: escKey, view: dashboard.Overview},
		{name: "tab while it loads", loading: true, key: tabKey, view: dashboard.Accounts},
		{name: "esc before it says it's loading", key: escKey, view: dashboard.Overview},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := pageHarness(t, promptedLines(2, start.Add(-30*time.Minute)))
			read, loading := h.askedPage(idD28C)
			if tt.loading {
				h.fire(loading)
			}
			h.keys(tt.key)
			if !tt.loading {
				h.fire(loading)
			}
			h.deliver(read...)
			if h.model.session != nil || h.model.view != tt.view {
				t.Errorf("closed, its read landing left the page %+v open, %s shown; want none, %s", h.model.session, h.model.view, tt.view)
			}
		})
	}
}

// While a session's page loads, the stream telling of its requests, or the
// router going silent, changes nothing of it; once its read lands, it's
// built of what they told: its newest turn grown by the request the stream
// told of; or, the router gone, running as its last listing had it, p
// faint, as no router answers to pin through.
func TestASessionsPageLoadingTakesInTheStreamAndTheRouterGoingSilent(t *testing.T) {
	tests := []struct {
		name     string
		meantime func(h *harness)
		requests int
		gone     bool
	}{
		{name: "the stream telling of its request", requests: 2, meantime: func(h *harness) {
			h.hear(alter(told(router.StreamSent, "r3", "work", 0), func(e *router.StreamEvent) { e.Prompt, e.Class = "prompt-2", ledger.ClassMain }))
		}},
		{name: "the router going silent", requests: 1, gone: true, meantime: func(h *harness) {
			h.stopRouter()
			h.tickUntilAsked()
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := pageHarness(t, promptedLines(2, start.Add(-30*time.Minute)))
			read, loading := h.askedPage(idD28C)
			h.fire(loading)
			tt.meantime(h)
			if s := h.model.session; s == nil || !s.shown.Loading || h.model.answering() == tt.gone {
				t.Fatalf("meantime, the page is %+v, the router answering %t; want it loading still, the router answering %t", s, h.model.answering(), !tt.gone)
			}

			h.deliver(read...)
			s := h.model.session
			if s == nil || s.shown.Loading || len(s.shown.Page.Turns) != 2 || s.shown.Page.Turns[0].Requests != tt.requests || !s.shown.Page.Running {
				t.Fatalf("its read landed, the page is %+v; want it built, running, its newest turn of %d requests", s, tt.requests)
			}
			if pin, listed := h.model.pinKey(); !listed || pin.Off != tt.gone {
				t.Errorf("its read landed, p is listed %t, faint %t; want it listed, faint %t", listed, pin.Off, tt.gone)
			}
		})
	}
}

// A tab whose read takes LoadingAfter or longer says so in its heading row,
// loading… after its sub-tabs, till the read lands; one landing sooner says
// nothing of it.
func TestATabSaysItsLoadingWhileItsReadTakesAWhile(t *testing.T) {
	tests := []struct {
		name string
		late bool
	}{
		{name: "its read landing late", late: true},
		{name: "its read landing soon"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, probedWithoutTheRouter())
			onTheLog(h, t.TempDir())
			unread := h.holdingReads(h.init()...)
			loading, ok := h.pending(loadingMsg{})
			if len(unread) == 0 || !ok {
				t.Fatalf("the Log's first read held back %d, its loading armed %t; want the read held, its loading armed", len(unread), ok)
			}
			if got := h.row(4); strings.Contains(got, loadingSaid) {
				t.Errorf("its read just asked for, the heading reads %q, want nothing of loading", got)
			}
			if !tt.late {
				h.deliver(unread...)
			}
			h.fire(loading)
			if got, want := h.row(4), tt.late; strings.Contains(got, "Requests  Events  [ ]   "+loadingSaid) != want {
				t.Errorf("%s on, the heading reads %q; want it saying loading… after the sub-tabs %t", LoadingAfter, got, want)
			}
			h.deliver(unread...)
			if got := h.row(4); strings.Contains(got, loadingSaid) {
				t.Errorf("its read landed, the heading reads %q, want nothing of loading", got)
			}
		})
	}
}

// What names the sessions running waits on the List held: Accounts' Detail
// of an account sessions run on, and the Overview's SESSIONS, say loading…
// while its first read takes LoadingAfter or longer, Detail in its heading
// row and the Overview, which has none, in SESSIONS' heading, till it lands.
func TestWhatNamesTheSessionsRunningSaysItsLoadingTillTheListLands(t *testing.T) {
	tests := []struct {
		name  string
		shown func(h *harness)
		// said is what the row saying it's loading reads.
		said string
	}{
		{name: "the Overview's SESSIONS", shown: func(h *harness) {
			h.model.sections = dashboard.Sections{}.Toggled(dashboard.SectionSessions)
		}, said: "SESSIONS   " + loadingSaid},
		{name: "Accounts' Detail of an account sessions run on", shown: func(h *harness) {
			h.model.view = dashboard.Accounts
			h.model.accounts.shown = dashboard.AccountsShown{Accounts: threeConfigured, Shown: dashboard.AccountsDetail, Chosen: "work"}
		}, said: "Detail  [ ]   " + loadingSaid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := routedHarness(t, routerDocument(three()...))
			h.source.sessions = []status.Session{sessionOn(idD28C, opus, "work", 0)}
			h.model.cfg.Ledger = ledger.NewFollower(t.TempDir(), h.clock.Now, ledger.Caps{Shared: policy.Shared}, logs.For("watch"))
			tt.shown(h)
			unread := holdingBack[listReadMsg](h, h.init()...)
			loading, ok := h.pending(loadingMsg{})
			if len(unread) == 0 || !ok {
				t.Fatalf("the List's first read held back %d, its loading armed %t; want the read held, its loading armed", len(unread), ok)
			}
			h.fire(loading)
			if !strings.Contains(h.view(), tt.said) {
				t.Errorf("%s on, the List unread, the screen is\n%s\nwant %q", LoadingAfter, h.view(), tt.said)
			}
			h.deliver(unread...)
			if strings.Contains(h.view(), loadingSaid) {
				t.Errorf("the List read, the screen is\n%s\nwant nothing of loading", h.view())
			}
		})
	}
}

// The Log shown the first time while another read of the ledger is under
// way, which its own waits for, says loading… once LoadingAfter has passed,
// as its own read under way would, till its own lands.
func TestTheLogShownBehindAnotherReadSaysItsLoading(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	h.source.sessions = []status.Session{sessionOn(idD28C, opus, "work", 0)}
	h.model.cfg.Ledger = ledger.NewFollower(t.TempDir(), h.clock.Now, ledger.Caps{Shared: policy.Shared}, logs.For("watch"))
	h.model.sections = dashboard.Sections{}.Toggled(dashboard.SectionSessions)
	list := holdingBack[listReadMsg](h, h.init()...)
	if len(list) == 0 {
		t.Fatal("no read of the List is under way")
	}
	unread := holdingBack[listReadMsg](h, h.update(shiftTabKey)...)
	if h.model.view != dashboard.Log || h.model.log.reading || len(unread) > 0 {
		t.Fatalf("shift-tab from the Overview shows %s, its read under way %t, want the Log, its read waiting on the List's", h.model.view, h.model.log.reading)
	}
	loading, ok := h.pending(loadingMsg{})
	if !ok {
		t.Fatal("the Log shown waiting on another read armed no loading")
	}
	h.fire(loading)
	if got := h.row(4); !strings.Contains(got, loadingSaid) {
		t.Errorf("%s on, its read waiting, the Log's heading reads %q, want it saying loading…", LoadingAfter, got)
	}
	h.deliver(list...)
	if got := h.row(4); !h.model.log.opened || strings.Contains(got, loadingSaid) {
		t.Errorf("the List's read landed, and the Log's own after it, the Log opened %t, its heading reading %q, want it open, saying nothing of loading", h.model.log.opened, got)
	}
}
