package dashboard_test

import (
	"fmt"
	"image/color"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// nordCanvas is Nord's canvas as a background's SGR parameters.
const nordCanvas = "48;2;46;52;64"

// now is the clock every frame is drawn at: a Monday, 13:12 an hour east of
// UTC.
var now = time.Date(2026, 9, 28, 13, 12, 0, 0, time.FixedZone("UTC+1", 60*60))

// claude judges windows as Claude's are judged.
var claude = score.Policy{Shared: []string{"5h", "7d"}, Perishable: "7d", Tiebreak: "5h", Started: "5h", Pressure: "5h"}

// threeAccounts is the router's document of three accounts, their windows
// heading every way: work, the primary, keeping pace; personal held back by
// the limit its session reached; and side, where new sessions go, its Fable
// week used.
func threeAccounts() status.Document {
	window := func(key, label string, used float64, resetsIn time.Duration) quota.Window {
		return quota.Window{Key: key, Label: label, Utilization: used, ResetsAt: now.Add(resetsIn).UTC()}
	}
	account := func(id string, windows ...quota.Window) status.Account {
		return status.Account{ID: id, Label: id, TokenSet: true, FetchedAt: now.Add(-time.Minute).UTC(), Windows: windows}
	}
	limited := window("5h", "Session", 1, time.Hour)
	limited.Status = quota.StatusRejected
	doc := status.Document{
		GeneratedAt: now.UTC(), Source: status.SourceRouter, Router: status.Health{Healthy: true}, Best: "side", Primary: "work", Sessions: 3,
		Accounts: []status.Account{
			account("work", window("5h", "Session", 0.37, 2*time.Hour), window("7d", "Week", 0.4, 3*24*time.Hour)),
			account("personal", limited, window("7d", "Week", 0.89, 2*24*time.Hour)),
			account("side", window("5h", "Session", 0.12, 4*time.Hour), window("7d", "Week", 0.33, 5*24*time.Hour), window("7d_oi", "Fable week", 0.2, 5*24*time.Hour)),
		},
		Events: []status.Event{{ID: 1, At: now.Add(-time.Minute).UTC(), Kind: status.EventRoom, Account: "work"}},
	}
	doc.Accounts[0].Primary = true
	doc.Accounts[1].Limit = status.Limit{Windows: []string{"5h"}, Until: limited.ResetsAt}
	return doc
}

// frames are frames of threeAccounts the tests draw, as text alone: full
// screen, at a size each, and printed once, as usage prints it, without the
// tabs, at a width each.
func frames() map[string]dashboard.Frame {
	frame := func(width, height int) dashboard.Frame {
		return dashboard.Frame{
			Width: width, Height: height, Views: dashboard.Views(), View: dashboard.Accounts, Policy: claude,
			Keys: []dashboard.Key{{Key: "q", Does: "quit", Always: true}}, Status: "read 4s ago",
		}
	}
	printed := func(width int) dashboard.Frame {
		return dashboard.Frame{Width: width, View: dashboard.Accounts, Policy: claude}
	}
	return map[string]dashboard.Frame{
		"wide": frame(160, 40), "two rows": frame(120, 50), "short, scrolling": frame(160, 20), "phone": frame(52, 36),
		"printed": printed(160), "printed narrow": printed(80),
	}
}

// drawn is the frame of threeAccounts in the look given.
func drawn(f dashboard.Frame, look dashboard.Look) []string {
	f.Look = look
	return f.Draw(threeAccounts(), now)
}

func TestAFrameFullScreenPaintsTheCanvasOnEveryCell(t *testing.T) {
	nord := builtin(t, "nord")
	for name, f := range frames() {
		if f.Height == 0 {
			continue
		}
		t.Run(name, func(t *testing.T) {
			unpainted := trimmed(ansi.Strip(strings.Join(drawn(f, dashboard.Print(nord, nil)), "\n")))
			for i, row := range drawn(f, dashboard.Screen(nord)) {
				if got := ansi.StringWidth(row); got != f.Width {
					t.Errorf("row %d is %d cells wide, want the whole width, %d", i+1, got, f.Width)
				}
				if got, want := strings.TrimRight(ansi.Strip(row), " "), unpainted[i]; got != want {
					t.Errorf("row %d reads %q, want %q, as without its canvas", i+1, got, want)
				}
				for _, run := range offCanvas(row) {
					t.Errorf("row %d has %q off the canvas", i+1, run)
				}
			}
		})
	}
}

func TestAFramePrintedPaintsNoCanvas(t *testing.T) {
	look := dashboard.Print(builtin(t, "nord"), color.White)
	for _, name := range []string{"printed", "printed narrow"} {
		t.Run(name, func(t *testing.T) {
			for i, row := range drawn(frames()[name], look) {
				if strings.Contains(row, nordCanvas) {
					t.Errorf("row %d paints the canvas:\n%q\nwant the terminal's own background left as it is", i+1, row)
				}
				if text := ansi.Strip(row); strings.TrimRight(text, " ") != text {
					t.Errorf("row %d reads %q, want it to end at its last glyph", i+1, text)
				}
			}
		})
	}
}

func TestAFrameWithoutColourTellsStateByGlyphsAndBold(t *testing.T) {
	for name, f := range frames() {
		t.Run(name, func(t *testing.T) {
			plain := strings.Join(drawn(f, dashboard.Look{}), "\n")
			frame := strings.Join(drawn(f, dashboard.NoColour()), "\n")
			if stripped := ansi.Strip(frame); stripped != plain {
				t.Errorf("drawn without colour, stripped of its escapes =\n%s\nwant the frame as text\n%s", stripped, plain)
			}
			for _, sgr := range regexp.MustCompile(`\x1b\[[0-9;]*m`).FindAllString(frame, -1) {
				if sgr != "\x1b[1m" && sgr != "\x1b[m" && sgr != "\x1b[0m" {
					t.Errorf("drawn without colour, it has %q, want bold alone", sgr)
				}
			}
			if !strings.Contains(frame, "\x1b[1m") {
				t.Error("drawn without colour, it has no bold, want state told by bold where colour told it")
			}
		})
	}
}

func TestAFrameReadsTheSameInEveryBuiltInThatBlends(t *testing.T) {
	f := frames()["wide"]
	text := func(look dashboard.Look) string {
		return strings.Join(trimmed(ansi.Strip(strings.Join(drawn(f, look), "\n"))), "\n")
	}
	nord := text(dashboard.Screen(builtin(t, "nord")))
	for _, b := range theme.Builtins() {
		if b.Slug == theme.Terminal {
			continue
		}
		t.Run(b.Slug, func(t *testing.T) {
			if got := text(dashboard.Screen(b)); got != nord {
				t.Errorf("drawn in %s, stripped of its escapes =\n%s\nwant it as in nord\n%s", b.Slug, got, nord)
			}
		})
	}
}

func TestTheTerminalThemeDrawsInTheTerminalsOwnColoursAndShade(t *testing.T) {
	look := dashboard.Screen(builtin(t, theme.Terminal))
	if look.Canvas() != nil {
		t.Errorf("Canvas() = %v, want none painted", look.Canvas())
	}
	frame := strings.Join(drawn(frames()["wide"], look), "\n")
	if strings.Contains(frame, "38;2;") || strings.Contains(frame, "48;2;") {
		t.Errorf("drawn in the terminal's own colours, it has a colour of its own:\n%q", frame)
	}
	text := ansi.Strip(frame)
	for _, want := range []string{"Session  ██████▋▒▒▒┃▒░░░░░░  37% → 62%", "▒▒ heading"} {
		if !strings.Contains(text, want) {
			t.Errorf("drawn in the terminal's own colours\n%s\nwant %q, where it can't fade a bar's projection, in shade", text, want)
		}
	}
	for i, row := range strings.Split(text, "\n") {
		if strings.TrimRight(row, " ") != row {
			t.Errorf("row %d reads %q, want it to end at its last glyph, the terminal's background past it", i+1, row)
		}
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
	for name, f := range frames() {
		if frame := strings.Join(drawn(f, look), "\n"); strings.Contains(frame, "\x1b") {
			t.Errorf("%s, drawn in the zero look, has escapes:\n%q", name, frame)
		}
	}
}

func TestTextFromElsewhereShowsItsControlCharactersAsSpaces(t *testing.T) {
	doc := threeAccounts()
	doc.Accounts[0].Label = "Work\x1b[31m\tteam\n"
	doc.Accounts[2].Error = "HTTP 500 ·\r\nbad\x07 gateway"
	doc.Accounts[2].Windows = nil
	doc.Router = status.Health{Reason: "7 of\x1b[31m the 9\r\nfailed"}
	doc.Restart = status.Restart{Reason: "config\x1b[2J changed", Since: now.Add(-time.Hour).UTC()}
	doc.Events = append(doc.Events, status.Event{ID: 2, At: now.UTC(), Kind: status.EventMoved, Session: "5b19\x1b[1m", From: "work", To: "side", Reason: "moved: \x07pinned\n"})
	probed := threeAccounts()
	probed.Source, probed.Fallback = status.SourceProbe, status.Fallback{Router: status.RouterUnhealthy, Reason: "the router answered\x07\n404"}
	for name, doc := range map[string]status.Document{"from the router": doc, "probing past it": probed} {
		t.Run(name, func(t *testing.T) {
			for _, f := range []dashboard.Frame{frames()["wide"], frames()["printed"]} {
				frame := strings.Join(f.Draw(doc, now), "\n")
				if strings.ContainsAny(frame, "\x1b\t\r\a") {
					t.Errorf("drew %q, want no control characters from the document", frame)
				}
			}
		})
	}
	frame := strings.Join(frames()["wide"].Draw(doc, now), "\n")
	for _, want := range []string{"Work [31m team", "HTTP 500 · bad gateway", "7 of [31m the 9 failed"} {
		if !strings.Contains(frame, want) {
			t.Errorf("drew\n%s\nwant it to say %q, its control characters as spaces", frame, want)
		}
	}
}

// trimmed are the rows of a frame, each without the blanks at its end.
func trimmed(frame string) []string {
	rows := strings.Split(frame, "\n")
	for i, row := range rows {
		rows[i] = strings.TrimRight(row, " ")
	}
	return rows
}

// offCanvas are the runs of a row drawn on no background: text after an SGR
// that sets none, or before the first SGR.
func offCanvas(row string) []string {
	var runs []string
	sgr := regexp.MustCompile(`\x1b\[([0-9;]*)m([^\x1b]*)`)
	for _, m := range sgr.FindAllStringSubmatch(row, -1) {
		if m[2] != "" && !strings.Contains(m[1], "48;") {
			runs = append(runs, m[2])
		}
	}
	if first := strings.Index(row, "\x1b"); first != 0 {
		runs = append(runs, row[:max(first, 0)])
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
