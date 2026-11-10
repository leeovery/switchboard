package watch

import (
	"errors"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/theme"
)

// countedPrefs is a preferences file in a directory of the test's own,
// counting the updates made of it.
type countedPrefs struct {
	file    theme.PrefsFile
	updates int
}

func newCountedPrefs(t *testing.T) *countedPrefs {
	t.Helper()
	return &countedPrefs{file: theme.NewPrefsFile(t.TempDir(), func() time.Time { return start })}
}

func (p *countedPrefs) Update(change func(*theme.Prefs)) error {
	p.updates++
	return p.file.Update(change)
}

// keepWritten has the preferences changed outside Update written, as Update
// would have them, and delivers how it went.
func (h *harness) keepWritten() {
	h.t.Helper()
	next, cmd := h.model.keptPrefs()
	h.model = next
	h.deliver(h.run(cmd)...)
}

func TestThePreferencesAreKeptInACommandNeverInUpdate(t *testing.T) {
	h := newHarness(t, calm())
	prefs := &fakePrefs{}
	h.model.cfg.Prefs = prefs
	h.start()

	next, cmd := h.model.Update(tabKey)
	if prefs.updates > 0 {
		t.Errorf("tab kept the preferences in Update, %d times, which holds every key and frame till it's done, want it done in a command", prefs.updates)
	}
	h.model = next.(Model)
	h.deliver(h.run(cmd)...)
	if prefs.updates != 1 || prefs.kept.Tab != string(dashboard.Accounts) {
		t.Errorf("once the commands ran, kept %d times, the tab %q; want accounts kept once", prefs.updates, prefs.kept.Tab)
	}
}

// While a write is under way, the changes made meanwhile wait for it, then
// go in one write, in the order made, so the latest of each is kept.
func TestChangesMadeWhileAWriteIsUnderWayAreKeptAfterItTheLatestStanding(t *testing.T) {
	h := routedHarness(t, routerDocument(three()...))
	prefs := newCountedPrefs(t)
	h.model.cfg.Prefs = prefs
	h.start()

	held := h.press("b")
	held = append(held, h.press("b")...)
	for range 3 {
		held = append(held, h.update(tabKey)...)
	}
	if prefs.updates != 1 {
		t.Fatalf("with a write under way, the keys after it wrote the preferences %d times in all, want once, the first, alone", prefs.updates)
	}
	h.deliver(held...)

	got := prefs.file.Read()
	if prefs.updates != 2 || got.Tab != string(dashboard.Sessions) || got.By != "window" {
		t.Errorf("written %d times, kept the tab %q, by %q; want twice, the latest of each, sessions and window", prefs.updates, got.Tab, got.By)
	}
}

func TestAThemePickedAndATabKeyAtOnceAreBothKept(t *testing.T) {
	h, _ := settledThemedHarness(t, theme.Choice{})
	prefs := newCountedPrefs(t)
	h.model.cfg.Prefs = prefs
	h.typed("t", "down")

	held := h.press("enter")
	held = append(held, h.press("esc")...)
	held = append(held, h.update(tabKey)...)
	h.deliver(held...)

	got := prefs.file.Read()
	if got.Choice != theme.One("nord-wave") || got.Tab != string(dashboard.Accounts) {
		t.Errorf("kept the choice %+v and the tab %q, want both, one nord-wave and accounts", got.Choice, got.Tab)
	}
	if h.model.choice != theme.One("nord-wave") || h.drawnIn() != "nord-wave" {
		t.Errorf("drawn by %+v, in %s; want the choice as kept, nord-wave", h.model.choice, h.drawnIn())
	}
}

// A watch that stops keeps what it changed and hadn't yet kept, the write
// under way's changes among them; and that write, should it run after, writes
// nothing over the later changes.
func TestAWatchThatStopsKeepsWhatItHadntYet(t *testing.T) {
	h := newHarness(t, calm())
	prefs := newCountedPrefs(t)
	h.model.cfg.Prefs = prefs
	h.start()

	next, underWay := h.model.Update(tabKey)
	h.model = next.(Model)
	next, _ = h.model.Update(tabKey)
	h.model = next.(Model)
	h.model.keepLeft()
	if got := prefs.file.Read().Tab; got != string(dashboard.History) {
		t.Errorf("stopped, the watch kept the tab %q, want history, the latest", got)
	}
	h.run(underWay)
	if got := prefs.file.Read().Tab; got != string(dashboard.History) || prefs.updates != 1 {
		t.Errorf("the write under way, run after the watch stopped, left the tab %q, written %d times in all; want history, written once", got, prefs.updates)
	}
}

func TestAWatchQuitJustAfterAKeyKeepsWhatTheKeyChanged(t *testing.T) {
	cfg := noColourConfig()
	prefs := newCountedPrefs(t)
	cfg.Prefs = prefs
	if _, err := watchUntil(t, cfg, "", "\tq", nil); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if got := prefs.file.Read().Tab; got != string(dashboard.Accounts) {
		t.Errorf("quit straight after tab, the watch kept the tab %q, want accounts", got)
	}
}

func TestAThemePickedWithNowhereToKeepItStandsAsItWas(t *testing.T) {
	log := logstest.Capture(t)
	h, _ := settledThemedHarness(t, theme.Choice{})
	h.model.cfg.Prefs = nil
	h.typed("t", "down", "enter")

	if h.model.choice != (theme.Choice{}) || h.model.picker.note != "not kept: see the log" {
		t.Errorf("the choice is %+v, the picker noting %q; want it as it was, the picker saying it wasn't kept", h.model.choice, h.model.picker.note)
	}
	if !log.Has("level=WARN", `msg="couldn't keep the theme chosen"`, "theme=nord-wave", `error="there are no preferences to keep it in"`) {
		t.Errorf("the log reads\n%s\nwant why the choice wasn't kept", log)
	}
}

func TestEveryChangeAWriteCouldntKeepIsLogged(t *testing.T) {
	log := logstest.Capture(t)
	h := routedHarness(t, routerDocument(three()...))
	h.model.cfg.Prefs = &fakePrefs{err: errors.New("keep the preferences: read-only file system")}
	h.start()

	held := h.press("b")
	held = append(held, h.update(tabKey)...)
	h.deliver(held...)
	for _, want := range [][]string{
		{"level=WARN", `msg="couldn't keep how the windows are grouped"`, "by=account"},
		{"level=WARN", `msg="couldn't keep the view shown"`, "view=accounts"},
	} {
		if !log.Has(want...) {
			t.Errorf("log reads\n%s\nwant a line with %q", log, want)
		}
	}
}
