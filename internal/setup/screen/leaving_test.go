package screen

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/service"
	"github.com/leeovery/switchboard/internal/setup"
)

// how is how setup was left, its summary aside, for a test holding the rest
// to what it wants.
func how(left Left) Left {
	left.Summary = setup.Summary{}
	return left
}

// givenBack is what setup left gives back of a making that ended as unsaved
// says: none where it's nil.
func givenBack(unsaved *setup.Tokened) []setup.Tokened {
	if unsaved == nil {
		return nil
	}
	return []setup.Tokened{*unsaved}
}

// tokeneds is how each of the makings left ended, without whose they are.
func tokeneds(left []TokenLeft) []setup.Tokened {
	var ended []setup.Tokened
	for _, l := range left {
		ended = append(ended, l.Tokened)
	}
	return ended
}

// sameLeft reports whether a and b say setup was left the same way.
func sameLeft(a, b Left) bool {
	return reflect.DeepEqual(a, b)
}

// read are lines as a test reads them, a line each: its mark, its label and
// its detail, and under it, what it leaves to add by hand, where it leaves
// anything.
func read(lines []setup.StepLine) []string {
	marks := map[setup.Mark]string{setup.Done: "✓", setup.Warning: "!", setup.Failed: "✗"}
	var got []string
	for _, l := range lines {
		got = append(got, marks[l.Mark]+" "+l.Label+" "+l.Detail)
		if l.Add != "" {
			got = append(got, "  "+l.Add)
		}
	}
	return got
}

// The lines each step leaves as fakeSteps answers.
const (
	accountsLine = "✓ Accounts 3, all working · primary: work"
	primingPast  = "✓ Priming "
	serviceLine  = "✓ Service installed · router healthy"
	linkPast     = "✓ claude link "
	skillLine    = "✓ Skill installed"
)

// sawEach is a channel closed once the screen has taken in a message of each
// kind given, as saw, for a terminal, tells it.
func sawEach(kinds ...func(tea.Msg) bool) (<-chan struct{}, func(tea.Msg)) {
	ready := make(chan struct{})
	var mu sync.Mutex
	var once sync.Once
	left := slices.Clone(kinds)
	return ready, func(msg tea.Msg) {
		mu.Lock()
		defer mu.Unlock()
		left = slices.DeleteFunc(left, func(kind func(tea.Msg) bool) bool { return kind(msg) })
		if len(left) == 0 {
			once.Do(func() { close(ready) })
		}
	}
}

// kind reports whether a message is an M.
func kind[M tea.Msg](msg tea.Msg) bool {
	_, ok := msg.(M)
	return ok
}

func TestRunGivesTheSummaryAsSetupIsLeft(t *testing.T) {
	tests := []struct {
		name string
		at   dashboard.SetupStep
		// failing has the skill step fail; keys are typed once the step's
		// action has answered, and the accounts have been read.
		failing bool
		keys    string
		want    Left
		lines   []string
	}{
		{name: "q mid-way: paused", at: dashboard.SetupService, keys: "q", want: Left{Ending: Paused}, lines: []string{accountsLine, primingPast, serviceLine}},
		{name: "q at Done: finished", at: dashboard.SetupSkill, keys: "\rq", want: Left{Ending: Finished}, lines: []string{accountsLine, primingPast, "✓ Service ", linkPast, skillLine}},
		{name: "enter at Done: to the dashboard", at: dashboard.SetupSkill, keys: "\r\r", want: Left{Ending: ToDashboard}, lines: []string{accountsLine, primingPast, "✓ Service ", linkPast, skillLine}},
		{
			name: "a step that failed: failing, its line saying why", at: dashboard.SetupSkill, failing: true, keys: "q",
			want:  Left{Ending: Failed, Err: errNoSkill},
			lines: []string{accountsLine, primingPast, "✓ Service ", linkPast, "✗ Skill not installed: " + errNoSkill.Error()},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps := newFakeSteps()
			if tt.failing {
				steps.said, steps.skillErr = notWritten, errNoSkill
			}
			// The service step asks for the accounts again once it's done,
			// and an answer to an ask before the last is dropped.
			acted, asks := kind[servicedMsg], 2
			if tt.at == dashboard.SetupSkill {
				acted, asks = kind[skilledMsg], 1
			}
			ready, saw := sawEach(acted, func(msg tea.Msg) bool {
				r, ok := msg.(reviewMsg)
				return ok && r.ask == asks
			})
			left, _, err := runUntil(t, Config{Steps: steps, Step: tt.at}, terminal{keys: tt.keys, ready: ready, saw: saw})
			if err != nil || !sameLeft(how(left), tt.want) {
				t.Errorf("run() = %+v, %v; want %+v, no error", how(left), err, tt.want)
			}
			if got := read(left.Summary.Lines()); !slices.Equal(got, tt.lines) {
				t.Errorf("the summary reads\n%q\nwant\n%q", got, tt.lines)
			}
		})
	}
}

func TestTheSummaryLeftAtDoneHasDonesLines(t *testing.T) {
	for _, found := range []setup.LinkFound{linkThrough, linkLeft, situated(setup.UnknownShell, "is fish, which setup can't ask")} {
		h := walkedToDone(t, found, config.Day{Start: 8 * time.Hour, End: 23 * time.Hour})
		h.press("q")
		want := h.model.page().Lines()
		if got := h.model.gone().leaving().Lines(); !slices.Equal(got, want) {
			t.Errorf("the summary as setup is left reads\n%q\nwant Done's\n%q", read(got), read(want))
		}
	}
}

func TestTheLinksLineAsSetupIsLeftSaysWhatsLeftToAddUnderIt(t *testing.T) {
	tests := []struct {
		name  string
		found setup.LinkFound
		want  []string
	}{
		{name: "the line", found: linkLeft, want: []string{"! claude link created, but it isn't first on your PATH. Add this line to ~/.zshrc:", "  " + shellLine}},
		{
			name:  "the directory, for a shell setup can't check",
			found: situated(setup.UnknownShell, "is fish, which setup can't ask"),
			want:  []string{"! claude link created, but setup can't check it for your shell. Put this directory first on your PATH:", "  " + binDir},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, dashboard.SetupLink)
			h.steps.found = tt.found
			h.start()
			h.pressAndDeliver("n")
			h.press("q")
			lines := h.model.gone().leaving().Lines()
			if got := read(lines[len(lines)-1:]); !slices.Equal(got, tt.want) {
				t.Errorf("the claude link's line reads\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// The service step's ends once setup's leaving has cut it short, as
// setup.Service and service.Install end then.
var (
	// swappedThenCut is the swap done, Install then failing with ctx's
	// cause, as it waited for the router.
	swappedThenCut = func(ctx context.Context) (setup.Serviced, error) {
		return setup.Serviced{Said: []setup.Said{serviceInstalled}, Swap: service.Swapped}, context.Cause(ctx)
	}
	// swapFailed is the swap begun, and failing at its end.
	swapFailed = func(context.Context) (setup.Serviced, error) {
		return setup.Serviced{Said: []setup.Said{notSetUp(errUnswapped)}, Swap: service.SwapFailed}, errUnswapped
	}
	// killedBeforeTheSwap is launchctl killed, as its context ended, as the
	// step asked launchd how the service stands, before any swap.
	killedBeforeTheSwap = func(context.Context) (setup.Serviced, error) {
		return setup.Serviced{Said: []setup.Said{notSetUp(errKilled)}}, errKilled
	}
	// unansweredBeforeTheSwap is the router asked, its context ended, so it
	// didn't answer, before any swap.
	unansweredBeforeTheSwap = func(context.Context) (setup.Serviced, error) {
		return setup.Serviced{Said: []setup.Said{routerUnanswered}}, errUnanswering
	}
	// interruptedThroughTheSwap is the swap done, an interrupt held through
	// it, which cut the wait for the router short.
	interruptedThroughTheSwap = func(context.Context) (setup.Serviced, error) {
		return setup.Serviced{Said: []setup.Said{serviceInstalled}, Swap: service.Swapped}, errInterruptedSwap
	}
)

var (
	errUnswapped       = errors.New("launchctl bootstrap gui/501 ~/Library/LaunchAgents/io.github.leeovery.switchboard.plist: Bootstrap failed: 5: Input/output error (exit status 5)")
	errKilled          = errors.New("launchctl print gui/501/io.github.leeovery.switchboard: signal: killed")
	errUnanswering     = errors.New("the router doesn't answer here")
	routerUnanswered   = setup.Said{Mark: setup.Failed, Label: "Router", Detail: "launchd runs one, pid 4242, that doesn't answer here"}
	errInterruptedSwap = fmt.Errorf("the service is installed, but %w before its router answered", accounts.ErrInterrupted)
)

// notSetUp is the row the service step ends with where it failed with err,
// with no row of its own saying why.
func notSetUp(err error) setup.Said {
	return setup.Said{Mark: setup.Failed, Label: "Not set up", Detail: err.Error()}
}

// serviceTillLeft is setup's steps whose Service says it's started, holds
// till its context is done, as setup's leaving does, then ends as ends has
// it.
type serviceTillLeft struct {
	*fakeSteps
	started chan struct{}
	ends    func(ctx context.Context) (setup.Serviced, error)
}

func (s *serviceTillLeft) Service(ctx context.Context) (setup.Serviced, error) {
	close(s.started)
	<-ctx.Done()
	return s.ends(ctx)
}

func TestQWhileTheServiceStepRunsSaysWhatItsSwapDid(t *testing.T) {
	tests := []struct {
		name  string
		ends  func(ctx context.Context) (setup.Serviced, error)
		want  Left
		lines []string
	}{
		{
			name: "the swap done, the wait for the router cut short: installed, not failed",
			ends: swappedThenCut, want: Left{Ending: Paused}, lines: []string{accountsLine, primingPast, "✓ Service installed"},
		},
		{
			name: "the swap failing at its end: failed, saying why",
			ends: swapFailed, want: Left{Ending: Failed, Err: errUnswapped},
			lines: []string{accountsLine, primingPast, "✗ Service not set up: " + errUnswapped.Error()},
		},
		{name: "launchctl killed before the swap: no line, as nothing changed", ends: killedBeforeTheSwap, want: Left{Ending: Paused}, lines: []string{accountsLine, primingPast}},
		{name: "the router's ask cut short before the swap: no line", ends: unansweredBeforeTheSwap, want: Left{Ending: Paused}, lines: []string{accountsLine, primingPast}},
		{
			name: "an interrupt held through the swap: installed, not failed",
			ends: interruptedThroughTheSwap, want: Left{Ending: Paused}, lines: []string{accountsLine, primingPast, "✓ Service installed"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps := &serviceTillLeft{fakeSteps: newFakeSteps(), started: make(chan struct{}), ends: tt.ends}
			read1, saw := sawEach(kind[reviewMsg])
			ready := make(chan struct{})
			go func() {
				<-read1
				<-steps.started
				close(ready)
			}()
			left, _, err := runUntil(t, Config{Steps: steps, Step: dashboard.SetupService}, terminal{keys: "q", ready: ready, saw: saw})
			if err != nil || !sameLeft(how(left), tt.want) {
				t.Errorf("run() = %+v, %v; want %+v, no error", how(left), err, tt.want)
			}
			if got := read(left.Summary.Lines()); !slices.Equal(got, tt.lines) {
				t.Errorf("the summary reads\n%q\nwant\n%q", got, tt.lines)
			}
		})
	}
}

func TestAnInterruptHeldThroughTheSwapIsNoFailureOfTheServiceStep(t *testing.T) {
	h := newHarness(t, dashboard.SetupService)
	h.steps.serviced, h.steps.serviceErr = setup.Serviced{Said: []setup.Said{serviceInstalled}, Swap: service.Swapped}, errInterruptedSwap
	h.start()
	if w := h.model.walk; !w.done || w.failed != nil {
		t.Errorf("the service step stands %+v, want it done, not failed: the service installed, the interrupt cutting short only the wait for its router", w)
	}
}

// settingTillGone is setup's steps whose SetDay says it's started, and ends
// only once it's let go.
type settingTillGone struct {
	*fakeSteps
	started, release chan struct{}
}

func (s *settingTillGone) SetDay(day string) (config.Day, error) {
	close(s.started)
	<-s.release
	return s.fakeSteps.SetDay(day)
}

func TestQWhileTheDayIsSetSaysTheDaySet(t *testing.T) {
	steps := &settingTillGone{fakeSteps: newFakeSteps(), started: make(chan struct{}), release: make(chan struct{})}
	primed, saw := sawEach(kind[primedMsg], kind[reviewMsg])
	gone := func(*jobs) <-chan time.Time {
		close(steps.release)
		return nil
	}
	term := terminal{keys: "09:00-18:00\r", ready: primed, then: "q", thenReady: steps.started, saw: saw, moment: gone}
	left, _, err := runUntil(t, Config{Steps: steps, Step: dashboard.SetupPriming}, term)
	if err != nil || !sameLeft(how(left), Left{Ending: Paused}) {
		t.Errorf("run() = %+v, %v; want setup paused, no error", how(left), err)
	}
	if got, want := read(left.Summary.Lines()), []string{accountsLine, "✓ Priming on, 09:00–18:00"}; !slices.Equal(got, want) {
		t.Errorf("the summary reads\n%q\nwant\n%q, the day as it was set once the screen had gone", got, want)
	}
}

// linkTillLeft is setup's steps whose Link says it's started, and ends only
// once its context is done, as the asking stopped then ends: with what it
// found, and no error, as setup's own Link does.
type linkTillLeft struct {
	*fakeSteps
	started chan struct{}
}

func (s *linkTillLeft) Link(ctx context.Context) (setup.LinkFound, error) {
	close(s.started)
	<-ctx.Done()
	return s.fakeSteps.Link(ctx)
}

func TestQAsTheShellIsAskedSaysNothingOfWhatTheAskingStoppedFound(t *testing.T) {
	steps := &linkTillLeft{fakeSteps: newFakeSteps(), started: make(chan struct{})}
	steps.found = situated(setup.UnknownShell, "setup couldn't ask your shell")
	read1, saw := sawEach(kind[reviewMsg])
	ready := make(chan struct{})
	go func() {
		<-read1
		<-steps.started
		close(ready)
	}()
	left, _, err := runUntil(t, Config{Steps: steps, Step: dashboard.SetupLink}, terminal{keys: "q", ready: ready, saw: saw})
	if err != nil || !sameLeft(how(left), Left{Ending: Paused}) {
		t.Errorf("run() = %+v, %v; want setup paused, no error", how(left), err)
	}
	if got, want := read(left.Summary.Lines()), []string{accountsLine, primingPast, "✓ Service "}; !slices.Equal(got, want) {
		t.Errorf("the summary reads\n%q\nwant\n%q, no line of the claude link's", got, want)
	}
}

func TestQAsTheLineIsAddedSaysItsAdded(t *testing.T) {
	steps := &addTillReleased{fakeSteps: newFakeSteps(), started: make(chan struct{}), release: make(chan struct{}), ended: make(chan struct{})}
	steps.found, steps.lineAdded = linkLeft, lineAdded
	linked, saw := sawEach(kind[linkedMsg], kind[reviewMsg])
	gone := func(*jobs) <-chan time.Time {
		close(steps.release)
		return nil
	}
	term := terminal{keys: "\r", ready: linked, then: "q", thenReady: steps.started, saw: saw, moment: gone}
	left, _, err := runUntil(t, Config{Steps: steps, Step: dashboard.SetupLink}, term)
	if err != nil || !sameLeft(how(left), Left{Ending: Paused}) {
		t.Errorf("run() = %+v, %v; want setup paused, no error", how(left), err)
	}
	want := "! claude link created, and switchboard's line is added to ~/.zshrc, but setup hasn't asked your shell again whether a new terminal's claude goes through it"
	if got := read(left.Summary.Lines()); len(got) != 4 || got[3] != want {
		t.Errorf("the summary reads\n%q\nwant its claude link line\n%q", got, want)
	}
}

func TestQWhileTokensAreCheckedSaysThemUnchecked(t *testing.T) {
	steps := &reviewTillCancelled{fakeSteps: frame3Steps(), started: make(chan string, 3), ended: make(chan string, 3)}
	all := make(chan struct{})
	go func() {
		for range 3 {
			<-steps.started
		}
		close(all)
	}()
	left, _, err := runUntil(t, Config{Steps: steps}, terminal{keys: "q", ready: all})
	if err != nil || !sameLeft(how(left), Left{Ending: Paused}) {
		t.Errorf("run() = %+v, %v; want setup paused, no error", how(left), err)
	}
	if got, want := read(left.Summary.Lines()), []string{"✓ Accounts 3, all unchecked · primary: work, with 5% reserved for the Claude apps"}; !slices.Equal(got, want) {
		t.Errorf("the summary reads\n%q\nwant\n%q, never checking…, nor all working", got, want)
	}
	for _, a := range left.Summary.Review.Accounts {
		if a.Token.State != setup.TokenUnchecked {
			t.Errorf("%s's token is %v as setup is left, want it unchecked", a.ID, a.Token.State)
		}
	}
}

func TestATokenTheScreenHeardCheckedIsLeftAsChecked(t *testing.T) {
	h := accountsHarness(t, frame3Steps())
	checks := h.held()
	h.deliver(checks[:1]...)
	left := h.model.gone().leaving()
	if got, want := read(left.Lines()), []string{"✓ Accounts 3: 1 working, 2 unchecked · primary: work, with 5% reserved for the Claude apps"}; !slices.Equal(got, want) {
		t.Errorf("the summary reads\n%q\nwant\n%q", got, want)
	}
	review := left.Review
	want := []setup.TokenState{setup.TokenWorking, setup.TokenUnchecked, setup.TokenUnchecked}
	var got []setup.TokenState
	for _, a := range review.Accounts {
		got = append(got, a.Token.State)
	}
	if !slices.Equal(got, want) {
		t.Errorf("the tokens as setup is left are %v, want %v: the one checked as it was checked, the rest unchecked", got, want)
	}
}

func TestAStepsJobStoppedBySetupsLeavingTellsNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	stopped := &serviceTillLeft{fakeSteps: newFakeSteps(), started: make(chan struct{}), ends: killedBeforeTheSwap}
	for name, j := range map[string]job{"the service step's": serviceStep(stopped), "the claude link step's": linkClaude(newFakeSteps())} {
		if msg := j(ctx); msg != nil {
			t.Errorf("%s job, stopped by its context, gave %T, want nothing", name, msg)
		}
	}
	swapped := &serviceTillLeft{fakeSteps: newFakeSteps(), started: make(chan struct{}), ends: swappedThenCut}
	if msg, ok := serviceStep(swapped)(ctx).(servicedMsg); !ok || msg.err != nil {
		t.Errorf("the service step's job, its swap done once its context was done, gave %+v, want what it did, not failed", msg)
	}
}
