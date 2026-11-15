package dashboard

import (
	"image/color"
	"math"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// In every built-in that gives its text colours of its own, a heading
// stands further from the canvas than the labels under it, and the values
// it heads further still, light canvas or dark.
func TestEveryBuiltInDrawsItsHeadingsBrighterThanItsLabelsAndDimmerThanItsValues(t *testing.T) {
	for _, th := range theme.Builtins() {
		if !th.Paints() || th.Faint(headingInk.token) {
			continue
		}
		canvas := th.Colour(theme.Canvas)
		label, heading, value := contrast(th.Colour(dimInk.token), canvas), contrast(th.Colour(headingInk.token), canvas), contrast(th.Colour(secondaryInk.token), canvas)
		if heading <= label || value <= heading {
			t.Errorf("%s stands its labels %.2f, its headings %.2f and its values %.2f from its canvas, want each further than the one before", th.Slug, label, heading, value)
		}
	}
}

// contrast is how far apart two colours stand, as WCAG's contrast ratio
// has it.
func contrast(a, b color.Color) float64 {
	la, lb := luminance(a), luminance(b)
	return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)
}

// luminance is c's relative luminance, as WCAG works it out.
func luminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	linear := func(v uint32) float64 {
		s := float64(v) / 0xffff
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*linear(r) + 0.7152*linear(g) + 0.0722*linear(b)
}

// The Overview's, Detail's, Compare's, a session's page's and a Log
// details card's headings are in text.muted, and the labels, a card's item
// names and a table's column heads under them in text.subtle.
func TestHeadingsAreAStepOverTheirLabelsOnTheOverviewDetailCompareASessionsPageAndALogCard(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	muted, subtle := shadeOf(nord.theme.Colour(theme.TextMuted)), shadeOf(nord.theme.Colour(theme.TextSubtle))
	overview, sessions, stream := overviewWorld()
	detail := detailFrame(nord, framedAccounts, "", "", detailsPage(t), nil)
	compare := compareFrame(t, threeFramed, comparedPage(views.PeriodThisWeek, comparedVersions(t), compareWork, comparePersonal, compareSide, compareAll))
	compare.Look = nord
	session := pageFrame(t, moved(), 160, 32)
	session.Look = nord
	card := chosenOn(cardPage(40), 22, 160, 37)
	card.Look = nord
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
		{
			name:     "a Log details card",
			rows:     opened(card).Draw(routerDoc("side", 0), logDayNoon),
			headings: []string{"ROUTED", "ASKED"},
			labels:   []string{"upstream", "max_tokens"},
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
