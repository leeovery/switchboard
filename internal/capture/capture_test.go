package capture

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
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
		name    string
		watched []Fixture
		over    []overWorld
	}{
		{name: "two the watch draws", watched: []Fixture{{Name: "accounts-1"}, {Name: "accounts-3"}, {Name: "accounts-1", Size: wide(10)}}},
		{name: "one the watch draws, and a printout", watched: []Fixture{{Name: "accounts-1"}}, over: []overWorld{printout{name: "usage-160"}, printout{name: "accounts-1"}}},
		{name: "a printout, and a page", over: []overWorld{printout{name: "accounts-1"}, page{name: "page-log"}, page{name: "accounts-1"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, want := panicked(t, func() { setOf(tt.watched, tt.over, worldsNow) }), `capture: two fixtures named "accounts-1"`; got != want {
				t.Errorf("setOf panicked with %q, want %q", got, want)
			}
		})
	}
}

func TestOnlyAFixtureOverTheWorldsBuildsThem(t *testing.T) {
	built := 0
	set := setOf(fixtures(time.UTC), overWorlds(), func() accountWorlds {
		built++
		return worldsNow()
	})

	names := set.names()
	for _, name := range []string{"accounts-3", "usage-160", "page-routing"} {
		if !slices.Contains(names, name) {
			t.Fatalf("names() = %v, want %s among them", names, name)
		}
	}
	if _, err := set.named("accounts-3"); err != nil {
		t.Fatal(err)
	}
	if built != 0 {
		t.Errorf("having listed the fixtures and asked for one over the samples, the worlds were built %d times, want none", built)
	}
	for i, name := range []string{"usage-160", "page-routing"} {
		if _, err := set.named(name); err != nil {
			t.Fatal(err)
		}
		if built != i+1 {
			t.Errorf("having asked for %s, the worlds were built %d times in all, want %d", name, built, i+1)
		}
	}
}

func TestAFixtureChangedChangesNoOtherCallers(t *testing.T) {
	f, p := fixture(t, "accounts-3-week"), fixture(t, "usage-5h-160")
	keys, size, windows := slices.Clone(f.keys), f.Size, slices.Clone(p.windows)
	f.keys[0], f.Size, p.windows[0] = tea.KeyPressMsg{Code: '2', Text: "2"}, wide(10), accountKey

	if got := fixture(t, "accounts-3-week"); !reflect.DeepEqual(got.keys, keys) || got.Size != size {
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
		{name: "accounts-2", want: `unknown fixture "accounts-2" ` + available},
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
	again := fixturesIn(time.Local)
	for _, name := range Names() {
		t.Run(name, func(t *testing.T) {
			first, second := frameOf(t, local(), name), frameOf(t, again, name)
			if first != second {
				t.Errorf("two frames of %s differ:\n%s\nthen\n%s", name, first, second)
			}
		})
	}
}

func TestFramesReadTheSameInEveryTimeZone(t *testing.T) {
	// Sydney's clocks go forward on the Sunday of the fixtures' week.
	sydney, err := time.LoadLocation("Australia/Sydney")
	if err != nil {
		t.Fatal(err)
	}
	utc := askedIn(time.UTC)
	others := []zonedSet{askedIn(time.FixedZone("UTC+10", 10*60*60)), askedIn(sydney)}
	for _, name := range Names() {
		t.Run(name, func(t *testing.T) {
			f, err := utc.set.named(name)
			if err != nil {
				t.Fatal(err)
			}
			if inTheMachinesZone(f) {
				drawnInTheMachinesZone(t, name, append([]zonedSet{utc}, others...))
				return
			}
			drawn := f.Frame(f.Size)
			for _, other := range others {
				if frame := frameOf(t, other.set, name); frame != drawn {
					t.Errorf("%s drawn in UTC\n%s\ndiffers from it drawn in %s\n%s", name, drawn, other.zone, frame)
				}
			}
		})
	}
}

// zonedSet is every fixture, asked for in a zone.
type zonedSet struct {
	zone *time.Location
	set  fixtureSet
}

func askedIn(zone *time.Location) zonedSet {
	return zonedSet{zone: zone, set: fixturesIn(zone)}
}

// inTheMachinesZone reports whether f is to be built and drawn in the
// machine's own zone, time.Local itself, as a fixture over a world is: over
// its records, or, as usage's printouts are, its document alone. One over
// milestone 5's samples is drawn in a zone whose clocks never change.
func inTheMachinesZone(f Fixture) bool {
	return f.records != nil || f.now.Location() == time.Local
}

// drawnInTheMachinesZone checks the fixture over a world with the given name,
// asked for in each zone given, is drawn as when asked for in the machine's,
// at the frames' moment there. The ledger and the views split its days in
// time.Local, which a test can't change in process: its zone is varied by
// running the tests under TZ, as testdata/vhs/README.md says. Its moment can
// be right while part of its world is still built in the zone asked for, which
// a zone not the machine's shows, wherever the machine is.
func drawnInTheMachinesZone(t *testing.T, name string, asked []zonedSet) {
	t.Helper()
	mine := frameOf(t, local(), name)
	for _, a := range asked {
		f, err := a.set.named(name)
		if err != nil {
			t.Fatal(err)
		}
		if f.now.Location() != time.Local || !f.now.Equal(localMoment()) {
			t.Errorf("%s, over a world, asked for in %s, is drawn at %s, want the frames' moment in the machine's zone", name, a.zone, f.now)
		}
		if drawn := f.Frame(f.Size); drawn != mine {
			t.Errorf("%s, over a world, asked for in %s, is drawn\n%s\nwant it as asked for in the machine's zone\n%s", name, a.zone, drawn, mine)
		}
	}
}

func TestAFixtureIsDrawnAtItsMoment(t *testing.T) {
	frame := ansi.Strip(frameOf(t, local(), "accounts-3"))

	for _, want := range []string{
		// The clock the frames read.
		"Thu 1 Oct  14:42:07",
		// The document read, rather than still being read.
		"● healthy",
		// side's session at its reading, its bar eased all the way there.
		"Session  ██▏┃█████▊░░░░░░░░  12% → 54%",
		// What the router tells of lately, none picked out as new, the first
		// look having seen them all.
		"14:41  ▲ c61b started on side, the best",
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
	f := fixture(t, "accounts-3")
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
	f := fixture(t, "accounts-3")
	f.keys = []tea.KeyPressMsg{{Code: 'w', Text: "w"}}

	if frame, want := ansi.Strip(f.Frame(f.Size)), "w window: 5h"; !strings.Contains(frame, want) {
		t.Errorf("having pressed w, which does nothing till a document is read, the frame\n%s\nhas no %q", frame, want)
	}
}

func TestAFixtureIsDrawnInItsTheme(t *testing.T) {
	amber, _ := theme.Builtin("amber")
	f := fixture(t, "accounts-3")
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

func TestTheThemesFixtureHasThePickerOpenOnTheThemeBeforeTheFrames(t *testing.T) {
	frame := ansi.Strip(fixture(t, "accounts-3-themes").Frame(wide(34)))

	for _, want := range []string{"│ Themes", "│ ▌ exchange", "│   nord                     ●", "╭─ 1 work ─"} {
		if !strings.Contains(frame, want) {
			t.Errorf("the frame\n%s\nhas no %q", frame, want)
		}
	}
}

func TestTheFlippedFixturesTurnTheCardsOverWithTheirKeys(t *testing.T) {
	utc := fixturesIn(time.UTC)
	tests := []struct {
		name string
		want []string
	}{
		{name: "accounts-flipped-all", want: []string{
			"┏━ 1 work ━", "sessions · ◆ primary ━┓", "─ sessions ─╮", "sessions · ▲ next ─╮",
			"3 moved to side at 14:12,", "when personal reached its limit",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := ansi.Strip(frameOf(t, utc, tt.name))
			for _, want := range tt.want {
				if !strings.Contains(frame, want) {
					t.Errorf("the frame\n%s\nhas no %q", frame, want)
				}
			}
		})
	}
}

func TestTheRunwayFixturesShowRunwayWithTheirKeys(t *testing.T) {
	utc := fixturesIn(time.UTC)
	tests := []struct {
		name string
		want []string
	}{
		{name: "runway-day-3", want: []string{
			" Runway ", "accounts with room", "w window: day",
			"reaches its reserve ~15:45  ·  back 17:10, as it resets", "limit reached 14:12  ·  back 15:54", "room all day",
		}},
		{name: "runway-day-5", want: []string{"accounts with room", " 5 spare", "week reaches its reserve ~16:04  ·  back Tue 09:00, as it resets"}},
		{name: "runway-week-3", want: []string{
			"weeks with room", "w window: week", "resets Mon 21:00  ·  87% used by then",
			"runs out ~Fri 04:06  ·  back Sun 02:00, as it resets", "one column ≈ 75 min · yesterday dimmed",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := ansi.Strip(frameOf(t, utc, tt.name))
			for _, want := range tt.want {
				if !strings.Contains(frame, want) {
					t.Errorf("the frame\n%s\nhas no %q", frame, want)
				}
			}
		})
	}
}

func TestTheChartFixturesDrawEachStyleWithTheirKeys(t *testing.T) {
	utc := fixturesIn(time.UTC)
	rate := "KEY      ▃▅ use per 10 min   ▆▆ too fast to last   ⠂⠂⠂ fastest that lasts   │"
	hourglass := "KEY      ▀▀ room left   ▄▄ used   ▐▌ its recent rate, falling while busy   │"
	tests := []struct {
		name string
		want []string
	}{
		{name: "accounts-3-rate", want: []string{rate, "g chart", "│  12:10                now               17:10  │", "█▁▁▁▁▁▁▁▁▁│▁▁▁▁▁▁▁▁▁"}},
		{name: "accounts-3-hourglass", want: []string{hourglass, "│                    ▜████████▛                  │", "│  12:10                now               17:10  │"}},
		{name: "accounts-4-rate", want: []string{rate, "│  12:10                now               17:10  │"}},
		{name: "accounts-4-hourglass", want: []string{hourglass, "│                  ▜████████████▛                │"}},
		{name: "accounts-8-rate", want: []string{rate, "│  Session  58%  → out ~15:45      resets 17:10  │"}},
		{name: "accounts-8-hourglass", want: []string{hourglass, "│                     ▗▄▟▟▟▄▄▖                   │"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := ansi.Strip(frameOf(t, utc, tt.name))
			for _, want := range tt.want {
				if !strings.Contains(frame, want) {
					t.Errorf("the frame\n%s\nhas no %q", frame, want)
				}
			}
		})
	}
}

func TestTheSessionsFixturesShowSessionsWithTheirKeys(t *testing.T) {
	utc := fixturesIn(time.UTC)
	tests := []struct {
		name string
		want []string
	}{
		{name: "sessions-1", want: []string{" Sessions ", "┌─ 1 · WORK ─ ● under pressure", "    ● db8a  sonnet  idle       3s", "  LOG"}},
		{name: "sessions-3", want: []string{
			"CALLS  sessions, by the line they are on", "LINES  accounts",
			"  d28c  opus    1m           ●━", "  41e0  sonnet  4m           ●━━", "╌╌╌╌╌╌╌○  Session", "└─ resets  session 18:50  ·  weeks Mon 10:00 ─",
		}},
		{name: "sessions-3-keys", want: []string{
			"─ HELP · SESSIONS ", "│  ●━━◉   a session's cord to its account", "│  ↓      streaming back, ~ its tokens so far",
		}},
		{name: "sessions-5", want: []string{"  9e21  sonnet  now          ●━", "┌─ 5 · SPARE ─ ○ idle", "└─ primed at 16:20 ─", "  14:39  ▸ 9e21 moved side → client (pin)"}},
		{name: "sessions-storyboard-1-request-out", want: []string{"  c61b  opus    now  ↑ ask   ●━"}},
		{name: "sessions-storyboard-2-streaming-back", want: []string{"  c61b  opus    now  ↓ ~1.2k ●━"}},
		{name: "sessions-storyboard-3-refused", want: []string{"  d28c  opus    now  ✕ 429   ●━", "━✕  Session", "┌─ 1 · WORK ─ ■ limit reached"}},
		{name: "sessions-storyboard-4-repatched", want: []string{
			"  d28c  ↪ moved to side", "╌╌╌╌╌╌╌╌╌○  Session", "  d28c  opus    now  ↪ new   ●━",
			"  14:43  ▸ d28c moved work → side: work reached its limit, so",
		}},
		{name: "accounts-flipped-all", want: []string{"● d28c  opus    streaming  ↓ ~1.2k", "● db8a  sonnet  waiting    38s", "● c61b  opus    streaming  ↓ ~3.4k", "● db8a  opus    waiting    12s"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := ansi.Strip(frameOf(t, utc, tt.name))
			for _, want := range tt.want {
				if !strings.Contains(frame, want) {
					t.Errorf("the frame\n%s\nhas no %q", frame, want)
				}
			}
		})
	}
}

func TestThePagesShowWhatTheyAreFor(t *testing.T) {
	tests := []struct {
		name       string
		want       []string
		wantNoneOf []string
	}{
		{name: "page-routing", want: []string{
			"╭─ ROUTING · NEW AND RUNNING SESSIONS ─", "│  ■ 2 personal    open", "│  ■ 3 side        open", "│  ■ m  also move the sessions running",
			"New sessions go to personal or side. When both are full, the router chooses.",
		}},
		{name: "page-routing-phone", want: []string{"╭─ ROUTING · NEW AND RUNNING SESSIONS ─", "The 4 sessions running elsewhere move there", "Rebuilding their caches costs ≈ $"}},
		{name: "page-help-scrolled", want: []string{"─ HELP · SESSIONS ", "│  ●━━◉   a session's cord to its account"}, wantNoneOf: []string{"t themes"}},
		{name: "page-log", want: []string{" Overview   Accounts   History   Sessions   Runway   Log   tab ⇥", "LOG  ─", "tab views   p pin    ? help  q quit"}},
		{name: "page-not-answering", want: []string{"■ the router isn't answering  ·  since 14:42:07  ·  auto  ·  6 sessions"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := ansi.Strip(frameOf(t, local(), tt.name))
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
	g, err := parse(frameOf(t, local(), "page-not-answering"))
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
		if c.glyph != ' ' && (c.bold || !c.fg.near(faint)) {
			t.Errorf("p pin's %s, want it faint, %s, not bold, as the router doesn't answer", c, faint)
		}
	}
}

func TestTheStoryboardsPulsesAreWhereItsFramesHaveThem(t *testing.T) {
	utc := fixturesIn(time.UTC)
	tests := []struct {
		name string
		// row is the frame's row the pulse is on, and head the column of
		// its head, drawn bold.
		row, head int
	}{
		{name: "sessions-storyboard-1-request-out", row: 12, head: 70},
		{name: "sessions-storyboard-3-refused", row: 8, head: 79},
		{name: "sessions-storyboard-4-repatched", row: 22, head: 94},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := strings.Split(frameOf(t, utc, tt.name), "\n")
			cells := ansi.Strip(rows[tt.row])
			if got := []rune(cells)[tt.head]; got != '━' {
				t.Fatalf("row %d reads %q, want the pulse's head, ━, at %d", tt.row, cells, tt.head)
			}
			if bold := ansi.Cut(rows[tt.row], tt.head, tt.head+1); !strings.Contains(bold, "\x1b[1") {
				t.Errorf("the cell at %d is drawn %q, want it bold, the pulse's head", tt.head, bold)
			}
		})
	}
}

// everyInput is every key the dashboard takes but q, as the terminal sends
// it, then clicks and turns of the wheel: on the title row's tabs, a phone's
// tabs, the heading row's, a view's rows and the key line of a terminal 28
// rows tall, and past the edge of a narrow one.
var everyInput = []tea.Msg{
	tea.KeyPressMsg{Code: tea.KeyTab}, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift},
	tea.KeyPressMsg{Code: 'w', Text: "w"}, tea.KeyPressMsg{Code: 'g', Text: "g"},
	tea.KeyPressMsg{Code: tea.KeyLeft}, tea.KeyPressMsg{Code: tea.KeyRight}, tea.KeyPressMsg{Code: tea.KeyUp}, tea.KeyPressMsg{Code: tea.KeyDown},
	tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}, tea.KeyPressMsg{Code: 's', Text: "s"},
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

// TestNoFixtureChartStyleKeyOrWidthPanics draws every fixture at every width
// from 1 to 240, at its height, so with their -rate and -hourglass fixtures,
// the cards of three, four and eight accounts draw each chart style at every
// width; then from each fixture, every key, click and turn of the wheel in
// turn, at every fourth width. A printout, which takes no keys, is drawn as
// tall as it is.
func TestNoFixtureChartStyleKeyOrWidthPanics(t *testing.T) {
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
	f := fixture(t, "accounts-3")
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

// frameOf is the frame of the set's fixture with the given name, at its own
// size.
func frameOf(t *testing.T, set fixtureSet, name string) string {
	t.Helper()
	f, err := set.named(name)
	if err != nil {
		t.Fatal(err)
	}
	return f.Frame(f.Size)
}
