package dashboard

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// A heading is a step over the labels under it and under the values it
// heads: text.muted, over text.subtle, under text.secondary.
func TestAHeadingIsAStepOverItsLabelsAndUnderItsValues(t *testing.T) {
	if headingInk.token != theme.TextMuted || dimInk.token != theme.TextSubtle || headingInk.token+1 != dimInk.token {
		t.Errorf("headings are in %v and labels in %v, want text.muted, a step over text.subtle", headingInk.token, dimInk.token)
	}
	if secondaryInk.token >= headingInk.token {
		t.Errorf("values are in %v, want them brighter than the headings' %v", secondaryInk.token, headingInk.token)
	}
}

// Every built-in that gives the text tokens colours of its own gives
// text.muted another than text.subtle, so a heading never reads level with
// its labels.
func TestEveryBuiltInSetsItsHeadingsApartFromItsLabels(t *testing.T) {
	for _, th := range theme.Builtins() {
		if th.Faint(theme.TextMuted) {
			continue
		}
		if muted, subtle := shadeOf(th.Colour(theme.TextMuted)), shadeOf(th.Colour(theme.TextSubtle)); muted == subtle {
			t.Errorf("%s gives text.muted and text.subtle one colour, %s", th.Slug, muted)
		}
	}
}

// The headings each tab draws its own way are in headingInk, and the
// labels and a table's column heads under them in dimInk.
func TestEveryTabsHeadingsAreAStepOverTheirLabels(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	muted, subtle := shadeOf(nord.theme.Colour(theme.TextMuted)), shadeOf(nord.theme.Colour(theme.TextSubtle))
	overview, sessions, stream := overviewWorld()
	detail := detailFrame(nord, framedAccounts, "", "", detailsPage(t), nil)
	compare := compareFrame(t, threeFramed, comparedPage(views.PeriodThisWeek, comparedVersions(t), compareWork, comparePersonal, compareSide, compareAll))
	compare.Look = nord
	session := pageFrame(t, moved(), 160, 32)
	session.Look = nord
	tests := []struct {
		name     string
		rows     []string
		headings []string
		labels   []string
	}{
		{
			name:     "the Overview",
			rows:     Frame{Width: 160, Height: 39, Look: nord, View: Overview, Policy: claudeLike, Sessions: sessions, Stream: stream, Sections: withSessions}.Draw(overview, routedAt),
			headings: []string{"USAGE", "ROUTING", "SESSIONS"},
			labels:   []string{"5-HOUR", "Σ"},
		},
		{
			name:     "Detail",
			rows:     detail.Draw(usageWorld(extraOff), routedAt),
			headings: []string{"ACCOUNTS", "USAGE", "THIS WEEK", "BY MODEL"},
			labels:   []string{"Week", "requests", "plan"},
		},
		{
			name:     "Compare",
			rows:     compare.Draw(usageWorld(extraOff), routedAt),
			headings: []string{"ACCOUNTS", "WEEK USED", "BY MODEL", "WEEKS", "WORTH"},
		},
		{
			name:     "a session's page",
			rows:     session.Draw(pageDoc(), sessionNow),
			headings: []string{"SESSION", "TURNS"},
		},
	}
	for _, tt := range tests {
		cells, text := screenOf(t, tt.rows), textOf(tt.rows)
		for _, want := range []struct {
			words []string
			shade shade
			is    string
		}{{tt.headings, muted, "text.muted"}, {tt.labels, subtle, "text.subtle"}} {
			for _, words := range want.words {
				x, y := wordsAt(text, words)
				if y < 0 {
					t.Errorf("%s doesn't show %q", tt.name, words)
					continue
				}
				if got := seenAt(cells, x, y); got.fg != want.shade {
					t.Errorf("%s draws %q in %s, want %s", tt.name, words, got, want.is)
				}
			}
		}
	}
}

// The headings a card's sections and a group's are drawn in are
// headingInk's, an account they name apart.
func TestCardsAndGroupsHeadingsAreInHeadingInk(t *testing.T) {
	for name, l := range map[string]line{
		"a card's section": sectionHeading("ROUTED"),
		"a card's windows": windowsHeading(""),
		"an account's":     windowsHeading("side")[1:],
		"a group's":        groupHead("WEEKS", "each week at its peak", false)[:1],
		"a narrow group's": groupHead("WEEKS", "", true),
		"a panel's":        {panelHeading("SESSIONS", false)},
	} {
		for _, s := range l {
			if s.ink != headingInk {
				t.Errorf("%s heading draws %q in %+v, want headingInk", name, s.text, s.ink)
			}
		}
	}
}

// wordsAt is the column and row the words first show at, as words alone,
// not inside others: -1, -1 where they don't.
func wordsAt(rows []string, words string) (x, y int) {
	for y, row := range rows {
		for at := 0; ; {
			i := strings.Index(row[at:], words)
			if i < 0 {
				break
			}
			i += at
			end := i + len(words)
			if (i == 0 || row[i-1] == ' ') && (end == len(row) || row[end] == ' ') {
				return ansi.StringWidth(row[:i]), y
			}
			at = end
		}
	}
	return -1, -1
}
