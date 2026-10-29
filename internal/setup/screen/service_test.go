package screen

import (
	"context"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/setup"
)

// The service page's rows at 160×37, under its heading and what it's for:
// the first fact, or what it says while it's under way, and the facts after
// it.
const (
	serviceFirstRow  = 8
	serviceSecondRow = 10
)

// underWayRow is the service page's row while its step is under way, its
// spinner's dot at the place given, of three.
func underWayRow(at int) string {
	dots := []rune("∙∙∙")
	dots[at] = '●'
	return "  " + string(dots) + "  Seeing to the service and its router"
}

func TestReachingTheServiceStepRunsItAndShowsWhatItSaid(t *testing.T) {
	h := newHarness(t, dashboard.SetupPriming)
	h.start()
	if h.steps.services != 0 {
		t.Fatalf("Service ran %d times before its step, want none", h.steps.services)
	}
	h.pressAndDeliver("enter")

	if h.steps.services != 1 {
		t.Errorf("Service ran %d times, want once, as the walk reached its step", h.steps.services)
	}
	rows := h.rows()
	want := map[int]string{
		4:                "THE SERVICE  " + strings.Repeat("─", 147),
		6:                "  launchd keeps the router running: it starts at login, and restarts it if it stops.",
		serviceFirstRow:  factRowOf("✓", serviceInstalled),
		serviceSecondRow: factRowOf("✓", routerHealthy),
		keyRow:           strings.Repeat(" ", 128) + "enter next: claude link   q quit",
	}
	for y, w := range want {
		if rows[y] != w {
			t.Errorf("row %d reads\n%q\nwant\n%q", y, rows[y], w)
		}
	}
	if got := h.model.page().Service; got == nil || got.Router == nil || *got.Router != health {
		t.Errorf("the page holds %+v, want the router's health the step saw", got)
	}
	h.pressAndDeliver("enter")
	if got := h.model.page().Step; got != dashboard.SetupLink {
		t.Errorf("enter once the service step is done shows %s, want the claude link", got.Title())
	}
}

func TestTheServiceStepRunsAsTheScreenOpensOnIt(t *testing.T) {
	h := newHarness(t, dashboard.SetupService)
	h.start()
	if h.steps.services != 1 || h.model.service == nil {
		t.Errorf("opening on the service step ran Service %d times, saying %v; want once, said", h.steps.services, h.model.service)
	}
}

func TestALineWalksSaidIsntDrawn(t *testing.T) {
	h := newHarness(t, dashboard.SetupService)
	h.steps.serviced.Said = []setup.Said{{Mark: setup.Done, Sentence: "The line walk's alone."}, routerHealthy}
	h.start()
	if got, want := h.rows()[serviceFirstRow], factRowOf("✓", routerHealthy); got != want {
		t.Errorf("the first fact's row reads\n%q\nwant\n%q, a Said with no label left off", got, want)
	}
}

// heldOutcomes starts h's model, delivering what its first commands send
// back but for the service step's outcome, which it returns, undelivered.
func heldOutcomes(h *harness) []tea.Msg {
	h.t.Helper()
	var serviced []tea.Msg
	for _, msg := range h.run(h.model.Init()) {
		if _, ok := msg.(servicedMsg); ok {
			serviced = append(serviced, msg)
			continue
		}
		h.deliver(msg)
	}
	return serviced
}

func TestWhileTheServiceStepRunsItsPageSaysSoAndEnterWaits(t *testing.T) {
	h := newHarness(t, dashboard.SetupService)
	serviced := heldOutcomes(h)

	rows := h.rows()
	if got, want := rows[serviceFirstRow], underWayRow(0); got != want {
		t.Errorf("while Service runs, row %d reads\n%q\nwant\n%q", serviceFirstRow, got, want)
	}
	if got := rows[serviceSecondRow]; got != "" {
		t.Errorf("while Service runs, row %d reads %q, want it blank, no fact said yet", serviceSecondRow, got)
	}
	if got := rows[keyRow]; strings.TrimLeft(got, " ") != "q quit" {
		t.Errorf("while Service runs, the key line reads %q, want q quit alone at its right", got)
	}
	h.pressAndDeliver("enter")
	if got := h.model.page().Step; got != dashboard.SetupService {
		t.Fatalf("enter while Service runs shows %s, want the service step", got.Title())
	}

	h.deliver(serviced...)
	if got, want := h.rows()[serviceFirstRow], factRowOf("✓", serviceInstalled); got != want {
		t.Errorf("once Service has answered, row %d reads\n%q\nwant\n%q", serviceFirstRow, got, want)
	}
	if got := h.rows()[keyRow]; strings.TrimLeft(got, " ") != "enter next: claude link   q quit" {
		t.Errorf("once Service has answered, the key line reads %q, want enter on to the claude link, then q quit", got)
	}
}

// spins are the spinner's moves h's model has armed.
func spins(h *harness) []timer {
	var armed []timer
	for _, tm := range h.timers {
		if _, ok := tm.msg.(spinMsg); ok {
			armed = append(armed, tm)
		}
	}
	return armed
}

func TestTheSpinnerMovesWhileTheServiceStepRuns(t *testing.T) {
	h := newHarness(t, dashboard.SetupService)
	h.clock.now = start.Add(100 * time.Millisecond)
	serviced := heldOutcomes(h)
	if got, want := spins(h), []timer{{delay: 150*time.Millisecond + dashboard.TickSlack, msg: spinMsg{}}}; !slices.Equal(got, want) {
		t.Fatalf("starting arms the spinner's moves %+v, want %+v: just past its dot's next place", got, want)
	}

	for i, at := range []int{1, 2, 1, 0} {
		h.clock.now = start.Add(time.Duration(i+1) * 250 * time.Millisecond)
		h.deliver(spinMsg{})
		if got, want := h.rows()[serviceFirstRow], underWayRow(at); got != want {
			t.Errorf("after %d moves, the row reads\n%q\nwant\n%q", i+1, got, want)
		}
	}
	if got := len(spins(h)); got != 5 {
		t.Errorf("the spinner's moves armed %d, want one more for each move", got)
	}

	h.deliver(serviced...)
	h.deliver(spinMsg{})
	if got := len(spins(h)); got != 5 {
		t.Errorf("once the step is done, a move arms %d more, want none: nothing spins", got-5)
	}
}

func TestReachingTheServiceStepStartsItsSpinner(t *testing.T) {
	h := newHarness(t, dashboard.SetupPriming)
	h.start()
	// The service step's outcome is held back, so it's still under way.
	h.press("enter")

	if got, want := h.rows()[serviceFirstRow], underWayRow(0); got != want {
		t.Errorf("reaching the service step, row %d reads\n%q\nwant\n%q", serviceFirstRow, got, want)
	}
	reached := timer{delay: 250*time.Millisecond + dashboard.TickSlack, msg: spinMsg{spins: 1}}
	if got := spins(h); !slices.Equal(got, []timer{reached}) {
		t.Errorf("reaching the service step arms the spinner's moves %+v, want %+v", got, reached)
	}
}

func TestASpinnerMoveArmedForAStepBeforeIsDropped(t *testing.T) {
	h := newHarness(t, dashboard.SetupPriming)
	h.start()
	h.press("enter")

	h.deliver(spinMsg{spins: 0})
	if got := len(spins(h)); got != 1 {
		t.Errorf("a move armed before the service step was reached arms %d moves, want none more than the step's own", got-1)
	}
	h.deliver(spinMsg{spins: 1})
	if got := len(spins(h)); got != 2 {
		t.Errorf("the step's own move arms %d more, want one", got-1)
	}
}

func TestNoSpinnerMovesOnAStepWithoutOne(t *testing.T) {
	h := newHarness(t, dashboard.SetupSkill)
	h.start()
	if got := spins(h); len(got) != 0 {
		t.Errorf("opening on the Skill step arms the spinner's moves %+v, want none", got)
	}
}

func TestAServiceStepThatFailedStopsSetupThere(t *testing.T) {
	h := newHarness(t, dashboard.SetupService)
	h.steps.serviced, h.steps.serviceErr = setup.Serviced{Said: []setup.Said{serviceInstalled, routerUnstarted}}, errNoService
	h.start()

	rows := h.rows()
	if got, want := rows[serviceSecondRow], factRowOf("✗", routerUnstarted); got != want {
		t.Errorf("row %d reads\n%q\nwant\n%q: the failure's row", serviceSecondRow, got, want)
	}
	if got := rows[keyRow]; strings.TrimLeft(got, " ") != "q quit" {
		t.Errorf("the key line reads %q, want q quit alone at its right", got)
	}
	if msgs := h.press("enter"); quits(msgs) || len(msgs) > 0 {
		t.Errorf("enter at a step that failed sends %v, want nothing", msgs)
	}
	if !quits(h.press("q")) {
		t.Fatal("q didn't quit")
	}
	if got, want := h.model.walk.left(), (Left{Ending: Failed, Err: errNoService}); got != want {
		t.Errorf("setup was left %+v, want %+v", got, want)
	}
}

func TestTheServiceStepsDetailsAreMadeSafeToShowAsTheyArrive(t *testing.T) {
	h := newHarness(t, dashboard.SetupService)
	told := []setup.Said{{Mark: setup.Done, Label: "Installed", Detail: "\x1b[2J~/Library\t LaunchAgents\n"}}
	h.steps.serviced.Said = told
	h.start()
	if got, want := h.model.service.Said[0].Detail, "[2J~/Library LaunchAgents"; got != want {
		t.Errorf("the detail held reads %q, want %q: its control characters spaces, each run of spaces one", got, want)
	}
	if told[0].Detail != "\x1b[2J~/Library\t LaunchAgents\n" {
		t.Errorf("the step's own Said reads %q, want it left as it told it", told[0].Detail)
	}
}

func TestTheStatusLineIsAskedAgainOnceTheServiceStepIsDone(t *testing.T) {
	h := newHarness(t, dashboard.SetupService)
	h.steps.standing = setup.RouterStanding{State: setup.RouterDown}
	serviced := heldOutcomes(h)
	if got, want := h.rows()[statusRow], "○ router not running yet  ·  3 accounts, all working"; got != want {
		t.Fatalf("before the service step is done, the status line reads %q, want %q", got, want)
	}

	h.steps.standing = setup.RouterStanding{State: setup.RouterHealthy}
	h.deliver(serviced...)
	if h.steps.standings != 2 || h.steps.reviews != 2 {
		t.Errorf("Standing was asked %d times and Review %d, want each again once the step is done", h.steps.standings, h.steps.reviews)
	}
	if got, want := h.rows()[statusRow], "● router up, healthy  ·  3 accounts, all working"; got != want {
		t.Errorf("once the service step is done, the status line reads %q, want %q, the router it started up", got, want)
	}
}

func TestTheStatusLineIsAskedAgainOnceTheServiceStepFails(t *testing.T) {
	h := newHarness(t, dashboard.SetupService)
	h.steps.serviceErr = errNoService
	h.start()
	if h.steps.standings != 2 || h.steps.reviews != 2 {
		t.Errorf("Standing was asked %d times and Review %d, want each again, as a step that failed may have started the router", h.steps.standings, h.steps.reviews)
	}
}

// serviceTillCancelled is setup's steps whose Service holds on until its
// context is cancelled, saying it's started, then till release is closed,
// where it's set, else for overlap, and says when it ended.
type serviceTillCancelled struct {
	*fakeSteps
	started, release, ended chan struct{}
}

func (s *serviceTillCancelled) Service(ctx context.Context) (setup.Serviced, error) {
	close(s.started)
	<-ctx.Done()
	if s.release != nil {
		<-s.release
	} else {
		time.Sleep(overlap)
	}
	close(s.ended)
	return setup.Serviced{}, ctx.Err()
}

// reviewHeld is setup's steps whose Review holds on till release is closed,
// saying it's started, as a read taking no context may.
type reviewHeld struct {
	*fakeSteps
	once             sync.Once
	started, release chan struct{}
}

func (s *reviewHeld) Review() (setup.Review, error) {
	s.once.Do(func() { close(s.started) })
	<-s.release
	return s.fakeSteps.Review()
}

// fired is a moment come already.
func fired(*jobs) <-chan time.Time {
	c := make(chan time.Time, 1)
	c <- start
	return c
}

// waitedFor is a moment that comes once every job of js has wholly ended,
// its write's slot given back, as wait has it.
func waitedFor(js *jobs) <-chan time.Time {
	c := make(chan time.Time, 1)
	go func() {
		js.wait()
		c <- start
	}()
	return c
}

func TestQWhileTheServiceStepRunsLeavesOnceItHasStopped(t *testing.T) {
	t.Run("ending before the moment to say so: nothing said", func(t *testing.T) {
		steps := &serviceTillCancelled{fakeSteps: newFakeSteps(), started: make(chan struct{}), ended: make(chan struct{})}
		left, out, err := runUntil(t, Config{Steps: steps, Step: dashboard.SetupService}, terminal{keys: "q", ready: steps.started, moment: waitedFor})
		if err != nil || left != (Left{Ending: Paused}) {
			t.Errorf("run() = %+v, %v; want setup paused, no error", left, err)
		}
		select {
		case <-steps.ended:
		default:
			t.Error("run() returned with Service still running, want it cancelled and waited for")
		}
		if strings.Contains(out, finishingSaid) {
			t.Errorf("the screen's output ends %q, want nothing said of what setup's finishing", out[max(len(out)-160, 0):])
		}
	})
	t.Run("running on past the moment: says so, then waits for it", func(t *testing.T) {
		steps := &serviceTillCancelled{fakeSteps: newFakeSteps(), started: make(chan struct{}), release: make(chan struct{}), ended: make(chan struct{})}
		var once sync.Once
		said := func(p []byte) {
			if strings.Contains(string(p), finishingSaid) {
				once.Do(func() { close(steps.release) })
			}
		}
		left, out, err := runUntil(t, Config{Steps: steps, Step: dashboard.SetupService}, terminal{keys: "q", ready: steps.started, moment: fired, wrote: said})
		if err != nil || left != (Left{Ending: Paused}) {
			t.Errorf("run() = %+v, %v; want setup paused, no error", left, err)
		}
		select {
		case <-steps.ended:
		default:
			t.Error("run() returned with Service still running, want it waited for")
		}
		if !strings.HasSuffix(out, finishingSaid+"\n") {
			t.Errorf("the screen's output ends %q, want it to end saying what setup's finishing", out[max(len(out)-160, 0):])
		}
	})
}

func TestSetupIsFinishingTheServiceStepWhileItsJobRuns(t *testing.T) {
	tests := []struct {
		name    string
		step    dashboard.SetupStep
		done    bool
		writing bool
		want    bool
	}{
		{name: "at Service, under way, its write running: finishing it", step: dashboard.SetupService, writing: true, want: true},
		{name: "at Service, under way, no write running: nothing to finish", step: dashboard.SetupService},
		{name: "at Service, done: nothing to finish", step: dashboard.SetupService, done: true, writing: true},
		{name: "at another step: not the Service step", step: dashboard.SetupSkill, writing: true},
		{name: "at Done: nothing to finish", step: dashboard.SetupDone, done: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			js := newJobs(t.Context())
			if tt.writing {
				js.writing <- struct{}{}
			}
			w := walk{step: tt.step, done: tt.done}
			if got := finishing(w, js); got != tt.want {
				t.Errorf("finishing() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNoOtherJobRunningOnSaysSetupsFinishingAnything(t *testing.T) {
	steps := &reviewHeld{fakeSteps: newFakeSteps(), started: make(chan struct{}), release: make(chan struct{})}
	time.AfterFunc(overlap, func() { close(steps.release) })
	_, out, err := runUntil(t, Config{Steps: steps, Step: dashboard.SetupSkill}, terminal{keys: "q", ready: steps.started, moment: fired})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if strings.Contains(out, "Setup is finishing") {
		t.Errorf("the screen's output ends %q, want nothing said: no step's own work runs on", out[max(len(out)-160, 0):])
	}
}

func TestWaitSayingSaysSoOnlyPastItsMoment(t *testing.T) {
	t.Run("ending before its moment: nothing said", func(t *testing.T) {
		said := false
		waitSaying(func() {}, make(chan time.Time), func() { said = true })
		if said {
			t.Error("said it was waiting, want nothing said")
		}
	})
	t.Run("waiting past its moment: said once, then waited on", func(t *testing.T) {
		release, moment := make(chan struct{}), make(chan time.Time, 1)
		var said atomic.Int32
		returned := make(chan struct{})
		go func() {
			waitSaying(func() { <-release }, moment, func() { said.Add(1) })
			close(returned)
		}()
		moment <- start
		time.Sleep(overlap)
		select {
		case <-returned:
			t.Fatal("returned while the wait ran on, want it waited for")
		default:
		}
		close(release)
		<-returned
		if got := said.Load(); got != 1 {
			t.Errorf("said it was waiting %d times, want once", got)
		}
	})
}
