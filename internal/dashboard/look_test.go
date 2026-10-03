package dashboard_test

import (
	"fmt"
	"image/color"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/theme"
)

// nordCanvas is Nord's canvas as a background's SGR parameters.
const nordCanvas = "48;2;46;52;64"

func TestRenderFullScreenPaintsTheCanvasOnEveryCell(t *testing.T) {
	look := dashboard.Screen(builtin(t, "nord"))
	for _, l := range layouts() {
		t.Run(l.name, func(t *testing.T) {
			opts := l.opts
			opts.Look = look
			frame := dashboard.Render(l.doc, now, opts)
			plain := strings.Split(dashboard.Render(l.doc, now, l.opts), "\n")
			for i, line := range strings.Split(frame, "\n") {
				if got := ansi.StringWidth(line); got != opts.Width {
					t.Errorf("line %d is %d cells wide, want the whole width, %d", i, got, opts.Width)
				}
				if got, want := strings.TrimRight(ansi.Strip(line), " "), plain[i]; got != want {
					t.Errorf("line %d reads %q, want %q, as without colour", i, got, want)
				}
				for _, run := range unpainted(line) {
					t.Errorf("line %d has %q off the canvas", i, run)
				}
			}
		})
	}
}

func TestRenderPrintedPaintsNoCanvas(t *testing.T) {
	l := layouts()[0]
	opts := l.opts
	opts.Look = dashboard.Print(builtin(t, "nord"), color.White)

	frame := dashboard.Render(l.doc, now, opts)

	if strings.Contains(frame, "\x1b[48") || strings.Contains(frame, ";48;") {
		t.Errorf("Render() printed paints a background:\n%q\nwant the terminal's own left as it is", frame)
	}
}

func TestRenderWithoutColourTellsStateByGlyphsAndBold(t *testing.T) {
	for _, l := range layouts() {
		t.Run(l.name, func(t *testing.T) {
			opts := l.opts
			opts.Look = dashboard.NoColour()
			frame := dashboard.Render(l.doc, now, opts)
			if stripped := ansi.Strip(frame); stripped != dashboard.Render(l.doc, now, l.opts) {
				t.Errorf("Render() without colour, stripped of its escapes =\n%s\nwant the frame as text", stripped)
			}
			for _, sgr := range regexp.MustCompile(`\x1b\[[0-9;]*m`).FindAllString(frame, -1) {
				if sgr != "\x1b[1m" && sgr != "\x1b[m" && sgr != "\x1b[0m" {
					t.Errorf("Render() without colour has %q, want bold alone", sgr)
				}
			}
		})
	}
	opts := layouts()[0].opts
	opts.Look = dashboard.NoColour()
	if frame := dashboard.Render(threeAccounts(), now, opts); !strings.Contains(frame, "\x1b[1m") {
		t.Error("Render() without colour has no bold, want state told by bold where colour told it")
	}
}

func TestRenderInEveryBuiltIn(t *testing.T) {
	l := layouts()[0]
	plain := dashboard.Render(l.doc, now, l.opts)
	for _, b := range theme.Builtins() {
		t.Run(b.Slug, func(t *testing.T) {
			opts := l.opts
			opts.Look = dashboard.Print(b, nil)
			frame := dashboard.Render(l.doc, now, opts)
			if stripped := ansi.Strip(frame); stripped != plain {
				t.Errorf("Render() in %s, stripped of its escapes =\n%s\nwant\n%s", b.Slug, stripped, plain)
			}
			if b.Slug == theme.Terminal && strings.Contains(frame, "38;2;") {
				t.Errorf("Render() in the terminal's own colours has a colour of its own:\n%q", frame)
			}
		})
	}
}

func TestTheTerminalThemeLeavesTheTerminalsBackground(t *testing.T) {
	look := dashboard.Screen(builtin(t, theme.Terminal))
	if look.Canvas() != nil {
		t.Errorf("Canvas() = %v, want none painted", look.Canvas())
	}
	opts := layouts()[0].opts
	opts.Look = look
	if frame := dashboard.Render(threeAccounts(), now, opts); strings.Contains(frame, "48;") || strings.Contains(frame, "\x1b[4") {
		t.Errorf("Render() in the terminal's theme paints a background:\n%q", frame)
	}
}

func TestBlendsAreWorkedOutAgainstTheColourBeneath(t *testing.T) {
	nord := builtin(t, "nord")
	red := color.RGBA{R: 0xBF, G: 0x61, B: 0x6A, A: 0xff}
	tests := []struct {
		name string
		look dashboard.Look
		want string
	}{
		{name: "full screen, against the canvas", look: dashboard.Screen(nord), want: "#774B55"},
		{name: "printed, against the terminal's background", look: dashboard.Print(nord, color.White), want: "#DFB0B5"},
		{name: "printed where the terminal didn't say, against the theme's canvas", look: dashboard.Print(nord, nil), want: "#774B55"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.look.Blend(red, 0.5)
			if !ok || hex(got) != tt.want {
				t.Errorf("Blend(red, half) = %s, %v; want %s", hex(got), ok, tt.want)
			}
		})
	}
	for name, look := range map[string]dashboard.Look{
		"without colour":            dashboard.NoColour(),
		"in the terminal's colours": dashboard.Screen(builtin(t, theme.Terminal)),
		"printed in them":           dashboard.Print(builtin(t, theme.Terminal), color.White),
	} {
		if got, ok := look.Blend(red, 0.5); ok {
			t.Errorf("%s, Blend() = %s, want none: shade glyphs stand in", name, hex(got))
		}
	}
}

func TestTheZeroLookDrawsTextAlone(t *testing.T) {
	var look dashboard.Look
	if look.Canvas() != nil {
		t.Errorf("Canvas() = %v, want none", look.Canvas())
	}
	if frame := dashboard.Render(threeAccounts(), now, dashboard.Options{Width: 160}); strings.Contains(frame, "\x1b") {
		t.Errorf("Render() in the zero look has escapes:\n%q", frame)
	}
}

// unpainted are the runs of a line drawn off the canvas: text after an SGR
// that sets no background, or Nord's canvas.
func unpainted(line string) []string {
	var runs []string
	sgr := regexp.MustCompile(`\x1b\[([0-9;]*)m([^\x1b]*)`)
	for _, m := range sgr.FindAllStringSubmatch(line, -1) {
		if m[2] != "" && !strings.Contains(m[1], nordCanvas) {
			runs = append(runs, m[2])
		}
	}
	if first := strings.Index(line, "\x1b"); first != 0 {
		runs = append(runs, line[:max(first, 0)])
	}
	return runs
}

// builtin is the built-in theme slug names.
func builtin(t *testing.T, slug string) theme.Theme {
	t.Helper()
	b, ok := theme.Builtin(slug)
	if !ok {
		t.Fatalf("no built-in theme %s", slug)
	}
	return b
}

// hex is c written #RRGGBB, or "none" for no colour.
func hex(c color.Color) string {
	if c == nil {
		return "none"
	}
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02X%02X%02X", r>>8, g>>8, b>>8)
}
