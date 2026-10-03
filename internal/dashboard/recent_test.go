package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

func TestRECENTSaysWhatEachKindOfEventTellsOf(t *testing.T) {
	doc := threeRouted()
	client := readAccount("client", sessionOf(0.3, time.Hour), weekOf(0.7, 2*day), windowOf("7d_oi", "Fable week", 0.2, 3*day))
	client.Reserve = 0.1
	doc.Accounts = append(doc.Accounts, client)
	at := now.Add(-4 * time.Minute).UTC()
	tests := []struct {
		name     string
		event    status.Event
		wantMark string
		wantInk  theme.Token
		want     string
	}{
		{
			name:     "a session starting on the best",
			event:    status.Event{Kind: status.EventStarted, Account: "side", Session: "c61b2f8d-04a7", Model: "claude-opus-5-5", Reason: "new"},
			wantMark: "▲", wantInk: theme.AccentMode, want: "c61b started on side, the best",
		},
		{
			name:     "a session starting where its pin sends it",
			event:    status.Event{Kind: status.EventStarted, Account: "client", Session: "9e21d4a8", Reason: "pinned"},
			wantMark: "▲", wantInk: theme.AccentMode, want: "9e21 started on client (pin)",
		},
		{
			name:     "a session starting on the best, passing over one under pressure",
			event:    status.Event{Kind: status.EventStarted, Account: "side", Session: "c61b2f8d", Reason: "new, work under pressure"},
			wantMark: "▲", wantInk: theme.AccentMode, want: "c61b started on side, the best, passing over work under pressure",
		},
		{
			name:     "a session starting for a reason of the router's own",
			event:    status.Event{Kind: status.EventStarted, Account: "side", Session: "c61b2f8d", Reason: "no account has room"},
			wantMark: "▲", wantInk: theme.AccentMode, want: "c61b started on side: no account has room",
		},
		{
			name:     "an account coming under pressure, at its recent rate",
			event:    status.Event{Kind: status.EventPressure, Account: "work", Windows: []string{"5h"}, Until: now.Add(53 * time.Minute).UTC(), Since: at.Add(-score.Recent)},
			wantMark: "●", wantInk: theme.AccentAttention, want: "work came under pressure: its session runs out ~14:05 at its last-30-min rate",
		},
		{
			name:     "at its recent rate over a gap in its readings",
			event:    status.Event{Kind: status.EventPressure, Account: "work", Windows: []string{"5h"}, Until: now.Add(53 * time.Minute).UTC(), Since: at.Add(-2 * time.Hour)},
			wantMark: "●", wantInk: theme.AccentAttention, want: "work came under pressure: its session runs out ~14:05 at its last-2h rate",
		},
		{
			name:     "at its pace",
			event:    status.Event{Kind: status.EventPressure, Account: "work", Windows: []string{"5h"}, Until: now.Add(53 * time.Minute).UTC()},
			wantMark: "●", wantInk: theme.AccentAttention, want: "work came under pressure: its session runs out ~14:05 at its pace",
		},
		{
			name:     "an account its reserve holds back, reaching it",
			event:    status.Event{Kind: status.EventPressure, Account: "client", Windows: []string{"5h"}, Until: now.Add(53 * time.Minute).UTC(), Since: at.Add(-score.Recent)},
			wantMark: "●", wantInk: theme.AccentAttention, want: "client came under pressure: its session reaches its reserve ~14:05 at its last-30-min rate",
		},
		{
			name:     "under pressure, without when it runs out",
			event:    status.Event{Kind: status.EventPressure, Account: "work", Windows: []string{"5h"}},
			wantMark: "●", wantInk: theme.AccentAttention, want: "work came under pressure",
		},
		{
			name:     "a limit, and the sessions it moved to one account",
			event:    status.Event{Kind: status.EventLimit, Account: "personal", Windows: []string{"5h"}, Until: now.Add(time.Hour).UTC(), Count: 3, To: "side"},
			wantMark: "■", wantInk: theme.StateDestructive, want: "personal reached its session limit; 3 sessions moved to side",
		},
		{
			name:     "a limit in two windows, moving a session to several",
			event:    status.Event{Kind: status.EventLimit, Account: "personal", Windows: []string{"5h", "7d"}, Count: 1},
			wantMark: "■", wantInk: theme.StateDestructive, want: "personal reached its session and week limits; 1 session moved",
		},
		{
			name:     "a limit the answer named no window of, moving nothing",
			event:    status.Event{Kind: status.EventLimit, Account: "client"},
			wantMark: "■", wantInk: theme.StateDestructive, want: "client reached its limit",
		},
		{
			name:     "a limit in a model's own week",
			event:    status.Event{Kind: status.EventLimit, Account: "client", Windows: []string{"7d_oi"}},
			wantMark: "■", wantInk: theme.StateDestructive, want: "client reached its Fable week limit",
		},
		{
			name:     "a move by pin",
			event:    status.Event{Kind: status.EventMoved, Session: "9e21d4a8", Model: "claude-sonnet-5-5", From: "side", To: "client", Reason: "pinned"},
			wantMark: "▸", wantInk: theme.TextSubtle, want: "9e21 moved side → client (pin)",
		},
		{
			name:     "a move as its account had no room",
			event:    status.Event{Kind: status.EventMoved, Session: "41e0b6c2", From: "personal", To: "side", Reason: "moved: personal has no room"},
			wantMark: "▸", wantInk: theme.TextSubtle, want: "41e0 moved personal → side: personal has no room",
		},
		{
			name:     "a token refused",
			event:    status.Event{Kind: status.EventRefused, Account: "work", Status: 401, Until: now.Add(9 * time.Hour).UTC()},
			wantMark: "■", wantInk: theme.StateDestructive, want: "work was refused (401) until 22:12",
		},
		{
			name:     "a model's requests refused",
			event:    status.Event{Kind: status.EventRefused, Account: "work", Status: 403, Family: "opus", Until: now.Add(3 * day).UTC()},
			wantMark: "■", wantInk: theme.AccentAttention, want: "work was refused (403, opus) until Thu 13:12",
		},
		{
			name:     "a prime",
			event:    status.Event{Kind: status.EventPrimed, Account: "side", Windows: []string{"5h"}, Until: now.Add(5 * time.Hour).UTC()},
			wantMark: "◇", wantInk: theme.AccentPrimary, want: "side primed: its 5-hour window started, resetting 18:12",
		},
		{
			name:     "a prime that didn't read its reset",
			event:    status.Event{Kind: status.EventPrimed, Account: "side", Windows: []string{"5h"}},
			wantMark: "◇", wantInk: theme.AccentPrimary, want: "side primed: its 5-hour window started",
		},
		{
			name:     "room again",
			event:    status.Event{Kind: status.EventRoom, Account: "personal"},
			wantMark: "●", wantInk: theme.StatePositive, want: "personal has room again",
		},
		{
			name:     "a restart falling due",
			event:    status.Event{Kind: status.EventRestart, Reason: "config changed"},
			wantMark: "!", wantInk: theme.AccentAttention, want: "restart due (config changed)",
		},
		{
			name:     "the router turning unhealthy",
			event:    status.Event{Kind: status.EventHealth, Reason: "failed 6 of 8 requests"},
			wantMark: "●", wantInk: theme.StateDestructive, want: "the router turned unhealthy: failed 6 of 8 requests",
		},
		{
			name:     "the router healthy again",
			event:    status.Event{Kind: status.EventHealth},
			wantMark: "●", wantInk: theme.StatePositive, want: "the router is healthy again",
		},
		{
			name:     "an account no longer configured, by its id",
			event:    status.Event{Kind: status.EventRoom, Account: "gone\x1b[31m"},
			wantMark: "●", wantInk: theme.StatePositive, want: "gone [31m has room again",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.event.At = at
			mark, words, ok := told(tt.event, doc, now)
			if !ok {
				t.Fatalf("told() = false, want RECENT to tell of %s", tt.event.Kind)
			}
			if mark.text != tt.wantMark || mark.ink.token != tt.wantInk {
				t.Errorf("its mark is %q in %v, want %q in %v", mark.text, mark.ink.token, tt.wantMark, tt.wantInk)
			}
			if got := words.plain(); got != tt.want {
				t.Errorf("RECENT says %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRECENTsPressureGoesByTheReserveAsTheAccountStands(t *testing.T) {
	reserved := pressedAccount("work")
	reserved.Reserve = 0.1
	pinned := routerDoc("work", 0, reserved)
	pinned.Pin = status.Pin{Accounts: []string{"work"}}
	e := status.Event{At: now.UTC(), Kind: status.EventPressure, Account: "work", Windows: []string{"5h"}, Until: now.Add(time.Hour).UTC()}
	tests := []struct {
		name string
		doc  status.Document
		want string
	}{
		{name: "its reserve holding it back", doc: routerDoc("side", 0, reserved), want: "work came under pressure: its session reaches its reserve ~14:12 at its pace"},
		{name: "a pin naming it", doc: pinned, want: "work came under pressure: its session runs out ~14:12 at its pace"},
		{name: "without a reserve", doc: threeRouted(), want: "work came under pressure: its session runs out ~14:12 at its pace"},
		{name: "no longer configured", doc: routerDoc("side", 0), want: "work came under pressure: its 5h runs out ~14:12 at its pace"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, words, _ := told(e, tt.doc, now); words.plain() != tt.want {
				t.Errorf("RECENT says %q, want %q", words.plain(), tt.want)
			}
		})
	}
}

func TestRECENTsWordsAreInTheirInks(t *testing.T) {
	e := status.Event{At: now.UTC(), Kind: status.EventLimit, Account: "personal", Windows: []string{"5h"}, Count: 3, To: "side"}
	_, words, _ := told(e, threeRouted(), now)

	want := line{{"personal", strongInk}, {" reached its session limit", mutedInk}, {"; ", mutedInk}, {"3 sessions", secondaryInk}, {" moved", mutedInk}, {" to ", mutedInk}, {"side", strongInk}}
	if !slices.Equal(words, want) {
		t.Errorf("RECENT says %+v, want %+v", words, want)
	}
}

func TestRECENTDoesntTellOfAKindItDoesntKnow(t *testing.T) {
	doc := threeRouted()
	doc.Events = []status.Event{
		{ID: 2, At: now.UTC(), Kind: "rumour", Account: "work"},
		{ID: 1, At: now.UTC(), Kind: status.EventRoom, Account: "work"},
	}
	if _, _, ok := told(doc.Events[0], doc, now); ok {
		t.Error("told() = true of a kind RECENT doesn't know, want false")
	}
	if got := recent(doc, now, 4); len(got) != 1 || got[0].ID != 1 {
		t.Errorf("recent() = %+v, want the event of the kind it knows", got)
	}
}

func TestRECENTLeavesTheMovesALimitForcedToItsLine(t *testing.T) {
	doc := threeRouted()
	doc.Events = []status.Event{
		{ID: 5, At: now.UTC(), Kind: status.EventMoved, Session: "9e21d4a8", From: "side", To: "client", Reason: "pinned"},
		{ID: 4, At: now.UTC(), Kind: status.EventMoved, Session: "41e0b6c2", From: "personal", To: "side", Reason: "moved: personal has no room", Limit: 2},
		{ID: 3, At: now.UTC(), Kind: status.EventMoved, Session: "db8a71e3", From: "personal", To: "side", Reason: "moved: personal hit its limit", Limit: 2},
		{ID: 2, At: now.UTC(), Kind: status.EventLimit, Account: "personal", Windows: []string{"5h"}, Count: 2, To: "side", Limit: 1},
		{ID: 1, At: now.UTC(), Kind: status.EventMoved, Session: "5c7a93f1", From: "personal", To: "side", Reason: "moved: personal has no room"},
	}
	var got []int
	for _, e := range recent(doc, now, 4) {
		got = append(got, e.ID)
	}
	if want := []int{5, 2, 1}; !slices.Equal(got, want) {
		t.Errorf("RECENT lists the events %v, want %v: the limit's moves told of by its line alone", got, want)
	}
}

func TestRECENTSaysWhyThereAreNone(t *testing.T) {
	stuck := probed()
	stuck.Fallback = status.Fallback{Router: status.RouterUnhealthy, Reason: "no answer within 500ms"}
	asked := probed()
	asked.Fallback = status.Fallback{}
	tests := []struct {
		name     string
		doc      status.Document
		outdated bool
		want     string
	}{
		{name: "probing, the router not running", doc: probed(), want: "the router isn't running"},
		{name: "probing, the router not answering as it should", doc: stuck, want: "the router isn't answering"},
		{name: "probing as asked", doc: asked, want: "none while probing, as asked"},
		{name: "a router from before it told of events", doc: threeRouted(), outdated: true, want: "restart the router for recent events"},
		{name: "a router that has told of none", doc: threeRouted(), want: "nothing lately"},
		{name: "nothing read yet", doc: status.Document{}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (Frame{Outdated: tt.outdated}).quiet(tt.doc).plain(); got != tt.want {
				t.Errorf("quiet() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRECENTsStrip(t *testing.T) {
	doc := threeRouted()
	doc.Events = []status.Event{
		{ID: 5, At: now.Add(-time.Minute).UTC(), Kind: status.EventStarted, Account: "side", Session: "c61b", Reason: "new"},
		{ID: 4, At: now.Add(-4 * time.Minute).UTC(), Kind: status.EventRoom, Account: "work"},
		{ID: 3, At: now.Add(-day - time.Hour).UTC(), Kind: status.EventPrimed, Account: "side", Windows: []string{"5h"}},
		{ID: 2, At: now.Add(-2 * day).UTC(), Kind: status.EventRoom, Account: "side"},
		{ID: 1, At: now.Add(-3 * day).UTC(), Kind: status.EventRoom, Account: "personal"},
	}
	tests := []struct {
		name  string
		width int
		want  []string
	}{
		{name: "its four newest, their times in a column", width: 120, want: []string{
			"13:11      ▲ c61b started on side, the best",
			"13:08      ● work has room again",
			"Sun 12:12  ◇ side primed: its 5-hour window started",
			"Sat 13:12  ● side has room again",
		}},
		{name: "on a phone, a single blank after its times, cut short", width: 40, want: []string{
			"13:11     ▲ c61b started on side, the…",
			"13:08     ● work has room again",
			"Sun 12:12 ◇ side primed: its 5-hour wi…",
			"Sat 13:12 ● side has room again",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(tt.width, 6)
			c := newCanvas(f.Width, f.Height)
			if got := f.recentStrip(c, doc, now, 0, 1, f.edge(), recentLines); got != 4 {
				t.Errorf("recentStrip() = %d, want 4 lines", got)
			}
			rows := c.rows(Look{})[1:5]
			if !slices.Equal(rows, tt.want) {
				t.Errorf("RECENT reads\n%s\nwant\n%s", strings.Join(rows, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

func TestRECENTsNewestTimeStandsOut(t *testing.T) {
	doc := threeRouted()
	doc.Events = []status.Event{
		{ID: 2, At: now.UTC(), Kind: status.EventRoom, Account: "work"},
		{ID: 1, At: now.UTC(), Kind: status.EventRoom, Account: "side"},
	}
	f := frameOf(80, 4)
	c := newCanvas(f.Width, f.Height)
	f.recentStrip(c, doc, now, 0, 0, f.edge(), recentLines)

	if got := c.at(0, 0).ink.token; got != theme.TextSecondary {
		t.Errorf("the newest's time is in %v, want text.secondary", got)
	}
	if got := c.at(0, 1).ink.token; got != theme.TextSubtle {
		t.Errorf("the next's time is in %v, want text.subtle", got)
	}
}

func TestRECENTPicksOutFreshEventsFadingBack(t *testing.T) {
	doc := threeRouted()
	doc.Events = []status.Event{
		{ID: 7, At: now.UTC(), Kind: status.EventRoom, Account: "work"},
		{ID: 6, At: now.UTC(), Kind: status.EventRoom, Account: "side"},
		{ID: 5, At: now.UTC(), Kind: status.EventRoom, Account: "personal"},
	}
	f := frameOf(80, 4)
	f.Fresh = map[int]float64{7: 0, 6: 0.75}
	c := newCanvas(f.Width, f.Height)
	f.recentStrip(c, doc, now, 10, 0, 60, recentLines)

	tests := []struct {
		row int
		on  hue
	}{{row: 0, on: hue{token: theme.BgAttention}}, {row: 1, on: hue{token: theme.BgAttention, fade: 0.75}}, {row: 2}}
	for _, tt := range tests {
		for _, x := range []int{10, 40, 59} {
			if got := c.at(x, tt.row).ink.on; got != tt.on {
				t.Errorf("row %d, column %d, is on %+v, want on %+v", tt.row+1, x, got, tt.on)
			}
		}
		if got := c.at(9, tt.row).ink.on; got != (hue{}) {
			t.Errorf("row %d is picked out before its strip, at column 9, want from where its lines start", tt.row+1)
		}
	}
}
