package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/status/statustest"
)

// routed is the healthy router's document of work, personal and side, each
// read, with six sessions, new ones going to side, as change leaves it.
func routed(change ...func(*status.Document)) status.Document {
	read := printNow.Add(-4 * time.Second).UTC()
	doc := status.Document{
		GeneratedAt: read, Source: status.SourceRouter, Best: "side", Router: status.Health{Healthy: true}, Sessions: 6,
		Accounts: []status.Account{
			{ID: "work", Label: "work", TokenSet: true, FetchedAt: read},
			{ID: "personal", Label: "personal", TokenSet: true, FetchedAt: read},
			{ID: "side", Label: "side", TokenSet: true, FetchedAt: read},
		},
	}
	for _, c := range change {
		c(&doc)
	}
	return doc
}

func TestTheStatusLineSaysHowTheRouterStands(t *testing.T) {
	probedAs := func(fallback status.Fallback) func(*status.Document) {
		return func(d *status.Document) {
			d.Source, d.Fallback, d.Router, d.Sessions = status.SourceProbe, fallback, status.Health{}, 0
		}
	}
	tests := []struct {
		name string
		doc  status.Document
		want string
	}{
		{name: "healthy, on auto", doc: routed(), want: "● healthy  ·  6 sessions  ·  new sessions → ▲ side"},
		{
			name: "unhealthy",
			doc:  routed(func(d *status.Document) { d.Router = status.Health{Reason: "failed 6 of 8 requests"} }),
			want: "● unhealthy  ·  6 sessions  ·  new sessions → ▲ side",
		},
		{
			name: "a restart due",
			doc:  routed(func(d *status.Document) { d.Restart = status.Restart{Reason: status.RestartForZone} }),
			want: "● healthy  ·  restart due (time zone changed)  ·  6 sessions  ·  new sessions → ▲ side",
		},
		{
			name: "pinned to two, in the config's order",
			doc:  routed(func(d *status.Document) { d.Pin = status.Pin{Accounts: []string{"personal", "side"}} }),
			want: "● healthy  ·  pinned to personal and side  ·  6 sessions  ·  new sessions → ▲ side",
		},
		{
			name: "pinned to three",
			doc:  routed(func(d *status.Document) { d.Pin = status.Pin{Accounts: []string{"work", "personal", "side"}} }),
			want: "● healthy  ·  pinned to work, personal and side  ·  6 sessions  ·  new sessions → ▲ side",
		},
		{name: "one session", doc: routed(func(d *status.Document) { d.Sessions = 1 }), want: "● healthy  ·  1 session  ·  new sessions → ▲ side"},
		{name: "no sessions", doc: routed(func(d *status.Document) { d.Sessions = 0 }), want: "● healthy  ·  no sessions  ·  new sessions → ▲ side"},
		{
			name: "no account with room",
			doc:  routed(func(d *status.Document) { d.Best = "" }),
			want: "● healthy  ·  6 sessions  ·  new sessions → no account has room",
		},
		{
			name: "nothing read yet",
			doc: routed(func(d *status.Document) {
				for i := range d.Accounts {
					d.Accounts[i].FetchedAt = time.Time{}
				}
				d.Best = ""
			}),
			want: "● healthy  ·  6 sessions  ·  new sessions → nothing read yet",
		},
		{
			name: "probed, the router not running",
			doc:  routed(probedAs(status.Fallback{Router: status.RouterNotRunning})),
			want: "○ probed · router not running",
		},
		{
			name: "probed, something answering not as a router does",
			doc:  routed(probedAs(status.Fallback{Router: status.RouterUnhealthy, Reason: "no answer within 500ms"})),
			want: "○ probed · router unhealthy: no answer within 500ms",
		},
		{name: "probed, as asked", doc: routed(probedAs(status.Fallback{})), want: "○ probed, as asked"},
		{
			name: "one account with room",
			doc:  routed(func(d *status.Document) { d.Accounts, d.Best = d.Accounts[:1], "work" }),
			want: "● healthy  ·  6 sessions  ·  new sessions → ▲ work",
		},
		{
			name: "one account without room, named all the same",
			doc:  routed(func(d *status.Document) { d.Accounts, d.Best = d.Accounts[:1], "" }),
			want: "● healthy  ·  6 sessions  ·  new sessions → ▲ work",
		},
		{
			name: "one account not read yet, named all the same",
			doc: routed(func(d *status.Document) {
				d.Accounts, d.Best = d.Accounts[:1], ""
				d.Accounts[0].FetchedAt = time.Time{}
			}),
			want: "● healthy  ·  6 sessions  ·  new sessions → ▲ work",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := newStatusLine(tt.doc.At(printNow, printPolicy)).fit(200).plain(); got != tt.want {
				t.Errorf("the status line reads %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTheStatusLineSaysItInItsInks(t *testing.T) {
	inkOf := func(l line, text string) (ink, bool) {
		for _, s := range l {
			if s.text == text {
				return s.ink, true
			}
		}
		return ink{}, false
	}
	tests := []struct {
		name, text string
		doc        status.Document
		want       ink
	}{
		{name: "healthy", text: "healthy", doc: routed(), want: secondaryInk},
		{name: "unhealthy", text: "● unhealthy", doc: routed(func(d *status.Document) { d.Router.Healthy = false }), want: errorInk},
		{name: "a separator", text: "  ·  ", doc: routed(), want: mutedInk},
		{name: "the sessions", text: "6 sessions", doc: routed(), want: mutedInk},
		{name: "where new sessions go", text: "▲ side", doc: routed(), want: nameInk},
		{name: "none with room", text: "no account has room", doc: routed(func(d *status.Document) { d.Best = "" }), want: exhaustedInk},
		{
			name: "a pin", text: "pinned to personal",
			doc: routed(func(d *status.Document) { d.Pin = status.Pin{Accounts: []string{"personal"}} }), want: alertInk,
		},
		{
			name: "a restart due", text: "restart due (config changed)",
			doc: routed(func(d *status.Document) { d.Restart = status.Restart{Reason: status.RestartForConfig} }), want: warningInk,
		},
		{
			name: "probed", text: "○ probed · router not running",
			doc: routed(func(d *status.Document) {
				d.Source, d.Fallback = status.SourceProbe, status.Fallback{Router: status.RouterNotRunning}
			}),
			want: dimInk,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := inkOf(newStatusLine(tt.doc.At(printNow, printPolicy)).fit(200), tt.text)
			if !ok || got != tt.want {
				t.Errorf("%q is drawn in %+v (found: %t), want %+v", tt.text, got, ok, tt.want)
			}
		})
	}
}

func TestTheStatusLineShortensAsAPhonesDoes(t *testing.T) {
	busy := routed(func(d *status.Document) {
		d.Restart = status.Restart{Reason: status.RestartForConfig}
		d.Pin = status.Pin{Accounts: []string{"personal"}}
	})
	forms := []string{
		"● healthy  ·  restart due (config changed)  ·  pinned to personal  ·  6 sessions  ·  new sessions → ▲ side",
		"● healthy · restart due (config changed) · pinned to personal · 6 sessions · new → ▲ side",
		"● healthy · restart due (config changed) · pinned to personal · new → ▲ side",
		"● healthy · restart due (config changed) · pinned to personal",
	}
	probed := routed(func(d *status.Document) {
		d.Source, d.Fallback = status.SourceProbe, status.Fallback{Router: status.RouterNotRunning}
	})
	tests := []struct {
		name  string
		doc   status.Document
		width int
		want  string
	}{
		{name: "in full where it fits", doc: busy, width: len([]rune(forms[0])), want: forms[0]},
		{name: "spaced as a phone's", doc: busy, width: len([]rune(forms[0])) - 1, want: forms[1]},
		{name: "without the sessions", doc: busy, width: len([]rune(forms[1])) - 1, want: forms[2]},
		{name: "without where new sessions go", doc: busy, width: len([]rune(forms[2])) - 1, want: forms[3]},
		{name: "without the restart, the state and the mode showing", doc: busy, width: len([]rune(forms[3])) - 1, want: "● healthy · pinned to personal"},
		{name: "cut short where even they don't fit", doc: busy, width: 29, want: "● healthy · pinned to person…"},
		{name: "probed, in full", doc: probed, width: 30, want: "○ probed · router not running"},
		{name: "probed, shortened", doc: probed, width: 28, want: "○ probed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := newStatusLine(tt.doc.At(printNow, printPolicy)).fit(tt.width).plain(); got != tt.want {
				t.Errorf("in %d cells, the status line reads %q, want %q", tt.width, got, tt.want)
			}
		})
	}
}

// lostAt is when the router stopped answering, in the frames: 14:40:55 in
// printNow's time zone.
var lostAt = printNow.Add(-72 * time.Second).UTC()

// pageStatusOf is the status line the page draws of doc at printNow, on a
// frame width cells wide, the router lost at lost: the canvas it's drawn on,
// and its row there.
func pageStatusOf(doc status.Document, width int, lost time.Time) (*canvas, int) {
	f := Frame{Width: width, Height: 8, Lost: lost}
	c := newCanvas(f.Width, f.Height)
	f.routerStatus(c, doc, printNow)
	return c, f.statusRow()
}

func TestThePagesStatusLine(t *testing.T) {
	probedLost := routed(func(d *status.Document) {
		d.Source, d.Fallback = status.SourceProbe, status.Fallback{Router: status.RouterNotRunning}
	})
	notAnswering := "■ the router isn't answering  ·  since 14:40:55  ·  auto  ·  6 sessions  ·  new sessions → ▲ side"
	tests := []struct {
		name  string
		doc   status.Document
		width int
		lost  time.Time
		want  string
	}{
		{name: "on auto, saying so, as overview-default-160 draws it", doc: routed(), width: 160, want: "● healthy  ·  auto  ·  6 sessions  ·  new sessions → ▲ side"},
		{
			name: "pinned, as overview-pinned-160 draws it", width: 160,
			doc:  routed(func(d *status.Document) { d.Pin, d.Best = status.Pin{Accounts: []string{"personal"}}, "personal" }),
			want: "● healthy  ·  pinned to personal  ·  6 sessions  ·  new sessions → ▲ personal",
		},
		{
			name: "unhealthy, the rest as its document has them", width: 160,
			doc:  routed(func(d *status.Document) { d.Router = status.Health{Reason: "failed 6 of 8 requests"} }),
			want: "● unhealthy  ·  auto  ·  6 sessions  ·  new sessions → ▲ side",
		},
		{name: "the router not answering, the rest as its last document had them, as log-not-live-160 draws it", doc: routed(), width: 160, lost: lostAt, want: notAnswering},
		{
			name: "not answering, the mode above all", width: 160, lost: lostAt,
			doc:  routed(func(d *status.Document) { d.Pin = status.Pin{Accounts: []string{"personal", "side"}} }),
			want: "■ the router isn't answering  ·  since 14:40:55  ·  pinned to personal and side  ·  6 sessions  ·  new sessions → ▲ side",
		},
		{name: "probed, the router lost before: the probed line", doc: probedLost, width: 160, lost: lostAt, want: "○ probed · router not running"},
		{name: "before the first document: blank", width: 160},
		{name: "before the first document, the router not answering: that alone", width: 160, lost: lostAt, want: "■ the router isn't answering  ·  since 14:40:55"},
		{name: "at 100 columns, in full", doc: routed(), width: 100, want: "● healthy  ·  auto  ·  6 sessions  ·  new sessions → ▲ side"},
		{name: "on a phone, as overview-phone-52 draws it", doc: routed(), width: 52, want: "● healthy · auto · 6 sessions · new → ▲ side"},
		{name: "on a phone of 99 columns, as a phone spaces it, where the full form would fit", doc: routed(), width: 99, want: "● healthy · auto · 6 sessions · new → ▲ side"},
		{name: "on a phone, not answering, without the sessions and where new sessions go", doc: routed(), width: 52, lost: lostAt, want: "■ the router isn't answering · since 14:40:55 · auto"},
		{name: "on a phone, not answering, shortened", doc: routed(), width: 51, lost: lostAt, want: "■ no router since 14:40 · auto"},
		{name: "on a phone, probed, shortened", doc: probedLost, width: 28, want: "○ probed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, y := pageStatusOf(tt.doc, tt.width, tt.lost)
			if got := strings.TrimRight(c.rows(Look{})[y], " "); got != tt.want {
				t.Errorf("the status line reads %q, want %q", got, tt.want)
			}
		})
	}
	if got := newStatusLine(routed().At(printNow, printPolicy)).fit(200).plain(); strings.Contains(got, "auto") {
		t.Errorf("the printout's status line reads %q, want auto said on the page alone", got)
	}
}

func TestThePagesStatusLineSaysWhyAReadFailed(t *testing.T) {
	failed := Failure{Reason: "connection\nrefused", Next: printNow.Add(2 * time.Minute)}
	probed := routed(func(d *status.Document) { d.Source = status.SourceProbe })
	tests := []struct {
		name  string
		doc   status.Document
		width int
		lost  time.Time
		want  string
	}{
		{name: "before the first document, why and when it reads next", width: 160, want: "! couldn't read usage: connection refused  ·  next 14:44"},
		{name: "probed, in the probed line's place", doc: probed, width: 160, want: "! couldn't read usage: connection refused  ·  next 14:44"},
		{name: "probed, the router lost before: in the probed line's place", doc: probed, width: 160, lost: lostAt, want: "! couldn't read usage: connection refused  ·  next 14:44"},
		{name: "the router not answering, which says it", doc: routed(), width: 160, lost: lostAt, want: "■ the router isn't answering  ·  since 14:40:55  ·  auto  ·  6 sessions  ·  new sessions → ▲ side"},
		{name: "on a phone, shortened", width: 40, want: "! couldn't read usage next 14:44"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Width: tt.width, Height: 8, Lost: tt.lost, Failed: failed}
			c := newCanvas(f.Width, f.Height)
			f.routerStatus(c, tt.doc, printNow)
			if got := strings.TrimRight(c.rows(Look{})[f.statusRow()], " "); got != tt.want {
				t.Errorf("the status line reads %q, want %q", got, tt.want)
			}
		})
	}

	f := Frame{Width: 160, Height: 8, Failed: failed}
	c := newCanvas(f.Width, f.Height)
	f.routerStatus(c, status.Document{}, printNow)
	for _, tt := range []struct {
		name string
		x    int
		want ink
	}{{name: "!", x: 0, want: errorInk}, {name: "couldn't read usage", x: 2, want: secondaryInk}, {name: "next 14:44", x: 47, want: mutedInk}} {
		if got := c.at(tt.x, f.statusRow()).ink; got != tt.want {
			t.Errorf("%q, at column %d, is in %+v, want %+v", tt.name, tt.x, got, tt.want)
		}
	}
}

func TestThePagesStatusLineSaysItInItsInks(t *testing.T) {
	c, y := pageStatusOf(routed(), 160, lostAt)
	tests := []struct {
		name string
		x    int
		want ink
	}{
		{name: "■", x: 0, want: errorInk},
		{name: "the router isn't answering", x: 2, want: secondaryInk},
		{name: "  ·  since 14:40:55  ·  ", x: 33, want: mutedInk},
		{name: "auto", x: 52, want: secondaryInk},
	}
	for _, tt := range tests {
		if got := c.at(tt.x, y).ink; got != tt.want {
			t.Errorf("%q, at column %d, is in %+v, want %+v", tt.name, tt.x, got, tt.want)
		}
	}
}

func TestTheStatusLineIsOnItsRowFromTheFirstColumn(t *testing.T) {
	for _, tt := range []struct {
		width, row int
	}{{width: 160, row: 2}, {width: 52, row: 3}} {
		f := frameOf(tt.width, 40)
		if got := textOf(f.Draw(routed(), printNow))[tt.row]; !strings.HasPrefix(got, "● healthy") {
			t.Errorf("at %d columns, row %d reads %q, want the status line from its first column", tt.width, tt.row+1, got)
		}
	}
}

// The page's status line says where new sessions go as the moment's Next
// has it, at each of the hard moments, which USAGE's ▲ marks: the router's
// says the account it names, or that none has room, or, of several accounts
// none of which is read, that nothing is; a probed document's says nothing
// of it, and is the probed line.
func TestTheStatusLineAgreesWithTheMomentAtEachHardMoment(t *testing.T) {
	for _, hm := range statustest.HardMoments(claudeLike) {
		t.Run(hm.Name, func(t *testing.T) {
			f := Frame{Width: 200, Height: 8, Policy: claudeLike}
			m := f.Moment(hm.Doc, hm.Now)
			s := f.pageStatus(hm.Doc, hm.Now)
			var want string
			switch {
			case m.Probed:
				if got := s.state.plain(); !strings.HasPrefix(got, "○ probed") {
					t.Errorf("the status line's state reads %q, want the probed line", got)
				}
			case len(m.Accounts) > 1 && !slices.ContainsFunc(m.Accounts, func(a status.AccountNow) bool { return a.Read }):
				want = "nothing read yet"
			case m.Next == "":
				want = "no account has room"
			default:
				want = "▲ " + named(m, m.Next)
			}
			if got := s.destination.plain(); got != want {
				t.Errorf("the status line says new sessions go to %q, want %q, as the moment's Next is %q", got, want, m.Next)
			}
		})
	}
}
