package watch

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// said is the dial's name for the period given on h's clock, as the
// heading names it.
func said(h *harness, p views.Period) string {
	return views.DialOf(p, h.clock.now, h.model.cfg.WeekStarts, h.model.begun.began).Name()
}

// accountsFrom is where Accounts' page of the period h shows reads the
// ledger's days from on h's clock, as AccountsFrom has it.
func accountsFrom(h *harness) time.Time {
	return h.model.period.AccountsFrom(h.clock.now, h.model.cfg.WeekStarts)
}

// pageOfThePeriod reports whether Accounts' page is built of the period the
// dial shows.
func pageOfThePeriod(h *harness) bool {
	page := h.model.accounts.page
	return page != nil && page.Period == h.model.period
}

// foldsRead delivers msgs and all they send back, as deliver does, and
// returns the dates of the days whose folds they read, in the order the
// reads landed.
func foldsRead(h *harness, msgs ...tea.Msg) []string {
	h.t.Helper()
	var dates []string
	held := holdingBack[pastDayReadMsg](h, msgs...)
	for len(held) > 0 {
		msg := held[0].(pastDayReadMsg)
		dates = append(dates, msg.date)
		held = append(held[1:], holdingBack[pastDayReadMsg](h, h.update(msg)...)...)
	}
	return dates
}

// foldsKeyed presses the keys given, a space between each, as keyed does,
// and returns the dates of the days whose folds they read, as foldsRead has
// them.
func foldsKeyed(h *harness, keys string) []string {
	h.t.Helper()
	var dates []string
	for k := range strings.FieldsSeq(keys) {
		dates = append(dates, foldsRead(h, h.press(k)...)...)
	}
	return dates
}

// A step back on Accounts reads the ledger's days once, from where the page
// of the period it steps to reads, as AccountsFrom has it, where the days
// held don't reach it: on a Monday, the 12 months' read reaches last week,
// but not the week before it, nor the 12 months to its Sunday. A step on
// reads nothing, nor a step back to days held; each page is of the period
// shown, counting the requests of its days alone, those before the 12
// months' read among them.
func TestAStepBackOnAccountsReadsTheDaysItReachesOnce(t *testing.T) {
	h, l := pastHarness(t, dashboard.Accounts)
	for date, n := range map[string]int{"2024-09-16": 3, "2024-10-01": 5, "2025-09-17": 7, "2026-09-16": 11, "2026-09-25": 13, "2026-09-28": 17} {
		l.days[date] = requested(date, n)
	}
	h.start()
	if !slices.EqualFunc(l.reads, []time.Time{yearFrom}, time.Time.Equal) || !pageOfThePeriod(h) {
		t.Fatalf("Accounts opened, the ledger was read from %v, its page of the period %t; want once, from %s, and built", l.reads, pageOfThePeriod(h), yearFrom)
	}
	steps := []struct {
		key   string
		reads bool
	}{
		{key: "<"},
		{key: "<", reads: true},
		{key: ">"},
		{key: "n"},
		{key: "<"},
		{key: "<"},
		{key: "+"},
		{key: "+"},
		{key: "+", reads: true},
		{key: "<", reads: true},
		{key: "n"},
		{key: "<"},
	}
	for i, step := range steps {
		was, read := h.model.period, len(l.reads)
		h.keyed(step.key)
		name := fmt.Sprintf("step %d, %s from %q to %q", i+1, step.key, said(h, was), said(h, h.model.period))
		var want []time.Time
		if step.reads {
			want = []time.Time{accountsFrom(h)}
		}
		if got := l.reads[read:]; !slices.EqualFunc(got, want, time.Time.Equal) {
			t.Fatalf("%s read the ledger from %v, want %v", name, got, want)
		}
		if !pageOfThePeriod(h) || h.model.awaiting() {
			t.Fatalf("%s, Accounts' page is built of the period %t, awaiting %t; want it built, nothing awaited", name, pageOfThePeriod(h), h.model.awaiting())
		}
		if got, want := h.model.accounts.page.All.Totals.Requests, requestsIn(h, l); got != want {
			t.Errorf("%s, Accounts' page counts %d requests, want %d, its days'", name, got, want)
		}
	}
}

// requestsIn are the requests of the days the ledger holds that lie in the
// period h shows, on h's clock, from its first day to its last, or today.
func requestsIn(h *harness, l *daysLedger) int {
	from, now := h.model.period.From(h.clock.now, time.Monday), h.clock.now
	until, gone := h.model.period.Until(now, time.Monday)
	if !gone {
		until = now
	}
	n := 0
	for date, d := range l.days {
		if start, _, _ := dayfile.Day(date); !start.Before(from) && start.Before(until) {
			n += d.Accounts[0].Models[0].Upstream
		}
	}
	return n
}

// A read back that a step overtakes as it's under way, as the week of 14
// September's, is kept, as it holds more than a step on needs, the page of
// the period stepped to built meanwhile of the days held; one further back
// is asked after it, one read at a time, the page awaiting it.
func TestAReadBackOvertakenByAStepIsKeptOrAskedAfterIt(t *testing.T) {
	tests := []struct {
		name string
		then string
		// further is set where the step needs more than the read under way.
		further bool
	}{
		{name: "a step on, needing less", then: ">"},
		{name: "back to now", then: "n"},
		{name: "a step further back, needing more", then: "<", further: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, l := pastHarness(t, dashboard.Accounts)
			h.start()
			h.keyed("<")
			held := holdingBack[pastReadMsg](h, h.press("<")...)
			first := accountsFrom(h)
			if len(held) != 1 || h.model.accounts.page != nil || !h.model.awaiting() {
				t.Fatalf("stepped back to the week of 14 September, %d reads under way, a page %t, awaiting %t; want its read under way, no page, awaiting it",
					len(held), h.model.accounts.page != nil, h.model.awaiting())
			}
			if more := holdingBack[pastReadMsg](h, h.press(tt.then)...); len(more) != 0 {
				t.Fatalf("%s pressed while the read is under way asked %d reads more, want none: one at a time", tt.then, len(more))
			}
			if built := h.model.accounts.page != nil; built == tt.further || h.model.awaiting() != tt.further {
				t.Errorf("%s pressed while the read is under way, a page %t, awaiting %t; want a page %t", tt.then, built, h.model.awaiting(), !tt.further)
			}
			h.deliver(held...)
			want := []time.Time{yearFrom, first}
			if tt.further {
				want = append(want, accountsFrom(h))
			}
			if !slices.EqualFunc(l.reads, want, time.Time.Equal) || !h.model.past.back.from.Equal(want[len(want)-1]) {
				t.Errorf("the read landed, the ledger was read from %v, held back from %s; want %v, held back from the last", l.reads, h.model.past.back.from, want)
			}
			if !pageOfThePeriod(h) || h.model.awaiting() {
				t.Errorf("the reads landed, Accounts' page is built of %q %t, awaiting %t; want it built, nothing awaited", said(h, h.model.period), pageOfThePeriod(h), h.model.awaiting())
			}
		})
	}
}

// Runway's lanes stepped back past the 12 months read the calendar year
// they reach, which serves Accounts' past periods it reaches too, reading
// nothing; a period reaching further reads back to it, and the lanes are
// served still, read for nothing as Runway shows again.
func TestRunwaysReadBackIsServedStillAsAccountsReachesBack(t *testing.T) {
	r := runwayHarness(t, dashboard.Runway, nil)
	for range 80 {
		if len(r.ledger.reads) > 1 {
			break
		}
		r.keyed("left")
	}
	year := views.DaysFrom(2025, r.clock.now, time.Sunday)
	if !slices.EqualFunc(r.ledger.reads, []time.Time{views.DaysFrom(0, r.clock.now, time.Sunday), year}, time.Time.Equal) {
		t.Fatalf("Runway stepped back past the 12 months, the ledger was read from %v, want the 12 months, then 2025's from %s", r.ledger.reads, year)
	}
	r.model.view = dashboard.Accounts
	r.deliver(r.update(nudgeMsg{})...)
	steps := []struct {
		keys  string
		reads bool
	}{
		{keys: "<"},
		{keys: "+ + +"},
		{keys: "<", reads: true},
	}
	for _, step := range steps {
		read := len(r.ledger.reads)
		r.keyed(step.keys)
		var want []time.Time
		if step.reads {
			want = []time.Time{accountsFrom(r.harness)}
		}
		if got := r.ledger.reads[read:]; !slices.EqualFunc(got, want, time.Time.Equal) || !pageOfThePeriod(r.harness) {
			t.Fatalf("%s to %q read the ledger from %v, its page built %t; want %v, built", step.keys, said(r.harness, r.model.period), got, pageOfThePeriod(r.harness), want)
		}
	}
	read := len(r.ledger.reads)
	r.model.view = dashboard.Runway
	r.deliver(r.update(nudgeMsg{})...)
	if len(r.ledger.reads) != read || len(r.from.Days) == 0 {
		t.Errorf("Runway shown again, the ledger was read %d times more, its lanes built of %d days; want no read, the days held", len(r.ledger.reads)-read, len(r.from.Days))
	}
}

// A pool turned to as Accounts' read back is under way reads, in the pool
// turned to, what the past period kept reaches; the read of the pool before
// never lands in its place. Each pool reads every pool's days, so each
// reaches back as far as the ledger, whose days of no work tell no pool's
// first day.
func TestAPoolTurnedMidReadReadsWhatThePastPeriodReaches(t *testing.T) {
	h := routedHarness(t, ofPool(routerDocument(three()...), "b2n8"))
	acmeDoc := ofPool(routerDocument(account("k3f9", "Acme 1", fableWeek(0.7))), "q7mt")
	acme := &fakeSource{doc: probedWithoutTheRouter(), router: &acmeDoc}
	pools := newPoolTurns(t, testPools, slices.Concat(defaultAccounts, acmeAccounts), map[string]Source{"b2n8": h.source, "q7mt": acme})
	l := &daysLedger{Empty: h.emptyLedger(), clock: h.clock, first: "2024-06-03", days: map[string]ledger.Summary{}}
	cfg := h.config(Size{Width: 160, Height: 40})
	cfg.Ledger, cfg.View, cfg.WeekStarts = l, dashboard.Accounts, time.Monday
	cfg = pools.opened(cfg, "b2n8")
	cfg.PoolRead = views.PoolRead{}
	h.model = New(t.Context(), cfg)
	h.start()
	h.keyed("<")
	stale := holdingBack[pastReadMsg](h, h.press("<")...)
	period, from := h.model.period, accountsFrom(h)
	if len(stale) != 1 {
		t.Fatalf("stepped back to the week of 14 September, %d reads under way, want one", len(stale))
	}

	tm := turns{Model: h.model}
	before := tm.poolTurns
	var fresh []tea.Msg
	var land func(msg tea.Msg)
	deliver := func(msgs ...tea.Msg) {
		for _, msg := range msgs {
			if s, ok := msg.(ofTurn); ok {
				if read, ok := s.msg.(pastReadMsg); ok && read.back {
					fresh = append(fresh, msg)
					continue
				}
			}
			land(msg)
		}
	}
	land = func(msg tea.Msg) {
		next, cmd := tm.Update(msg)
		tm = next.(turns)
		deliver(h.run(cmd)...)
	}
	turned := pools.pooled("q7mt")
	turned.Read = views.PoolRead{}
	land(ofTurn{turn: before, msg: turnedMsg{ask: NextPool, pooled: turned}})
	if tm.period != period || len(fresh) != 1 || !tm.past.reading {
		t.Fatalf("turned to Acme, the dial shows %q, %d reads back under way, one under way %t; want %q kept, Acme's read back under way", said(h, tm.period), len(fresh), tm.past.reading, said(h, period))
	}
	land(ofTurn{turn: before, msg: stale[0]})
	if !tm.past.reading || tm.past.back.held() {
		t.Fatalf("Default's read landed after the turn, Acme's read under way %t, the days back held %t; want Acme's under way still, none held", tm.past.reading, tm.past.back.held())
	}
	land(fresh[0])
	if want := []time.Time{yearFrom, from, yearFrom, from}; !slices.EqualFunc(l.reads, want, time.Time.Equal) {
		t.Errorf("the ledger was read from %v, want %v: each pool's 12 months, then the past period's reach", l.reads, want)
	}
	if page := tm.accounts.page; page == nil || page.Period != period || !tm.past.back.from.Equal(from) {
		t.Errorf("Acme's read landed, Accounts' page is %v, held back from %s; want the past period's, held back from %s", page, tm.past.back.from, from)
	}
}

// A day gone's fold is read once, as the dial steps to it on a tab that
// charts its hours, the page waiting on it, then charting the day by the
// hour; a read the dial steps past as it's under way is dropped, and the
// day it steps to read in its place.
func TestADayGonesFoldIsReadOnceAndDroppedAsTheDialLeavesIt(t *testing.T) {
	tests := []struct {
		name   string
		view   dashboard.View
		hourly func(m Model) bool
	}{
		{name: "Compare", view: dashboard.Accounts, hourly: func(m Model) bool {
			return m.accounts.page != nil && m.accounts.page.All.WorthBy[0].Unit == views.ByHour
		}},
		{name: "Projects", view: dashboard.Projects, hourly: func(m Model) bool {
			p := m.projects.shown.Project
			return p != nil && p.WorthBy[0].Unit == views.ByHour
		}},
	}
	yesterday, dayBefore, threeBack := dayfile.DateOf(start.AddDate(0, 0, -1)), dayfile.DateOf(start.AddDate(0, 0, -2)), dayfile.DateOf(start.AddDate(0, 0, -3))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := withDelta(projectsHarness(t, tt.view))
			h.start()
			h.keyed("-")
			held := holdingBack[pastDayReadMsg](h, h.press("<")...)
			if len(held) != 1 || held[0].(pastDayReadMsg).date != yesterday || tt.hourly(h.model) || !h.model.awaiting() {
				t.Fatalf("stepped back to yesterday, %d folds read, charted by the hour %t, awaiting %t; want yesterday's read, the page awaiting it", len(held), tt.hourly(h.model), h.model.awaiting())
			}
			if got := foldsRead(h, held...); !slices.Equal(got, []string{yesterday}) || !tt.hourly(h.model) || h.model.awaiting() {
				t.Fatalf("yesterday's fold landed, the folds of %v read, charted by the hour %t, awaiting %t; want its own alone, the day by the hour", got, tt.hourly(h.model), h.model.awaiting())
			}
			h.tickUntilAsked()
			if got := foldsKeyed(h, "R"); len(got) != 0 {
				t.Errorf("looks on yesterday read the folds of %v, want none", got)
			}

			reads := h.model.pastDay.read
			held = holdingBack[pastDayReadMsg](h, h.press("<")...)
			held = slices.Concat(held, h.press("<"))
			got := foldsRead(h, held...)
			if d := h.model.pastDay; !slices.Equal(got, []string{dayBefore, threeBack}) || d.date != threeBack || d.read != reads+1 {
				t.Errorf("< twice, the folds of %v read, holding %s's, %d held since; want the day before yesterday's, dropped as the dial left it, then %s's, held, the one",
					got, d.date, d.read-reads, threeBack)
			}
			if got := foldsKeyed(h, "n"); len(got) != 0 {
				t.Errorf("back to today, the folds of %v were read, want none: today's hours are the tail's", got)
			}
		})
	}
}

// A day gone's page never follows the ledger's tail, nor the hour: the hour
// turning, then a read of the tail gaining a line today, each build today's
// page afresh, but leave yesterday's as it was built.
func TestADayGoneIsNeverBuiltAfreshByTheTailOrTheHour(t *testing.T) {
	tests := []struct {
		name    string
		keys    string
		rebuilt bool
	}{
		{name: "today", keys: "-", rebuilt: true},
		{name: "yesterday", keys: "- <"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := projectsHarness(t, dashboard.Accounts)
			h.start()
			h.keyed(tt.keys)
			if h.model.accounts.page == nil || h.model.accountsToday() != tt.rebuilt {
				t.Fatalf("on %s, Accounts' page built %t, following the tail %t; want built, following it %t",
					tt.name, h.model.accounts.page != nil, h.model.accountsToday(), tt.rebuilt)
			}
			changes := []struct {
				name   string
				change func()
			}{
				{name: "the hour turning", change: func() {
					h.clock.now = dayfile.DayStart(start.Local(), 0).Add(time.Duration(start.Local().Hour()+1)*time.Hour + time.Minute)
					h.deliver(h.update(nudgeMsg{})...)
				}},
				{name: "a read of the tail gaining a line", change: func() {
					projectsLedgerOf(h).write(projectAsks(betaOnly, "/code/beta", h.clock.now.Add(-30*time.Second), 1)...)
					m, cmd := h.model.readTail(h.clock.now)
					h.model = m
					h.deliver(h.run(cmd)...)
				}},
			}
			for _, c := range changes {
				built := h.model.accounts.page
				c.change()
				if got := h.model.accounts.page != built; got != tt.rebuilt {
					t.Errorf("on %s, %s built the page afresh %t, want %t", tt.name, c.name, got, tt.rebuilt)
				}
			}
		})
	}
}

// A fold of yesterday read just after midnight, before the ledger's tail
// has read the day's lines it owes, is read again once it has: the day's
// last request, its line filed after the first read, is charted in its hour.
func TestAFoldOfYesterdayReadJustAfterMidnightIsReadAgainOnceTheTailHasItsLines(t *testing.T) {
	h := projectsHarness(t, dashboard.Accounts)
	today := dayfile.DayStart(start.Local(), 0)
	h.clock.now = today.Add(23*time.Hour + 50*time.Minute)
	h.start()
	h.keyed("-")
	if h.model.tail.day.Date != dayfile.DateOf(today) {
		t.Fatalf("on today, the tail was last read on %q, want today", h.model.tail.day.Date)
	}

	h.clock.now = today.Add(day + 30*time.Second)
	date := dayfile.DateOf(today)
	if got := foldsKeyed(h, "<"); !slices.Equal(got, []string{date}) || !h.model.pastDay.early {
		t.Fatalf("stepped back to the day just ended, the folds of %v read, early %t; want its own read, early, the tail owing its lines", got, h.model.pastDay.early)
	}
	late := projectAsks(betaOnly, "/code/beta", today.Add(23*time.Hour+58*time.Minute), 1)
	projectsLedgerOf(h).write(late...)
	charted := func() views.Worth { return h.model.accounts.page.All.WorthBy[0].Buckets[23].Worth }
	if got := charted(); got.Amount != nil {
		t.Fatalf("before the tail's read, 23:00 is charted as %s, want nothing: the line came after", got.Cents())
	}

	pending := h.lastTick()
	h.clock.now, pending.fired = today.Add(day+time.Minute), true
	if got := foldsRead(h, h.update(pending.msg)...); !slices.Equal(got, []string{date}) || h.model.pastDay.early {
		t.Fatalf("the tail read past midnight, the folds of %v read, early %t; want the day's read again, no longer early", got, h.model.pastDay.early)
	}
	if got := charted(); got.Amount == nil {
		t.Errorf("read again, 23:00 is charted as nothing, want the day's last request")
	}
	if got := foldsKeyed(h, "R"); len(got) != 0 {
		t.Errorf("a look after, the folds of %v were read, want none", got)
	}
}

// A past period keeps its reads as midnight passes: the look after it reads
// the days since the day just ended alone, and each tab's past reads none
// again; the period's dates, and its page, are as they were.
func TestMidnightKeepsAPastPeriodsReads(t *testing.T) {
	tests := []struct {
		name  string
		view  dashboard.View
		shown dashboard.AccountsView
		keys  string
		// reads counts the past period's reads of what the tab shows.
		reads func(m Model) int
	}{
		{name: "Compare, the week of 14 September", view: dashboard.Accounts, keys: "< <", reads: func(m Model) int { return m.past.backs }},
		{name: "Compare, yesterday", view: dashboard.Accounts, keys: "- <", reads: func(m Model) int { return m.pastDay.read }},
		{name: "Detail, yesterday", view: dashboard.Accounts, shown: dashboard.AccountsDetail, keys: "- <", reads: func(m Model) int { return m.pastLately.held }},
		{name: "Projects, last week", view: dashboard.Projects, keys: "<", reads: func(m Model) int { return m.sessionDays.asked }},
		{name: "Projects, yesterday", view: dashboard.Projects, keys: "- <", reads: func(m Model) int { return m.pastDay.read }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := projectsHarness(t, tt.view)
			h.model.accounts.shown.Shown = tt.shown
			h.model.cfg.Events = pastLatelyFiles(h.clock)
			h.start()
			h.keyed(tt.keys)
			period, reads := h.model.period, tt.reads(h.model)
			if reads == 0 {
				t.Fatalf("on %q, the past period read nothing, want its read", said(h, period))
			}
			h.lookAt(dayfile.DayStart(start.Local(), 1).Add(30 * time.Minute))
			if h.model.period != period || tt.reads(h.model) != reads {
				t.Errorf("past midnight, the dial shows %q, read %d times; want %q kept, read %d times", said(h, h.model.period), tt.reads(h.model), said(h, period), reads)
			}
			if p := h.model.accounts.page; tt.view == dashboard.Accounts && !pageOfThePeriod(h) {
				t.Errorf("past midnight, Accounts' page is %v, want one of %q", p, said(h, period))
			}
			if p := h.model.projects.shown.Page; tt.view == dashboard.Projects && (p == nil || p.Period != period) {
				t.Errorf("past midnight, Projects' page is %v, want one of %q", p, said(h, period))
			}
		})
	}
}

// Past midnight, the look on a past period reads Accounts' days since the
// day just ended alone, never again those it reached back to.
func TestMidnightReadsTheDaysSinceAlone(t *testing.T) {
	h, l := pastHarness(t, dashboard.Accounts)
	h.start()
	h.keyed("< <")
	back := h.model.past.back.from
	read := len(l.reads)
	h.lookAt(dayfile.DayStart(start.Local(), 1).Add(30 * time.Minute))
	if got := l.reads[read:]; !slices.EqualFunc(got, []time.Time{todayStart}, time.Time.Equal) || !h.model.past.back.from.Equal(back) {
		t.Errorf("past midnight, the ledger was read from %v, held back from %s; want the day just ended alone, %s, held back from %s", got, h.model.past.back.from, todayStart, back)
	}
}

// Detail in a past period lists no sessions running, as none runs in the
// past: → doesn't move into its SESSIONS, and enter, faint, says why in the
// key line's place; at now, both act. Stepped back from its SESSIONS, the
// list has the choice again.
func TestDetailInAPastPeriodListsNoSessions(t *testing.T) {
	tests := []struct {
		name       string
		keys       string
		inSessions bool
		note       string
	}{
		{name: "at now", inSessions: true},
		{name: "a past period", keys: "<", note: "no sessions in a past period"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := accountsHarness(t, dashboard.AccountsShown{Shown: dashboard.AccountsDetail, Chosen: "work"})
			h.keyed(tt.keys)
			h.keys(rightKey)
			if got := h.model.accounts.shown.InSessions; got != tt.inSessions {
				t.Errorf("→ in %s took the focus into SESSIONS %t, want %t", tt.name, got, tt.inSessions)
			}
			h.keys(enterKey)
			if opened := h.model.session != nil; opened != tt.inSessions || h.model.note != tt.note {
				t.Errorf("enter in %s opened a page %t, noting %q; want a page %t, noting %q", tt.name, opened, h.model.note, tt.inSessions, tt.note)
			}
		})
	}

	h := accountsHarness(t, dashboard.AccountsShown{Shown: dashboard.AccountsDetail, Chosen: "work"})
	h.keys(rightKey, downKey)
	h.keyed("<")
	if s := h.model.accounts.shown; s.InSessions || s.Selected != "" {
		t.Errorf("stepped back from SESSIONS, ↑↓ in them %t, %q selected; want the list's choice, none selected", s.InSessions, s.Selected)
	}
}

// Detail's LATELY in a past period is told in the watch of the router's
// events up to its end, of the account Detail shows, and handed to the
// frame: told once, a look leaving it as told, told afresh as another
// account is chosen; none at now.
func TestDetailsPastLATELYIsToldInTheWatchOnce(t *testing.T) {
	h := accountsHarness(t, dashboard.AccountsShown{Shown: dashboard.AccountsDetail, Chosen: "work"})
	h.model.cfg.Events = pastLatelyFiles(h.clock)
	h.keyed("- <")
	told := func(id string) []views.Sentence {
		held, until, ok := h.model.pastLatelyHeld()
		if !ok {
			t.Fatal("on yesterday, Detail holds no events for its LATELY")
		}
		return views.AccountLatelyTo(h.model.moment(h.clock.now), held, id, until, everyEventTold, views.SessionDirsOf(h.model.sessions, h.model.held))
	}
	lately := h.model.frame(h.clock.now).Accounts.Lately
	if want := told("work"); len(lately) == 0 || !slices.EqualFunc(lately, want, sameSentence) {
		t.Fatalf("on yesterday, Detail's LATELY is %v, want work's to yesterday's end, %v", sentenceIDs(lately), sentenceIDs(want))
	}
	h.tickUntilAsked()
	if again := h.model.frame(h.clock.now).Accounts.Lately; len(again) == 0 || &again[0] != &lately[0] {
		t.Errorf("a look told Detail's LATELY afresh, want it as told")
	}
	h.keys(upKey)
	if got, want := h.model.frame(h.clock.now).Accounts.Lately, told(""); !slices.EqualFunc(got, want, sameSentence) || len(got) == len(lately) && &got[0] == &lately[0] {
		t.Errorf("all accounts chosen, Detail's LATELY is %v, want every account's, %v, told afresh", sentenceIDs(got), sentenceIDs(want))
	}
	h.keyed("n")
	if got := h.model.frame(h.clock.now).Accounts.Lately; got != nil {
		t.Errorf("back at now, Detail's LATELY handed to the frame is %v, want none: Detail tells the document's", sentenceIDs(got))
	}
}

// everyEventTold asks LATELY for every event the router's files hold.
const everyEventTold = 1 << 30

// sameSentence reports whether a and b say alike, by their ids, times and
// full words.
func sameSentence(a, b views.Sentence) bool {
	return a.ID == b.ID && a.At.Equal(b.At) && fmt.Sprint(a.Full) == fmt.Sprint(b.Full)
}

// sentenceIDs are the sentences' ids, in their order.
func sentenceIDs(sentences []views.Sentence) []int {
	var ids []int
	for _, s := range sentences {
		ids = append(ids, s.ID)
	}
	return ids
}

// A page of a past period is built of the List's sessions running alone, by
// their ids, as a session of the period that runs now went on after it,
// though it lists none: one running moving to another account builds last
// week's page nothing, as it builds now's afresh; one starting to run builds
// both afresh.
func TestAPastPeriodsPageIsBuiltOfTheSessionsRunningByIdAlone(t *testing.T) {
	tests := []struct {
		name  string
		keys  string
		moved int
	}{
		{name: "this week", moved: 1},
		{name: "last week", keys: "<"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := projectsHarness(t, dashboard.Projects)
			h.source.sessions = []status.Session{sessionOn(alphaFirst, opus, "work", 10*time.Second)}
			h.start()
			h.keyed(tt.keys)
			pages := h.model.projects.pages
			if p := h.model.projects.shown.Page; p == nil || p.Period != h.model.period {
				t.Fatalf("on %s, Projects shows %v, want its page", tt.name, p)
			}
			h.source.sessions = []status.Session{sessionOn(alphaFirst, opus, "side", 10*time.Second)}
			h.tickUntilAsked()
			if got := h.model.projects.pages - pages; got != tt.moved {
				t.Errorf("on %s, a session running moved to side built the page %d times, want %d", tt.name, got, tt.moved)
			}
			pages = h.model.projects.pages
			h.source.sessions = append(h.source.sessions, sessionOn(gammaOnly, opus, "work", 10*time.Second))
			h.tickUntilAsked()
			if h.model.projects.pages == pages {
				t.Errorf("on %s, gamma starting to run built the page no times, want it built afresh", tt.name)
			}
		})
	}
}

// A step back on Projects reads the sessions' days once, from the first
// day of the period it steps to, or the first the ledger holds, where those
// held don't reach it; a step on reads nothing, nor a step back to days
// held.
func TestAStepBackOnProjectsReadsTheSessionsDaysItReachesOnce(t *testing.T) {
	h := withDelta(projectsHarness(t, dashboard.Projects))
	h.start()
	steps := []struct {
		key   string
		reads bool
	}{
		{key: "<", reads: true},
		{key: ">"},
		{key: "<"},
		{key: "<", reads: true},
		{key: "n"},
	}
	for i, step := range steps {
		was, asked := h.model.period, h.model.sessionDays.asked
		h.keyed(step.key)
		name := fmt.Sprintf("step %d, %s from %q to %q", i+1, step.key, said(h, was), said(h, h.model.period))
		reads := h.model.sessionDays.asked - asked
		if want := map[bool]int{true: 1}[step.reads]; reads != want {
			t.Fatalf("%s read the sessions' days %d times, want %d", name, reads, want)
		}
		if want := h.model.daysFrom(h.clock.now); step.reads && !h.model.sessionDays.from.Equal(want) {
			t.Errorf("%s read them from %s, want %s, the period's first day or the ledger's", name, h.model.sessionDays.from, want)
		}
		if p := h.model.projects.shown.Page; p == nil || p.Period != h.model.period {
			t.Fatalf("%s, Projects shows %v, want the period's page", name, p)
		}
	}
}
