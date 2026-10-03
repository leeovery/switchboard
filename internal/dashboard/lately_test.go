package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

func TestLatelyTellsOfWhatBefellAnAccountFromItsSide(t *testing.T) {
	ago := func(d time.Duration) time.Time { return now.Add(-d).UTC() }
	// spread are the moves of personal's limit, which spread its sessions
	// over side and client: c61b's two models to side, and 41e0 to client.
	spread := []status.Event{
		{Kind: status.EventMoved, Session: idC61B, Model: opus, From: "personal", To: "side", Limit: 9},
		{Kind: status.EventMoved, Session: idC61B, Model: sonnet, From: "personal", To: "side", Limit: 9},
		{Kind: status.EventMoved, Session: id41E0, Model: sonnet, From: "personal", To: "client", Limit: 9},
	}
	tests := []struct {
		name    string
		account string
		event   status.Event
		// alongside are the document's other events, which none of its
		// lines tell of.
		alongside []status.Event
		// want are the lines told, each its mark, its words and what follows
		// them, a bar between.
		want []string
	}{
		{
			name: "a session starting on it", account: "side",
			event: status.Event{Kind: status.EventStarted, Account: "side", Session: idC61B, Reason: "new, work under pressure"},
			want:  []string{"▲|c61b started here|, the best, passing over work under pressure"},
		},
		{
			name: "coming under pressure", account: "work",
			event: status.Event{Kind: status.EventPressure, Account: "work", Windows: []string{"5h"}, Until: now.Add(time.Hour).UTC(), Since: ago(31 * time.Minute)},
			want:  []string{"●|came under pressure|: its session runs out ~14:12 at its last-30-min rate"},
		},
		{
			name: "reaching its limit, and the sessions it moved", account: "personal",
			event: status.Event{Kind: status.EventLimit, Account: "personal", Windows: []string{"5h"}, Count: 3, To: "side"},
			want:  []string{"■|reached its session limit|", "▸|3 sessions moved to side|"},
		},
		{
			name: "another's limit moving its sessions here", account: "side",
			event: status.Event{Kind: status.EventLimit, Account: "personal", Windows: []string{"5h"}, Count: 1, To: "side"},
			want:  []string{"▸|1 session arrived from personal|"},
		},
		{
			name: "another's limit spreading its sessions, one of them here", account: "side",
			event: status.Event{Kind: status.EventLimit, Account: "personal", Windows: []string{"5h"}, Count: 2}, alongside: spread,
			want: []string{"▸|1 session arrived from personal|"},
		},
		{
			name: "another's limit spreading its sessions, none here", account: "work",
			event: status.Event{Kind: status.EventLimit, Account: "personal", Windows: []string{"5h"}, Count: 2}, alongside: spread,
		},
		{
			name: "a session moving off it", account: "side",
			event: status.Event{Kind: status.EventMoved, Session: idC61B, From: "side", To: "work", Reason: "pinned"},
			want:  []string{"▸|c61b moved to work| (pin)"},
		},
		{
			name: "a session moving to it", account: "work",
			event: status.Event{Kind: status.EventMoved, Session: idC61B, From: "side", To: "work", Reason: "moved: side has no room"},
			want:  []string{"▸|c61b moved here from side|: side has no room"},
		},
		{
			name: "a refusal", account: "work",
			event: status.Event{Kind: status.EventRefused, Account: "work", Status: 403, Family: "opus", Until: now.Add(time.Hour).UTC()},
			want:  []string{"■|was refused (403, opus)| until 14:12"},
		},
		{
			name: "a prime", account: "side",
			event: status.Event{Kind: status.EventPrimed, Account: "side", Windows: []string{"5h"}, Until: now.Add(4 * time.Hour).UTC()},
			want:  []string{"◇|primed: its 5-hour window started|, resetting 17:12"},
		},
		{
			name: "room again", account: "work",
			event: status.Event{Kind: status.EventRoom, Account: "work"},
			want:  []string{"●|has room again|"},
		},
		{
			name: "a move a limit forced, which the limit's lines count", account: "side",
			event: status.Event{Kind: status.EventMoved, Session: idC61B, From: "personal", To: "side", Limit: 1},
		},
		{
			name: "another account's", account: "side",
			event: status.Event{Kind: status.EventPrimed, Account: "work", Windows: []string{"5h"}},
		},
		{
			name: "the router's own", account: "work",
			event: status.Event{Kind: status.EventRestart, Reason: "config changed"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := flipping()
			tt.event.ID, tt.event.At = 9, ago(time.Minute)
			doc.Events = slices.Concat(tt.alongside, []status.Event{tt.event})
			var got []string
			for _, l := range latelyOf(doc, tt.account, now) {
				got = append(got, l.mark.text+"|"+l.words+"|"+text(l.more))
				if l.id != 9 || !l.at.Equal(tt.event.At) {
					t.Errorf("a line tells of event %d at %s, want event 9 at %s", l.id, l.at, tt.event.At)
				}
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("tells %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLatelyShowsWhatFollowsAnEventsWordsWhereItFitsWhole(t *testing.T) {
	doc := flipping()
	doc.Events = []status.Event{{ID: 1, At: now.Add(-time.Hour).UTC(), Kind: status.EventPrimed, Account: "side", Windows: []string{"5h"}, Until: now.Add(4 * time.Hour).UTC()}}
	for _, tt := range []struct {
		width int
		want  string
	}{
		{width: 98, want: "12:12  ◇ primed: its 5-hour window started, resetting 17:12"},
		{width: 44, want: "12:12  ◇ primed: its 5-hour window started"},
		{width: 30, want: "12:12  ◇ primed: its 5-hour w…"},
	} {
		c := newCanvas(tt.width, 6)
		Frame{}.lately(c, doc, "side", now, 0, 0, tt.width, 5)
		rows := c.rows(Look{})
		if rows[1] != "LATELY" || strings.TrimRight(rows[2], " ") != tt.want {
			t.Errorf("%d cells wide, reads\n%s\nwant LATELY over %q", tt.width, strings.Join(rows, "\n"), tt.want)
		}
	}
}

func TestLatelyShowsAsManyAsFitTheNewestFirstEachPickedOutWhileFresh(t *testing.T) {
	doc := flipping()
	f := Frame{Fresh: map[int]float64{7: 0.5}}
	c := newCanvas(44, 6)
	f.lately(c, doc, "side", now, 2, 0, 42, 5)
	rows := c.rows(Look{})
	want := []string{"", "  LATELY", "  13:11  ▲ c61b started here, the best", "  12:22  ▸ 3 sessions arrived from personal", "", ""}
	for i := range rows {
		rows[i] = strings.TrimRight(rows[i], " ")
	}
	if !slices.Equal(rows, want) {
		t.Errorf("reads\n%s\nwant\n%s: a blank row, the label, and two of three, leaving a blank over the keys", strings.Join(rows, "\n"), strings.Join(want, "\n"))
	}
	for x := range 44 {
		if got := c.at(x, 2).ink.on; got != (hue{token: theme.BgAttention, fade: 0.5}) {
			t.Fatalf("cell %d of the fresh event's row is on %+v, want bg.attention, side to side", x, got)
		}
	}
	if got := c.at(0, 3).ink.on; got != (hue{}) {
		t.Errorf("the next row is on %+v, want the canvas", got)
	}
}

func TestLatelyWithoutRoomForOneIsntShown(t *testing.T) {
	c := newCanvas(44, 4)
	Frame{}.lately(c, flipping(), "side", now, 0, 0, 44, 3)
	for _, row := range c.rows(Look{}) {
		if strings.TrimSpace(row) != "" {
			t.Fatalf("drew\n%s\nwant nothing: no room for a line under the label and a blank", strings.Join(c.rows(Look{}), "\n"))
		}
	}
}
