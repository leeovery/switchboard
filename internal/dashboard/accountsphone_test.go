package dashboard

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// phoneDays are where accounts-phone-detail-52 draws BY DAY's days, its row
// and the column its first day is at.
const (
	phoneDaysRow = 27
	phoneDaysAt  = 10
)

// phonePage is Accounts' page of the phone's frames, as Compare's frames have
// it, this week, its six weeks to the frames' week; each row's last three
// weeks of days as accounts-phone-detail-52 draws work's, read off its cells
// as nord shades each level, the weeks before them of no use, and its last
// three days to come; and each row's peaks, work's.
func phonePage(t *testing.T) *views.AccountsPage {
	t.Helper()
	nord := Screen(builtin(t, "nord"))
	page := comparedPage(views.AccountsThisWeek, comparedVersions(t), compareWork, comparePersonal, compareSide, compareAll)
	page.Weeks = []string{"2026-08-24", "2026-08-31", "2026-09-07", "2026-09-14", "2026-09-21", "2026-09-28"}
	page.ToCome = 3
	frame := reference(t, "accounts-phone-detail-52")
	levels := make(map[shade]int)
	for level := range shadeLevels {
		levels[shadeOf(nord.colour(dayInk(level)))] = level
	}
	days := make([]views.StripDay, 7*stackWeeks, 7*2*stackWeeks)
	for i := range 7 * stackWeeks {
		cell := seenAt(frame, phoneDaysAt+i/7*stackLayout.step()+i%7*swatchCells, phoneDaysRow)
		days = append(days, views.StripDay{Level: levels[cell.bg], Limit: cell.glyph == '▪'})
	}
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
	f.View, f.Look, f.WeekWindow = Accounts, Screen(builtin(t, "nord")), "7d"
	f.Accounts = AccountsShown{Shown: shown, Accounts: threeFramed, Chosen: chosen, Page: page}
	return f
}

// phoneDoc is the world of the phone's frames: the Overview's, on auto,
// every account's extra usage read off.
func phoneDoc() status.Document {
	return usageWorld(extraOff, func(d *status.Document) { d.Events = onAuto() })
}

// The cells the phone's frames draw otherwise than the design, or than
// their world.
var (
	// phoneNoBest are side's ▲, and the blank before it, on each of its rows
	// of the phone's Compare, which Accounts doesn't show, as it shows no
	// routing (P1).
	phoneNoBest = []spot{{14, 5, 6}, {24, 5, 6}, {31, 5, 6}, {37, 5, 6}, {43, 5, 6}}
	// phoneOlderHaiku is Haiku 4.5's part of personal's and all accounts'
	// bars, which the phone's Compare frame draws in Haiku's own shade, as
	// olderHaiku has it.
	phoneOlderHaiku = []spot{{22, 32, 32}, {26, 32, 32}}
	// stackedOlderHaiku is Haiku 4.5's part of work's bar on the phone's
	// Detail, and its ●, drawn as phoneOlderHaiku's are.
	stackedOlderHaiku = []spot{{23, 49, 49}, {25, 31, 31}}
	// stackedMovedOn is work's LATELY's last row, where the frame tells its
	// prime at 12:10 and leaves out db8a's move onto it at 13:22, which
	// LATELY tells from work's side, as any move onto it (P12).
	stackedMovedOn = spot{37, 0, 51}
	// stackedBest is ", the best" after 7f3a's start on work's LATELY,
	// which the frame leaves out, as startedBest has it.
	stackedBest = spot{36, 26, 35}
)

// A phone's Compare and Detail are drawn as their frames, cell for cell, in
// nord: Compare turned, a group at a time, an account a row, all accounts
// last; Detail stacked, each part under its heading. Each passes over the
// cells it draws otherwise than its frame, for the reason given.
func TestAPhonesAccountsAreDrawnAsTheirFrames(t *testing.T) {
	page := phonePage(t)
	tests := []struct {
		stem   string
		height int
		shown  AccountsView
		chosen string
		in     region
		passed []spot
	}{
		{
			stem: "accounts-phone-compare-52", height: 48, shown: AccountsCompare, in: region{top: 8, bottom: 46, right: 51},
			passed: append(phoneNoBest, phoneOlderHaiku...),
		},
		{
			stem: "accounts-phone-detail-52", height: 41, shown: AccountsDetail, chosen: "work", in: region{top: 8, bottom: 39, right: 51},
			passed: append(stackedOlderHaiku, stackedMovedOn, stackedBest),
		},
	}
	for _, tt := range tests {
		t.Run(tt.stem, func(t *testing.T) {
			f := phoneFrame(t, tt.height, tt.shown, tt.chosen, page)
			drawn := screenOf(t, f.Draw(phoneDoc(), routedAt))
			heldTo(t, tt.stem, reference(t, tt.stem), drawn, tt.in, shadeOf(f.Look.Canvas()), tt.passed, nil)
		})
	}
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
		{name: "past the foot, as far as it goes", scroll: 40, want: map[int]string{27: " all         $600/mo      $635      4.6×", 28: "▲ 17 more above · j/k"}},
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
			29: " work        —         —           —",
			35: " work        —            —         0%",
			41: " work        $200/mo      —         —",
			44: " all         $600/mo      —         —",
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
		14: "Week     ██████████┃         59%  1.2 of 3 left",
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

// A phone's Compare cuts each figure a cell short of the next column, and
// the last at the frame's edge.
func TestAPhonesCompareCutsEachFigureACellShortOfTheNext(t *testing.T) {
	page := phonePage(t)
	page.Rows[0].Totals.Requests = 12345678
	rows := textOf(phoneText(t, 52, 48, AccountsCompare, "", page, nil).Draw(phoneDoc(), routedAt))
	if got, want := rows[29], " work        …         9           3 out"; got != want {
		t.Errorf("work's requests are %q, want %q, 12,345,678 a cell too wide", got, want)
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
