package dashboard_test

import (
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

var update = flag.Bool("update", false, "rewrite the golden files with what the tests render")

// now is the clock every frame is drawn at: a Monday, 13:12 an hour east of UTC.
var now = time.Date(2026, 9, 28, 13, 12, 0, 0, time.FixedZone("UTC+1", 60*60))

// layout is a frame the tests draw: a document and how to draw it.
type layout struct {
	name string
	doc  status.Document
	opts dashboard.Options
}

func layouts() []layout {
	three, mixed := threeAccounts(), mixedAccounts()
	return []layout{
		{name: "three-wide", doc: three, opts: dashboard.Options{Width: 160}},
		{name: "three-two-columns", doc: three, opts: dashboard.Options{Width: 110}},
		{name: "three-one-column", doc: three, opts: dashboard.Options{Width: 80}},
		{name: "three-compact-from-width", doc: three, opts: dashboard.Options{Width: 44}},
		{name: "three-compact-from-height", doc: three, opts: dashboard.Options{Width: 160, Height: 12}},
		{name: "three-with-footer", doc: three, opts: dashboard.Options{Width: 160, Footer: "updated 13:12 · next 13:42 · r refresh · q quit"}},
		{name: "no-best", doc: noBest(), opts: dashboard.Options{Width: 110}},
		{name: "nothing-read", doc: nothingRead(), opts: dashboard.Options{Width: 110}},
		{name: "exhausted", doc: exhausted(), opts: dashboard.Options{Width: 80}},
		{name: "back-in-seconds", doc: backInSeconds(), opts: dashboard.Options{Width: 80}},
		{name: "failure", doc: failure(), opts: dashboard.Options{Width: 80}},
		{name: "errors", doc: mixed, opts: dashboard.Options{Width: 180}},
		{name: "errors-compact", doc: mixed, opts: dashboard.Options{Width: 100, Height: 10}},
		{name: "unknown-reset", doc: unknownReset(), opts: dashboard.Options{Width: 80}},
		{name: "long-labels", doc: longLabels(), opts: dashboard.Options{Width: 120}},
		{name: "router-pinned", doc: routed(), opts: dashboard.Options{Width: 160}},
		{name: "router-pinned-elsewhere", doc: pinnedElsewhere(), opts: dashboard.Options{Width: 110}},
		{name: "router-pinned-to-two", doc: pinnedToTwo(), opts: dashboard.Options{Width: 160}},
		{name: "router-pinned-to-two-compact", doc: pinnedToTwo(), opts: dashboard.Options{Width: 100, Height: 10}},
		{name: "router-automatic", doc: routedAutomatically(), opts: dashboard.Options{Width: 160}},
		{name: "router-unhealthy", doc: routerUnhealthy(), opts: dashboard.Options{Width: 160}},
		{name: "router-restart-due", doc: restartDue(), opts: dashboard.Options{Width: 160}},
		{name: "router-refused", doc: routedRefused(), opts: dashboard.Options{Width: 160}},
		{name: "router-compact", doc: pinnedElsewhere(), opts: dashboard.Options{Width: 100, Height: 10}},
		{name: "router-compact-narrow", doc: pinnedElsewhere(), opts: dashboard.Options{Width: 72, Height: 10}},
		{name: "router-not-running", doc: probedWithoutTheRouter(), opts: dashboard.Options{Width: 110}},
		{name: "router-not-answering", doc: probedPastAStuckRouter(), opts: dashboard.Options{Width: 110}},
		{name: "router-reserve", doc: atReserve(), opts: dashboard.Options{Width: 160}},
		{name: "router-reserve-pinned", doc: spendingReserve(), opts: dashboard.Options{Width: 160}},
		{name: "router-reserve-compact", doc: atReserve(), opts: dashboard.Options{Width: 130, Height: 10}},
		{name: "router-reserve-compact-narrow", doc: spendingReserve(), opts: dashboard.Options{Width: 100, Height: 10}},
		{name: "router-pressure", doc: underPressure(), opts: dashboard.Options{Width: 160}},
		{name: "router-pressure-compact", doc: underPressure(), opts: dashboard.Options{Width: 130, Height: 10}},
		{name: "router-started-again", doc: startedAgain(), opts: dashboard.Options{Width: 110}},
		{name: "router-lapsed", doc: lapsed(), opts: dashboard.Options{Width: 110}},
		{name: "router-lapsed-compact", doc: lapsed(), opts: dashboard.Options{Width: 100, Height: 8}},
		{name: "router-priming", doc: priming(), opts: dashboard.Options{Width: 110}},
		{name: "router-priming-compact", doc: priming(), opts: dashboard.Options{Width: 100, Height: 9}},
		{name: "probed-priming", doc: probedPriming(), opts: dashboard.Options{Width: 110}},
	}
}

func TestRenderGolden(t *testing.T) {
	for _, l := range layouts() {
		t.Run(l.name, func(t *testing.T) {
			got := dashboard.Render(l.doc, now, l.opts) + "\n"
			path := filepath.Join("testdata", l.name+".golden")
			if *update {
				if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden file (run with -update to create it): %v", err)
			}
			if got != string(want) {
				t.Errorf("Render() differs from %s; run with -update to accept it\ngot:\n%s\nwant:\n%s", path, got, want)
			}
		})
	}
}

func TestRenderFitsEveryWidth(t *testing.T) {
	for _, l := range layouts() {
		t.Run(l.name, func(t *testing.T) {
			t.Parallel()
			for width := 30; width <= 200; width++ {
				opts := l.opts
				opts.Width = width
				frame := dashboard.Render(l.doc, now, opts)
				for i, line := range strings.Split(frame, "\n") {
					if got := lipgloss.Width(line); got > width {
						t.Errorf("at width %d, line %d is %d cells wide:\n%s", width, i, got, line)
					}
					if strings.TrimRight(line, " ") != line {
						t.Errorf("at width %d, line %d ends in spaces: %q", width, i, line)
					}
				}
				checkCards(t, width, frame)
			}
		})
	}
}

func TestRenderInColor(t *testing.T) {
	for _, l := range layouts() {
		t.Run(l.name, func(t *testing.T) {
			plain := dashboard.Render(l.doc, now, l.opts)
			colored := l.opts
			colored.Color = true
			got := dashboard.Render(l.doc, now, colored)
			if !strings.Contains(got, "\x1b[38;2;") {
				t.Error("Render() with color has no color escapes")
			}
			if stripped := ansi.Strip(got); stripped != plain {
				t.Errorf("Render() with color, stripped of its escapes =\n%s\nwant the frame without color:\n%s", stripped, plain)
			}
		})
	}
}

func TestRenderWithoutColorHasNoEscapes(t *testing.T) {
	for _, l := range layouts() {
		if frame := dashboard.Render(l.doc, now, l.opts); strings.Contains(frame, "\x1b") {
			t.Errorf("%s: Render() without color has escape codes", l.name)
		}
	}
}

func TestRenderSurvivesTinyWidths(t *testing.T) {
	for _, l := range layouts() {
		t.Run(l.name, func(t *testing.T) {
			t.Parallel()
			for width := range 30 {
				opts := l.opts
				opts.Width = width
				for i, line := range strings.Split(dashboard.Render(l.doc, now, opts), "\n") {
					if got := lipgloss.Width(line); got > width {
						t.Errorf("at width %d, line %d is %d cells wide: %q", width, i, got, line)
					}
				}
			}
		})
	}
}

func TestRenderGivesWayToHeight(t *testing.T) {
	doc := threeAccounts()
	footer := "r refresh · q quit"
	cards := dashboard.Render(doc, now, dashboard.Options{Width: 150, Footer: footer})
	height := strings.Count(cards, "\n") + 1
	tests := []struct {
		name      string
		height    int
		wantCards bool
	}{
		{name: "unknown", height: 0, wantCards: true},
		{name: "as tall as the cards, footer and all", height: height, wantCards: true},
		{name: "a line short of them", height: height - 1, wantCards: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dashboard.Render(doc, now, dashboard.Options{Width: 150, Height: tt.height, Footer: footer})
			if hasCards := strings.Contains(got, "╭"); hasCards != tt.wantCards {
				t.Errorf("Render() at height %d =\n%s\nwant cards: %v", tt.height, got, tt.wantCards)
			}
		})
	}
}

func TestRenderWithoutAccounts(t *testing.T) {
	doc := status.Document{GeneratedAt: now.UTC(), Source: status.SourceProbe}

	got := dashboard.Render(doc, now, dashboard.Options{Width: 80})
	want := " Switchboard  Mon 28 Sep · 13:12"
	if got != want {
		t.Errorf("Render() =\n%q\nwant\n%q", got, want)
	}
}

func TestRenderCleansText(t *testing.T) {
	doc := status.Document{
		GeneratedAt: now.UTC(),
		Source:      status.SourceProbe,
		Accounts: []status.Account{
			{ID: "work", Label: "Work\x1b[31m\tteam\n", TokenSet: true, Error: "HTTP 500 ·\r\nbad\x07 gateway"},
		},
	}

	frame := dashboard.Render(doc, now, dashboard.Options{Width: 80})
	if strings.ContainsAny(frame, "\x1b\t\r\a") {
		t.Errorf("Render() = %q, want no control characters from the document", frame)
	}
	for _, want := range []string{"work · Work [31m team", "✗ HTTP 500 · bad gateway"} {
		if !strings.Contains(frame, want) {
			t.Errorf("Render() =\n%s\nwant it to contain %q", frame, want)
		}
	}
}

func TestRenderCleansWhatTheRouterSays(t *testing.T) {
	for _, doc := range []status.Document{
		{
			Source:   status.SourceRouter,
			Router:   status.Health{Healthy: true},
			Pin:      status.Pin{Accounts: []string{"work"}},
			Accounts: []status.Account{{ID: "work", Label: "Work\x1b[31m\tteam\n", TokenSet: true}},
		},
		{
			Source:   status.SourceRouter,
			Router:   status.Health{Reason: "7 of\x1b[31m the 9\r\nfailed"},
			Accounts: []status.Account{{ID: "work", Label: "Work", TokenSet: true}},
		},
		{
			Source:   status.SourceProbe,
			Fallback: status.Fallback{Router: status.RouterUnhealthy, Reason: "the router answered\x07\n404"},
			Accounts: []status.Account{{ID: "work", Label: "Work", TokenSet: true}},
		},
	} {
		frame := dashboard.Render(doc, now, dashboard.Options{Width: 150})
		if strings.ContainsAny(frame, "\x1b\t\r\a") {
			t.Errorf("Render() = %q, want no control characters from the document", frame)
		}
	}
}

func TestRenderShowsALimitWhileItHolds(t *testing.T) {
	tests := []struct {
		name  string
		until time.Time
		want  bool
	}{
		{name: "holding", until: now.Add(time.Minute), want: true},
		{name: "lifted", until: now, want: false},
		{name: "none", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := routed()
			doc.Accounts[2].Limit.Until = tt.until
			if shown := strings.Contains(dashboard.Render(doc, now, dashboard.Options{Width: 150}), "limit until"); shown != tt.want {
				t.Errorf("limit shown = %v, want %v", shown, tt.want)
			}
		})
	}
}

func TestRenderShowsARefusalWhileItHolds(t *testing.T) {
	tests := []struct {
		name  string
		until time.Time
		want  bool
	}{
		{name: "holding", until: now.Add(time.Minute), want: true},
		{name: "lifted", until: now, want: false},
		{name: "none", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := routed()
			doc.Accounts[1].Refused = status.Refusal{Until: tt.until, Status: 403, Family: "opus"}
			if shown := strings.Contains(dashboard.Render(doc, now, dashboard.Options{Width: 150}), "refused (403, opus) until"); shown != tt.want {
				t.Errorf("refusal shown = %v, want %v", shown, tt.want)
			}
		})
	}
}

func TestRenderShowsAReserveOnceAWindowReachesIt(t *testing.T) {
	short := atReserve()
	short.Accounts[0].AtReserve = nil
	tests := []struct {
		name string
		doc  status.Document
		// want is what the card says of its reserve, "" for nothing.
		want string
	}{
		{name: "holding its account back", doc: atReserve(), want: "at its reserve (90%)"},
		{name: "spent by the global pin", doc: spendingReserve(), want: "spending its reserve (pinned)"},
		{name: "short of it", doc: short},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, opts := range []dashboard.Options{{Width: 160}, {Width: 130, Height: 10}} {
				opts.Color = true
				frame := dashboard.Render(tt.doc, now, opts)
				shown := strings.Contains(ansi.Strip(frame), "reserve")
				if shown != (tt.want != "") {
					t.Errorf("at width %d, the reserve shown: %v, want %v", opts.Width, shown, tt.want != "")
				}
				if tt.want != "" && !strings.Contains(frame, lipgloss.NewStyle().Foreground(lipgloss.Color("#D08770")).Render(tt.want)) {
					t.Errorf("at width %d, the frame doesn't say %q in the warning colour:\n%s", opts.Width, tt.want, frame)
				}
			}
		})
	}
}

func TestRenderSaysWhyNoAccountIsTheOneToUseNext(t *testing.T) {
	tests := []struct {
		name string
		doc  status.Document
		// want is what the heading says, in the colour wantColor.
		want, wantColor string
	}{
		{name: "none has room", doc: noBest(), want: "no account has room right now", wantColor: "#BF616A"},
		{name: "nothing has been read of any", doc: nothingRead(), want: "nothing read yet", wantColor: "#616E88"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := dashboard.Render(tt.doc, now, dashboard.Options{Width: 110, Color: true})
			if !strings.Contains(frame, lipgloss.NewStyle().Foreground(lipgloss.Color(tt.wantColor)).Render(tt.want)) {
				t.Errorf("the frame doesn't say %q in %s:\n%s", tt.want, tt.wantColor, frame)
			}
		})
	}
}

func TestCountsSeconds(t *testing.T) {
	tests := []struct {
		name string
		doc  status.Document
		want bool
	}{
		{name: "an exhausted window back within ten minutes", doc: backInSeconds(), want: true},
		{name: "one back in ten minutes", doc: document("", read("1", "Work", windows(refused(session(1, 10*time.Minute))))), want: false},
		{name: "one back in hours", doc: exhausted(), want: false},
		{name: "one refused below its limit", doc: document("", read("1", "Work", windows(refused(session(0.8, 5*time.Minute))))), want: true},
		{name: "one used up but not refused", doc: document("", read("1", "Work", windows(session(1.02, 5*time.Minute)))), want: true},
		{name: "one that has reset since it was read", doc: document("", read("1", "Work", windows(refused(session(1, -time.Minute))))), want: false},
		{name: "a window with room that resets within ten minutes", doc: document("1", read("1", "Work", windows(session(0.4, 5*time.Minute)))), want: false},
		{name: "an account that couldn't be read", doc: mixedAccounts(), want: false},
		{name: "no accounts", doc: document(""), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dashboard.CountsSeconds(tt.doc, now); got != tt.want {
				t.Errorf("CountsSeconds() = %v, want %v", got, tt.want)
			}
		})
	}
}

// checkCards checks that each row's cards start and end on the same lines,
// so they're the same height, and that their borders line up.
func checkCards(t *testing.T, width int, frame string) {
	t.Helper()
	lines := strings.Split(frame, "\n")
	for i := 0; i < len(lines); i++ {
		lefts := columns(lines[i], '╭')
		if len(lefts) == 0 {
			continue
		}
		rights := columns(lines[i], '╮')
		edges := slices.Sorted(slices.Values(slices.Concat(lefts, rights)))
		j := i + 1
		for ; j < len(lines) && !strings.Contains(lines[j], "╰"); j++ {
			if got := columns(lines[j], '│'); !slices.Equal(got, edges) {
				t.Errorf("at width %d, line %d has side borders at %v, want %v:\n%s", width, j, got, edges, frame)
				return
			}
		}
		if j == len(lines) {
			t.Errorf("at width %d, cards opened on line %d never close:\n%s", width, i, frame)
			return
		}
		if got := columns(lines[j], '╰'); !slices.Equal(got, lefts) {
			t.Errorf("at width %d, cards opened at %v on line %d close at %v on line %d:\n%s", width, lefts, i, got, j, frame)
		}
		if got := columns(lines[j], '╯'); !slices.Equal(got, rights) {
			t.Errorf("at width %d, cards opened at %v on line %d close at %v on line %d:\n%s", width, rights, i, got, j, frame)
		}
		i = j
	}
}

// columns lists the cells of line where glyph is drawn.
func columns(line string, glyph rune) []int {
	var cells []int
	cell := 0
	for i, r := range line {
		if r == glyph {
			cells = append(cells, cell)
		}
		cell += ansi.StringWidth(line[i : i+utf8.RuneLen(r)])
	}
	return cells
}

// window is a window resetting a duration from now.
func window(key, label string, utilization float64, resetsIn time.Duration) quota.Window {
	return quota.Window{Key: key, Label: label, Utilization: utilization, ResetsAt: now.Add(resetsIn).UTC()}
}

func session(utilization float64, resetsIn time.Duration) quota.Window {
	return window("5h", "Session", utilization, resetsIn)
}

func week(utilization float64, resetsIn time.Duration) quota.Window {
	return window("7d", "Week", utilization, resetsIn)
}

func fableWeek(utilization float64, resetsIn time.Duration) quota.Window {
	return window("7d_oi", "Fable week", utilization, resetsIn)
}

func refused(w quota.Window) quota.Window {
	w.Status = quota.StatusRejected
	return w
}

// read is an account whose usage was read now.
func read(id, label string, usage quota.Usage) status.Account {
	return status.Account{ID: id, Label: label, TokenSet: true, FetchedAt: now.UTC(), Usage: usage}
}

func windows(ws ...quota.Window) quota.Usage {
	return quota.Usage{Windows: ws}
}

func document(best string, accounts ...status.Account) status.Document {
	return status.Document{GeneratedAt: now.UTC(), Source: status.SourceProbe, Best: best, Accounts: accounts}
}

const (
	hour = time.Hour
	day  = 24 * time.Hour
)

// threeAccounts are windows heading every way: work, the primary, keeping
// pace, personal running out of its week, and side out of its session.
func threeAccounts() status.Document {
	doc := document("2",
		read("1", "Work", windows(
			session(0.37, 5*time.Minute),
			week(0.96, 2*hour+55*time.Minute),
			fableWeek(0.15, 4*day),
		)),
		read("2", "Personal", windows(
			session(0.05, 15*time.Minute),
			week(0.28, 6*day),
		)),
		read("3", "Side", windows(
			refused(session(1, hour+20*time.Minute)),
			week(0.64, 3*day+4*hour),
			fableWeek(0.41, 3*day+4*hour),
		)),
	)
	doc.Primary, doc.Accounts[0].Primary = "1", true
	return doc
}

// noBest has no account with room in every window every model shares.
func noBest() status.Document {
	return document("",
		read("1", "Work", windows(
			session(0.42, 2*hour),
			refused(week(1, 2*day+3*hour)),
		)),
		read("2", "Personal", windows(
			refused(session(1, 38*time.Minute)),
			week(0.91, day),
		)),
	)
}

// nothingRead is the router's document as it starts, before anything has been
// read of either account, so none can be named the one to use next.
func nothingRead() status.Document {
	doc := document("",
		status.Account{ID: "1", Label: "Work", TokenSet: true},
		status.Account{ID: "2", Label: "Side", TokenSet: true},
	)
	doc.Source = status.SourceRouter
	doc.Router = status.Health{Healthy: true}
	return doc
}

// exhausted has a session that's back soon, and a Fable week over its limit
// whose reset is unknown.
func exhausted() status.Document {
	fable := quota.Window{Key: "7d_oi", Label: "Fable week", Utilization: 1.04}
	return document("",
		read("1", "Work", windows(
			refused(session(1, hour+5*time.Minute)),
			week(0.55, 2*day),
			fable,
		)),
	)
}

// backInSeconds has a session back within ten minutes, so it counts down to
// the second.
func backInSeconds() status.Document {
	return document("",
		read("1", "Work", windows(
			refused(session(1, 7*time.Minute+42*time.Second)),
			week(0.55, 2*day),
		)),
	)
}

// failure couldn't read the Fable week, for a reason too long for two lines.
func failure() status.Document {
	return document("1",
		read("1", "Work", quota.Usage{
			Windows: []quota.Window{session(0.23, 4*hour), week(0.61, 3*day)},
			Failures: []quota.Failure{{
				Label:  "Fable",
				Window: "7d_oi",
				Error:  "HTTP 529 · Overloaded: the model is temporarily unable to serve this account, so retry after the cooldown has passed",
			}},
		}),
	)
}

// mixedAccounts has one account read but for its Fable week, one without a
// token and one whose token was refused, at length.
func mixedAccounts() status.Document {
	return document("work",
		read("work", "Work", quota.Usage{
			Windows:  []quota.Window{session(0.12, 3*hour), week(0.33, 5*day)},
			Failures: []quota.Failure{{Label: "Fable", Window: "7d_oi", Error: "HTTP 529 · Overloaded"}},
		}),
		status.Account{ID: "personal", Label: "Personal", Error: "token missing: write it to /Users/tester/.local/state/switchboard/tokens/personal"},
		status.Account{
			ID: "side", Label: "Side", TokenSet: true,
			Error: "HTTP 401 · Invalid bearer token: the token has expired or been revoked, so create a new one with claude setup-token and set it again",
		},
	)
}

// unknownReset has a window whose reset wasn't given, and one whose length
// can't be read from its key.
func unknownReset() status.Document {
	return document("1",
		read("1", "Work", windows(
			quota.Window{Key: "5h", Label: "Session", Utilization: 0.23},
			week(0.4, 4*day),
			window("burst", "burst", 0.62, 42*time.Minute),
		)),
	)
}

// routed is the router's document of threeAccounts: a session on work and
// two on personal, the best, which the router pins new sessions to, and side
// held back by the limit it reached until its session resets.
func routed() status.Document {
	doc := threeAccounts()
	doc.Source = status.SourceRouter
	doc.Router = status.Health{Healthy: true, Requests: 42}
	doc.Pin = status.Pin{Accounts: []string{"2"}, Since: now.Add(-hour).UTC()}
	doc.Sessions = 3
	doc.Accounts[0].Sessions = 1
	doc.Accounts[1].Sessions = 2
	doc.Accounts[2].Limit = status.Limit{Windows: []string{"5h"}, Until: now.Add(hour + 20*time.Minute).UTC()}
	return doc
}

// routedRefused is routed with work's token refused, and side, held back by
// its limit, refused its Opus requests too.
func routedRefused() status.Document {
	doc := routed()
	doc.Accounts[0].Refused = status.Refusal{Until: now.Add(10 * time.Minute).UTC(), Status: 401}
	doc.Accounts[2].Refused = status.Refusal{Until: now.Add(8 * time.Minute).UTC(), Status: 403, Family: "opus"}
	return doc
}

// pinnedElsewhere is routed, pinned to work rather than the best.
func pinnedElsewhere() status.Document {
	doc := routed()
	doc.Pin.Accounts = []string{"1"}
	return doc
}

// pinnedToTwo is routed, pinned to work and personal, the best.
func pinnedToTwo() status.Document {
	doc := routed()
	doc.Pin.Accounts = []string{"1", "2"}
	return doc
}

// routedAutomatically is routed without a pin.
func routedAutomatically() status.Document {
	doc := routed()
	doc.Pin = status.Pin{}
	return doc
}

// atReserve is routed with work, the primary, keeping a tenth of every
// window back: its week has reached its reserve.
func atReserve() status.Document {
	doc := routed()
	doc.Accounts[0].Reserve = 0.1
	doc.Accounts[0].AtReserve = []string{"7d"}
	return doc
}

// spendingReserve is atReserve pinned to work, whose reserve the pin spends.
func spendingReserve() status.Document {
	doc := atReserve()
	doc.Pin.Accounts = []string{"1"}
	return doc
}

// lapsed is the router's document of two accounts: work, the primary, whose
// session runs, and side, whose session has lapsed, its reset passed with
// nothing read of side since, so it reads empty, its week standing as read.
func lapsed() status.Document {
	doc := document("1",
		read("1", "Work", windows(session(0.37, 2*hour), week(0.4, 3*day))),
		read("2", "Side", windows(quota.Window{Key: "5h", Label: "Session"}, week(0.64, 3*day+4*hour))),
	)
	doc.Source = status.SourceRouter
	doc.Router = status.Health{Healthy: true, Requests: 12}
	doc.Primary, doc.Accounts[0].Primary, doc.Accounts[0].Reserve = "1", true, 0.1
	doc.Accounts[1].FetchedAt = now.Add(-6 * hour).UTC()
	doc.Accounts[1].Lapsed = []string{"5h"}
	return doc
}

// underPressure is the router's document of three accounts, each read over
// the last half hour: work, the primary, keeping a tenth of every window back,
// at 40% of its session an hour reaches its reserve before its session
// resets, and personal, at 50% an hour, runs out before its does, so both are
// under pressure; side, at 5% an hour, isn't, and is the best. Personal's
// week, at 3% an hour over the 18 minutes it has levels for, runs out sooner
// than its use since it started says; work's, at a tenth of a percent, later.
func underPressure() status.Document {
	doc := document("3",
		read("1", "Work", windows(session(0.55, 4*hour), week(0.4, 3*day))),
		read("2", "Personal", windows(session(0.3, 2*hour), week(0.5, 2*day))),
		read("3", "Side", windows(session(0.1, 4*hour), week(0.3, 5*day))),
	)
	doc.Source = status.SourceRouter
	doc.Router = status.Health{Healthy: true, Requests: 42}
	doc.Primary, doc.Accounts[0].Primary, doc.Accounts[0].Reserve = "1", true, 0.1
	doc.Accounts[0].Pressure = status.Pressure{Window: "5h", Rate: 0.4, Recent: true, RunsOut: now.Add(52*time.Minute + 30*time.Second).UTC(), Under: true}
	doc.Accounts[0].Rates = []status.Rate{{Window: "5h", Rate: 0.4}, {Window: "7d", Rate: 0.001}}
	doc.Accounts[1].Pressure = status.Pressure{Window: "5h", Rate: 0.5, Recent: true, RunsOut: now.Add(hour + 24*time.Minute).UTC(), Under: true}
	doc.Accounts[1].Rates = []status.Rate{{Window: "5h", Rate: 0.5}, {Window: "7d", Rate: 0.03, Since: now.Add(-18 * time.Minute).UTC()}}
	doc.Accounts[2].Pressure = status.Pressure{Window: "5h", Rate: 0.05, Recent: true, RunsOut: now.Add(18 * hour).UTC()}
	doc.Accounts[2].Rates = []status.Rate{{Window: "5h", Rate: 0.05}}
	return doc
}

// startedAgain is the router's document of two accounts whose weeks began
// three days ago and reset in four, each 4% used: work's was reset by hand
// six hours ago, keeping its reset, and runs from then; side's runs from a
// week before its reset.
func startedAgain() status.Document {
	doc := document("1",
		read("1", "Work", windows(session(0.2, 3*hour), week(0.04, 4*day))),
		read("2", "Side", windows(session(0.2, 3*hour), week(0.04, 4*day))),
	)
	doc.Source = status.SourceRouter
	doc.Router = status.Health{Healthy: true, Requests: 42}
	doc.Accounts[0].Windows[1].RestartedAt = now.Add(-6 * hour).UTC()
	return doc
}

// priming is lapsed, primed on an 08:00-23:00 day: work next as its session
// resets, and side, whose last prime failed, five minutes on.
func priming() status.Document {
	doc := lapsed()
	doc.Prime = status.Prime{Day: "08:00-23:00", Window: "5h", Slots: []status.Slot{
		{Account: "1", At: "04:10", Next: now.Add(2 * hour).UTC()},
		{Account: "2", At: "06:40", Next: now.Add(5 * time.Minute).UTC()},
	}}
	return doc
}

// probedPriming is threeAccounts, probed on a day of priming: the router
// isn't running to say when it next primes each account.
func probedPriming() status.Document {
	doc := probedWithoutTheRouter()
	doc.Prime = status.Prime{Day: "08:00-23:00", Window: "5h", Slots: []status.Slot{{Account: "1", At: "03:50"}, {Account: "2", At: "05:30"}, {Account: "3", At: "07:10"}}}
	return doc
}

// routerUnhealthy is routed by a router failing most of what it routes.
func routerUnhealthy() status.Document {
	doc := routed()
	doc.Router = status.Health{Requests: 9, Failures: 7, Reason: "7 of the 9 requests in the last 5 minutes failed"}
	return doc
}

// restartDue is routedAutomatically by a router with a restart due.
func restartDue() status.Document {
	doc := routedAutomatically()
	doc.Restart = status.Restart{Reason: "upgraded", Since: now.Add(-time.Hour), InFlight: 3}
	return doc
}

// probedWithoutTheRouter is threeAccounts, probed as the router isn't running.
func probedWithoutTheRouter() status.Document {
	doc := threeAccounts()
	doc.Fallback = status.Fallback{Router: status.RouterNotRunning}
	return doc
}

// probedPastAStuckRouter is threeAccounts, probed as the router didn't answer.
func probedPastAStuckRouter() status.Document {
	doc := threeAccounts()
	doc.Fallback = status.Fallback{Router: status.RouterUnhealthy, Reason: "no answer within 500ms"}
	return doc
}

// longLabels has an account and a window whose labels are too long to show whole.
func longLabels() status.Document {
	return document("2",
		read("1", "Work", windows(session(0.3, 2*hour), week(0.5, 3*day))),
		read("2", "Research and development sandbox for the platform team", windows(
			session(0.1, hour),
			window("7d_preview", "Weekly cap on the experimental research preview models", 0.72, 2*day),
		)),
	)
}
