package capture

import (
	"fmt"
	"maps"
	"math"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// looksEvery is how often the watch looks at the router's document, as
// its lookEvery has it: what the document carries shows by the look after.
const looksEvery = 5 * time.Second

// A demo's tape plays its scenario at its size, in its theme, pressing its
// keys at its times, as demo/README.md's table and each tape's sleeps have
// them; and what it shows, what each scenario's cues are timed for, shows as
// each key and look comes. The keys and times are the tapes': retimed, the
// two change together.
func TestEachTapePlaysItsScenarioAsItsCuesAreTimedFor(t *testing.T) {
	type press struct {
		at   float64
		keys []tea.KeyPressMsg
	}
	type shows struct {
		at   float64
		says []string
		// not are what it no longer shows by then.
		not []string
		// tab is the tab the title row shows by then, where it's checked.
		tab string
		// theme is the theme it's drawn in by then, where it's checked.
		theme string
	}
	shiftTab := []tea.KeyPressMsg{{Code: tea.KeyTab, Mod: tea.ModShift}}
	up, down := []tea.KeyPressMsg{upKey}, []tea.KeyPressMsg{downKey}
	tests := []struct {
		tape, scenario, theme string
		size                  watch.Size
		keys                  []press
		shows                 []shows
	}{
		{
			tape: "tour", scenario: "routing", theme: "tokyo-night", size: wide(37),
			keys: []press{
				{4.5, shiftTab}, {7.4, up}, {8.1, keysOf("s")},
				{11.6, shiftTab}, {14.6, shiftTab}, {17.6, shiftTab},
				{21.6, shiftTab}, {24.6, shiftTab}, {27.6, shiftTab},
			},
			shows: []shows{
				{at: 0, says: []string{"auto  ·  6 sessions  ·  new sessions → ▲ side", "ROUTING   auto"}},
				{at: 7.3, says: []string{"Requests", "● following"}},
				{at: 8.5, says: []string{"SESSION   ", "esc back"}},
				{at: 11.5, says: []string{"SESSION   ", "esc back"}},
				{at: 11.7, says: []string{"WEEK"}, not: []string{"SESSION   ", "Requests", "RUNWAY"}, tab: "Graphs"},
				{at: 14.5, says: []string{"WEEK"}, not: []string{"RUNWAY"}, tab: "Graphs"},
				{at: 17.5, says: []string{"RUNWAY", "< week >"}},
				{at: 21.5, says: []string{"SESSIONS    Switchboard"}},
				{at: 24.5, says: []string{"HISTORY    Year"}},
				{at: 27.5, says: []string{"ACCOUNTS    Compare"}},
				{at: 30, says: []string{"7 sessions", "ROUTING   auto", "▲ b3e9 started on side"}},
			},
		},
		{
			tape: "routing", scenario: "routing", theme: "tokyo-night", size: wide(37),
			keys: []press{{9, keysOf("p")}, {10.5, keysOf("3")}, {12, keysOf("m")}, {14.5, []tea.KeyPressMsg{enterKey}}},
			shows: []shows{
				{at: 0, says: []string{"auto  ·  6 sessions  ·  new sessions → ▲ side", "ROUTING   auto"}},
				{at: 5.5, says: []string{"7 sessions", "▲ b3e9 started on side"}},
				{at: 13, says: []string{"The 2 sessions running elsewhere move there too.", "Rebuilding their caches costs ≈ $"}},
				{at: 15.5, says: []string{
					"pinned to side  ·  7 sessions", "ROUTING   pinned to side since 14:42   p back to auto",
					"● pin set by hand: new and running sessions go to side",
				}},
				{at: 20.5, says: []string{"▸ 41e0 moved personal → side: pinned to side"}, not: []string{"9c2e moved"}},
				{at: 25.5, says: []string{"pinned to side  ·  7 sessions", "▸ 9c2e moved personal → side: pinned to side"}},
			},
		},
		{
			tape: "dashboard", scenario: "dashboard", theme: "tokyo-night", size: wide(37),
			keys: []press{{6, shiftTab}, {6.3, shiftTab}, {11.3, shiftTab}, {11.6, keysOf(">")}},
			shows: []shows{
				{at: 0, says: []string{"5 sessions  ·  new sessions → ▲ work", "~16:05     work reaches its cap", "◔ work came under pressure"}},
				{at: 6.2, says: []string{"Requests"}, tab: "Log"},
				{at: 11.2, says: []string{"WEEK", "5-HOUR", "1 work"}, not: []string{"RUNWAY"}, tab: "Graphs"},
				{at: 15.5, says: []string{"RUNWAY", "< 5-hour >", "1 work ◆ ▲", "5-hour 59%"}, not: []string{"Σ pool"}},
			},
		},
		{
			tape: "repatch", scenario: "repatch", theme: "tokyo-night", size: wide(37),
			keys: []press{{0, repeated(tabKey, 3)}},
			shows: []shows{
				{at: 2, says: []string{"SESSIONS    Switchboard", "2 personal ◆    ● under pressure"}},
				{at: 15.5, says: []string{"■ limit reached · back 16:30", "100%  limit · back 16:30"}, not: []string{"╌"}},
				{at: 20.5, says: []string{"╌╌○ 2 personal"}, not: []string{"╌╌○  Week"}},
				{at: 25.5, says: []string{"╌╌○ 2 personal", "╌╌○  Week"}},
			},
		},
		{
			tape: "themes", scenario: "routing", theme: "tokyo-night", size: wide(37),
			keys: []press{
				{1.2, keysOf("t")}, {2.2, down},
				{3.6, up}, {3.85, up}, {4.35, up}, {5.65, up}, {6.95, up},
				{8.35, []tea.KeyPressMsg{{Code: tea.KeyEscape}}},
			},
			shows: []shows{
				{at: 1.1, says: []string{"ROUTING   auto"}, theme: "tokyo-night"},
				{at: 3.5, says: []string{"▌ tokyo-night-day"}, theme: "tokyo-night-day"},
				{at: 3.7, says: []string{"▌ tokyo-night "}, theme: "tokyo-night"},
				{at: 4.3, says: []string{"▌ terminal"}},
				{at: 5.6, says: []string{"▌ nord"}, theme: "nord"},
				{at: 6.9, says: []string{"▌ exchange"}, theme: "exchange"},
				{at: 8.3, says: []string{"▌ amber"}, theme: "amber"},
				{at: 9.3, says: []string{"ROUTING   auto"}, not: []string{"tokyo-night-day"}, theme: "tokyo-night"},
			},
		},
		{
			tape: "theme-picker", scenario: "routing", theme: "tokyo-night", size: wide(37),
			keys:  []press{{3, keysOf("t")}, {3.3, down}},
			shows: []shows{{at: 5, says: []string{"│ ▌ tokyo-night-day", "│   tokyo-night              ●", "ROUTING   auto"}, theme: "tokyo-night-day"}},
		},
		{
			tape: "runway-week", scenario: "routing", theme: "tokyo-night", size: wide(37),
			keys:  []press{{2, repeated(tabKey, 4)}},
			shows: []shows{{at: 5, says: []string{"RUNWAY", "< week >", "1 work", "2 personal ◆", "3 side ▲"}}},
		},
		{
			tape: "phone", scenario: "routing", theme: "tokyo-night", size: watch.Size{Width: 52, Height: 36},
			shows: []shows{{at: 6, says: []string{"7 sessions"}}},
		},
		{
			tape: "help", scenario: "routing", theme: "tokyo-night", size: wide(37),
			keys:  []press{{3, keysOf("?")}},
			shows: []shows{{at: 4.7, says: []string{"HELP · OVERVIEW"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.tape, func(t *testing.T) {
			s, err := ScenarioNamed(tt.scenario)
			if err != nil {
				t.Fatal(err)
			}
			th, ok := theme.Builtin(tt.theme)
			if !ok {
				t.Fatalf("the tape's theme, %q, is no built-in", tt.theme)
			}
			s.Size = tt.size
			p := played(t, s.InTheme(th))
			if shown := p.themeShown(); shown != tt.theme {
				t.Errorf("it starts drawn in %q, want the tape's %q", shown, tt.theme)
			}
			keys := slices.Clone(tt.keys)
			for _, want := range tt.shows {
				for len(keys) > 0 && keys[0].at <= want.at {
					p.press(keys[0].at, keys[0].keys...)
					keys = keys[1:]
				}
				p.until(want.at)
				screen := p.screen()
				for _, says := range want.says {
					if !strings.Contains(screen, says) {
						t.Errorf("%vs in, it shows\n%s\nwant %q", want.at, screen, says)
					}
				}
				for _, not := range want.not {
					if strings.Contains(screen, not) {
						t.Errorf("%vs in, it shows\n%s\nwant no %q", want.at, screen, not)
					}
				}
				if want.tab != "" {
					if shown := p.tabShown(); shown != want.tab {
						t.Errorf("%vs in, the title row shows %q, want %q", want.at, shown, want.tab)
					}
				}
				if shown := p.themeShown(); want.theme != "" && shown != want.theme {
					t.Errorf("%vs in, it's drawn in %q, want %q", want.at, shown, want.theme)
				}
			}
		})
	}
}

// Each scenario is made for its tape's terminal: the frames' size.
func TestEachScenarioIsMadeForTheFramesSize(t *testing.T) {
	for _, s := range scenarios() {
		if s.Size != wide(37) {
			t.Errorf("%s is made for %v, want 160 × 37, the frames' size", s.Name, s.Size)
		}
	}
}

// The routing scenario starts as the Overview's frames draw the auto world,
// and the repatch scenario plays personal's limit as Sessions' frames at it
// draw it: refused at 16:05:41, and moved at 16:05:42. They agree on what
// each draws from, not cell for cell, as eases and pulses differ: each
// account's windows, but for what the scenario's own answers have read
// since it started; its limit; the pin and the best; the account each
// session's model is on; and every event the frame's world tells, by its
// kind, account and count, the scenario's telling besides only the moves
// the limit counts.
func TestTheScenariosAgreeWithTheFramesTheyPlay(t *testing.T) {
	worlds := worldsNow()
	tests := []struct {
		name, scenario string
		at             time.Duration
		frame          accountWorld
		frameAt        time.Time
	}{
		{name: "routing as it starts, overview-default-160 and routing-auto-160", scenario: "routing", frame: worlds.auto(), frameAt: worlds.now},
		{name: "repatch at sessions-refused-160", scenario: "repatch", at: repatchLead, frame: worlds.refusedAt(), frameAt: worlds.now.Add(refusedAfter)},
		{name: "repatch at sessions-moved-160", scenario: "repatch", at: repatchLead + time.Second, frame: worlds.movedAt(), frameAt: worlds.now.Add(movedAfter)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := ScenarioNamed(tt.scenario)
			if err != nil {
				t.Fatal(err)
			}
			b := s.based()
			now := b.start.Add(tt.at)
			if !now.Equal(tt.frameAt) {
				t.Fatalf("the scenario is at %s, want the frame's moment, %s", now, tt.frameAt)
			}
			p := b.played(nil, now)
			frame := routerOf(tt.frame, now)
			agree(t, p.world.document(now), frame.doc, p.world.used)
			if got, want := placed(p.world.listing(now, p.told)), placed(frame.sessions); !maps.Equal(got, want) {
				t.Errorf("the sessions are on %v, want %v", got, want)
			}
		})
	}
}

// agree fails the test where the document played disagrees with the
// frame's, as TestTheScenariosAgreeWithTheFramesTheyPlay has it, used being
// how much of each window the scenario's answers have used.
func agree(t *testing.T, played, frame status.Document, used map[windowOf]float64) {
	t.Helper()
	if played.Best != frame.Best || !slices.Equal(played.Pin.Accounts, frame.Pin.Accounts) || played.Pin.Move != frame.Pin.Move {
		t.Errorf("the best is %q, the pin %+v, want %q and %+v", played.Best, played.Pin, frame.Best, frame.Pin)
	}
	if len(played.Accounts) != len(frame.Accounts) {
		t.Fatalf("the accounts are %d, want %d", len(played.Accounts), len(frame.Accounts))
	}
	for i, a := range played.Accounts {
		f := frame.Accounts[i]
		if a.ID != f.ID || a.Limit.ID != f.Limit.ID || !a.Limit.Until.Equal(f.Limit.Until) || !slices.Equal(a.Limit.Windows, f.Limit.Windows) {
			t.Errorf("%s's limit is %+v, want %s's, %+v", a.ID, a.Limit, f.ID, f.Limit)
		}
		for _, w := range f.Windows {
			got, ok := a.Window(w.Key)
			if slack := used[windowOf{account: a.ID, key: w.Key}] + 0.005; !ok || math.Abs(got.Utilization-w.Utilization) > slack || !got.ResetsAt.Equal(w.ResetsAt) {
				t.Errorf("%s's %s reads %v, to reset at %s, want %v, but for its own answers', %v, at %s",
					a.ID, w.Key, got.Utilization, got.ResetsAt, w.Utilization, slack, w.ResetsAt)
			}
		}
	}
	told, extra := told(played.Events), told(frame.Events)
	for _, e := range extra {
		i := slices.Index(told, e)
		if i < 0 {
			t.Errorf("the router tells %q, want %q among it", told, e)
			continue
		}
		told = slices.Delete(told, i, i+1)
	}
	for _, e := range told {
		if !strings.HasPrefix(e, status.EventMoved+" ") || !strings.Contains(e, "counted") {
			t.Errorf("the router tells %q, which the frame's doesn't, want only the moves its limit counts", e)
		}
	}
}

// told are the events, each as its kind, account and count, where it counts
// any, where they went, and, of a move, whether the limit it left counts it.
func told(events []status.Event) []string {
	var said []string
	for _, e := range events {
		s := fmt.Sprintf("%s %s %d", e.Kind, e.Account, e.Count)
		if e.Count > 0 {
			s += " to " + e.To
		}
		if e.Kind == status.EventMoved && e.Limit > 0 {
			s += " counted"
		}
		said = append(said, s)
	}
	return said
}

// placed are the accounts each session's models are on, as listed, by the
// session's short id and the model.
func placed(sessions []status.Session) map[string]string {
	on := make(map[string]string)
	for _, s := range sessions {
		for _, a := range s.Assignments {
			on[status.ShortID(s.ID)+" "+a.Model] = a.Account
		}
	}
	return on
}

// The readings the scenario's answers read reach Runway's now column at the
// look after: work's 5-hour window, read a hundredth up, at 59%.
func TestTheReadingsTheAnswersReadDrawOnRunwayAtTheNextLook(t *testing.T) {
	s, err := ScenarioNamed("dashboard")
	if err != nil {
		t.Fatal(err)
	}
	p := played(t, s)
	read := p.base.played(nil, time.Time{}).world.readings.read
	i := slices.IndexFunc(read, func(r readings.Reading) bool { return r.Account == "work" && r.Key == fiveHourKey })
	if i < 0 {
		t.Fatal("the scenario's answers read nothing of work's 5-hour window")
	}
	at := read[i].At.Sub(p.base.start).Seconds()
	p.press(1, toRunway(">")...)
	now := regexp.MustCompile(`5-hour (\d+)%`)
	p.until(at - 0.001)
	if got := now.FindStringSubmatch(p.screen()); got == nil || got[1] != "58" {
		t.Errorf("before the reading, Runway shows work's 5-hour at %v, want 58%%", got)
	}
	p.until(at + looksEvery.Seconds())
	if got := now.FindStringSubmatch(p.screen()); got == nil || got[1] != "59" {
		t.Errorf("by the look after the reading, Runway shows work's 5-hour at %v, want 59%%, as read", got)
	}
}

// A look at the router as a scenario plays reads its base in place: its
// year of readings, lines and events are never copied, so a look allocates
// far fewer times than the year has readings, and fewer bytes than a copy of
// them, however far the scenario has played: what it allocates is what has
// played since. The race detector triples what a look allocates, so the
// bytes are held to a copy's, not under it.
func TestALookReadsTheBaseInPlace(t *testing.T) {
	s, err := ScenarioNamed("routing")
	if err != nil {
		t.Fatal(err)
	}
	tl, c := timelineOf(s, 14500*time.Millisecond)
	if err := tl.Pin(t.Context(), []string{"side"}, true); err != nil {
		t.Fatal(err)
	}
	c.advance(10 * time.Second)
	look := func() {
		if _, _, err := tl.Read(t.Context(), status.Read{}); err != nil {
			t.Fatal(err)
		}
		if _, err := tl.Sessions(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	year := len(tl.base.world.history)
	if allocs := testing.AllocsPerRun(5, look); allocs > float64(year)/10 {
		t.Errorf("a look allocates %v times, want under a tenth of the year's %d readings", allocs, year)
	}
	yearSize := uint64(year) * uint64(reflect.TypeFor[readings.Reading]().Size())
	if size := allocated(look); size > yearSize {
		t.Errorf("a look allocates %d bytes, want under the year's readings' own, %d, as a copy of them alone would", size, yearSize)
	}
}

// allocated is how many bytes do allocates, as the runtime counts them.
func allocated(do func()) uint64 {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	do()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}
