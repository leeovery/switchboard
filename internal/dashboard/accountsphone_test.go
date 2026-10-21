package dashboard

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// phoneDays are each row's last three weeks of days on the phone's Detail:
// each day's level, and whether a limit was reached on it.
var phoneDays = []views.StripDay{
	{Level: 3, Limit: true}, {Level: 3, Limit: true}, {Level: 4, Limit: true}, {Level: 4}, {Level: 2, Limit: true}, {Level: 1}, {Level: 1},
	{Level: 4, Limit: true}, {Level: 3, Limit: true}, {Level: 3, Limit: true}, {Level: 4, Limit: true}, {Level: 4, Limit: true}, {Level: 1}, {Level: 1},
	{Level: 2}, {Level: 2, Limit: true}, {Level: 2}, {Level: 2, Limit: true}, {}, {}, {},
}

// phonePage is Accounts' page of the phone's frames, as Compare's frames have
// it, this week, its six weeks to the frames' week; each row's last three
// weeks of days phoneDays, the weeks before them of no use, and its last
// three days to come; and each row's peaks, work's.
func phonePage(t *testing.T) *views.AccountsPage {
	t.Helper()
	page := comparedPage(views.AccountsThisWeek, comparedVersions(t), compareWork, comparePersonal, compareSide, compareAll)
	page.Weeks = []string{"2026-08-24", "2026-08-31", "2026-09-07", "2026-09-14", "2026-09-21", "2026-09-28"}
	page.ToCome = 3
	days := slices.Concat(make([]views.StripDay, 7*stackWeeks), phoneDays)
	var peaks []views.WeekPeak
	for i, p := range []float64{1, 0.94, 1, 0.81, 1, 0.34} {
		peaks = append(peaks, views.WeekPeak{Share: new(p), Open: i == 5})
	}
	for i := range page.Rows {
		page.Rows[i].Days, page.Rows[i].Peaks = days, peaks
	}
	page.All.Days, page.All.Peaks = days, peaks
	return &page
}

// phoneFrame is a frame of Accounts at routedAt, 52 columns by the rows
// given, in nord, of work, personal and side, their week 7d, the sub-tab
// shown, the account chosen and the page given.
func phoneFrame(t *testing.T, height int, shown AccountsView, chosen string, page *views.AccountsPage) Frame {
	f := frameOf(52, height)
	f.View, f.Look, f.WeekWindow, f.VersionNames = Accounts, Screen(builtin(t, "nord")), "7d", views.VersionNameCells(ledger.Pricing)
	f.Accounts = AccountsShown{Shown: shown, Accounts: threeFramed, Chosen: chosen, Page: page}
	return f
}

// phoneDoc is the world of the phone's frames: the Overview's, on auto,
// every account's extra usage read off.
func phoneDoc() status.Document {
	return usageWorld(extraOff, func(d *status.Document) { d.Events = onAuto() })
}

// phoneText is a frame of Accounts at routedAt, as text alone, as
// phoneFrame has it, of a terminal width by height, the router listing the
// sessions given.
func phoneText(t *testing.T, width, height int, shown AccountsView, chosen string, page *views.AccountsPage, sessions []status.Session) Frame {
	f := phoneFrame(t, height, shown, chosen, page)
	f.Width, f.Look, f.Sessions = width, Look{}, sessions
	return f
}

// rowsFrom are the rows drawn, from the first given, each trimmed at its
// right.
func rowsFrom(rows []string, first int) []string {
	return trimmed(rows, first, len(rows)-1)
}

// A phone's Compare taller than the screen scrolls a row at a time, saying
// over the key line how many rows are out of view, and how to scroll to
// them, as far as its last row and no further.
func TestAPhonesCompareScrollsByRows(t *testing.T) {
	tests := []struct {
		name   string
		scroll int
		want   map[int]string
	}{
		{name: "at the top", want: map[int]string{8: "WEEK USED", 10: " work        ██████▊    ┃       ╎  34%", 28: "▼ 17 more below · j/k"}},
		{name: "part-way", scroll: 4, want: map[int]string{8: " personal ◆  █████████████┃        66%", 28: "▲ 4 more above · ▼ 13 more below · j/k"}},
		{name: "past the foot, as far as it goes", scroll: 40, want: map[int]string{27: " all         $600/mo      $635         4.6×", 28: "▲ 17 more above · j/k"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := phoneText(t, 52, 30, AccountsCompare, "", phonePage(t), nil)
			f.Scroll = tt.scroll
			rows := textOf(f.Draw(phoneDoc(), routedAt))
			for y, want := range tt.want {
				if rows[y] != want {
					t.Errorf("row %d is %q, want %q", y, rows[y], want)
				}
			}
		})
	}
	f := phoneText(t, 52, 30, AccountsCompare, "", phonePage(t), nil)
	if got, want := f.Scrolling(phoneDoc(), routedAt), (Scrolling{Most: 17, Page: 20}); got != want {
		t.Errorf("a phone's Compare at 30 rows scrolls %+v, want %+v", got, want)
	}
	f.Height = 46
	if rows := textOf(f.Draw(phoneDoc(), routedAt)); rows[44] != "▼ 1 more below · j/k" {
		t.Errorf("a row short of room for every row, the row over the key line is %q, want it to say one more", rows[44])
	}
	f.Height = 47
	if rows := textOf(f.Draw(phoneDoc(), routedAt)); rows[45] != "" {
		t.Errorf("with room for every row, the row over the key line is %q, want it blank", rows[45])
	}
}

// Before the period's days are read, a phone's Compare draws what's live
// alone, WEEK USED; over another period than this week, WEEK USED says it's
// now.
func TestAPhonesCompareDrawsWEEKUSEDAloneTillItsDaysAreRead(t *testing.T) {
	f := phoneText(t, 52, 48, AccountsCompare, "", nil, nil)
	f.Accounts.Period = views.AccountsThirtyDays
	got := rowsFrom(textOf(f.Draw(phoneDoc(), routedAt)), 8)
	want := []string{"WEEK USED  now", "", " work", "", " personal ◆", "", " side", "", " all"}
	for i, w := range want {
		if !strings.HasPrefix(got[i], w) {
			t.Errorf("row %d is %q, want it to start %q", 8+i, got[i], w)
		}
	}
	if rest := strings.Join(got[len(want):len(got)-1], ""); rest != "" {
		t.Errorf("under WEEK USED, it draws %q, want nothing till the days are read", rest)
	}
}

// A phone's WEEK USED shows the week the frame names the accounts' own, as
// Compare's does: of every account, then of all accounts.
func TestAPhonesWEEKUSEDShowsTheWeekTheFrameNames(t *testing.T) {
	tests := []struct {
		name, week string
		want       []string
	}{
		{name: "the accounts' week", week: "7d", want: []string{"34%", "66%", "78%", "59%"}},
		{name: "Fable's week named so", week: "7d_oi", want: []string{"12%", "0%", "4%", "5%"}},
		{name: "none named", want: []string{"—", "—", "—", "—"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := phoneText(t, 52, 48, AccountsCompare, "", nil, nil)
			f.WeekWindow = tt.week
			week := strings.Join(rowsFrom(textOf(f.Draw(phoneDoc(), routedAt)), 9)[:8], "\n")
			if got := percentages.FindAllString(week, -1); !slices.Equal(got, tt.want) {
				t.Errorf("WEEK USED reads %q, want %q", got, tt.want)
			}
		})
	}
}

// A period with nothing recorded, as on a fresh install or a week's first
// morning, says a dash in each of its own figures on a phone too, Compare's
// and Detail's THIS WEEK's; left at its last reset, the plan and extra
// usage, no figures of the period, say what they are.
func TestAPhonesAccountsSayADashForEachFigureOfAPeriodWithNothingRecorded(t *testing.T) {
	page := phonePage(t)
	page.Recorded = false
	tests := []struct {
		shown AccountsView
		want  map[int]string
	}{
		{shown: AccountsCompare, want: map[int]string{
			29: " work        —            —            —",
			35: " work        —            —            0%",
			41: " work        $200/mo      —            —",
			44: " all         $600/mo      —            —",
		}},
		{shown: AccountsDetail, want: map[int]string{
			19: "requests  —               worth     —",
			20: "limits    —               extra use off",
			21: "plan      Max 20x         against   —",
		}},
	}
	for _, tt := range tests {
		f := phoneText(t, 52, 48, tt.shown, "work", page, nil)
		rows := textOf(f.Draw(phoneDoc(), routedAt))
		for y, want := range tt.want {
			if rows[y] != want {
				t.Errorf("row %d is %q, want %q", y, rows[y], want)
			}
		}
	}
}

// A phone's Detail of all accounts picks all accounts out between ◂ and ▸,
// counts its accounts' states under it, and words each window's room, to
// each account's limit, of how many accounts it counts; its THIS WEEK says
// how many limits, the plans summed, and extra usage taken together.
func TestAPhonesDetailOfAllAccounts(t *testing.T) {
	rows := textOf(phoneText(t, 52, 60, AccountsDetail, "", phonePage(t), nil).Draw(phoneDoc(), routedAt))
	for y, want := range map[int]string{
		8:  "◂ 0 all accounts ▸             ↑↓ an account, or all",
		9:  "2 open · 1 at cap",
		14: "5-hour   ████████┃           49%  1.5 of 3 left",
		19: "requests  5,410           worth     $635",
		20: "limits    3               extra use off",
		21: "plan      $600/mo         against   4.6×",
		31: "room to spare together",
		32: "30% left at a reset",
		35: "14:41  ▲ c61b started on side, the best",
	} {
		if rows[y] != want {
			t.Errorf("row %d is %q, want %q", y, rows[y], want)
		}
	}
}

// Where an account's plan isn't known, a phone's against it, on Compare, 12
// cells from column 39 at 52 columns, and against, on Detail, 15 cells from
// column 36, say every account's ratio over the plans known, compact, the
// plans known over the accounts: "4.6× · 2/3"; with every plan known, the
// ratio alone (the owner's call, 15 October 2026).
func TestAPhonesAgainstSaysTheRatioOverThePlansKnownCompact(t *testing.T) {
	for _, tt := range []struct {
		name  string
		known views.PlansKnown
		want  string
	}{
		{name: "a plan not known", known: views.PlansKnown{Known: 2, Of: 3}, want: "4.6× · 2/3"},
		{name: "every plan known", known: views.PlansKnown{Known: 3, Of: 3}, want: "4.6×"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			page := phonePage(t)
			page.All.Plan.Plans = tt.known
			compare := textOf(phoneText(t, 52, 60, AccountsCompare, "", page, nil).Draw(phoneDoc(), routedAt))
			if want := " all         $600/mo      $635         " + tt.want; !slices.Contains(compare, want) {
				t.Errorf("Compare reads\n%s\nwant a row %q", strings.Join(compare, "\n"), want)
			}
			detail := textOf(phoneText(t, 52, 60, AccountsDetail, "", page, nil).Draw(phoneDoc(), routedAt))
			if want := "plan      $600/mo         against   " + tt.want; detail[21] != want {
				t.Errorf("Detail's row 21 is %q, want %q", detail[21], want)
			}
		})
	}
}

// All accounts' Detail on a phone counts the configured accounts' states:
// one the document lacks yet as not read, and one the document holds and the
// config doesn't, not at all.
func TestAPhonesDetailCountsTheConfiguredAccountsStates(t *testing.T) {
	tests := []struct {
		name       string
		configured config.Accounts
		want       string
	}{
		{name: "one added the document lacks", configured: append(slices.Clone(threeFramed), config.Account{ID: "added"}), want: "2 open · 1 at cap · 1 not read"},
		{name: "one the config no longer holds", configured: threeFramed[1:], want: "2 open"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := phoneText(t, 52, 41, AccountsDetail, "", nil, nil)
			f.Accounts.Accounts = tt.configured
			if got := textOf(f.Draw(phoneDoc(), routedAt))[9]; got != tt.want {
				t.Errorf("all accounts' states read %q, want %q", got, tt.want)
			}
		})
	}
}

// An account the config has and the document lacks yet is picked on a
// phone's Detail as the config names it, and stands not read.
func TestAPhonesDetailOfAnAccountNotReadYet(t *testing.T) {
	f := phoneText(t, 52, 41, AccountsDetail, "added", nil, nil)
	f.Accounts.Accounts = append(slices.Clone(threeFramed), config.Account{ID: "added"})
	rows := textOf(f.Draw(phoneDoc(), routedAt))
	if !strings.HasPrefix(rows[8], "◂ 4 added ▸") || rows[9] != "○ not read" {
		t.Errorf("Detail of an account not read yet heads\n%s\n%s", rows[8], rows[9])
	}
}

// With one account, a phone's Detail has no other to pick: its name stands
// alone, with neither ◂ and ▸ nor the keys that pick.
func TestAPhonesDetailOfOneAccountNamesItAlone(t *testing.T) {
	f := phoneText(t, 52, 41, AccountsDetail, "", nil, nil)
	f.Accounts.Accounts = threeFramed[:1]
	if got := textOf(f.Draw(phoneDoc(), routedAt))[8]; got != "1 work" {
		t.Errorf("the picker is %q, want %q", got, "1 work")
	}
}

// The picker's keys stand at the right where they fit beside it, as far
// right as the frame is wide, and go where they don't.
func TestAPhonesPickerSaysItsKeysWhereTheyFit(t *testing.T) {
	tests := []struct {
		width int
		want  string
	}{
		{width: 52, want: "◂ 2 personal ◆ ▸               ↑↓ an account, or all"},
		{width: 60, want: "◂ 2 personal ◆ ▸                       ↑↓ an account, or all"},
		{width: 38, want: "◂ 2 personal ◆ ▸ ↑↓ an account, or all"},
		{width: 37, want: "◂ 2 personal ◆ ▸"},
	}
	for _, tt := range tests {
		f := phoneText(t, tt.width, 41, AccountsDetail, "personal", nil, nil)
		if got := textOf(f.Draw(phoneDoc(), routedAt))[8]; got != tt.want {
			t.Errorf("at %d columns, the picker is %q, want %q", tt.width, got, tt.want)
		}
	}
}

// moreOnSide are the Overview's sessions, and two more on side, idle, six
// running there in all.
func moreOnSide() []status.Session {
	sessions := overviewSessions()
	for _, id := range []string{"e04f2a71-6b3c-4d85-9e10-a2b3c4d5e6f7", "f19b3c82-7c4d-4e96-8f21-b3c4d5e6f7a8"} {
		seat := status.Assignment{Model: opus, Family: "opus", Account: "side", Reason: status.ReasonNew, AssignedAt: routedClock(1, 14, 0), LastSeen: routedAt.Add(-10 * time.Minute)}
		sessions = append(sessions, status.Session{ID: id, Assignments: []status.Assignment{seat}})
	}
	return sessions
}

// With sessions running on the account, a phone's Detail stacks SESSIONS
// before LATELY (C18): ▸ before the one selected, the first where none is;
// as many as four rows show, scrolled with the one selected, the last saying
// how many more are below; and two rows under them, what enter opens. A
// click on a session's row selects it, where the stack has it scrolled.
func TestAPhonesDetailStacksSESSIONSBeforeLATELY(t *testing.T) {
	_, _, stream := overviewWorld()
	f := phoneText(t, 52, 60, AccountsDetail, "side", phonePage(t), overviewSessions())
	f.Stream = stream
	rows := textOf(f.Draw(phoneDoc(), routedAt))
	at := slices.Index(rows, "SESSIONS  now")
	if at < 0 {
		t.Fatalf("no SESSIONS among\n%s", strings.Join(rows, "\n"))
	}
	want := []string{
		"SESSIONS  now",
		"▸ ● c61b  opus    ↓ ~1.2k  now",
		"  ● d28c  opus    ↑ ask    now",
		"  ○ 7f3a  sonnet  idle      1m",
		"  ○ db8a  opus    idle      4m",
		"",
		"  enter opens c61b's page",
		"",
		"LATELY",
	}
	if got := trimmed(rows, at, at+len(want)-1); !slices.Equal(got, want) {
		t.Errorf("SESSIONS on side is\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	f.Sessions, f.Accounts.Selected = moreOnSide(), idDB8A
	rows = textOf(f.Draw(phoneDoc(), routedAt))
	if got := trimmed(rows, at+1, at+6); got[0] != "  ● d28c  opus    ↑ ask    now" || got[2] != "▸ ○ db8a  opus    idle      4m" || got[3] != "  +2 more" || got[5] != "  enter opens db8a's page" {
		t.Errorf("SESSIONS with db8a selected of six is\n%s", strings.Join(got, "\n"))
	}
	f.Accounts.Selected = moreOnSide()[len(moreOnSide())-1].ID
	rows = textOf(f.Draw(phoneDoc(), routedAt))
	if got := trimmed(rows, at+1, at+4); got[0] != "  ○ 7f3a  sonnet  idle      1m" || !strings.HasPrefix(got[3], "▸ ○ f19b") {
		t.Errorf("SESSIONS with the last of six selected is\n%s\nwant the last four, none more", strings.Join(got, "\n"))
	}

	f.Height, f.Scroll, f.Accounts.Selected = 30, 10, idDB8A
	var on []Target
	for _, target := range f.Targets(phoneDoc(), routedAt) {
		if target.On == OnSession {
			on = append(on, target)
		}
	}
	top := f.At(phoneDoc(), routedAt).top()
	if len(on) != 3 || on[0].Session != idD28C || on[0].Y != top+at-8-10+1 || on[0].X != 0 || on[0].Width != sessionLit {
		t.Errorf("the session targets scrolled 10 rows are %+v, want d28c's first on row %d, %d wide", on, top+at-8-10+1, sessionLit)
	}
}

// Where the frame is too narrow for Detail's wide form, its body is the
// phone's stack under the wide chrome, its right-aligned parts at the
// frame's width.
func TestDetailsPhoneBodyFollowsTheWidth(t *testing.T) {
	f := phoneText(t, 120, 50, AccountsDetail, "work", phonePage(t), nil).At(phoneDoc(), routedAt)
	c := newCanvas(f.Width, f.Height)
	top := f.top()
	f.detailPhoneBody(phoneDoc(), routedAt).draw(c, top)
	rows := textOf(c.rows(Look{}))
	if want := "◂ 1 work ▸" + strings.Repeat(" ", 89) + "↑↓ an account, or all"; rows[top] != want {
		t.Errorf("the picker at 120 columns is %q, want %q", rows[top], want)
	}
	if !strings.HasPrefix(rows[top+15], "BY MODEL  ") {
		t.Errorf("row %d is %q, want BY MODEL, as on a phone", top+15, rows[top+15])
	}
}

// A phone's bars ease after a read as the wide forms' do, each account's by
// its id and its window's key, and all accounts' by the key alone; and what
// a look reads brightens as it changes: each account's week's percentage,
// all accounts' and their room each a figure of its own, and their states
// counted; and the period's figures, each by its row.
func TestAPhonesBarsEaseAndItsLookReadNumbersBrighten(t *testing.T) {
	compare := phoneText(t, 52, 48, AccountsCompare, "", phonePage(t), nil)
	all := compare.Moment(phoneDoc(), routedAt).All().Windows
	week := all[slices.IndexFunc(all, func(w status.AllWindowNow) bool { return w.Key == "7d" })].Used
	want := map[Ref]float64{{Account: "work", Window: "7d"}: 0.34, {Account: "personal", Window: "7d"}: 0.66, {Account: "side", Window: "7d"}: 0.78, {Window: "7d"}: week}
	if got := compare.Filled(phoneDoc(), routedAt); !maps.Equal(got, want) {
		t.Errorf("a phone's Compare's bars fill %v, want %v", got, want)
	}
	got := compare.Numbers(phoneDoc(), routedAt)
	delete(got, Number{Kind: NumberRouted})
	if said := got[Number{Kind: NumberPeriod, Account: "work", Of: "reqs"}]; said != "1,204" {
		t.Errorf("a phone's Compare numbers work's requests %q, want 1,204", said)
	}
	maps.DeleteFunc(got, func(n Number, _ string) bool { return n.Kind == NumberPeriod })
	wantNumbers := map[Number]string{
		{Kind: NumberUsed, Account: "work", Of: "7d"}: "34%", {Kind: NumberUsed, Account: "personal", Of: "7d"}: "66%",
		{Kind: NumberUsed, Account: "side", Of: "7d"}: "78%", {Kind: NumberAllUsed, Of: "7d"}: "59%",
	}
	if !maps.Equal(got, wantNumbers) {
		t.Errorf("a phone's Compare's numbers are %v, want %v", got, wantNumbers)
	}

	detail := phoneText(t, 52, 60, AccountsDetail, "", phonePage(t), nil)
	got = detail.Numbers(phoneDoc(), routedAt)
	for _, n := range []Number{
		{Kind: NumberAllUsed, Of: "7d"}, {Kind: NumberAllRoom, Of: "7d"}, {Kind: NumberAllUsed, Of: "5h"},
		{Kind: NumberAccountStates, Of: "open"}, {Kind: NumberPeriod, Of: "requests"},
	} {
		if _, ok := got[n]; !ok {
			t.Errorf("a phone's Detail of all accounts numbers no %+v among %v", n, got)
		}
	}
	for n := range got {
		if n.Kind == NumberUsed || n.Kind == NumberRoom {
			t.Errorf("a phone's Detail of all accounts numbers %+v, the pool's kind", n)
		}
	}
	if filled := detail.Filled(phoneDoc(), routedAt); filled[Ref{Window: "7d"}] != week || filled[Ref{Account: "work", Window: "7d"}] != 0.34 {
		t.Errorf("a phone's Detail of all accounts fills %v, want all accounts' week, %v, and every account's", filled, week)
	}
}

// The account's state is cued on a phone's Detail as its word changes, as
// Detail's head is, and not at a change ROUTING alone sees.
func TestAPhonesDetailCuesTheStateAsItsWordChanges(t *testing.T) {
	tests := []struct {
		name          string
		changed, word map[string]float64
		cued          bool
	}{
		{name: "its word changed", word: map[string]float64{"work": 0.25}, cued: true},
		{name: "ROUTING's state alone changed", changed: map[string]float64{"work": 0.25}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := phoneFrame(t, 41, AccountsDetail, "work", phonePage(t))
			f.Changed, f.StateChanged = tt.changed, tt.word
			c := f.At(phoneDoc(), routedAt).page(phoneDoc(), routedAt)
			for x := range 20 {
				cued := c.at(x, 9).ink.on.token == theme.BgAttention
				if want := tt.cued && x < ansi.StringWidth("● at its cap"); cued != want {
					t.Errorf("column %d of the state cued: %v, want %v", x, cued, want)
				}
			}
		})
	}
}

// A phone's BY MODEL names the versions over two rows: as many as fit on the
// first, in the table's order; the rest on the second, the smallest left
// out where they don't all fit, and how many more there are.
func TestAPhonesBYMODELNamesTheVersionsOverTwoRows(t *testing.T) {
	versions := comparedVersions(t)
	tests := []struct {
		name   string
		shares []int
		width  int
		want   []string
	}{
		{name: "one row", shares: []int{82, -1, 16, -1, -1, -1}, width: 51, want: []string{"● Opus 5.5 82%  ● Sonnet 5.5 16%"}},
		{
			name: "two rows", shares: []int{55, 7, 28, 3, 5, 2}, width: 51,
			want: []string{"● Opus 5.5 55%  ● Opus 5 7%  ● Sonnet 5.5 28%", "● Sonnet 5 3%  ● Fable 5.1 5%  ● Haiku 4.5 2%"},
		},
		{
			name: "the smallest left out", shares: []int{55, 7, 28, 3, 5, 2}, width: 30,
			want: []string{"● Opus 5.5 55%  ● Opus 5 7%", "● Sonnet 5.5 28%  +3 more"},
		},
		{
			name: "each row as wide as the width", shares: []int{55, 7, 28, 3, 5, 2}, width: 45,
			want: []string{"● Opus 5.5 55%  ● Opus 5 7%  ● Sonnet 5.5 28%", "● Sonnet 5 3%  ● Fable 5.1 5%  ● Haiku 4.5 2%"},
		},
		{name: "none", shares: []int{-1, -1, -1, -1, -1, -1}, width: 51},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, l := range versionsOver(versionsUsed(compareFramed{shares: tt.shares}.row(), versions), tt.width) {
				got = append(got, l.plain())
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("the versions read %q, want %q", got, tt.want)
			}
		})
	}
}

// The version a row used most is its largest share's, the first in the
// table's order of those alike; none where it used none, or none's share is
// known.
func TestTheVersionARowUsedMostIsItsLargestShares(t *testing.T) {
	versions := comparedVersions(t)
	unknown := compareWork.row()
	unknown.Totals.ByModel = unknown.Totals.ByModel[:1]
	unknown.Totals.ByModel[0].Share = nil
	if most, ok := largestVersion(versionsUsed(unknown, versions)); ok {
		t.Errorf("of one used, its share not known, the most used is %q, want none", most.version.Name)
	}
	if most, ok := largestVersion(versionsUsed(compareFramed{shares: []int{40, -1, 40, 20, -1, -1}}.row(), versions)); !ok || most.version.Name != "Opus 5.5" {
		t.Errorf("of two alike, the most used is %q, want Opus 5.5, the first", most.version.Name)
	}
	if most, ok := largestVersion(versionsUsed(compareFramed{shares: []int{-1, -1, -1, -1, -1, -1}}.row(), versions)); ok {
		t.Errorf("of none used, the most used is %q, want none", most.version.Name)
	}
}

// A phone's Compare lays its parts across the width: its names at their
// widest whole, at most 24 cells, the bars turnedApart after them; WEEK
// USED's and BY MODEL's bars one width, from 20 growing with the room to 40
// at most; their figures in one column, WEEK USED's percentage right-aligned
// in the 4 cells after its bar and BY MODEL's largest version two cells
// after it; and each group's columns a third of the room from the bars to
// the frame's edge, every group alike.
func TestAPhonesCompareLaysItsPartsAcrossTheWidth(t *testing.T) {
	long := map[string]string{"work": "work-team-plan", "personal": "personal-research", "side": "side-hustle-consulting"}
	tests := []struct {
		name         string
		width        int
		labels       map[string]string
		at, bar      int
		groups       []int
		named, would string
	}{
		{name: "at 52, its bars at their least", width: 52, at: 13, bar: 20, groups: []int{13, 26, 39}, named: " side        "},
		{name: "at 73, its bars at their most", width: 73, at: 13, bar: 40, groups: []int{13, 33, 53}, named: " side        "},
		{name: "at 99, its bars held at their most", width: 99, at: 13, bar: 40, groups: []int{13, 42, 70}, named: " side        "},
		{name: "at 73, long names whole", width: 73, labels: long, at: 25, bar: 28, groups: []int{25, 41, 57}, named: " side-hustle-consulting  "},
		{name: "at 52, long names cut, the figures whole", width: 52, labels: long, at: 13, bar: 20, groups: []int{13, 26, 39}, named: " side-hust…  "},
		{name: "at 60, long names cut, the figures whole", width: 60, labels: long, at: 21, bar: 20, groups: []int{21, 34, 47}, named: " side-hustle-consu…  "},
		{name: "at 66, long names whole, the figures whole", width: 66, labels: long, at: 25, bar: 21, groups: []int{25, 39, 52}, named: " side-hustle-consulting  "},
		{
			name: "at 73, a name past the most cut to it", width: 73,
			labels: map[string]string{"side": "a-label-well-past-the-most-of-them"}, at: 27, bar: 26, groups: []int{27, 42, 57},
			named: " a-label-well-past-the-m…  ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := phoneText(t, tt.width, 60, AccountsCompare, "", phonePage(t), nil)
			f.Accounts.Accounts = slices.Clone(threeFramed)
			doc := phoneDoc()
			for i, a := range f.Accounts.Accounts {
				if label, ok := tt.labels[a.ID]; ok {
					f.Accounts.Accounts[i].Label = label
					account(a.ID, func(a *status.Account) { a.Label = label })(&doc)
				}
			}
			rows := textOf(f.Draw(doc, routedAt))
			week, byModel := rows[10], rows[20]
			if got := columnOf(week, "34%") + len("34%"); got != tt.at+tt.bar+1+percentCells {
				t.Errorf("WEEK USED's percentage ends at column %d, want %d, in %q", got, tt.at+tt.bar+1+percentCells, week)
			}
			if got := columnOf(byModel, "█"); got != tt.at {
				t.Errorf("BY MODEL's bar starts at column %d, want %d, in %q", got, tt.at, byModel)
			}
			if got := strings.Count(byModel, "█"); got != tt.bar {
				t.Errorf("BY MODEL's bar is %d cells, want %d, in %q", got, tt.bar, byModel)
			}
			if got := columnOf(byModel, "●"); got != tt.at+tt.bar+2 {
				t.Errorf("BY MODEL's largest version is at column %d, want %d, in %q", got, tt.at+tt.bar+2, byModel)
			}
			for y, want := range map[int]string{20: "● Opus 5.5 55%", 22: "● Sonnet 5.5 60%"} {
				if !strings.HasSuffix(rows[y], want) {
					t.Errorf("row %d is %q, want its largest version whole, %q", y, rows[y], want)
				}
			}
			if got := ansi.Cut(rows[14], 0, ansi.StringWidth(tt.named)); got != tt.named {
				t.Errorf("side's row starts %q, want %q", got, tt.named)
			}
			for _, group := range []struct {
				row   int
				heads []string
			}{{28, []string{"reqs", "sessions", "moved"}}, {34, []string{"hit", "at cap", "left"}}, {40, []string{"plan", "worth", "against it"}}} {
				for i, head := range group.heads {
					if got := columnOf(rows[group.row], head); got != tt.groups[i] {
						t.Errorf("%s is at column %d, want %d, in %q", head, got, tt.groups[i], rows[group.row])
					}
				}
			}
		})
	}
}

// A phone's Compare holds every column still as the period's page is read:
// its figures' column is sized for the widest largest version any version
// can show, not those the page names, so WEEK USED's names, bars and
// percentages are where they were before it.
func TestAPhonesCompareHoldsStillAsItsPageIsRead(t *testing.T) {
	long := map[string]string{"work": "work-team-plan", "personal": "personal-research", "side": "side-hustle-consulting"}
	for _, width := range []int{52, 60, 66, 73} {
		for _, labels := range []map[string]string{nil, long} {
			f := phoneText(t, width, 60, AccountsCompare, "", nil, nil)
			f.Accounts.Accounts = slices.Clone(threeFramed)
			doc := phoneDoc()
			for i, a := range f.Accounts.Accounts {
				if label, ok := labels[a.ID]; ok {
					f.Accounts.Accounts[i].Label = label
					account(a.ID, func(a *status.Account) { a.Label = label })(&doc)
				}
			}
			before := textOf(f.Draw(doc, routedAt))
			f.Accounts.Page = phonePage(t)
			after := textOf(f.Draw(doc, routedAt))
			for _, y := range []int{10, 12, 14, 16} {
				if before[y] != after[y] {
					t.Errorf("at %d, long names %v, row %d reads %q before the page and %q after", width, labels != nil, y, before[y], after[y])
				}
			}
		}
	}
}

// A largest version that doesn't fit its cells cuts its name, never its
// share: a share of 100%, and a model the version table doesn't name, by its
// id.
func TestALargestVersionThatDoesntFitCutsItsNameNeverItsShare(t *testing.T) {
	sonnet, ok := ledger.Pricing.Version("claude-sonnet-5-5")
	if !ok {
		t.Fatal("the version table names no claude-sonnet-5-5")
	}
	unnamed := ledger.Version{IDs: []string{"claude-sonnet-4-20250514"}, Name: ledger.ShortModelID("claude-sonnet-4-20250514"), Family: "sonnet"}
	tests := []struct {
		name    string
		version ledger.Version
		share   float64
		cells   int
		want    string
	}{
		{name: "whole where it fits", version: sonnet, share: 1, cells: 17, want: "● Sonnet 5.5 100%"},
		{name: "a share of 100% a cell over", version: sonnet, share: 1, cells: 16, want: "● Sonnet 5… 100%"},
		{name: "a model the table doesn't name", version: unnamed, share: 0.55, cells: 16, want: "● sonnet-4-… 55%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got strings.Builder
			for _, s := range largestIn(versionUsed{version: tt.version, share: &tt.share}, tt.cells) {
				got.WriteString(s.text)
			}
			if got.String() != tt.want {
				t.Errorf("it reads %q in %d cells, want %q", got.String(), tt.cells, tt.want)
			}
		})
	}
}

// At 52 columns, a 100% share of the widest version keeps its share whole,
// its name cut a cell short to make room.
func TestAPhonesCompareKeepsAHundredPercentShareWhole(t *testing.T) {
	page := phonePage(t)
	sonnet := slices.Index(comparedIDs, "claude-sonnet-5-5")
	page.Rows[0].Totals.ByModel = []views.ModelShare{{Model: comparedIDs[sonnet], Share: new(1.0), Amount: usd(100)}}
	rows := textOf(phoneText(t, 52, 60, AccountsCompare, "", page, nil).Draw(phoneDoc(), routedAt))
	if want := "● Sonnet 5… 100%"; !strings.HasSuffix(rows[20], want) {
		t.Errorf("work's BY MODEL row is %q, want it to end %q", rows[20], want)
	}
}

// Where the config prices a model the version table doesn't name, its id,
// wider than any version's name, sizes the figures' column; on the narrowest
// phone the names keep their least, the bars theirs, and the figures narrow
// to what's left, a model's id cut, its share whole.
func TestAPhonesCompareFitsAConfigPricedModelsIDCutToItsShare(t *testing.T) {
	const id = "claude-sonnet-4-20250514"
	page := phonePage(t)
	page.Versions = append(slices.Clone(page.Versions), ledger.Version{IDs: []string{id}, Name: ledger.ShortModelID(id), Family: "sonnet"})
	page.Rows[0].Totals.ByModel = []views.ModelShare{{Model: id, Share: new(0.55), Amount: usd(55)}}
	f := phoneText(t, 52, 60, AccountsCompare, "", page, nil)
	f.VersionNames = len(ledger.ShortModelID(id))
	rows := textOf(f.Draw(phoneDoc(), routedAt))
	if got := columnOf(rows[20], "█"); got != 11 {
		t.Errorf("work's bar starts at column %d, want 11, its name at its least of 8, in %q", got, rows[20])
	}
	if want := "● sonnet-4-20… 55%"; !strings.HasSuffix(rows[20], want) {
		t.Errorf("work's BY MODEL row is %q, want it to end %q", rows[20], want)
	}
}

// A phone's Compare cuts each figure a cell short of the next column, and
// the last at the frame's edge.
func TestAPhonesCompareCutsEachFigureACellShortOfTheNext(t *testing.T) {
	page := phonePage(t)
	page.Rows[0].Totals.Requests = 1234567890
	rows := textOf(phoneText(t, 52, 48, AccountsCompare, "", page, nil).Draw(phoneDoc(), routedAt))
	if got, want := rows[29], " work        …            9            3 out"; got != want {
		t.Errorf("work's requests are %q, want %q, 1,234,567,890 a cell too wide", got, want)
	}
}

// On a phone, Detail's LATELY tells an account's events in their full words,
// never the short words the Overview's LATELY tells there, cut to 44 cells,
// or short of the frame's edge, where that comes first: work's cap that
// moved its sessions to other accounts, 51 cells in full.
func TestAPhonesDetailTellsLATELYInFullCutTo44(t *testing.T) {
	doc := eventsOf(
		status.Event{At: now.Add(-time.Minute).UTC(), Kind: status.EventMoved, Session: "d28c0b1e", From: "work", To: "side", Reason: "moved: work is at its reserve", ForcedBy: 1},
		status.Event{At: now.Add(-time.Minute).UTC(), Kind: status.EventMoved, Session: "9e21d4a8", From: "work", To: "personal", Reason: "moved: work is at its reserve", ForcedBy: 1},
		status.Event{At: now.Add(-time.Minute).UTC(), Kind: status.EventCap, Account: "work", Count: 2},
	)
	tests := []struct {
		width int
		want  string
	}{
		{width: 52, want: "13:11  ╎ reached its cap: 2 sessions moved to othe…"},
		{width: 60, want: "13:11  ╎ reached its cap: 2 sessions moved to other…"},
	}
	for _, tt := range tests {
		rows := textOf(phoneText(t, tt.width, 41, AccountsDetail, "work", phonePage(t), nil).Draw(doc, now))
		if !slices.Contains(rows, tt.want) {
			t.Errorf("at %d columns, a phone's Detail doesn't tell %q in LATELY:\n%s", tt.width, tt.want, strings.Join(rows, "\n"))
		}
	}
}

// A phone's Detail picks out each event of LATELY while it's fresh.
func TestAPhonesDetailPicksOutAFreshEvent(t *testing.T) {
	f := phoneFrame(t, 41, AccountsDetail, "work", phonePage(t))
	newest := views.AccountLately(f.Moment(phoneDoc(), routedAt), "work", 1)[0]
	f.Fresh = map[int]float64{newest.ID: 0.25}
	c := f.At(phoneDoc(), routedAt).page(phoneDoc(), routedAt)
	if c.at(0, 35).ink.on.token != theme.BgAttention || c.at(0, 36).ink.on.token == theme.BgAttention {
		t.Errorf("the fresh event's row isn't picked out alone")
	}
}
