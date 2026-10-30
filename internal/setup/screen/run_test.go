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
	if got.err != nil || got.left != (Left{Ending: Paused}) {
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
	if got.err != nil || got.left != (Left{Ending: Paused}) {
		t.Errorf("run() = %+v, %v; want setup paused, no error", got.left, got.err)
	}
}

func TestRunPutsTheBackgroundBackAndSaysHowSetupWasLeft(t *testing.T) {
	cfg := Config{Steps: newFakeSteps(), Step: dashboard.SetupService, Pair: nord(t)}
	left, out, err := runUntil(t, cfg, terminal{answer: darkAnswer, keys: "q"})
	if err != nil || left != (Left{Ending: Paused}) {
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
	_, out, err := runUntil(t, cfg, terminal{answer: darkAnswer, keys: "\r"})
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
	left, err := run(ctx, cfg, &out, moment, tea.WithInput(in), tea.WithoutSignals(), tea.WithFilter(filter))
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
