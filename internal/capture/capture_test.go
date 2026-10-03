package capture

import (
	"slices"
	"strings"
	"testing"
	"time"

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
	for _, name := range Names() {
		t.Run(name, func(t *testing.T) {
			first, second := frameOf(t, name, time.Local), frameOf(t, name, time.Local)
			if first != second {
				t.Errorf("two frames of %s differ:\n%s\nthen\n%s", name, first, second)
			}
		})
	}
}

func TestFramesReadTheSameInEveryTimeZone(t *testing.T) {
	for _, name := range Names() {
		t.Run(name, func(t *testing.T) {
			utc, east := frameOf(t, name, time.UTC), frameOf(t, name, time.FixedZone("UTC+10", 10*60*60))
			if utc != east {
				t.Errorf("%s drawn in UTC\n%s\ndiffers from it drawn ten hours east\n%s", name, utc, east)
			}
		})
	}
}

func TestAFixtureIsDrawnAtItsMoment(t *testing.T) {
	frame := ansi.Strip(frameOf(t, "accounts-3", time.Local))

	for _, want := range []string{
		// The clock the frames read.
		"Thu 1 Oct  14:42:07",
		// The document, read readAgo before, rather than still being read.
		"read 4s ago",
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
	f.keys = []tea.KeyPressMsg{{Code: '2', Text: "2"}}

	if frame, want := f.Frame(f.Size), "new sessions go to personal · personal"; !strings.Contains(frame, want) {
		t.Errorf("having pressed 2, the frame\n%s\nhas no %q", frame, want)
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

func TestTheThemesFixtureHasThePickerOpenOnTheThemeBeforeTheFrames(t *testing.T) {
	frame := ansi.Strip(fixture(t, "accounts-3-themes").Frame(wide(34)))

	for _, want := range []string{"│ Themes", "│ ▌ exchange", "│   nord                     ●", "5 sessions · auto"} {
		if !strings.Contains(frame, want) {
			t.Errorf("the frame\n%s\nhas no %q", frame, want)
		}
	}
}

func TestTheFlippedFixturesTurnTheCardsOverWithTheirKeys(t *testing.T) {
	tests := []struct {
		name string
		want []string
	}{
		{name: "accounts-flipped-all", want: []string{
			"┏━ 1 work ━", "sessions · ◆ primary ━┓", "─ sessions ─╮", "sessions · ▲ next ─╮",
			"3 moved to side at 14:12,", "when personal reached its limit",
		}},
		{name: "accounts-flipped-selected", want: []string{
			"┏━ 1 work ━", "┃  ▸ ● d28c  opus", "↑↓ select   1-3 move d28c to that account   space flip back   esc done", "d28c selected on work",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := ansi.Strip(frameOf(t, tt.name, time.UTC))
			for _, want := range tt.want {
				if !strings.Contains(frame, want) {
					t.Errorf("the frame\n%s\nhas no %q", frame, want)
				}
			}
		})
	}
}

func TestTheRunwayFixturesShowRunwayWithTheirKeys(t *testing.T) {
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
			frame := ansi.Strip(frameOf(t, tt.name, time.UTC))
			for _, want := range tt.want {
				if !strings.Contains(frame, want) {
					t.Errorf("the frame\n%s\nhas no %q", frame, want)
				}
			}
		})
	}
}

func TestTheChartFixturesDrawEachStyleWithTheirKeys(t *testing.T) {
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
			frame := ansi.Strip(frameOf(t, tt.name, time.UTC))
			for _, want := range tt.want {
				if !strings.Contains(frame, want) {
					t.Errorf("the frame\n%s\nhas no %q", frame, want)
				}
			}
		})
	}
}

func TestTheSessionsFixturesShowSessionsWithTheirKeys(t *testing.T) {
	tests := []struct {
		name string
		want []string
	}{
		{name: "sessions-1", want: []string{" Sessions ", "┌─ 1 · WORK ─ ● under pressure", "    ● db8a  sonnet  idle       3s", "  LOG"}},
		{name: "sessions-3", want: []string{
			"CALLS  sessions, by the line they are on", "LINES  accounts",
			"  d28c  opus    1m           ●━", "  41e0  sonnet  4m           ●━━", "╌╌╌╌╌╌╌○  Session", "└─ resets  session 18:50  ·  weeks Mon 10:00 ─",
		}},
		{name: "sessions-3-keys", want: []string{"│  ●━━◉   a session's cord to its account", "│  ↓      streaming back, ~ its tokens so far"}},
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
			frame := ansi.Strip(frameOf(t, tt.name, time.UTC))
			for _, want := range tt.want {
				if !strings.Contains(frame, want) {
					t.Errorf("the frame\n%s\nhas no %q", frame, want)
				}
			}
		})
	}
}

func TestTheStoryboardsPulsesAreWhereItsFramesHaveThem(t *testing.T) {
	tests := []struct {
		name string
		// row is the frame's row the pulse is on, and head the column of
		// its head, drawn bold.
		row, head int
	}{
		{name: "sessions-storyboard-1-request-out", row: 13, head: 70},
		{name: "sessions-storyboard-3-refused", row: 9, head: 79},
		{name: "sessions-storyboard-4-repatched", row: 23, head: 94},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := strings.Split(frameOf(t, tt.name, time.UTC), "\n")
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

// frameOf is the frame of the fixture with the given name, drawn in the time
// zone given, at its own size.
func frameOf(t *testing.T, name string, loc *time.Location) string {
	t.Helper()
	f, err := named(name, loc)
	if err != nil {
		t.Fatal(err)
	}
	return f.Frame(f.Size)
}
