package dashboard

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
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
			if got := newStatusLine(tt.doc).fit(200).plain(); got != tt.want {
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
			got, ok := inkOf(newStatusLine(tt.doc).fit(200), tt.text)
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
			if got := newStatusLine(tt.doc).fit(tt.width).plain(); got != tt.want {
				t.Errorf("in %d cells, the status line reads %q, want %q", tt.width, got, tt.want)
			}
		})
	}
}
