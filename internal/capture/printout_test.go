package capture

import (
	"context"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

func TestAPrintoutIsWhatThePathDrawsForItsRouter(t *testing.T) {
	f := printoutOf(t)
	for _, width := range []int{160, 120, 80} {
		rows := dashboard.Printout{Width: width, Look: dashboard.Print(f.theme, nil), Policy: claude.Policy}.Draw(f.router.snapshot(), f.now)

		if got, want := f.Frame(watch.Size{Width: width, Height: f.Size.Height}), strings.Join(rows, "\n")+"\n"; got != want {
			t.Errorf("at %d columns, the printout reads\n%s\nwant what the path draws\n%s", width, got, want)
		}
	}
}

func TestAPrintoutIsAsTallAsItIsAndPaintsNoBackground(t *testing.T) {
	f := printoutOf(t)
	screen := f
	screen.Printout = false
	if !strings.Contains(screen.Frame(screen.Size), nordCanvas) {
		t.Fatalf("the watch's frame of the same fixture paints no %s either: the check below reads nothing", nordCanvas)
	}
	for _, size := range []watch.Size{{Width: 160, Height: 10}, {Width: 160, Height: 80}} {
		frame := f.Frame(size)
		lines := strings.Split(strings.TrimSuffix(frame, "\n"), "\n")
		if want := f.Frame(watch.Size{Width: 160}); frame != want {
			t.Errorf("on a terminal %d tall, the printout is %d lines, want as many as it has, whatever the height", size.Height, len(lines))
		}
		if !strings.Contains(frame, "38;2;") {
			t.Errorf("the printout\n%q\nwant it in its theme's colours", frame)
		}
		for i, line := range lines {
			if strings.Contains(line, nordCanvas) {
				t.Errorf("line %d paints the canvas, want the terminal's own background left as it is:\n%q", i+1, line)
			}
			if text := ansi.Strip(line); strings.TrimRight(text, " ") != text {
				t.Errorf("line %d reads %q, want it to end at its last glyph, as nothing fills the row", i+1, text)
			}
		}
	}
}

func TestAPrintoutWithoutColourIsBoldAlone(t *testing.T) {
	frame := printoutOf(t).WithoutColour().Frame(watch.Size{Width: 160})

	if !strings.Contains(frame, "\x1b[1m") {
		t.Errorf("without colour, the printout\n%q\nwant its names and percentages bold", frame)
	}
	for _, m := range sgrs.FindAllStringSubmatch(frame, -1) {
		if m[1] != "1" && m[1] != "" && m[1] != "0" {
			t.Errorf("without colour, the printout has %q, want bold alone", m[0])
		}
	}
}

func TestAPrintoutReadsItsRouterDirectly(t *testing.T) {
	now := moment(time.UTC)
	router := threeAccounts(now)
	snap := router.snapshot()

	doc, _, _ := router.Read(context.Background(), status.Read{})
	sessions, _ := router.Sessions(context.Background())
	if snap.Doc.GeneratedAt != doc.GeneratedAt || len(snap.Doc.Accounts) != len(doc.Accounts) || snap.Doc.Accounts[0].Pressure != doc.Accounts[0].Pressure {
		t.Errorf("read the document %+v, want what the router gives, %+v", snap.Doc, doc)
	}
	if !slices.EqualFunc(snap.Sessions, sessions, func(a, b status.Session) bool { return a.ID == b.ID }) {
		t.Errorf("read the sessions %+v, want those the router lists, %+v", snap.Sessions, sessions)
	}
	if snap.Outdated {
		t.Error("read the router as outdated, want it giving its history")
	}
	if len(snap.History) != len(router.trails) {
		t.Errorf("read the history of %d windows, want the router's %d", len(snap.History), len(router.trails))
	}
	for _, tr := range router.trails {
		got := snap.History[dashboard.Ref{Account: tr.account, Window: tr.window}]
		if !got.Start.Equal(tr.start) || len(got.Readings) != len(tr.readings) {
			t.Errorf("%s's %s reads from %s, %d readings; want from %s, %d", tr.account, tr.window, got.Start, len(got.Readings), tr.start, len(tr.readings))
			continue
		}
		for i, r := range tr.readings {
			if !got.Readings[i].At.Equal(r.At) || got.Readings[i].Utilization != r.Utilization {
				t.Errorf("%s's %s reading %d is %+v, want %+v", tr.account, tr.window, i, got.Readings[i], r)
			}
		}
	}
}

// nordCanvas is Nord's canvas as a background's SGR parameters.
const nordCanvas = "48;2;46;52;64"

// sgrs are the SGR sequences in text, each one's parameters captured.
var sgrs = regexp.MustCompile(`\x1b\[([0-9;]*)m`)

// printoutOf is a printout of usage over milestone 5's three accounts, in the
// frames' theme, at the frames' moment, in UTC.
func printoutOf(t *testing.T) Fixture {
	t.Helper()
	now := moment(time.UTC)
	nord, ok := theme.Builtin(theme.DefaultDark)
	if !ok {
		t.Fatal("no built-in nord")
	}
	return Fixture{Name: "usage-3", Size: wide(34), Printout: true, now: now, router: threeAccounts(now), theme: nord}
}
