package screen

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/setup"
)

// The ends of a making that made the token but saved nothing, which give
// it back to copy: unread, and not saved.
var (
	madeUnread   = setup.Tokened{Result: setup.TokenUnread, Why: "setup-token showed an error", Shown: []string{"Your OAuth token (valid for 1 year):", "test-token-made"}}
	madeNotSaved = setup.Tokened{Result: setup.TokenNotSaved, Why: "write the token file: permission denied", Shown: []string{"test-token-made"}}
)

// esc at a paste card opened from a card that gives a token made back to
// copy, unread or not saved, goes back to that card, the token there to copy
// again, and p opens the paste card again; opened from any other card, esc
// closes it, with no card behind it.
func TestEscAtThePasteCardGoesBackToATokenMadeButUnsaved(t *testing.T) {
	tests := []struct {
		name string
		ends setup.Tokened
		back bool
	}{
		{name: "unread", ends: madeUnread, back: true},
		{name: "not saved", ends: madeNotSaved, back: true},
		{name: "setup-token's error", ends: notMade},
		{name: "refused", ends: setup.Tokened{Result: setup.TokenRefused}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := tokening(t, fakeMaking{steps: []setup.Making{privately, approving}, ends: &tt.ends}, 1)
			h.steps.nextMake(t)
			h.hearAll(3)
			_, ended := h.card()
			h.pressAndDeliver("p")
			h.typeIn("test-token-typed")
			h.pressAndDeliver("esc")

			if h.model.paste != nil {
				t.Fatalf("esc left the paste card %s, want it closed", pasteSaid(h.model.paste))
			}
			if !tt.back {
				if h.model.token != nil {
					t.Error("esc went back to the token card, want no card behind the paste card")
				}
				return
			}
			title, rows := h.card()
			if title != "NEW TOKEN · PERSONAL" || !slices.Equal(rows, ended) {
				t.Fatalf("esc shows the card %q, reading it as it did: %t; want the token card back, as it was", title, slices.Equal(rows, ended))
			}
			h.pressAndDeliver("p")
			if h.model.paste == nil || h.model.paste.from == nil {
				t.Errorf("p back at the token card opened the paste card %s, going back: %t; want it open again, going back", pasteSaid(h.model.paste), h.model.paste != nil && h.model.paste.from != nil)
			}
		})
	}
}

// A paste that fails, opened from a card that gives a token made back to
// copy, fails no step: the card it opened from comes back as a token made
// that couldn't be saved, why the paste's, never what was pasted, then
// what the card showed, the token, or setup-token's lines, to copy again,
// p pasting and esc closing, as that card's do.
func TestAPasteThatFailsGoesBackToATokenMadeButUnsaved(t *testing.T) {
	tests := []struct {
		name  string
		ends  setup.Tokened
		shown []string
	}{
		{name: "unread", ends: madeUnread, shown: []string{"       │ Your OAuth token (valid for 1 year):", "       │ test-token-made"}},
		{name: "not saved", ends: madeNotSaved, shown: []string{"       │ test-token-made"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := pastingFor(t, tt.ends)
			h.steps.pasteErr = errors.New("write the token file for test-token-typed: no space left on device")
			h.typeIn("test-token-typed")
			h.pressAndDeliver("enter")

			if h.model.paste != nil || h.model.token == nil {
				t.Fatalf("failed, the paste card is %s, the token card open: %t; want the token card back", pasteSaid(h.model.paste), h.model.token != nil)
			}
			want := append([]string{
				"",
				"  !    The token was created, but setup couldn't save it",
				"       write the token file for [redacted]: no space left on device",
				"",
			}, tt.shown...)
			want = append(want,
				"",
				"       It lasts a year on the account, so copy it, then press p to paste it",
				"       once that's put right.",
				"",
				keysOf("p paste a token   esc cancel"),
			)
			checkCard(t, h, "NEW TOKEN · PERSONAL", want)
			if got := h.model.walk.left(); got.Ending == Failed {
				t.Errorf("setup would be left %+v, want the step going on", got)
			}
			h.pressAndDeliver("p")
			if h.model.paste == nil || h.model.paste.from == nil {
				t.Fatalf("p opened the paste card %s, going back: %t; want it open again, going back", pasteSaid(h.model.paste), h.model.paste != nil && h.model.paste.from != nil)
			}
			h.pressAndDeliver("esc")
			h.pressAndDeliver("esc")
			if h.model.carded() {
				t.Error("esc at the token card left a card open, want it closed")
			}
		})
	}
}

// ctrl+c at a paste card leaves setup as the card it opened from would,
// where that gives a token made back to copy: Run gives it back, unread or
// not saved; opened from any other card, Run gives nothing back. A paste
// being checked as the screen goes that saves the token ends that, though
// its answer lands once the screen has gone; one that fails there doesn't,
// and is logged, as nothing else can show it.
func TestRunGivesATokenMadeBackThroughThePasteCard(t *testing.T) {
	tests := []struct {
		name string
		ends setup.Tokened
		// then are the keys typed at the card the making ends at; pasted,
		// how the paste ends once the screen has gone, where enter's
		// pressed, failing with pasteErr where it's set.
		then     string
		pasteErr error
		unsaved  *setup.Tokened
		replaced int
		logged   string
	}{
		{name: "unread", ends: madeUnread, then: "px\x03", unsaved: &madeUnread},
		{name: "not saved", ends: madeNotSaved, then: "px\x03", unsaved: &madeNotSaved},
		{name: "setup-token's error", ends: notMade, then: "px\x03"},
		{name: "unread, the paste saved once the screen's gone", ends: madeUnread, then: "px\r\x03", replaced: 1},
		{
			name: "unread, the paste failing once the screen's gone", ends: madeUnread, then: "px\r\x03",
			pasteErr: errors.New("write the token file: no space left on device"), unsaved: &madeUnread,
			logged: "pasting a token failed once setup's screen had gone",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			steps := newFakeSteps()
			steps.making = fakeMaking{steps: []setup.Making{privately, approving}, ends: &tt.ends}
			steps.pasteUntilGone, steps.pasteErr = true, tt.pasteErr
			reviewed, ended := make(chan struct{}), make(chan struct{})
			var reviewOnce, endOnce sync.Once
			term := terminal{keys: "t", ready: reviewed, then: tt.then, thenReady: ended, saw: func(msg tea.Msg) {
				switch msg.(type) {
				case reviewMsg:
					reviewOnce.Do(func() { close(reviewed) })
				case tokenedMsg:
					endOnce.Do(func() { close(ended) })
				}
			}}
			type ran struct {
				left Left
				err  error
			}
			returned := make(chan ran, 1)
			go func() {
				left, _, err := runUntil(t, Config{Steps: steps, Step: dashboard.SetupAccounts}, term)
				returned <- ran{left: left, err: err}
			}()
			var got ran
			select {
			case got = <-returned:
			case <-time.After(time.Minute):
				t.Fatal("a minute on, run() hadn't returned")
			}
			if got.err != nil || got.left.Ending != Paused {
				t.Errorf("run() = %+v, %v; want setup paused by ctrl+c", got.left.Ending, got.err)
			}
			if !reflect.DeepEqual(got.left.Unsaved, tt.unsaved) {
				t.Errorf("run() gives back %+v, want %+v", got.left.Unsaved, tt.unsaved)
			}
			if got := steps.replacedOf("work"); got != tt.replaced {
				t.Errorf("work's token was replaced %d times, want %d", got, tt.replaced)
			}
			if tt.logged != "" && !log.Has("level=ERROR", tt.logged, "account=work", "no space left on device") {
				t.Errorf("the log reads\n%s\nwant %q, of work, and why", log, tt.logged)
			}
		})
	}
}

// After a paste of a token made but unsaved fails, the accounts are read
// again, the failure is noted in the log once, of the account, without
// what was pasted; then p and a paste that saves close the card, the step
// going on, the account's row chosen once the accounts are read, and
// nothing's left to give back.
func TestAfterAFailedPasteAPasteThatSavesClosesTheCard(t *testing.T) {
	for _, ends := range []setup.Tokened{madeUnread, madeNotSaved} {
		t.Run(ends.Why, func(t *testing.T) {
			log := logstest.Capture(t)
			h := pastingFor(t, ends)
			h.steps.pasteErr = errors.New("write the token file: no space left on device")
			h.typeIn("test-token-typed")
			reviews := h.steps.reviews
			h.pressAndDeliver("enter")

			if h.steps.reviews != reviews+1 {
				t.Errorf("the paste failed, the accounts were read %d more times, want once", h.steps.reviews-reviews)
			}
			var logged []string
			for _, line := range log.Lines() {
				if strings.Contains(line, "a token pasted couldn't be saved") {
					logged = append(logged, line)
				}
			}
			if len(logged) != 1 || !strings.Contains(logged[0], "level=WARN") || !strings.Contains(logged[0], "account=personal") || strings.Contains(log.String(), "test-token-typed") {
				t.Errorf("the log reads\n%s\nwant the failure warned of once, of personal, without what was pasted", log)
			}

			h.steps.pasteErr = nil
			h.pressAndDeliver("p")
			h.typeIn("test-token-again")
			h.pressAndDeliver("enter")
			if h.model.carded() {
				t.Fatalf("saved, a card is open: token %t, paste %s; want none", h.model.token != nil, pasteSaid(h.model.paste))
			}
			if got := h.model.walk.left(); got.Ending == Failed {
				t.Errorf("setup would be left %+v, want the step going on", got)
			}
			if h.model.chosen != 1 || h.steps.replacedOf("personal") != 1 {
				t.Errorf("saved, row %d is chosen, personal's token replaced %d times; want personal's row chosen, its token replaced once", h.model.chosen, h.steps.replacedOf("personal"))
			}
			if unsaved, _ := h.model.tokenLeft(); unsaved != nil {
				t.Errorf("saved, setup would give back %v, want nothing", unsaved.Result)
			}
		})
	}
}

// ctrl+c once a paste of a token made but unsaved has failed, the screen
// up, the not-saved card back, gives back the making's own outcome, as it
// ended: unread still unread, or not saved, each with its own why and what
// it showed.
func TestRunGivesBackTheMakingsOwnOutcomeAfterAFailedPaste(t *testing.T) {
	for _, ends := range []setup.Tokened{madeUnread, madeNotSaved} {
		t.Run(ends.Why, func(t *testing.T) {
			steps := newFakeSteps()
			steps.making = fakeMaking{steps: []setup.Making{privately, approving}, ends: &ends}
			steps.pasteErr = errors.New("write the token file: no space left on device")
			reviewed, ended, pasted := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var reviewOnce, endOnce, pasteOnce sync.Once
			term := terminal{keys: "t", ready: reviewed, then: "px\r", thenReady: ended, last: "\x03", lastReady: pasted, saw: func(msg tea.Msg) {
				switch msg.(type) {
				case reviewMsg:
					reviewOnce.Do(func() { close(reviewed) })
				case tokenedMsg:
					endOnce.Do(func() { close(ended) })
				case pastedMsg:
					pasteOnce.Do(func() { close(pasted) })
				}
			}}
			type ran struct {
				left Left
				err  error
			}
			returned := make(chan ran, 1)
			go func() {
				left, _, err := runUntil(t, Config{Steps: steps, Step: dashboard.SetupAccounts}, term)
				returned <- ran{left: left, err: err}
			}()
			var got ran
			select {
			case got = <-returned:
			case <-time.After(time.Minute):
				t.Fatal("a minute on, run() hadn't returned")
			}
			if got.err != nil || got.left.Ending != Paused {
				t.Errorf("run() = %v, %v; want setup paused by ctrl+c", got.left.Ending, got.err)
			}
			if !reflect.DeepEqual(got.left.Unsaved, &ends) {
				t.Errorf("run() gives back %+v, want the making's own outcome %+v", got.left.Unsaved, ends)
			}
		})
	}
}
