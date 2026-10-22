package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

func TestCOMINGUPSaysWhatsNextSoonestFirst(t *testing.T) {
	lapsed := readAccount("spare", quota.Window{Key: "5h", Label: "Session"}, weekOf(0.05, 6*quota.Day))
	lapsed.Lapsed = []string{"5h"}
	pastLimit := readAccount("over", sessionOf(0.3, time.Hour), weekOf(0.2, 3*quota.Day))
	pastLimit.Limit = status.Limit{Windows: []string{"5h"}, Until: now.Add(-time.Minute)}
	spent := readAccount("spent", sessionOf(0.2, 3*time.Hour), weekOf(0.3, 3*quota.Day), windowOf("7d_oi", "Fable week", 1, 2*quota.Day))
	running := readAccount("client", sessionOf(0.1, 3*time.Hour), weekOf(0.7, 3*quota.Day))
	fable := readAccount("fable", sessionOf(0.1, 2*time.Hour), weekOf(0.2, 3*quota.Day), windowOf("7d_oi", "Fable week", 1, 4*quota.Day))
	fable.Limit = status.Limit{Windows: []string{"7d_oi"}, Until: now.Add(4 * quota.Day)}
	reserved := pressedAccount("work")
	reserved.Reserve = 0.1
	pinned := routerDoc("work", 0, reserved)
	pinned.Pin = status.Pin{Accounts: []string{"work"}}
	capped := readAccount("capped", sessionOf(0.2, 2*time.Hour), weekOf(0.95, 2*quota.Day))
	capped.Reserve = 0.1
	modelCapped := readAccount("fable", sessionOf(0.2, 2*time.Hour), weekOf(0.3, 3*quota.Day), windowOf("7d_oi", "Fable week", 0.95, 2*quota.Day))
	modelCapped.Reserve = 0.1
	// refusedAt is an account at its limit till now and limit, holding back
	// its every request, its token refused till now and refused.
	refusedAt := func(limit, refused time.Duration) status.Account {
		a := readAccount("held", sessionOf(0.3, 3*time.Hour), weekOf(0.3, 3*quota.Day))
		a.Limit = status.Limit{ID: 1, Until: now.Add(limit)}
		a.Refused = status.Refusal{Until: now.Add(refused), Status: 401}
		return a
	}
	cappedRefused := capped
	cappedRefused.Refused = status.Refusal{Until: now.Add(10 * time.Minute), Status: 401}
	tests := []struct {
		name  string
		doc   status.Document
		slots []status.Slot
		want  []string
	}{
		{
			name: "the frames': a limit lifting, a session running out, and resetting, a week running out, and every week resetting",
			doc:  threeRouted(),
			want: []string{
				"14:12 personal back from its limit",
				"14:42 work runs out at its pace",
				"16:12 work's 5-hour resets",
				"17:12 side's 5-hour resets",
				"Thu 13:12 personal's week resets",
				"Fri 13:12 work's week resets",
				"Fri 14:39 side's week runs out at its pace",
				"Sat 13:12 side's week resets",
			},
		},
		{
			name:  "primes from the router, an idle account's among them",
			doc:   routerDoc("a", 0, readAccount("a", sessionOf(0.1, 2*time.Hour)), lapsed),
			slots: []status.Slot{{Account: "spare", At: "13:50", Next: now.Add(38 * time.Minute)}, {Account: "a", At: "03:50", Next: now.Add(14 * time.Hour)}},
			want: []string{
				"13:50 spare is primed",
				"15:12 a's 5-hour resets",
				"Tue 03:12 a is primed",
				"Sun 13:12 spare's week resets",
			},
		},
		{
			name:  "nothing that's passed, nor a prime the document doesn't time",
			doc:   routerDoc("a", 0, readAccount("a", sessionOf(0.1, -time.Minute)), pastLimit),
			slots: []status.Slot{{Account: "a", At: "03:50"}},
			want:  []string{"14:12 over's 5-hour resets", "Thu 13:12 over's week resets"},
		},
		{
			name: "a limit on a model's own window, which leaves the session running",
			doc:  routerDoc("fable", 0, fable),
			want: []string{"15:12 fable's 5-hour resets", "Thu 13:12 fable's week resets", "Fri 13:12 fable back from its Fable wk limit"},
		},
		{
			name: "an account its reserve holds back, reaching it before its limit",
			doc:  routerDoc("work", 0, reserved),
			want: []string{"14:20 work reaches its cap", "16:12 work's 5-hour resets", "Fri 13:12 work's week resets"},
		},
		{
			name: "an account a pin names, its reserve not holding it back",
			doc:  pinned,
			want: []string{"14:42 work runs out at its pace", "16:12 work's 5-hour resets", "Fri 13:12 work's week resets"},
		},
		{
			name: "a window other than the session running out, named",
			doc:  routerDoc("client", 0, running),
			want: []string{"16:12 client's 5-hour resets", "Wed 06:20 client's week runs out at its pace", "Thu 13:12 client's week resets"},
		},
		{
			name: "a window read spent, without the router, back as it resets",
			doc:  probed(spent, limitedAccount("held")),
			want: []string{
				"14:12 held back from its limit",
				"16:12 spent's 5-hour resets",
				"Wed 13:12 spent back from its Fable wk limit",
				"Thu 13:12 spent's week resets",
				"Thu 13:12 held's week resets",
			},
		},
		{
			name: "an account back from its cap as its week resets",
			doc:  routerDoc("capped", 0, capped),
			want: []string{"15:12 capped's 5-hour resets", "Wed 13:12 capped back from its cap"},
		},
		{
			name: "an account back from the cap on a model's own week",
			doc:  routerDoc("fable", 0, modelCapped),
			want: []string{"15:12 fable's 5-hour resets", "Wed 13:12 fable back from its Fable wk cap", "Thu 13:12 fable's week resets"},
		},
		{
			name: "an account at its limit, its token refused till after it lifts, back from the refusal",
			doc:  routerDoc("held", 0, refusedAt(5*time.Minute, 10*time.Minute)),
			want: []string{"13:22 held back from its refused token", "16:12 held's 5-hour resets", "Thu 13:12 held's week resets"},
		},
		{
			name: "an account at its limit, its token refused till before it lifts, back from the limit",
			doc:  routerDoc("held", 0, refusedAt(time.Hour, 10*time.Minute)),
			want: []string{"14:12 held back from its limit", "16:12 held's 5-hour resets", "Thu 13:12 held's week resets"},
		},
		{
			name: "an account at its limit, its token refused till it lifts, back from the limit",
			doc:  routerDoc("held", 0, refusedAt(10*time.Minute, 10*time.Minute)),
			want: []string{"13:22 held back from its limit", "16:12 held's 5-hour resets", "Thu 13:12 held's week resets"},
		},
		{
			name: "an account at its cap, its token refused till before it lets go, back from the cap",
			doc:  routerDoc("capped", 0, cappedRefused),
			want: []string{"15:12 capped's 5-hour resets", "Wed 13:12 capped back from its cap"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := tt.doc
			doc.Prime = status.Prime{Day: "08:00-22:00", Window: "5h", Slots: tt.slots}
			var got []string
			for _, h := range (Frame{Policy: claudeLike}).upcoming(doc.WorkedOut(claudeLike), now) {
				got = append(got, prose.Dated(now, h.at)+" "+h.name.text+h.what)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("upcoming() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCOMINGUPDrawsWhatTheDocumentGivesAfterNow(t *testing.T) {
	doc := routerDoc("work", 0, readAccount("work", sessionOf(0.1, 2*time.Hour)))
	doc.ComingUp = []status.Upcoming{
		{At: now.Add(-time.Minute).UTC(), Account: "work", Kind: status.UpcomingReset, Window: "7d"},
		{At: now.UTC(), Account: "work", Kind: status.UpcomingPrime},
		{At: now.Add(time.Hour).UTC(), Account: "work", Kind: "later", Window: "5h"},
		{At: now.Add(2 * time.Hour).UTC(), Account: "work", Kind: status.UpcomingReset, Window: "5h"},
	}
	var got []string
	for _, h := range (Frame{Policy: claudeLike}).upcoming(doc, now) {
		got = append(got, prose.Dated(now, h.at)+" "+h.name.text+h.what)
	}
	if want := []string{"15:12 work's 5-hour resets"}; !slices.Equal(got, want) {
		t.Errorf("upcoming() = %q, want %q: what the document gives, after now, of the kinds it knows", got, want)
	}
}

func TestCOMINGUPListsEverythingTheDocumentGivesWhateverItsWindow(t *testing.T) {
	doc := routerDoc("work", 0, readAccount("work", sessionOf(0.1, 2*time.Hour), windowOf("7d_oi", "Fable week", 0, 2*quota.Day)))
	if windows := windowKeys(doc.At(now, claudeLike).Hidden); !slices.Equal(windows, []string{"7d_oi"}) {
		t.Fatalf("the hidden windows = %q, want the Fable week, unused on every account", windows)
	}
	doc.ComingUp = []status.Upcoming{
		{At: now.Add(time.Hour).UTC(), Account: "work", Kind: status.UpcomingRunsOut, Window: "7d_oi"},
		{At: now.Add(2 * time.Hour).UTC(), Account: "work", Kind: status.UpcomingBack, Window: "7d_oi"},
		{At: now.Add(3 * time.Hour).UTC(), Account: "work", Kind: status.UpcomingBack, Window: "7d_oi", Cap: true},
		{At: now.Add(4 * time.Hour).UTC(), Account: "work", Kind: status.UpcomingReset, Window: "7d_oi"},
	}

	var got []string
	for _, h := range (Frame{Policy: claudeLike}).upcoming(doc, now) {
		got = append(got, prose.Dated(now, h.at)+" "+h.name.text+h.what)
	}
	want := []string{
		"14:12 work's Fable wk runs out at its pace",
		"15:12 work back from its Fable wk limit",
		"16:12 work back from its Fable wk cap",
		"17:12 work's Fable wk resets",
	}
	if !slices.Equal(got, want) {
		t.Errorf("upcoming() = %q, want %q: all the document gives, though no account uses the Fable week, a model's own week named short", got, want)
	}
}

// comingDrawn is COMING UP of doc at now drawn in body, on a canvas of the
// frame's size, in its look.
func comingDrawn(f Frame, doc status.Document, now time.Time, body bounds) []string {
	c := newCanvas(f.Width, f.Height)
	f.comingSection(doc, now).draw(c, body)
	return c.rows(f.Look)
}

// comingOf is a document of the accounts work, personal and side whose
// COMING UP is the things given.
func comingOf(things ...status.Upcoming) status.Document {
	doc := routerDoc("side", 0, readAccount("work", sessionOf(0.2, time.Hour), weekOf(0.3, 3*quota.Day)), readAccount("personal", sessionOf(0.4, 2*time.Hour), weekOf(0.6, 2*quota.Day)), readAccount("side", sessionOf(0.1, 4*time.Hour), weekOf(0.7, 5*quota.Day)))
	doc.ComingUp = things
	return doc
}

func TestCOMINGUPsInks(t *testing.T) {
	doc := comingOf(
		status.Upcoming{At: now.Add(time.Hour).UTC(), Account: "work", Kind: status.UpcomingRunsOut, Window: "5h"},
		status.Upcoming{At: now.Add(2 * time.Hour).UTC(), Account: "work", Kind: status.UpcomingReset, Window: "5h"},
		status.Upcoming{At: now.Add(3 * time.Hour).UTC(), Account: "side", Kind: status.UpcomingPrime},
		status.Upcoming{At: now.Add(4 * time.Hour).UTC(), Account: "personal", Kind: status.UpcomingBack},
	)
	f := frameOf(160, 10)
	c := newCanvas(f.Width, f.Height)
	f.comingSection(doc, now).draw(c, bounds{width: 72, rows: 4})
	want := []struct {
		text             string
		at, words, until theme.Token
	}{
		{text: "~14:12  work runs out at its pace", at: theme.AccentAttention, words: theme.AccentAttention, until: theme.AccentAttention},
		{text: "15:12   work's 5-hour resets", at: theme.TextSecondary, words: theme.TextMuted, until: theme.TextSubtle},
		{text: "16:12   side is primed", at: theme.TextSecondary, words: theme.TextMuted, until: theme.TextSubtle},
		{text: "17:12   personal back from its limit", at: theme.TextSecondary, words: theme.TextMuted, until: theme.TextSubtle},
	}
	name := len("~14:12") + len(afterTime)
	for i, w := range want {
		row := c.cells[i]
		if got := c.row(i, Look{})[:len(w.text)]; got != w.text {
			t.Fatalf("row %d = %q, want %q", i+1, got, w.text)
		}
		checks := []struct {
			what string
			cell cell
			want ink
		}{
			{"its time", row[0], ink{token: w.at}},
			{"its account", row[name], titleInk},
			{"its words", row[len(w.text)-1], ink{token: w.words}},
			{"its countdown", row[71], ink{token: w.until}},
		}
		for _, check := range checks {
			if check.cell.ink != check.want {
				t.Errorf("row %d, %q: %s is in %+v, want %+v", i+1, w.text, check.what, check.cell.ink, check.want)
			}
		}
	}
}

func TestCOMINGUPDrawsAsManyAsItsBodyHoldsTheirCountdownsEndingAtItsRight(t *testing.T) {
	f := frameOf(160, 10)
	body := bounds{x: 10, y: 1, width: 72, rows: 2}
	rows := comingDrawn(f, threeRouted(), now, body)

	from := spaces(body.x).text
	want := []string{
		"",
		from + padded("14:12   personal back from its limit", body.width-len("in 1h")) + "in 1h",
		from + padded("~14:42  work runs out at its pace", body.width-len("in 1h 30m")) + "in 1h 30m",
		"",
	}
	for i, w := range want {
		if got := strings.TrimRight(rows[i], " "); got != w {
			t.Errorf("row %d = %q, want %q: as many as its rows, from its left, its countdowns ending at its right", i+1, got, w)
		}
	}
}

func TestCOMINGUPCutsItsWordsShortOfItsCountdown(t *testing.T) {
	f := frameOf(160, 10)
	doc := comingOf(status.Upcoming{At: now.Add(time.Hour).UTC(), Account: "personal", Kind: status.UpcomingBack, Window: "5h"})
	rows := comingDrawn(f, doc, now, bounds{width: 40, rows: 3})

	if got, want := strings.TrimRight(rows[0], " "), "14:12  personal back from i…       in 1h"; got != want {
		t.Errorf("row 1 = %q, want %q: its words cut to leave its countdown room", got, want)
	}
}

func TestCOMINGUPPadsItsTimesToTheLongestOfThoseShown(t *testing.T) {
	doc := comingOf(
		status.Upcoming{At: now.Add(time.Hour).UTC(), Account: "personal", Kind: status.UpcomingBack},
		status.Upcoming{At: now.Add(2 * time.Hour).UTC(), Account: "side", Kind: status.UpcomingReset, Window: "5h"},
		status.Upcoming{At: now.Add(2 * quota.Day).UTC(), Account: "personal", Kind: status.UpcomingRunsOut, Window: "7d"},
	)
	f := frameOf(216, 10)
	tests := []struct {
		rows int
		want []string
	}{
		{rows: 3, want: []string{"14:12       personal back from its limit", "15:12       side's 5-hour resets", "~Wed 13:12  personal's week runs out at its pace"}},
		{rows: 2, want: []string{"14:12  personal back from its limit", "15:12  side's 5-hour resets"}},
	}
	for _, tt := range tests {
		rows := comingDrawn(f, doc, now, bounds{width: 97, rows: tt.rows})
		for i, w := range tt.want {
			if got := strings.TrimRight(rows[i][:97-comingTail], " "); got != w {
				t.Errorf("%d shown, row %d = %q, want %q", tt.rows, i+1, got, w)
			}
		}
	}
}

func TestCOMINGUPAndLATELYPutTwoSpacesAfterTheirTimesAtEveryWidth(t *testing.T) {
	doc := comingOf(status.Upcoming{At: now.Add(time.Hour).UTC(), Account: "work", Kind: status.UpcomingReset, Window: "5h"})
	doc.Events = []status.Event{{ID: 1, At: now.Add(-time.Minute).UTC(), Kind: status.EventRoom, Account: "work"}}
	for _, width := range []int{52, 160} {
		f := frameOf(width, 10)
		body := bounds{width: width, rows: 1}
		if coming := comingDrawn(f, doc, now, body)[0]; !strings.HasPrefix(coming, "14:12  work") {
			t.Errorf("at %d columns, COMING UP's line is %q, want its time two spaces from its words", width, coming)
		}
		c := newCanvas(f.Width, f.Height)
		f.latelySection(doc, now).draw(c, body)
		if lately := c.row(0, Look{}); !strings.HasPrefix(lately, "13:11  ✓ work") {
			t.Errorf("at %d columns, LATELY's line is %q, want its time two spaces from its mark", width, lately)
		}
	}
}

func TestCOMINGUPTakesItsLinesAndTwoRows(t *testing.T) {
	things := func(n int) status.Document {
		doc := comingOf()
		for i := range n {
			doc.ComingUp = append(doc.ComingUp, status.Upcoming{At: now.Add(time.Duration(i+1) * time.Hour).UTC(), Account: "work", Kind: status.UpcomingPrime})
		}
		return doc
	}
	for _, tt := range []struct{ things, least, most int }{{0, 5, 5}, {2, 5, 5}, {7, 5, 9}, {20, 5, 14}} {
		s := frameOf(160, 40).comingSection(things(tt.things), now)
		if s.least != tt.least || s.mostRows() != tt.most {
			t.Errorf("with %d things coming, COMING UP takes %d rows to %d, want %d to %d", tt.things, s.least, s.mostRows(), tt.least, tt.most)
		}
	}
}

func TestCOMINGUPFoldsToItsFirstLineButItsCountdownThenAnEllipsisWhereItHasMore(t *testing.T) {
	runsOut := status.Upcoming{At: now.Add(2 * quota.Day).UTC(), Account: "personal", Kind: status.UpcomingRunsOut, Window: "7d"}
	resets := status.Upcoming{At: now.Add(3 * quota.Day).UTC(), Account: "side", Kind: status.UpcomingReset, Window: "7d"}
	first := line{{"~Wed 13:12  ", warningInk}, {"personal", titleInk}, {"'s week runs out at its pace", warningInk}}
	tests := []struct {
		name   string
		coming []status.Upcoming
		want   line
	}{
		{name: "more than one thing coming", coming: []status.Upcoming{runsOut, resets}, want: slices.Concat(first, line{{"  …", dimInk}})},
		{name: "one thing coming", coming: []status.Upcoming{runsOut}, want: first},
		{name: "nothing coming"},
	}
	f := frameOf(160, 40)
	for _, tt := range tests {
		if got := f.says(planned{section: f.comingSection(comingOf(tt.coming...), now), fold: true}, f.Width); !slices.Equal(got, tt.want) {
			t.Errorf("folded with %s, COMING UP's heading says %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

func TestCOMINGUPNamesAnAccountOutOfThePoolFaint(t *testing.T) {
	doc := comingOf(
		status.Upcoming{At: now.Add(time.Hour).UTC(), Account: "work", Kind: status.UpcomingPrime},
		status.Upcoming{At: now.Add(2 * time.Hour).UTC(), Account: "personal", Kind: status.UpcomingReset, Window: "5h"},
	)
	doc.Pin = status.Pin{Accounts: []string{"work"}}
	faint := ink{token: theme.TextFaint, bold: true}
	want := []span{{"work", titleInk}, {"personal", faint}}
	var got []span
	for _, h := range (Frame{Policy: claudeLike}).upcoming(doc, now) {
		got = append(got, h.said(now, 0)[1])
	}
	if !slices.Equal(got, want) {
		t.Errorf("with the pool work alone, COMING UP names %+v, want %+v", got, want)
	}
}

func TestCOMINGUPsTimeCarriesItsWeekdayOnAnotherDayEvenWithinADay(t *testing.T) {
	tests := map[string]struct {
		h    happening
		want string
	}{
		"later today":               {happening{at: now.Add(2 * time.Hour)}, "15:12"},
		"tomorrow, within a day":    {happening{at: now.Add(22 * time.Hour)}, "Tue 11:12"},
		"running out tomorrow":      {happening{at: now.Add(22 * time.Hour), runsOut: true}, "~Tue 11:12"},
		"running out in a few days": {happening{at: now.Add(5 * quota.Day), runsOut: true}, "~Sat 13:12"},
	}
	for name, tt := range tests {
		if got := tt.h.when(now); got != tt.want {
			t.Errorf("%s, when() = %q, want %q", name, got, tt.want)
		}
	}
}
