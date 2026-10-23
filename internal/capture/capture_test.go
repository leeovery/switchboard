package capture

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	_ "time/tzdata"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/theme"
)

func TestNamesNameEveryFixtureOnceSorted(t *testing.T) {
	names := Names()
	if len(names) == 0 {
		t.Fatal("there are no fixtures")
	}
	if !slices.IsSorted(names) {
		t.Errorf("Names() = %v, want them sorted", names)
	}
	if compacted := slices.Compact(slices.Clone(names)); len(compacted) != len(names) {
		t.Errorf("Names() = %v names a fixture twice", names)
	}
	for _, name := range names {
		f, err := ByName(name)
		if err != nil {
			t.Errorf("ByName(%q): %v", name, err)
			continue
		}
		if f.Name != name {
			t.Errorf("ByName(%q) gave the fixture named %q", name, f.Name)
		}
	}
}

func TestTwoFixturesOfANamePanicNamingIt(t *testing.T) {
	tests := []struct {
		name string
		over []overWorld
	}{
		{name: "two printouts", over: []overWorld{printout{name: "sessions-1"}, printout{name: "usage-160"}, printout{name: "sessions-1"}}},
		{name: "a printout, and a page", over: []overWorld{printout{name: "sessions-1"}, page{name: "log-requests-160"}, page{name: "sessions-1"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, want := panicked(t, func() { setOf(tt.over, worldsNow) }), `capture: two fixtures named "sessions-1"`; got != want {
				t.Errorf("setOf panicked with %q, want %q", got, want)
			}
		})
	}
}

func TestTheWorldsAreBuiltForAFixtureAskedForAndNoSooner(t *testing.T) {
	built := 0
	set := setOf(overWorlds(), func() accountWorlds {
		built++
		return worldsNow()
	})

	names := set.names()
	for _, name := range []string{"usage-160", "page-routing"} {
		if !slices.Contains(names, name) {
			t.Fatalf("names() = %v, want %s among them", names, name)
		}
	}
	if built != 0 {
		t.Errorf("having listed the fixtures, the worlds were built %d times, want none", built)
	}
	for i, name := range []string{"usage-160", "page-routing"} {
		if _, err := set.named(name); err != nil {
			t.Fatal(err)
		}
		if built != i+1 {
			t.Errorf("having asked for %s, the worlds were built %d times in all, want %d", name, built, i+1)
		}
	}
	for _, name := range []string{"usage-160", "page-routing"} {
		if _, err := set.named(name); err != nil {
			t.Fatal(err)
		}
	}
	if built != 2 {
		t.Errorf("having asked for a printout and a page twice each, the worlds were built %d times in all, want twice: each is built the first time", built)
	}
}

func TestAFixtureChangedChangesNoOtherCallers(t *testing.T) {
	f, p := fixture(t, "page-log-find"), fixture(t, "usage-5h-160")
	keys, size, windows := slices.Clone(f.keys), f.Size, slices.Clone(p.windows)
	f.keys[0], f.Size, p.windows[0] = tea.KeyPressMsg{Code: '2', Text: "2"}, wide(10), accountKey

	if got := fixture(t, "page-log-find"); !reflect.DeepEqual(got.keys, keys) || got.Size != size {
		t.Errorf("having changed one caller's, another's keys are %v at %v, want %v at %v", got.keys, got.Size, keys, size)
	}
	if got := fixture(t, "usage-5h-160").windows; !slices.Equal(got, windows) {
		t.Errorf("having changed one caller's, another's windows are %v, want %v", got, windows)
	}
}

func TestAnEmptyOrUnknownNameListsTheFixtures(t *testing.T) {
	available := "(available: " + strings.Join(Names(), ", ") + ")"
	tests := []struct {
		name, want string
	}{
		{name: "", want: "name a fixture " + available},
		{name: "sessions-2", want: `unknown fixture "sessions-2" ` + available},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := ByName(tt.name)
			if err == nil || err.Error() != tt.want {
				t.Errorf("ByName(%q) error = %v, want %s", tt.name, err, tt.want)
			}
			if f.Name != "" {
				t.Errorf("ByName(%q) gave the fixture named %q, want none", tt.name, f.Name)
			}
		})
	}
}

func TestFramesAreDeterministic(t *testing.T) {
	// local's set is over worldsNow, whose worlds, once built, are kept: the
	// second set is over worlds of its own, so the year and each world are
	// built twice.
	again := setOf(overWorlds(), sync.OnceValue(func() accountWorlds { return newAccountWorlds(localMoment()) }))
	for _, name := range Names() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			first, second := frameOf(t, name), drawnFrom(t, again, name)
			if first != second {
				t.Errorf("two frames of %s differ:\n%s\nthen\n%s", name, first, second)
			}
		})
	}
}

// Every fixture is drawn at its frame's moment in the machine's own zone,
// time.Local itself: the ledger and the views split its world's days in it,
// which a test can't change in process, so its zone is varied by running the
// tests under TZ, as testdata/vhs/README.md says.
func TestEveryFixtureIsDrawnAtTheFramesMomentInTheMachinesZone(t *testing.T) {
	for _, name := range Names() {
		if f := fixture(t, name); f.now.Location() != time.Local || !f.now.Equal(framesMoment(name)) {
			t.Errorf("%s is drawn at %s, want its frame's moment in the machine's zone", name, f.now)
		}
	}
}

// framesMoment is the moment the frame with the given stem is drawn at, in
// the machine's zone: the frames', but where a page's later look has its own.
func framesMoment(stem string) time.Time {
	if i := slices.IndexFunc(pages, func(p page) bool { return p.name == stem && p.looked != nil }); i >= 0 {
		return localMoment().Add(pages[i].looked.after)
	}
	return localMoment()
}

func TestAFixtureIsDrawnAtItsMoment(t *testing.T) {
	frame := ansi.Strip(frameOf(t, "overview-default-160"))

	for _, want := range []string{
		// The clock the frames read.
		"Thu 1 Oct  14:42:07",
		// The document read, rather than still being read.
		"● healthy",
		// side's 5-hour window at its reading, its bar eased all the way
		// there, after its week.
		"78%  out ~Fri 12:20                ▍ ┃                                   13%  21%/h till 18:50",
		// What the router tells of lately, none picked out as new, the first
		// look having seen them all.
		"14:41  ▲ c61b started on side",
	} {
		if !strings.Contains(frame, want) {
			t.Errorf("the frame\n%s\nhas no %q", frame, want)
		}
	}
	if strings.Contains(frame, "restart the router") {
		t.Errorf("the frame\n%s\nasks for the router to be restarted, want its history read", frame)
	}
}

func TestAFrameFillsATerminalOfTheSizeGiven(t *testing.T) {
	f := fixture(t, "overview-default-160")
	for _, size := range []watch.Size{f.Size, {Width: 160, Height: 10}, {Width: 200, Height: 80}} {
		frame := f.Frame(size)
		if !strings.HasSuffix(frame, "\n") {
			t.Errorf("at %d×%d, the frame doesn't end its last line", size.Width, size.Height)
		}
		if lines := strings.Count(frame, "\n"); lines != size.Height {
			t.Errorf("at %d×%d, the frame is %d lines, want %d", size.Width, size.Height, lines, size.Height)
		}
	}
}

func TestAFixturesKeysArePressedOnceItsDocumentIsRead(t *testing.T) {
	f := fixture(t, "overview-default-160")
	f.keys = []tea.KeyPressMsg{{Code: 'p', Text: "p"}}

	if frame, want := ansi.Strip(f.Frame(f.Size)), "╭─ ROUTING · "; !strings.Contains(frame, want) {
		t.Errorf("having pressed p, which does nothing till a document is read, the frame\n%s\nhas no %q", frame, want)
	}
}

func TestAFixtureIsDrawnInItsTheme(t *testing.T) {
	amber, _ := theme.Builtin("amber")
	f := fixture(t, "overview-default-160")
	tests := []struct {
		name    string
		fixture Fixture
		// want is the canvas the frame is painted on, "" for none.
		want string
	}{
		{name: "nord, the frames', unless given", fixture: f, want: "48;2;46;52;64"},
		{name: "the theme given", fixture: f.InTheme(amber), want: "48;2;14;11;6"},
		{name: "without colour", fixture: f.WithoutColour()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := tt.fixture.Frame(tt.fixture.Size)
			if tt.want == "" {
				if strings.Contains(frame, "38;") || strings.Contains(frame, "48;") {
					t.Errorf("the frame is in colour: %q", frame)
				}
				return
			}
			for i, line := range strings.Split(strings.TrimSuffix(frame, "\n"), "\n") {
				if !strings.Contains(line, tt.want) {
					t.Fatalf("line %d is off the canvas %s: %q", i+1, tt.want, line)
				}
			}
		})
	}
}

func TestAThemeFileTakingABuiltInsSlugIsListedAsReserved(t *testing.T) {
	nord, _ := theme.Builtin("nord")
	var file strings.Builder
	for tok := theme.TextPrimary; tok <= theme.TextOnAttention; tok++ {
		r, g, b, _ := nord.Colour(tok).RGBA()
		if tok == theme.Canvas {
			r, g, b = 0, 0, 0
		}
		fmt.Fprintf(&file, "%s = #%02X%02X%02X\n", tok, r>>8, g>>8, b>>8)
	}
	fileNord, err := theme.Parse("nord", []byte(file.String()))
	if err != nil {
		t.Fatal(err)
	}

	var named []string
	for _, e := range listing(fileNord).Entries {
		if strings.HasPrefix(e.Name, "nord") {
			named = append(named, e.Name+" "+e.Slug)
		}
	}
	if want := []string{"nord nord", "nord.theme "}; !slices.Equal(named, want) {
		t.Errorf("the picker lists %q of nord, want %q: the built-in, and the file, which can't be picked", named, want)
	}
}

func TestThePagesShowWhatTheyAreFor(t *testing.T) {
	tests := []struct {
		name       string
		want       []string
		wantNoneOf []string
	}{
		{name: "page-routing", want: []string{
			"╭─ ROUTING · NEW AND RUNNING SESSIONS ─", "│  ■ 2 personal  open", "│  ■ 3 side      open", "│  ■ m  also move the sessions running",
			"New sessions go to personal or side. When both are full, the router", "│  chooses.",
		}},
		{name: "page-routing-phone", want: []string{"╭─ ROUTING · NEW AND RUNNING SESSIONS ─", "The 4 sessions running elsewhere move there", "Rebuilding their caches costs ≈ $"}},
		{name: "page-help-scrolled", want: []string{"─ HELP · OVERVIEW ", "│  USAGE"}, wantNoneOf: []string{"t themes"}},
		{name: "page-not-answering", want: []string{"■ the router isn't answering  ·  since 14:42:07  ·  auto  ·  6 sessions"}},
		{name: "page-session-loading", want: []string{"SESSION   d28c   switchboard  ~/Code/switchboard   loading… ─", "esc back    ? help  q quit"},
			wantNoneOf: []string{"STORY", "TURNS", "↑↓ turn"}},
		{name: "page-sessions-chosen", want: []string{"d28c switchboard              ↑ asking     ▸ from work      ●"}},
		{name: "page-sessions-short", want: []string{"▲ 2 more accounts", "3 side ▲", "db8a infra"}, wantNoneOf: []string{"1 work", "41e0 docs"}},
		{name: "page-sessions-not-answering", want: []string{"■ the router isn't answering", "c61b mobile                   ↓ answering"}},
		{name: "page-year-160x24", want: []string{"limits hit   week ×3 · 5-hour ×412", "▲ 3 more above   ·   ▼ 17 more below   ·   j/k to scroll"}},
		{name: "page-routing-160x16", want: []string{"╭─ ROUTING · NEW SESSIONS ─", "■ 0 auto", "enter apply   esc cancel  │", "╰────"}},
		{name: "page-detail-list-focus", want: []string{"SESSIONS  now", "▸ ● c61b", "enter opens c61b's page", "↑↓ account  enter its page"}},
		{name: "page-detail-sessions-focus", want: []string{"▸3 side", "SESSIONS  now", "▸ ● d28c", "enter opens d28c's page", "↑↓ session  ← accounts  enter its page"}},
		{name: "overview-burn-down-160x39", want: []string{"GRAPHS   work     ⠢ heading   ✕ runs out   ⠤ cap   ┄ full", "v chart: burn-down"}, wantNoneOf: []string{"·····"}},
		{
			name:       "overview-by-account-graphs-160",
			want:       []string{"u   GRAPHS   · even pace   ╌ cap   ┄ limit", "5-HOUR ┄", "WEEK ┄", "ROUTING   auto"},
			wantNoneOf: []string{"FABLE WEEK", "↑↓ focus"},
		},
		{name: "overview-by-account-burn-down-220x50", want: []string{"u   GRAPHS   ⠢ heading", "FABLE WEEK ┄", "✕", "ROUTING   auto ─"}, wantNoneOf: []string{"↑↓ focus"}},
		{name: "overview-by-account-graphs-140", want: []string{"GRAPHS   work     · even pace", "FABLE WEEK ┄"}},
		{name: "overview-pool-burn-down-160", want: []string{"1.2 of 3 left             WEEK ┄", "24 Sep 14:42          now  09:42                 now", "v chart: burn-down"}},
		{name: "overview-pool-220x50", want: []string{"1.2 of 3 left             WEEK ┄", "24 Sep 14:42                     now  09:42                            now  24 Sep 14:42                     now", "v chart: burn-up"}},
		{name: "overview-phone-burn-down-52", want: []string{"GRAPHS   work     ⠢ heading", "5-HOUR ┄", "FABLE WEEK ┄"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := ansi.Strip(frameOf(t, tt.name))
			for _, want := range tt.want {
				if !strings.Contains(frame, want) {
					t.Errorf("the frame\n%s\nhas no %q", frame, want)
				}
			}
			for _, unwanted := range tt.wantNoneOf {
				if strings.Contains(frame, unwanted) {
					t.Errorf("the frame\n%s\nhas %q", frame, unwanted)
				}
			}
		})
	}
}

func TestTheRouterGoneHasPPinFaint(t *testing.T) {
	g, err := parse(frameOf(t, "page-not-answering"))
	if err != nil {
		t.Fatal(err)
	}
	keys, line := g[len(g)-1], g.text()[len(g)-1]
	before, _, found := strings.Cut(line, "p pin")
	if !found {
		t.Fatalf("the key line reads %q, want p pin on it", line)
	}
	from := utf8.RuneCountInString(before)
	faint := rgb(0x4C, 0x56, 0x6A)
	for _, c := range keys[from : from+len("p pin")] {
		if c.glyph != ' ' && (c.bold || c.fg != faint) {
			t.Errorf("p pin's %s, want it faint, %s, not bold, as the router doesn't answer", c, faint)
		}
	}
}

// everyInput is every key the dashboard takes but q, as the terminal sends
// it, then clicks and turns of the wheel: on the title row's tabs, a phone's
// tabs, the heading row's, a view's rows and the key line of a terminal 28
// rows tall, and past the edge of a narrow one.
var everyInput = []tea.Msg{
	tea.KeyPressMsg{Code: tea.KeyTab}, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift},
	tea.KeyPressMsg{Code: '<', Text: "<"}, tea.KeyPressMsg{Code: '>', Text: ">"}, tea.KeyPressMsg{Code: 'g', Text: "g"},
	tea.KeyPressMsg{Code: tea.KeyLeft}, tea.KeyPressMsg{Code: tea.KeyRight}, tea.KeyPressMsg{Code: tea.KeyUp}, tea.KeyPressMsg{Code: tea.KeyDown},
	tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}, tea.KeyPressMsg{Code: 's', Text: "s"}, tea.KeyPressMsg{Code: 'u', Text: "u"},
	tea.KeyPressMsg{Code: 'c', Text: "c"}, tea.KeyPressMsg{Code: 'b', Text: "b"},
	tea.KeyPressMsg{Code: tea.KeyEscape}, tea.KeyPressMsg{Code: tea.KeyEnter},
	tea.KeyPressMsg{Code: 'j', Text: "j"}, tea.KeyPressMsg{Code: 'k', Text: "k"}, tea.KeyPressMsg{Code: tea.KeyPgDown}, tea.KeyPressMsg{Code: tea.KeyPgUp},
	tea.KeyPressMsg{Code: '1', Text: "1"}, tea.KeyPressMsg{Code: '2', Text: "2"}, tea.KeyPressMsg{Code: '3', Text: "3"},
	tea.KeyPressMsg{Code: 'a', Text: "a"}, tea.KeyPressMsg{Code: 'm', Text: "m"}, tea.KeyPressMsg{Code: 'r', Text: "r"},
	tea.KeyPressMsg{Code: 't', Text: "t"}, tea.KeyPressMsg{Code: 'd', Text: "d"}, tea.KeyPressMsg{Code: 'l', Text: "l"},
	tea.KeyPressMsg{Code: '?', Text: "?"}, tea.KeyPressMsg{Code: 'p', Text: "p"}, tea.KeyPressMsg{Code: 'R', Text: "R"},
	tea.MouseClickMsg{X: 20, Y: 0, Button: tea.MouseLeft}, tea.MouseClickMsg{X: 60, Y: 0, Button: tea.MouseLeft},
	tea.MouseClickMsg{X: 30, Y: 1, Button: tea.MouseLeft}, tea.MouseClickMsg{X: 14, Y: 4, Button: tea.MouseLeft},
	tea.MouseClickMsg{X: 10, Y: 9, Button: tea.MouseLeft}, tea.MouseClickMsg{X: 2, Y: 27, Button: tea.MouseLeft},
	tea.MouseClickMsg{X: 200, Y: 40, Button: tea.MouseRight},
	tea.MouseWheelMsg{X: 40, Y: 12, Button: tea.MouseWheelDown}, tea.MouseWheelMsg{X: 40, Y: 12, Button: tea.MouseWheelUp},
}

// TestNoFixtureKeyOrWidthPanics draws every fixture at every width from 1 to
// 240, at its height; then from each fixture, every key, click and turn of
// the wheel in turn, at every fourth width. A printout, which takes no keys,
// is drawn as tall as it is.
func TestNoFixtureKeyOrWidthPanics(t *testing.T) {
	for _, name := range Names() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := fixture(t, name)
			if f.Printout {
				for width := 1; width <= 240; width++ {
					printedAt(t, f, width)
				}
				return
			}
			m := f.settle(f.Size)
			for width := 1; width <= 240; width++ {
				m = drawnAt(t, m, watch.Size{Width: width, Height: f.Size.Height})
			}
			m = f.settle(f.Size)
			for i, width := 0, 1; width <= 240; i, width = i+1, width+4 {
				input := everyInput[i%len(everyInput)]
				m = drawnAt(t, m, watch.Size{Width: width, Height: f.Size.Height}, input)
			}
		})
	}
}

// drawnAt is m once the inputs given are taken and the terminal resized to
// size, drawn, failing the test where taking them, resizing or drawing
// panics.
func drawnAt(t *testing.T, m watch.Model, size watch.Size, inputs ...tea.Msg) watch.Model {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("at %d×%d, taking %v: panicked: %v", size.Width, size.Height, inputs, r)
		}
	}()
	m = deliver(m, inputs...)
	m = deliver(m, tea.WindowSizeMsg{Width: size.Width, Height: size.Height})
	m.View()
	return m
}

// printedAt draws the printout f on a terminal as wide as given, failing the
// test where drawing panics.
func printedAt(t *testing.T, f Fixture, width int) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("at %d columns: panicked: %v", width, r)
		}
	}()
	f.Frame(watch.Size{Width: width})
}

func TestAFixturesModelStartsWhereItsFrameIs(t *testing.T) {
	f := fixture(t, "overview-default-160")
	m := f.Model()

	if cmd := m.Init(); cmd != nil {
		t.Errorf("starting the model returned a command, sending %#v, want none: its first read is taken", cmd())
	}
	if got, want := m.View().Content, f.settle(f.Size).View().Content; got != want {
		t.Errorf("the model starts on\n%s\nwant the fixture's frame\n%s", got, want)
	}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if _, ok := next.(settled); !ok {
		t.Errorf("after a message, the model is a %T, want it settled still", next)
	}
}

// fixture is the fixture with the given name, drawn in the local time zone.
func fixture(t *testing.T, name string) Fixture {
	t.Helper()
	f, err := ByName(name)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// frames are the frames of the fixtures drawn in the local time zone, by
// name, each drawn as frameOf draws it, the first time a test asks for it.
var frames sync.Map

// frameOf is the frame of the fixture with the given name, drawn in the
// local time zone, at its own size: drawn once, as several tests hold the
// same frame, and drawing one takes a while.
func frameOf(t *testing.T, name string) string {
	t.Helper()
	draw, _ := frames.LoadOrStore(name, sync.OnceValues(func() (string, error) {
		f, err := ByName(name)
		if err != nil {
			return "", err
		}
		return f.Frame(f.Size), nil
	}))
	frame, err := draw.(func() (string, error))()
	if err != nil {
		t.Fatal(err)
	}
	return frame
}

// drawnFrom is the frame of the set's fixture with the given name, at its
// own size, drawn afresh.
func drawnFrom(t *testing.T, set fixtureSet, name string) string {
	t.Helper()
	f, err := set.named(name)
	if err != nil {
		t.Fatal(err)
	}
	return f.Frame(f.Size)
}
