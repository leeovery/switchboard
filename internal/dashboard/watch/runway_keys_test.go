package watch

import (
	"cmp"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/status"
)

// runwayKeysHarness is a model of the router of the accounts given, the
// readings history filed holding none, showing Runway in a terminal width
// cells wide, or 160, and 37 tall, the keys given typed. It has started.
func runwayKeysHarness(t *testing.T, doc status.Document, width int, keys string) *harness {
	t.Helper()
	h := routedHarness(t, doc)
	h.model.cfg.Readings, h.model.view = &filedReadings{}, dashboard.Runway
	h.deliver(h.update(tea.WindowSizeMsg{Width: cmp.Or(width, 160), Height: 37})...)
	h.start()
	h.keyed(keys)
	return h
}

// Runway's key lines are the design's: < > window and ←→ day, or ←→ week
// in the week view, live or stepped back to, as the key line stays the live
// view's; p pin where a pin can be set; and a phone's keeping both, tab views
// leading.
func TestRunwaysKeyLinesAreTheDesigns(t *testing.T) {
	const (
		day  = "tab views   < > window  ←→ day    p pin    ? help  q quit"
		week = "tab views   < > window  ←→ week    p pin    ? help  q quit"
	)
	tests := []struct {
		name  string
		doc   status.Document
		width int
		keys  string
		want  string
	}{
		{name: "the week view, which Runway opens on", want: week},
		{name: "this week", keys: "left", want: week},
		{name: "a week stepped back to", keys: "left left", want: week},
		{name: "the 5-hour view", keys: ">", want: day},
		{name: "today, from its midnight", keys: "> left", want: day},
		{name: "a day stepped back to", keys: "> left left", want: day},
		{name: "the week view again", keys: "> <", want: week},
		{name: "one account, with nowhere else to pin", doc: routerDocument(three()[0]), want: "tab views   < > window  ←→ week    ? help  q quit"},
		{name: "a phone", width: 52, want: "tab views   < > window   ←→ week   ? keys   q quit"},
		{name: "a phone in the 5-hour view", width: 52, keys: ">", want: "tab views   < > window   ←→ day   ? keys   q quit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := tt.doc
			if doc.Accounts == nil {
				doc = routerDocument(three()...)
			}
			h := runwayKeysHarness(t, doc, tt.width, tt.keys)
			rows := strings.Split(h.view(), "\n")
			if got := strings.TrimRight(rows[len(rows)-1], " "); got != tt.want {
				t.Errorf("after %q, the key line reads\n%q\nwant\n%q", tt.keys, got, tt.want)
			}
		})
	}
}

// Help over Runway lists its keys first, as the key line words them: tab,
// < > and ←→, p, t in colour, R, ? and q.
func TestRunwaysHelpListsItsKeys(t *testing.T) {
	for _, tt := range []struct {
		name, keys string
		want       []string
	}{
		{name: "the week view, which Runway opens on", want: []string{"tab views", "< > window", "←→ week", "p pin", "t themes", "R refresh", "? help", "q quit"}},
		{name: "the 5-hour view", keys: ">", want: []string{"tab views", "< > window", "←→ day", "p pin", "t themes", "R refresh", "? help", "q quit"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := routedHarness(t, routerDocument(three()...))
			h.model.cfg.Themes = &fakeThemes{}
			h.model.cfg.Readings, h.model.view = &filedReadings{}, dashboard.Runway
			h.deliver(h.update(tea.WindowSizeMsg{Width: 160, Height: 37})...)
			h.start()
			h.keyed(tt.keys)
			var got []string
			for _, k := range helpOver(h.model) {
				got = append(got, k.Key+" "+k.Does)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("help lists %q, want %q", got, tt.want)
			}
		})
	}
}

// Where Runway's lanes scroll, at the last step of its compaction, help
// lists j and k, PgUp and PgDn, and the wheel, which the key line leaves to
// it.
func TestRunwaysHelpListsTheKeysThatScrollItsLanes(t *testing.T) {
	h := runwayKeysHarness(t, twelve(), 160, "")
	if f, doc, at := h.model.screen(h.clock.now); f.Scrolling(doc, at).Most == 0 {
		t.Fatal("Runway's twelve lanes don't scroll at 160x37, want them to")
	}
	scrolling := []string{"j k", "PgUp PgDn", "wheel"}
	if got := listedKeys(helpOver(h.model)); !containsInOrder(got, scrolling) {
		t.Errorf("help lists %q, want %q among them", got, scrolling)
	}
	if got := h.listed(); slices.ContainsFunc(scrolling, func(k string) bool { return slices.Contains(got, k) }) {
		t.Errorf("the key line lists %q, want the scrolling keys left to help", got)
	}
}
