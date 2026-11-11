package dashboard

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// teamRemoved is BY ACCOUNT's strip of an account removed on Friday 4
// September, labelled team: every day at level 2, so a day drawn after the
// one it was removed on shows, and two 5-hour limits.
func teamRemoved(gone int) views.DaysStrip {
	return views.DaysStrip{Of: "h7c2", Label: "team", Removed: "removed 4 Sep", Gone: gone, Limits: 2, Days: stripOf(strings.Repeat("2", 53), 15, 22)}
}

// workAsTeam is work's strip, configured under the label the account removed
// showed.
var workAsTeam = views.DaysStrip{Of: "work", Label: "team", Limits: 1, Days: stripOf(strings.Repeat("1", 53), 50)}

// Days draws an account removed by the label it showed last, in text.faint,
// told apart from the configured account given that label since: its days to
// the one it was removed on, none after, nor any to come, and when it was
// removed from the first week after.
func TestDaysDrawsAnAccountRemovedByItsLastLabel(t *testing.T) {
	f := daysFrame(t, daysWorld(workAsTeam, teamRemoved(26)), 160, 37)
	f.Look = NoColour()
	c := historyDrawn(f)
	rows := textOf(c.rows(Look{}))
	l := f.layOutDays(*f.History.Days, f.top())
	configured, removed := l.accountRow(0), l.accountRow(1)

	if !strings.HasPrefix(rows[configured], "team ") || c.at(0, configured).ink != titleInk {
		t.Errorf("the configured team's row reads %q, its name in %+v, want team in text.primary", rows[configured], c.at(0, configured).ink)
	}
	if !strings.HasPrefix(rows[removed], "team ") || c.at(0, removed).ink != faintInk {
		t.Errorf("the removed team's row reads %q, its name in %+v, want team in text.faint", rows[removed], c.at(0, removed).ink)
	}
	said := l.row.weeks[4].At
	if got := string([]rune(rows[removed])[said:]); !strings.HasPrefix(got, "removed 4 Sep") || c.at(said, removed).ink != faintInk {
		t.Errorf("from the week of 7 Sep, the removed team's row reads %q in %+v, want removed 4 Sep in text.faint", got, c.at(said, removed).ink)
	}
	row := []rune(rows[removed])
	if got := string(row[l.dayAt(25)]); got != "▒" {
		t.Errorf("the day team was removed on reads %q, want its shade, ▒", got)
	}
	if got := string(row[l.dayAt(26) : l.dayAt(27)+swatchCells]); strings.TrimSpace(got) != "" {
		t.Errorf("the days after team was removed read %q, want nothing", got)
	}
	if strings.Contains(rows[removed], toCome) || !strings.HasSuffix(rows[removed], "2 5-hour limits") {
		t.Errorf("the removed team's row reads %q, want no day to come, and its 5-hour limits", rows[removed])
	}
}

// Where the account was removed in the last week shown, when is said from the
// day after; where on its last day, or after, it isn't said; and in the
// narrow form, where it was removed before the weeks shown, it's said from
// the first, under its name.
func TestDaysSaysWhenAnAccountWasRemovedWhereItsDaysAreGone(t *testing.T) {
	f := daysFrame(t, daysWorld(teamRemoved(51)), 160, 37)
	f.Look = NoColour()
	rows := textOf(historyDrawn(f).rows(Look{}))
	l := f.layOutDays(*f.History.Days, f.top())
	if got := string([]rune(rows[l.accountRow(0)])[l.dayAt(51):]); !strings.HasPrefix(got, "removed 4") {
		t.Errorf("removed in the last week, from the day after, the row reads %q, want when, cut short of the week's end", got)
	}

	f = daysFrame(t, daysWorld(teamRemoved(56)), 160, 37)
	f.Look = NoColour()
	if row := textOf(historyDrawn(f).rows(Look{}))[l.accountRow(0)]; strings.Contains(row, "removed") {
		t.Errorf("removed on the last day shown, the row reads %q, want nothing said", row)
	}

	page := daysWorld(teamRemoved(26))
	page.ByModel = page.ByModel[:1]
	f = daysFrame(t, page, 52, 35)
	f.Look = NoColour()
	narrow := textOf(historyDrawn(f).rows(Look{}))
	named := slices.IndexFunc(narrow, func(row string) bool { return strings.HasPrefix(row, "team ") })
	if named < 0 || narrow[named+1] != "removed 4 Sep" {
		t.Errorf("in the narrow form, the removed team reads\n%s\nwant its days under its name to read removed 4 Sep alone", strings.Join(narrow, "\n"))
	}
}

// Tokens names an account removed by the label it showed last in
// text.faint, on its row and among the accounts a version ran on.
func TestTokensNamesAnAccountRemovedInTextFaint(t *testing.T) {
	removed := views.AccountTokens{Account: "h7c2", Label: "team", Removed: true}
	row := tokensRow{}.ofAccount(removed)
	if got := row.name[0].said; len(got) != 2 || got[1].text != "team" || got[1].ink != faintInk {
		t.Errorf("the removed team's row is named %+v, want team in text.faint", got)
	}
	if got := (tokensRow{}).ofAccount(views.AccountTokens{Account: "work", Label: "team"}).name[0].said[1].ink; got != secondaryInk {
		t.Errorf("the configured team's row is named in %+v, want text.secondary", got)
	}
	on := tokensRow{}.on([]string{"work", "h7c2"}, tokensAccounts([]views.AccountTokens{{Account: "work", Label: "team"}, removed}))
	want := line{{"team", dimInk}, {" ", dimInk}, {"team", faintInk}}
	if got := on.of[0].said; !slices.Equal(got, want) {
		t.Errorf("a version ran on %+v, want %+v", got, want)
	}
}

// Detail's LATELY names an account removed by the label it showed last, in
// text.faint, and one neither the moment nor the record knows by its id.
func TestDetailNamesAnAccountRemovedByItsLastLabel(t *testing.T) {
	removed := status.RemovedAccounts{{ID: "h7c2", Label: "team"}}
	if got := detailNamed(status.Moment{}, removed, "h7c2"); got != (span{"team", faintInk}) {
		t.Errorf("detailNamed(h7c2) = %+v, want team in text.faint", got)
	}
	if got := detailNamed(status.Moment{}, removed, "zzzz"); got != (span{"zzzz", mutedInk}) {
		t.Errorf("detailNamed(zzzz) = %+v, want its id in text.muted", got)
	}
}
