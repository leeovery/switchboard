package screen

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dashboard/backdrop"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/setup"
)

func TestRunRefusesAnOutputThatIsntATerminal(t *testing.T) {
	if _, err := Run(t.Context(), Config{Steps: newFakeSteps()}, &bytes.Buffer{}, nil); err != ErrNotTerminal {
		t.Errorf("Run() on a buffer: error = %v, want ErrNotTerminal", err)
	}
}

// standingTillCancelled is setup's steps whose Standing holds on until its
// context is cancelled, saying it's started, and once it's cancelled, ends
// only once it's let go.
type standingTillCancelled struct {
	*fakeSteps
	started, cancelled, release chan struct{}
	ended                       chan struct{}
}

func (s *standingTillCancelled) Standing(ctx context.Context) (setup.RouterStanding, error) {
	close(s.started)
	<-ctx.Done()
	close(s.cancelled)
	<-s.release
	close(s.ended)
	return setup.RouterStanding{}, ctx.Err()
}

func TestRunEndsEveryJobBeforeItReturns(t *testing.T) {
	steps := &standingTillCancelled{
		fakeSteps: newFakeSteps(),
		started:   make(chan struct{}), cancelled: make(chan struct{}), release: make(chan struct{}), ended: make(chan struct{}),
	}
	type ran struct {
		left Left
		err  error
	}
	returned := make(chan ran, 1)
	go func() {
		left, _, err := runUntil(t, Config{Steps: steps, Step: dashboard.SetupLink}, terminal{keys: "q", ready: steps.started})
		returned <- ran{left: left, err: err}
	}()

	<-steps.cancelled
	select {
	case <-returned:
		t.Fatal("run() returned while Standing ran on, want it to wait for it to end")
	case <-time.After(overlap):
	}
	close(steps.release)
	got := <-returned
	select {
	case <-steps.ended:
	default:
		t.Error("run() returned before Standing ended")
	}
	if got.err != nil || how(got.left) != (Left{Ending: Paused}) {
		t.Errorf("run() = %+v, %v; want setup paused, no error", got.left, got.err)
	}
}

// settingTillReleased is setup's steps whose SetDay says it's started, and
// ends only once it's let go, saying it has.
type settingTillReleased struct {
	*fakeSteps
	started, release, ended chan struct{}
}

func (s *settingTillReleased) SetDay(day string) (config.Day, error) {
	close(s.started)
	<-s.release
	defer close(s.ended)
	return config.ParseDay(day)
}

func TestQWhileTheDayIsSetLeavesOnceTheWriteHasEnded(t *testing.T) {
	steps := &settingTillReleased{
		fakeSteps: newFakeSteps(),
		started:   make(chan struct{}), release: make(chan struct{}), ended: make(chan struct{}),
	}
	primed := make(chan struct{})
	var once sync.Once
	term := terminal{
		keys: "09:00-18:00\r", ready: primed,
		then: "q", thenReady: steps.started,
		saw: func(msg tea.Msg) {
			if _, ok := msg.(primedMsg); ok {
				once.Do(func() { close(primed) })
			}
		},
	}
	type ran struct {
		left Left
		err  error
	}
	returned := make(chan ran, 1)
	go func() {
		left, _, err := runUntil(t, Config{Steps: steps, Step: dashboard.SetupPriming}, term)
		returned <- ran{left: left, err: err}
	}()

	select {
	case <-steps.started:
	case got := <-returned:
		t.Fatalf("run() = %+v, %v before SetDay started, want the day typed and entered to set it", got.left, got.err)
	case <-time.After(time.Minute):
		t.Fatal("SetDay hadn't started a minute on, want the day typed and entered to set it")
	}
	select {
	case <-returned:
		t.Fatal("run() returned while SetDay ran on, want it to wait for the write to end")
	case <-time.After(overlap):
	}
	close(steps.release)
	got := <-returned
	select {
	case <-steps.ended:
	default:
		t.Error("run() returned before SetDay ended")
	}
	if got.err != nil || how(got.left) != (Left{Ending: Paused}) {
		t.Errorf("run() = %+v, %v; want setup paused, no error", got.left, got.err)
	}
}

func TestRunPutsTheBackgroundBackAndSaysHowSetupWasLeft(t *testing.T) {
	cfg := Config{Steps: newFakeSteps(), Step: dashboard.SetupService, Pair: nord(t)}
	left, out, err := runUntil(t, cfg, terminal{answer: darkAnswer, keys: "q"})
	if err != nil || how(left) != (Left{Ending: Paused}) {
		t.Errorf("run() = %+v, %v; want setup paused, no error", left, err)
	}
	if want := ansi.SetBackgroundColor("#1a1b26"); !strings.HasSuffix(out, want) {
		t.Errorf("the screen's output ends %q, want the background put back with %q", out[max(len(out)-80, 0):], want)
	}
}

// skillPanicking is setup's steps whose Skill panics.
type skillPanicking struct {
	*fakeSteps
}

func (skillPanicking) Skill() (setup.Said, error) {
	panic("a bug")
}

func TestAStepsPanicStopsTheScreenAndPutsTheBackgroundBack(t *testing.T) {
	cfg := Config{Steps: skillPanicking{newFakeSteps()}, Step: dashboard.SetupLink, Pair: nord(t)}
	hooked := make(chan struct{})
	var once sync.Once
	saw := func(msg tea.Msg) {
		if _, ok := msg.(hookedMsg); ok {
			once.Do(func() { close(hooked) })
		}
	}
	_, out, err := runUntil(t, cfg, terminal{answer: darkAnswer, keys: "\r", ready: hooked, saw: saw})
	if !errors.Is(err, tea.ErrProgramPanic) {
		t.Errorf("run() error = %v, want the panic Bubble Tea recovered from", err)
	}
	if want := ansi.SetBackgroundColor("#1a1b26"); !strings.HasSuffix(out, want) {
		t.Errorf("the screen's output ends %q, want the background put back with %q", out[max(len(out)-80, 0):], want)
	}
}

// reviewPanicking is setup's steps whose Review panics once the screen has
// settled, so the background is set to be put back.
type reviewPanicking struct {
	*fakeSteps
	settled chan struct{}
}

func (s reviewPanicking) Review() (setup.Review, error) {
	<-s.settled
	panic("a bug")
}

func TestAPanicAsTheScreenOpensIsLoggedAndPutsTheBackgroundBack(t *testing.T) {
	log := logstest.Capture(t)
	steps := reviewPanicking{fakeSteps: newFakeSteps(), settled: make(chan struct{})}
	cfg := Config{Steps: steps, Step: dashboard.SetupService, Pair: nord(t)}
	_, out, err := runUntil(t, cfg, terminal{answer: darkAnswer, settled: steps.settled})
	if !errors.Is(err, tea.ErrProgramPanic) {
		t.Errorf("run() error = %v, want the panic Bubble Tea recovered from", err)
	}
	if want := ansi.SetBackgroundColor("#1a1b26"); !strings.HasSuffix(out, want) {
		t.Errorf("the screen's output ends %q, want the background put back with %q", out[max(len(out)-80, 0):], want)
	}
	if !log.Has("level=ERROR", "panicked", "a bug", "stack=") {
		t.Errorf("the log reads\n%s\nwant Review's panic, and where it was", log)
	}
}

// darkAnswer is a terminal's answer that its background is #1a1b26.
const darkAnswer = "\x1b]11;rgb:1a1a/1b1b/2626\x07"

// terminal is how a test's terminal acts as setup's screen runs on it: it
// answers the screen's question of its background with answer, "" for
// none, and once the screen has settled, closing settled, where it's set,
// and once ready, where it's set, it types keys; then once thenReady, where
// it's set, it types then; then once lastReady, where it's set, it types
// last. It tells saw, where it's set, each message the screen takes in,
// before it does. Once the screen has gone, the moment to say what setup's
// finishing comes as moment, given the jobs run waits for, has it, never
// where it's nil; and wrote, where it's set, is told of each write to the
// terminal.
type terminal struct {
	answer, keys, then, last    string
	ready, thenReady, lastReady <-chan struct{}
	settled                     chan struct{}
	saw                         func(tea.Msg)
	moment                      func(*jobs) <-chan time.Time
	wrote                       func(p []byte)
}

// runUntil runs setup's screen of cfg on a terminal acting as term does,
// until it stops, as runIn has it, under the test's context.
func runUntil(t *testing.T, cfg Config, term terminal) (Left, string, error) {
	t.Helper()
	return runIn(t, t.Context(), cfg, term)
}

// runIn runs setup's screen of cfg under ctx on a terminal acting as term
// does, until it stops. It returns how setup was left, what the screen
// wrote, and how it stopped.
func runIn(t *testing.T, ctx context.Context, cfg Config, term terminal) (Left, string, error) {
	t.Helper()
	// Bubble Tea reads these itself: set, they have it write a trace, or a
	// log of a panic it recovers from in the working directory.
	t.Setenv("TEA_TRACE", "")
	t.Setenv("TEA_DEBUG", "")
	out := lockedBuffer{wrote: term.wrote}
	in, typing := io.Pipe()
	t.Cleanup(func() { _ = typing.Close() })
	var settle sync.Once
	settled := term.settled
	if settled == nil {
		settled = make(chan struct{})
	}
	filter := func(_ tea.Model, msg tea.Msg) tea.Msg {
		if term.saw != nil {
			term.saw(msg)
		}
		switch msg.(type) {
		case tea.BackgroundColorMsg, backdrop.Unanswered:
			settle.Do(func() { close(settled) })
		}
		return msg
	}
	cfg.After = func(time.Duration, tea.Msg) tea.Cmd { return nil }
	go func() {
		if term.answer != "" {
			_, _ = io.WriteString(typing, term.answer)
			<-settled
		}
		if term.ready != nil {
			<-term.ready
		}
		_, _ = io.WriteString(typing, term.keys)
		if term.thenReady != nil {
			<-term.thenReady
			_, _ = io.WriteString(typing, term.then)
		}
		if term.lastReady != nil {
			<-term.lastReady
			_, _ = io.WriteString(typing, term.last)
		}
	}()
	moment := func(js *jobs) <-chan time.Time {
		if term.moment == nil {
			return nil
		}
		return term.moment(js)
	}
	left, err := run(ctx, cfg, &out, moment, filter, tea.WithInput(in), tea.WithoutSignals())
	return left, out.String(), err
}

// lockedBuffer is a buffer Bubble Tea's goroutines can all write to,
// telling wrote, where it's set, of each write.
type lockedBuffer struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	wrote func(p []byte)
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.wrote != nil {
		b.wrote(p)
	}
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// holdingTold notes what Holding is told: how many times, and whether it was
// first told only once the screen had left the alternate screen, which
// Bubble Tea does as it gives the terminal back.
type holdingTold struct {
	mu                sync.Mutex
	told              int
	leftAlt, lateTold bool
}

func (h *holdingTold) holding() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.told == 0 {
		h.lateTold = h.leftAlt
	}
	h.told++
}

func (h *holdingTold) wrote(p []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.leftAlt = h.leftAlt || bytes.Contains(p, []byte(ansi.ResetModeAltScreenSaveCursor))
}

func (h *holdingTold) state() (told int, leftAlt, lateTold bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.told, h.leftAlt, h.lateTold
}

// Holding is told as the screen goes with a token's making in hand, which
// Run may give back, before Bubble Tea gives the terminal back, where a
// ctrl+c is an interrupt again; where Bubble Tea passes its filter by, as
// the context ending does, as Run returns.
func TestHoldingIsToldAsTheScreenGoesWithATokensMakingInHand(t *testing.T) {
	tests := []struct {
		name string
		// ctrlC leaves by ctrl+c, else the context ends.
		ctrlC bool
		// late is whether Holding may first be told once the terminal is
		// given back.
		late bool
	}{
		{name: "ctrl+c", ctrlC: true},
		{name: "the context ending", late: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps := newFakeSteps()
			reported := make(chan struct{})
			steps.making = fakeMaking{steps: []setup.Making{privately, approving}, reported: reported}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			reviewed := make(chan struct{})
			var once sync.Once
			told := &holdingTold{}
			term := terminal{keys: "t", ready: reviewed, wrote: told.wrote, saw: func(msg tea.Msg) {
				if _, ok := msg.(reviewMsg); ok {
					once.Do(func() { close(reviewed) })
				}
			}}
			if tt.ctrlC {
				term.then, term.thenReady = "\x03", reported
			} else {
				go func() {
					<-reported
					cancel()
				}()
			}

			if _, _, err := runIn(t, ctx, Config{Steps: steps, Step: dashboard.SetupAccounts, Holding: told.holding}, term); tt.ctrlC && err != nil {
				t.Fatalf("run() error = %v, want none", err)
			}
			n, leftAlt, late := told.state()
			if n == 0 {
				t.Fatal("Holding was never told, want it told as the screen went with the token card open")
			}
			if !tt.late && (!leftAlt || late) {
				t.Errorf("Holding was first told once the alternate screen was left: %t, the alternate screen left at all: %t; want it told before, as Bubble Tea heeds the quit", late, leftAlt)
			}
		})
	}
}

func TestHoldingIsntToldWhereTheScreenGoesWithNoTokensMakingInHand(t *testing.T) {
	told := &holdingTold{}
	cfg := Config{Steps: newFakeSteps(), Step: dashboard.SetupService, Holding: told.holding}
	if _, _, err := runUntil(t, cfg, terminal{keys: "q"}); err != nil {
		t.Fatalf("run() error = %v, want none", err)
	}
	if n, _, _ := told.state(); n != 0 {
		t.Errorf("Holding was told %d times, want never, with no token card open", n)
	}
}

func TestATokensMakingIsInHandWhileItsCardIsOpenOrThePasteCardOpenedFromIt(t *testing.T) {
	card := &tokenCard{}
	tests := []struct {
		name string
		m    Model
		want bool
	}{
		{name: "no card open"},
		{name: "the token card open", m: Model{token: card}, want: true},
		{name: "the paste card open, opened from the token card", m: Model{paste: &pasteCard{from: card}}, want: true},
		{name: "the paste card open, opened from the add card", m: Model{paste: &pasteCard{}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var told bool
			tt.m.holding(func() { told = true })
			if told != tt.want {
				t.Errorf("Holding told: %t, want %t", told, tt.want)
			}
			tt.m.holding(nil)
		})
	}
}

// Bubble Tea's filter tells Holding of the screen quitting, or interrupted
// from outside, as an interrupt sent to the process during the screen is,
// with a token's making in hand, and of nothing else; and passes every
// message on to heed.
func TestTheFilterTellsHoldingOfAQuitOrAnInterruptWithATokensMakingInHand(t *testing.T) {
	inHand := Model{token: &tokenCard{}}
	tests := []struct {
		name string
		m    Model
		msg  tea.Msg
		want bool
	}{
		{name: "a quit, a making in hand", m: inHand, msg: tea.QuitMsg{}, want: true},
		{name: "an interrupt, a making in hand", m: inHand, msg: tea.InterruptMsg{}, want: true},
		{name: "a key, a making in hand", m: inHand, msg: tea.KeyPressMsg{Code: 'q', Text: "q"}},
		{name: "a quit, no making in hand", msg: tea.QuitMsg{}},
		{name: "an interrupt, no making in hand", msg: tea.InterruptMsg{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var told bool
			var heeded tea.Msg
			filter := heeding(func() { told = true }, func(_ tea.Model, msg tea.Msg) tea.Msg {
				heeded = msg
				return msg
			})
			if got := filter(tt.m, tt.msg); got != tt.msg || heeded != tt.msg {
				t.Errorf("the filter gave %v, heed was given %v; want the message passed through heed", got, heeded)
			}
			if told != tt.want {
				t.Errorf("Holding told: %t, want %t", told, tt.want)
			}
		})
	}
}
