package dashboard

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

func TestAWindowsWordsSayHowItStandsInFullAndShort(t *testing.T) {
	state := func(kind status.WindowKind, at time.Time) status.WindowState {
		return status.WindowState{Kind: kind, At: at}
	}
	tests := []struct {
		name        string
		state       status.WindowState
		allowance   quota.Allowance
		full, short string
		ink         ink
	}{
		{name: "no token", state: state(status.WindowTokenless, time.Time{}), full: "no token", short: "no token", ink: errorInk},
		{name: "nothing read", state: state(status.WindowUnread, time.Time{}), full: "not read yet", short: "not read", ink: dimInk},
		{name: "stale since today", state: state(status.WindowStale, on(1, 14, 20)), full: "not read since 14:20", short: "read 14:20", ink: dimInk},
		{name: "stale since another day, too long in full", state: state(status.WindowStale, on(30, 22, 10).AddDate(0, -1, 0)), full: "read Wed", short: "read Wed", ink: dimInk},
		{name: "stale since this day last week", state: state(status.WindowStale, on(1, 9, 5).AddDate(0, 0, -7)), full: "read Thu", short: "read Thu", ink: dimInk},
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
			w := wordsOf(tt.state, tt.allowance, printNow)
			if got := w.pick(fullWords).plain(); got != tt.full {
				t.Errorf("in full the words read %q, want %q", got, tt.full)
			}
			if got := w.pick(fullWords - 1).plain(); got != tt.short {
				t.Errorf("short the words read %q, want %q", got, tt.short)
			}
			if len(w.full) > 0 && w.full[0].ink != tt.ink {
				t.Errorf("the words lead in %+v, want %+v", w.full[0].ink, tt.ink)
			}
		})
	}
}

func TestAnAllowancesTillAndDotAreSubtlerThanItsRateAndTime(t *testing.T) {
	w := wordsOf(status.WindowState{Kind: status.WindowOpen, At: on(4, 21, 0)}, quota.Allowance{Share: 0.19, Per: quota.PerDay}, printNow)
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
		wordsOf(status.WindowState{Kind: status.WindowOpen, At: on(4, 21, 0)}, quota.Allowance{Share: 0.19, Per: quota.PerDay}, printNow),
		wordsOf(status.WindowState{Kind: status.WindowCapped, At: on(1, 17, 10)}, quota.Allowance{}, printNow),
		wordsOf(status.WindowState{Kind: status.WindowTokenless}, quota.Allowance{}, printNow),
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
		want     string
	}{
		{name: "on auto", room: 1.48, accounts: 3, width: 160, want: "1.5 of 3 left"},
		{name: "a half, to the even tenth", room: 0.25, accounts: 3, width: 160, want: "0.2 of 3 left"},
		{name: "pinned", room: 0.9, accounts: 2, pinned: true, width: 160, want: "0.9 of 2 left · pinned"},
		{name: "pinned, short of room", room: 0.9, accounts: 2, pinned: true, width: 52, want: "0.9 of 2 left"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := poolWords(tt.room, tt.accounts, tt.pinned).pick(tt.width)
			if got := l.plain(); got != tt.want {
				t.Errorf("the pool's words read %q, want %q", got, tt.want)
			}
			if l[0].ink != strongInk {
				t.Errorf("its room is drawn in %+v, want bold text.secondary", l[0].ink)
			}
		})
	}
}
