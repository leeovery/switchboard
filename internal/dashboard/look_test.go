package dashboard_test

import (
	"fmt"
	"image/color"
	"regexp"
	"slices"
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

// now is the clock every frame is drawn at: a Monday, 13:12 an hour east of
// UTC.
var now = time.Date(2026, 9, 28, 13, 12, 0, 0, time.FixedZone("UTC+1", 60*60))

// claude judges windows as Claude's are judged.
var claude = score.Policy{Shared: []string{"5h", "7d"}, Perishable: "7d", Tiebreak: "5h", Started: "5h", Pressure: "5h"}

// threeAccounts is the router's document of three accounts, their windows
// heading every way: work, the primary, keeping pace; personal held back by
// the limit its session reached; and side, where new sessions go, its Fable
// week used. It has what's worked out of it, as the dashboard is given it.
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
	return doc.WorkedOut(claude)
}

// frames are frames of threeAccounts the tests draw, as text alone: full
// screen, at a size each, Sessions, and Runway over the day and the week,
// among them.
func frames() map[string]dashboard.Frame {
	frame := func(width, height int) dashboard.Frame {
		return dashboard.Frame{
			Width: width, Height: height, View: dashboard.Overview, Policy: claude,
			KeyLine: dashboard.KeyLine{Groups: [][]dashboard.Key{{{Key: "q", Does: "quit"}}}},
		}
	}
	runway := func(span dashboard.Span) dashboard.Frame {
		f := frame(160, 26)
		f.View, f.Span = dashboard.Runway, span
		return f
	}
	sessions := frame(160, 40)
	sessions.View = dashboard.Sessions
	return map[string]dashboard.Frame{
		"wide": frame(160, 40), "two rows": frame(120, 50), "short, scrolling": frame(160, 20), "phone": frame(52, 36),
		"sessions": sessions, "runway, the day": runway(dashboard.Day), "runway, the week": runway(dashboard.Week),
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

func TestAFrameWithoutColourTellsStateByGlyphsBoldAndReverse(t *testing.T) {
	for name, f := range frames() {
		t.Run(name, func(t *testing.T) {
			plain := strings.Join(drawn(f, dashboard.Look{}), "\n")
			frame := strings.Join(drawn(f, dashboard.NoColour()), "\n")
			if stripped := ansi.Strip(frame); stripped != plain {
				t.Errorf("drawn without colour, stripped of its escapes =\n%s\nwant the frame as text\n%s", stripped, plain)
			}
			for _, sgr := range regexp.MustCompile(`\x1b\[[0-9;]*m`).FindAllString(frame, -1) {
				if !slices.Contains([]string{"\x1b[1m", "\x1b[7m", "\x1b[1;7m", "\x1b[m", "\x1b[0m"}, sgr) {
					t.Errorf("drawn without colour, it has %q, want bold and a selected row's reverse alone", sgr)
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
	frame := strings.Join(drawn(frames()["sessions"], look), "\n")
	if strings.Contains(frame, "38;2;") || strings.Contains(frame, "48;2;") {
		t.Errorf("drawn in the terminal's own colours, it has a colour of its own:\n%q", frame)
	}
	text := ansi.Strip(frame)
	for _, want := range []string{"Session  ████████▉▒▒▒▒▒┃░░░░░░░░░  37%  → 62%", "Week     █████████▋▒▒▒┃▒▒▒░░░░░░░  40%  → 70%"} {
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

func TestTheTerminalThemeDrawsWhatsDimFaintAndWhatsPickedOutReversed(t *testing.T) {
	f := frames()["sessions"]
	f.Fresh = map[int]float64{1: 0.2}
	rows := drawn(f, dashboard.Screen(builtin(t, theme.Terminal)))

	for i, row := range rows {
		for _, params := range sgrs.FindAllStringSubmatch(row, -1) {
			for p := range strings.SplitSeq(params[1], ";") {
				if p == "90" || p == "100" {
					t.Fatalf("row %d is drawn in the terminal's bright black, Solarized Dark's background: %q", i+1, row)
				}
			}
		}
	}
	for _, tt := range []struct {
		name, text, want string
	}{
		{name: "a panel's border, faint", text: "┌─ ", want: "2"},
		{name: "the tab shown, reversed", text: "Sessions", want: "7"},
		{name: "an event looked at newly, picked out reversed", text: "has room again", want: "7"},
	} {
		if params := drawnWith(rows, tt.text); !slices.Contains(params, tt.want) {
			t.Errorf("%s: %q is drawn with SGR %q, want %s among them", tt.name, tt.text, params, tt.want)
		}
	}
}

// sgrs are the SGR sequences in a row, their parameters captured.
var sgrs = regexp.MustCompile(`\x1b\[([0-9;]*)m`)

// drawnWith is the SGR parameters text is drawn with where it first shows in
// rows: those of the last SGR before it.
func drawnWith(rows []string, text string) []string {
	for _, row := range rows {
		before, _, found := strings.Cut(row, text)
		if !found {
			continue
		}
		set := sgrs.FindAllStringSubmatch(before, -1)
		if len(set) == 0 {
			return nil
		}
		return strings.Split(set[len(set)-1][1], ";")
	}
	return nil
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
	views := []dashboard.Frame{frames()["wide"], frames()["sessions"], frames()["runway, the day"]}
	for name, doc := range map[string]status.Document{"from the router": doc, "probing past it": probed} {
		t.Run(name, func(t *testing.T) {
			for _, f := range views {
				if frame := strings.Join(f.Draw(doc, now), "\n"); strings.ContainsAny(frame, "\x1b\t\r\a") {
					t.Errorf("drew %q, want no control characters from the document", frame)
				}
			}
		})
	}
	var frame string
	for _, f := range views {
		frame += strings.Join(f.Draw(doc, now), "\n") + "\n"
	}
	for _, want := range []string{"Work [31m team", "HTTP 500 · bad gateway", "config [2J changed"} {
		if !strings.Contains(frame, want) {
			t.Errorf("drew\n%s\nwant it to say %q, its control characters as spaces", frame, want)
		}
	}
}

func TestTextFromElsewherePrintsItsControlCharactersAsSpaces(t *testing.T) {
	doc := threeAccounts()
	doc.Accounts[0].Label = "Work\x1b[31m\ateam"
	doc.Accounts[2].Windows[2].Label = "Fable\x1b\aOI"
	doc.Router = status.Health{Reason: "7 of\x1b[31m the 9\afailed"}
	doc.Restart = status.Restart{Reason: "config\x1b[2J\achanged", Since: now.Add(-time.Hour).UTC()}
	doc.Pin = status.Pin{Accounts: []string{"work", "gone\x1b[1m\aaway"}}
	probed := threeAccounts()
	probed.Accounts[1].Label = "Personal\x1b[31m\aplan"
	probed.Source, probed.Fallback = status.SourceProbe, status.Fallback{Router: status.RouterUnhealthy, Reason: "the router\x1b[31m\aanswered 404"}
	tests := []struct {
		name string
		doc  status.Document
		// want is in lower case, as by window the windows' names are in capitals.
		want []string
	}{
		{name: "from the router", doc: doc, want: []string{"work [31m team", "fable oi", "config [2j changed", "gone [1m away"}},
		{name: "probing past it", doc: probed, want: []string{"personal [31m plan", "the router [31m answered 404"}},
	}
	for _, tt := range tests {
		for _, p := range []dashboard.Printout{{Width: 160}, {Width: 160, By: dashboard.ByAccount}, {Width: 80}} {
			t.Run(fmt.Sprintf("%s, %d columns, by %d", tt.name, p.Width, p.By), func(t *testing.T) {
				printed := strings.Join(p.Draw(tt.doc, now), "\n")
				if strings.ContainsAny(printed, "\x1b\a") {
					t.Errorf("printed %q, want no control characters from the document", printed)
				}
				if p.Width < 160 {
					return
				}
				for _, want := range tt.want {
					if !strings.Contains(strings.ToLower(printed), want) {
						t.Errorf("printed\n%s\nwant it to say %q, its control characters as spaces", printed, want)
					}
				}
			})
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
