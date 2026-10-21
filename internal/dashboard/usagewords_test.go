package dashboard

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

func TestAWindowsWordsSayHowItStandsInFullAndShort(t *testing.T) {
	state := func(kind status.WindowKind, at time.Time) status.WindowNow {
		return status.WindowNow{Kind: kind, At: at}
	}
	tests := []struct {
		name        string
		state       status.WindowNow
		allowance   quota.Allowance
		full, short string
		ink         ink
	}{
		{name: "no token", state: state(status.WindowTokenless, time.Time{}), full: "no token", short: "no token", ink: errorInk},
		{name: "nothing read", state: state(status.WindowUnread, time.Time{}), full: "not read yet", short: "not read", ink: dimInk},
		{name: "stale since today", state: state(status.WindowStale, on(1, 14, 20)), full: "not read since 14:20", short: "read 14:20", ink: dimInk},
		{name: "stale since another day", state: state(status.WindowStale, on(30, 22, 10).AddDate(0, -1, 0)), full: "not read since Wed 22:10", short: "read Wed", ink: dimInk},
		{name: "stale since six days back", state: state(status.WindowStale, on(1, 9, 5).AddDate(0, 0, -6)), full: "not read since Fri 09:05", short: "read Fri", ink: dimInk},
		{name: "stale since this day last week, by its date", state: state(status.WindowStale, on(1, 9, 5).AddDate(0, 0, -7)), full: "not read since 24 Sep 09:05", short: "read 24 Sep", ink: dimInk},
		{name: "stale since 9 days back, by its date", state: state(status.WindowStale, on(1, 14, 20).AddDate(0, 0, -9)), full: "not read since 22 Sep 14:20", short: "read 22 Sep", ink: dimInk},
		{name: "at Claude's limit", state: state(status.WindowLimited, on(1, 15, 54)), full: "limit · back 15:54", short: "back 15:54", ink: errorInk},
		{name: "at Claude's limit, back none knows when", state: state(status.WindowLimited, time.Time{}), full: "limit", short: "limit", ink: errorInk},
		{name: "at its cap, back tomorrow", state: state(status.WindowCapped, on(2, 2, 0)), full: "at cap · back Fri 02:00", short: "back Fri", ink: warningInk},
		{name: "running out today", state: state(status.WindowRunsOut, on(1, 16, 5)), full: "out ~16:05", short: "out ~16:05", ink: warningInk},
		{name: "running out another day", state: state(status.WindowRunsOut, on(3, 6, 0)), full: "out ~Sat 06:00", short: "out ~Sat", ink: warningInk},
		{name: "not started", state: state(status.WindowNotStarted, time.Time{}), full: "not started", short: "not started", ink: mutedInk},
		{
			name: "the allowance an hour", state: state(status.WindowOpen, on(1, 16, 30)), allowance: quota.Allowance{Share: 0.339, Per: quota.PerHour},
			full: "34%/h till 16:30", short: "34%/h · 16:30", ink: mutedInk,
		},
		{
			name: "the allowance a day", state: state(status.WindowOpen, on(4, 21, 0)), allowance: quota.Allowance{Share: 0.187, Per: quota.PerDay},
			full: "19%/day till Sun 21:00", short: "19%/day · Sun", ink: mutedInk,
		},
		{
			name: "the room itself, its reset within the span", state: state(status.WindowOpen, on(1, 15, 30)), allowance: quota.Allowance{Share: 0.34},
			full: "34% till 15:30", short: "34% · 15:30", ink: mutedInk,
		},
		{
			name: "an allowance on a half, to the even percent", state: state(status.WindowOpen, on(1, 16, 30)), allowance: quota.Allowance{Share: 0.125, Per: quota.PerHour},
			full: "12%/h till 16:30", short: "12%/h · 16:30", ink: mutedInk,
		},
		{name: "no allowance", state: state(status.WindowOpen, on(1, 16, 30))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := tt.state
			st.Allowance = tt.allowance
			w := wordsOf(st, printNow)
			if got := w.full.plain(); got != tt.full {
				t.Errorf("in full the words read %q, want %q", got, tt.full)
			}
			if got := w.short.plain(); got != tt.short {
				t.Errorf("short the words read %q, want %q", got, tt.short)
			}
			if len(w.full) > 0 && w.full[0].ink != tt.ink {
				t.Errorf("the words lead in %+v, want %+v", w.full[0].ink, tt.ink)
			}
		})
	}
}

func TestWordsAreInFullFrom70ColumnsWhereTheyFitTheirRoom(t *testing.T) {
	w := wordsOf(status.WindowNow{Kind: status.WindowStale, At: on(30, 12, 42).AddDate(0, -1, 0)}, printNow)
	tests := []struct {
		name        string
		width, room int
		want        string
	}{
		{name: "in full, filling their room", width: fullWords, room: 24, want: "not read since Wed 12:42"},
		{name: "short, a cell short of room in full", width: fullWords, room: 23, want: "read Wed"},
		{name: "short under 70 columns, whatever their room", width: fullWords - 1, room: 40, want: "read Wed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := w.pick(tt.width, tt.room).plain(); got != tt.want {
				t.Errorf("at %d columns in %d cells, the words read %q, want %q", tt.width, tt.room, got, tt.want)
			}
		})
	}
}

// The widest words in full are as wide as the widest any window's words say
// within the week, a reading on another day by its weekday, so every full
// form that names a weekday fits them, with its time: a cap's or a limit's
// end on another day, and an allowance of the whole window a day, among
// them. A reading over a week back, by its date, is wider, and takes its
// short form there, which says the date still. Short, a block draws its
// words in shortCells.
func TestTheWidestWordsHoldEveryFullForm(t *testing.T) {
	widest := wordsOf(status.WindowNow{Kind: status.WindowStale, At: on(30, 22, 10).AddDate(0, -1, 0)}, printNow).full
	if got := fullWidest(); got != 24 || got != widest.width() {
		t.Errorf("the widest words in full are %d cells, want 24, of %q", got, widest.plain())
	}
	weekOld := wordsOf(status.WindowNow{Kind: status.WindowStale, At: on(1, 14, 20).AddDate(0, 0, -9)}, printNow)
	if got := weekOld.pick(fullWords, fullWidest()).plain(); got != "read 22 Sep" {
		t.Errorf("a reading over a week back reads %q in the widest words' cells, want its short form, read 22 Sep", got)
	}
	for _, w := range []status.WindowNow{
		{Kind: status.WindowCapped, At: on(4, 21, 0)},
		{Kind: status.WindowLimited, At: on(4, 21, 0)},
		{Kind: status.WindowRunsOut, At: on(4, 21, 0)},
		{Kind: status.WindowStale, At: on(30, 22, 10).AddDate(0, -1, 0)},
		{Kind: status.WindowOpen, At: on(4, 21, 0), Allowance: quota.Allowance{Share: 1, Per: quota.PerDay}},
	} {
		if full := wordsOf(w, printNow).full; full.width() > fullWidest() {
			t.Errorf("%q is wider than the widest words in full, %d cells", full.plain(), fullWidest())
		}
	}
	for _, tt := range []struct{ width, want int }{{fullWords, fullWidest()}, {fullWords - 1, shortCells}} {
		if got := (usageBlock{width: tt.width}).widestWords(); got != tt.want {
			t.Errorf("a block %d columns wide draws its words in %d cells at their widest, want %d", tt.width, got, tt.want)
		}
	}
}

func TestAnAllowancesTillAndDotAreSubtlerThanItsRateAndTime(t *testing.T) {
	w := wordsOf(status.WindowNow{Kind: status.WindowOpen, At: on(4, 21, 0), Allowance: quota.Allowance{Share: 0.19, Per: quota.PerDay}}, printNow)
	for _, l := range []line{w.full, w.short} {
		if len(l) != 3 || l[0].ink != mutedInk || l[1].ink != dimInk || l[2].ink != mutedInk {
			t.Errorf("the allowance's words are %+v, want the rate and the time in text.muted, between them text.subtle", l)
		}
	}
}

func TestThePercentageIsColouredByThePercentageShown(t *testing.T) {
	tests := []struct {
		name         string
		bar          usageBar
		want         string
		token        theme.Token
		bold, styled bool
	}{
		{name: "89.6% shows 90%, at the ramp's last stop", bar: usageBar{used: 0.896}, want: "90%", token: theme.VizRamp4, bold: true},
		{name: "69.49%", bar: usageBar{used: 0.6949}, want: "69%", token: theme.VizRamp1, bold: true},
		{name: "69.51% shows 70%, at the ramp's second stop", bar: usageBar{used: 0.6951}, want: "70%", token: theme.VizRamp2, bold: true},
		{name: "12.5% to the even 12%", bar: usageBar{used: 0.125}, want: "12%", token: theme.VizRamp1, bold: true},
		{name: "62.5% to the even 62%", bar: usageBar{used: 0.625}, want: "62%", token: theme.VizRamp1, bold: true},
		{name: "over 100%", bar: usageBar{used: 1.05}, want: "105%", token: theme.VizRamp4, bold: true},
		{name: "stale", bar: usageBar{used: 0.95, stale: true}, want: "95%", token: theme.TextSubtle},
		{name: "nothing read", bar: usageBar{used: 0.95, empty: true}, want: "—", token: theme.TextFaint},
		{name: "out of the pool, on the Overview", bar: usageBar{used: 0.95, out: true}, want: "95%", token: theme.TextFaint, bold: true},
		{name: "out of the pool and stale, faint winning", bar: usageBar{used: 0.34, out: true, stale: true}, want: "34%", token: theme.TextFaint, bold: true},
		{name: "out of the pool, nothing read", bar: usageBar{used: 0.95, out: true, empty: true}, want: "—", token: theme.TextFaint},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.bar.percentage()
			if want := (line{{tt.want, ink{token: tt.token, bold: tt.bold}}}); len(got) != 1 || got[0] != want[0] {
				t.Errorf("the percentage is %+v, want %+v", got, want)
			}
		})
	}
}

func TestFaintWordsAreAllInTextFaintInFullAndShort(t *testing.T) {
	for _, w := range []usageWords{
		wordsOf(status.WindowNow{Kind: status.WindowOpen, At: on(4, 21, 0), Allowance: quota.Allowance{Share: 0.19, Per: quota.PerDay}}, printNow),
		wordsOf(status.WindowNow{Kind: status.WindowCapped, At: on(1, 17, 10)}, printNow),
		wordsOf(status.WindowNow{Kind: status.WindowTokenless}, printNow),
	} {
		faint := w.faint()
		for _, form := range []struct{ was, is line }{{w.full, faint.full}, {w.short, faint.short}} {
			if form.is.plain() != form.was.plain() {
				t.Errorf("faint, the words read %q, want %q, as they were", form.is.plain(), form.was.plain())
			}
			for _, s := range form.is {
				if s.ink != faintInk {
					t.Errorf("faint, %q is drawn in %+v, want text.faint", s.text, s.ink)
				}
			}
		}
	}
}

func TestThePoolsWordsAreItsRoomToATenth(t *testing.T) {
	tests := []struct {
		name     string
		room     float64
		accounts int
		pinned   bool
		width    int
		cells    int
		want     string
	}{
		{name: "on auto", room: 1.48, accounts: 3, width: 160, cells: 24, want: "1.5 of 3 left"},
		{name: "a half, to the even tenth", room: 0.25, accounts: 3, width: 160, cells: 24, want: "0.2 of 3 left"},
		{name: "pinned", room: 0.9, accounts: 2, pinned: true, width: 160, cells: 24, want: "0.9 of 2 left · pinned"},
		{name: "pinned, short of room", room: 0.9, accounts: 2, pinned: true, width: 52, cells: 15, want: "0.9 of 2 left"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := poolWords(tt.room, tt.accounts, tt.pinned).pick(tt.width, tt.cells)
			if got := l.plain(); got != tt.want {
				t.Errorf("the pool's words read %q, want %q", got, tt.want)
			}
			if l[0].ink != strongInk {
				t.Errorf("its room is drawn in %+v, want bold text.secondary", l[0].ink)
			}
		})
	}
}
