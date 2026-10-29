package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/setup"
)

// What the service step says, as frame 4 has it.
var (
	serviceInstalled = setup.Said{Mark: setup.Done, Label: "Installed", Detail: "~/Library/LaunchAgents/io.github.leeovery.switchboard.plist"}
	serviceUpdated   = setup.Said{Mark: setup.Warning, Label: "Updated", Detail: "reinstalled: its plist had no ProcessType · kept its --log-level"}
	routerHealthy    = setup.Said{Mark: setup.Done, Label: "Router", Detail: "healthy · 127.0.0.1:4747 · v0.2.10"}
)

// serviceScreen is the screen at the Service step, the two before it passed
// through, the step done as frame 4 has it, with the changes given.
func serviceScreen(changes ...func(*Setup)) Setup {
	serviced := setup.Serviced{
		Said:   []setup.Said{serviceInstalled, serviceUpdated, routerHealthy},
		Router: &router.Health{OK: true, Listen: "127.0.0.1:4747", Version: "0.2.10"},
	}
	return setupScreen(append([]func(*Setup){func(s *Setup) {
		s.Step, s.Passed, s.Skill, s.Service = SetupService, []SetupStep{SetupAccounts, SetupPriming}, nil, &serviced
	}}, changes...)...)
}

// underWay is the service step under way, at the clock's time given.
func underWay(at time.Time) func(*Setup) {
	return func(s *Setup) { s.Now, s.Service, s.UnderWay = at, nil, true }
}

func TestTheServicePageSaysWhatTheServiceIsForAndWhatTheStepDid(t *testing.T) {
	rows := textOf(serviceScreen().Draw(160, 37, Look{}))
	want := map[int]string{
		4:  "THE SERVICE  " + strings.Repeat("─", 147),
		6:  "  launchd keeps the router running: it starts at login, and restarts it if it stops.",
		8:  textsAt(160, map[int]string{2: "✓", 5: "Installed", 26: serviceInstalled.Detail}),
		10: textsAt(160, map[int]string{2: "!", 5: "Updated", 26: serviceUpdated.Detail}),
		12: textsAt(160, map[int]string{2: "✓", 5: "Router", 26: routerHealthy.Detail}),
		36: blanks(128) + "enter next: claude link   q quit",
	}
	for y, w := range want {
		if rows[y] != w {
			t.Errorf("row %d reads\n%q\nwant\n%q", y, rows[y], w)
		}
	}
	for y := range rows {
		if _, said := want[y]; !said && y > 2 && rows[y] != "" {
			t.Errorf("row %d reads %q, want it blank", y, rows[y])
		}
	}
}

func TestTheServicePagesPurposeIsMuted(t *testing.T) {
	c := setupDrawn(serviceScreen(), 160, 37)
	if says := inksFrom(c, 2, 6, len([]rune(servicePurpose)), mutedInk); says != "" {
		t.Errorf("what the service is for, text.muted, as frame 4 has it: %s", says)
	}
}

func TestASaidWithNoLabelIsTheLineWalksAlone(t *testing.T) {
	walkOnly := setup.Said{Mark: setup.Done, Sentence: "The service is installed."}
	rows := textOf(serviceScreen(func(s *Setup) {
		s.Service.Said = []setup.Said{walkOnly, routerHealthy}
	}).Draw(160, 37, Look{}))
	if got, want := rows[8], textsAt(160, map[int]string{2: "✓", 5: "Router", 26: routerHealthy.Detail}); got != want {
		t.Errorf("row 8 reads\n%q\nwant\n%q: a Said with no label left off", got, want)
	}
	if rows[10] != "" {
		t.Errorf("row 10 reads %q, want it blank", rows[10])
	}
}

func TestWhileTheServiceStepIsUnderWayItsPageSaysSo(t *testing.T) {
	c := setupDrawn(serviceScreen(underWay(now)), 160, 37)
	if got, want := rowOf(c, 8), "  ●∙∙  Seeing to the service and its router"; got != want {
		t.Errorf("row 8 reads\n%q\nwant\n%q", got, want)
	}
	if got, want := rowOf(c, 36), blanks(154)+"q quit"; got != want {
		t.Errorf("the key line reads\n%q\nwant\n%q: enter waits for the step to be done", got, want)
	}
	for _, run := range []struct {
		what string
		x, n int
		k    ink
	}{
		{what: "the spinner's dot, bold accent.mode", x: 2, n: 1, k: nameInk},
		{what: "the spinner's other places, text.faint", x: 3, n: 2, k: faintInk},
		{what: "what it says, bold text.primary", x: 7, n: len("Seeing to the service and its router"), k: titleInk},
	} {
		if says := inksFrom(c, run.x, 8, run.n, run.k); says != "" {
			t.Errorf("%s: %s", run.what, says)
		}
	}
}

func TestTheSpinnersDotGoesAlongAndBack(t *testing.T) {
	tests := []struct {
		after time.Duration
		want  string
	}{
		{after: 0, want: "●∙∙"},
		{after: 249 * time.Millisecond, want: "●∙∙"},
		{after: 250 * time.Millisecond, want: "∙●∙"},
		{after: 500 * time.Millisecond, want: "∙∙●"},
		{after: 750 * time.Millisecond, want: "∙●∙"},
		{after: time.Second, want: "●∙∙"},
	}
	for _, tt := range tests {
		rows := textOf(serviceScreen(underWay(now.Add(tt.after))).Draw(160, 37, Look{}))
		if got := []rune(rows[8])[2:5]; string(got) != tt.want {
			t.Errorf("%v on, the spinner reads %q, want %q", tt.after, string(got), tt.want)
		}
	}
}

func TestTheSpinnerMovesOnJustPastItsDotsNextPlace(t *testing.T) {
	tests := []struct {
		name   string
		screen Setup
		want   time.Duration
		spins  bool
	}{
		{name: "under way, on a place", screen: serviceScreen(underWay(now)), want: 250*time.Millisecond + TickSlack, spins: true},
		{name: "under way, between places", screen: serviceScreen(underWay(now.Add(100 * time.Millisecond))), want: 150*time.Millisecond + TickSlack, spins: true},
		{name: "done: none", screen: serviceScreen()},
		{name: "failed: none", screen: serviceScreen(func(s *Setup) { s.Failed = true })},
		{name: "a step under way whose page has no spinner: none", screen: setupScreen(func(s *Setup) { s.Skill, s.UnderWay = nil, true })},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, spins := tt.screen.SpinDelay()
			if got != tt.want || spins != tt.spins {
				t.Errorf("SpinDelay() = %v, %v; want %v, %v", got, spins, tt.want, tt.spins)
			}
		})
	}
}
