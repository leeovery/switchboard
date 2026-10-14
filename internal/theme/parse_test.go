package theme_test

import (
	"errors"
	"image/color"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/theme"
)

// base are a theme file's lines for the 19 base tokens, Nord's, which every
// theme file gives.
var base = []string{
	"text.primary = #ECEFF4",
	"text.secondary = #E5E9F0",
	"text.tertiary = #D8DEE9",
	"text.muted = #939EB2",
	"text.subtle = #73819B",
	"text.faint = #4C566A",
	"text.on-selection = #FFFFFF",
	"accent.primary = #B48EAD",
	"accent.key = #81A1C1",
	"accent.mode = #88C0D0",
	"accent.attention = #EBCB8B",
	"state.positive = #A7C492",
	"state.destructive = #DD8188",
	"canvas = #2E3440",
	"bg.selection = #434C5E",
	"bg.attention = #3D4046",
	"bg.subtle = #3B4252",
	"border = #4C566A",
	"text.on-attention = #ECEFF4",
}

// file is a theme file of base's lines but those whose keys drop names,
// then more's, a line each.
func file(drop []string, more ...string) string {
	var lines []string
	for _, line := range base {
		key, _, _ := strings.Cut(line, " =")
		if !slices.Contains(drop, key) {
			lines = append(lines, line)
		}
	}
	return strings.Join(append(lines, more...), "\n") + "\n"
}

func TestParseReadsEveryToken(t *testing.T) {
	got, err := theme.Parse("lake", []byte(file(nil)))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got.Slug != "lake" {
		t.Errorf("Slug = %q, want lake", got.Slug)
	}
	for _, line := range base {
		key, value, _ := strings.Cut(line, " = ")
		if c := hex(got.Colour(token(t, key))); c != value {
			t.Errorf("%s = %s, want %s", key, c, value)
		}
	}
}

func TestParseReadsAFileAsWritten(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{name: "comments and blank lines", text: "# Lake: a theme.\n\n" + file(nil, "", "  # indented, a comment all the same", "")},
		{name: "spaces around the equals, or none", text: strings.ReplaceAll(file(nil), " = ", "=")},
		{name: "lower-case colours", text: strings.ToLower(file(nil))},
		{name: "Windows line endings", text: strings.ReplaceAll(file(nil), "\n", "\r\n")},
		{name: "a byte order mark", text: string(rune(0xFEFF)) + file(nil)},
		{name: "a key it doesn't know, passed over", text: file(nil, "bg.hover = #123456", "glyph.tick = ✓")},
		{name: "the cords' viz.series keys, gone, passed over unread", text: file(nil, "viz.series.1 = #81A1C1", "viz.series.6 = amber")},
		{name: "the base tokens alone, under a comment", text: "# Lake: the base tokens alone.\n" + file(nil)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := theme.Parse("lake", []byte(tt.text))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if c := hex(got.Colour(theme.Canvas)); c != "#2E3440" {
				t.Errorf("canvas = %s, want #2E3440", c)
			}
		})
	}
}

func TestParseRefusesWhatIsntATheme(t *testing.T) {
	tests := []struct {
		name string
		text string
		// want is the problem's reason and detail, as the log has it.
		want string
	}{
		{
			name: "a key twice",
			text: file(nil, "accent.key = #81A1C1"),
			want: "bad syntax: line 20: duplicate key accent.key",
		},
		{
			name: "a key it doesn't know, twice",
			text: file(nil, "bg.hover = #123456", "bg.hover = #654321"),
			want: "bad syntax: line 21: duplicate key bg.hover",
		},
		{
			name: "a line that isn't a pair",
			text: "text.primary #ECEFF4\n" + file(nil),
			want: "bad syntax: line 1: not a key = value pair",
		},
		{
			name: "a pair without a key",
			text: file(nil, "= #ECEFF4"),
			want: "bad syntax: line 20: not a key = value pair",
		},
		{
			name: "a key with a space in it",
			text: file(nil, "accent key = #81A1C1"),
			want: "bad syntax: line 20: not a key = value pair",
		},
		{
			name: "a quoted value",
			text: file([]string{"canvas"}, `canvas = "#2E3440"`),
			want: "bad syntax: line 19: quoted value",
		},
		{
			name: "a value singly quoted",
			text: file([]string{"canvas"}, "canvas = '#2E3440'"),
			want: "bad syntax: line 19: quoted value",
		},
		{
			name: "a missing token",
			text: file([]string{"accent.key"}),
			want: "missing tokens: missing accent.key",
		},
		{
			name: "every missing token, named",
			text: file([]string{"text.faint", "canvas", "border"}),
			want: "missing tokens: missing text.faint, canvas, border",
		},
		{
			name: "nothing at all",
			text: "# nothing yet\n",
			want: "missing tokens: missing text.primary, text.secondary, text.tertiary, text.muted, text.subtle, text.faint, text.on-selection, " +
				"accent.primary, accent.key, accent.mode, accent.attention, state.positive, state.destructive, canvas, bg.selection, bg.attention, " +
				"bg.subtle, border, text.on-attention",
		},
		{
			name: "a missing token's key mistyped in its case",
			text: file([]string{"canvas"}, "Canvas = #2E3440"),
			want: "missing tokens: missing canvas",
		},
		{
			name: "bad colours, every one named",
			text: file([]string{"canvas", "border"}, "canvas = #2E344", "border = slate"),
			want: "bad colour: canvas = #2E344, border = slate",
		},
		{
			name: "a short colour",
			text: file([]string{"canvas"}, "canvas = #234"),
			want: "bad colour: canvas = #234",
		},
		{
			name: "a colour with alpha",
			text: file([]string{"canvas"}, "canvas = #2E3440FF"),
			want: "bad colour: canvas = #2E3440FF",
		},
		{
			name: "a colour that isn't hexadecimal",
			text: file([]string{"canvas"}, "canvas = #2E344G"),
			want: "bad colour: canvas = #2E344G",
		},
		{
			name: "a colour without its hash",
			text: file([]string{"canvas"}, "canvas = 2E3440"),
			want: "bad colour: canvas = 2E3440",
		},
		{
			name: "a hash after a value, which starts no comment",
			text: file([]string{"canvas"}, "canvas = #2E3440 # Nord's"),
			want: "bad colour: canvas = #2E3440 # Nord's",
		},
		{
			name: "a bad colour of one of switchboard's own",
			text: file(nil, "viz.track = track"),
			want: "bad colour: viz.track = track",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := theme.Parse("lake", []byte(tt.text))
			if err == nil || err.Error() != tt.want {
				t.Fatalf("Parse() error = %v, want %s", err, tt.want)
			}
			if _, ok := errors.AsType[*theme.Problem](err); !ok {
				t.Errorf("Parse() error is a %T, want a *theme.Problem", err)
			}
		})
	}
}

func TestWhatAThemeLeavesOutOfItsChartsIsWorkedOutFromTheRest(t *testing.T) {
	lake, err := theme.Parse("lake", []byte(file(nil)))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		token theme.Token
		want  string
	}{
		{token: theme.VizRamp1, want: "#A7C492"},
		{token: theme.VizRamp2, want: "#EBCB8B"},
		// Halfway from accent.attention, #EBCB8B, to state.destructive, #DD8188.
		{token: theme.VizRamp3, want: "#E4A68A"},
		{token: theme.VizRamp4, want: "#DD8188"},
		// bg.subtle.
		{token: theme.VizTrack, want: "#3B4252"},
		{token: theme.VizPace, want: "#ECEFF4"},
		// accent.attention.
		{token: theme.VizReserve, want: "#EBCB8B"},
		// accent.key, accent.mode, accent.primary and text.tertiary.
		{token: theme.VizFamily1, want: "#81A1C1"},
		{token: theme.VizFamily2, want: "#88C0D0"},
		{token: theme.VizFamily3, want: "#B48EAD"},
		{token: theme.VizFamily4, want: "#D8DEE9"},
		// A family's newest version in its hue, an older one its hue mixed
		// 30% toward the canvas, #2E3440.
		{token: theme.VizVersion1, want: "#81A1C1"},
		{token: theme.VizVersion2, want: "#68809A"},
		{token: theme.VizVersion3, want: "#88C0D0"},
		{token: theme.VizVersion4, want: "#6D96A5"},
		{token: theme.VizVersion5, want: "#B48EAD"},
		{token: theme.VizVersion6, want: "#D8DEE9"},
		// bg.subtle, then 40% and 70% of the way to accent.key, accent.key,
		// and accent.key 55% toward white.
		{token: theme.VizDay0, want: "#3B4252"},
		{token: theme.VizDay1, want: "#57687E"},
		{token: theme.VizDay2, want: "#6C85A0"},
		{token: theme.VizDay3, want: "#81A1C1"},
		{token: theme.VizDay4, want: "#C6D5E3"},
	}
	for _, tt := range tests {
		t.Run(tt.token.String(), func(t *testing.T) {
			if got := hex(lake.Colour(tt.token)); got != tt.want {
				t.Errorf("left out, %s = %s, want %s", tt.token, got, tt.want)
			}
			given, err := theme.Parse("lake", []byte(file(nil, tt.token.String()+" = #102030")))
			if err != nil {
				t.Fatal(err)
			}
			if got := hex(given.Colour(tt.token)); got != "#102030" {
				t.Errorf("given, %s = %s, want #102030", tt.token, got)
			}
		})
	}
}

// A version's and a day's shades are worked out from what the file gives:
// its family's hue, the empty day's shade, and the canvas.
func TestTheShadesFollowWhatAThemeGives(t *testing.T) {
	lake, err := theme.Parse("lake", []byte(file(nil, "viz.family.1 = #5E81AC", "viz.day.0 = #102030")))
	if err != nil {
		t.Fatal(err)
	}
	for tok, want := range map[theme.Token]string{
		theme.VizVersion1: "#5E81AC",
		// #5E81AC 30% toward the canvas, #2E3440.
		theme.VizVersion2: "#506A8C",
		// 40% and 70% of the way from #102030 to accent.key, #81A1C1.
		theme.VizDay1: "#3D546A",
		theme.VizDay2: "#5F7A96",
	} {
		if got := hex(lake.Colour(tok)); got != want {
			t.Errorf("%s = %s, want %s", tok, got, want)
		}
	}
}

// Every ramp of shades is drawn by one rule: from its base, 40% and then 70%
// of the way to its hue, the hue itself, then the hue mixed 55% toward white,
// as nord's frame of the Year filtered to Opus 5.5 draws them; or toward black
// on a light canvas. A theme's ramps start from its viz.day.0, on its canvas.
func TestEveryRampOfShadesIsDrawnByOneRule(t *testing.T) {
	base, opus := color.RGBA{R: 0x3B, G: 0x42, B: 0x52, A: 0xff}, color.RGBA{R: 0x5E, G: 0x81, B: 0xAC, A: 0xff}
	tests := []struct {
		light bool
		want  [4]string
	}{
		{want: [4]string{"#495B76", "#546E91", "#5E81AC", "#B7C6DA"}},
		{light: true, want: [4]string{"#495B76", "#546E91", "#5E81AC", "#2A3A4D"}},
	}
	for _, tt := range tests {
		var got [4]string
		for i, c := range theme.Shades(base, opus, tt.light) {
			got[i] = hex(c)
		}
		if got != tt.want {
			t.Errorf("Shades(#3B4252, #5E81AC, light: %v) = %q, want %q", tt.light, got, tt.want)
		}
	}
	nord, _ := theme.Builtin("nord")
	if got, want := nord.Steps(opus), theme.Shades(base, opus, false); got != want {
		t.Errorf("nord's steps toward Opus are %v, want from its viz.day.0 toward white, %v", got, want)
	}
	day, _ := theme.Builtin("tokyo-night-day")
	if got, want := day.Steps(opus), theme.Shades(day.Colour(theme.VizDay0), opus, true); got != want {
		t.Errorf("tokyo-night-day's steps toward Opus are %v, want from its viz.day.0 toward black, %v", got, want)
	}
}

func TestValidSlug(t *testing.T) {
	tests := []struct {
		slug string
		want bool
	}{
		{slug: "nord", want: true},
		{slug: "tokyo-night-day", want: true},
		{slug: "2026", want: true},
		{slug: "a", want: true},
		{slug: "x-", want: true},
		{slug: ""},
		{slug: "-nord"},
		{slug: "Nord"},
		{slug: "nord_lee"},
		{slug: "nord.theme"},
		{slug: "../nord"},
		{slug: "nord/day"},
		{slug: "nörd"},
		{slug: "nord day"},
	}
	for _, tt := range tests {
		t.Run(tt.slug, func(t *testing.T) {
			if got := theme.ValidSlug(tt.slug); got != tt.want {
				t.Errorf("ValidSlug(%q) = %v, want %v", tt.slug, got, tt.want)
			}
		})
	}
}

// token is the token a .theme file's key names.
func token(t *testing.T, key string) theme.Token {
	t.Helper()
	for tok := theme.TextPrimary; tok.String() != ""; tok++ {
		if tok.String() == key {
			return tok
		}
	}
	t.Fatalf("no token is keyed %s", key)
	return 0
}

// hex is c written #RRGGBB, or "none" for no colour.
func hex(c color.Color) string {
	if c == nil {
		return "none"
	}
	r, g, b, _ := c.RGBA()
	const digits = "0123456789ABCDEF"
	out := []byte{'#'}
	for _, v := range []uint32{r >> 8, g >> 8, b >> 8} {
		out = append(out, digits[v>>4], digits[v&0xf])
	}
	return string(out)
}
