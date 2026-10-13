package viewstest

import (
	"encoding/json"
	"maps"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// AccountsMoment is the request ledger's days, the router's document, its
// sessions and what the request stream told, at a moment Accounts has to get
// right, the period it's shown over, and what's wanted of it then.
type AccountsMoment struct {
	Name string
	// Accounts are the configured accounts, in the config's order, and
	// WeekStarts the day the calendar's weeks start on.
	Accounts   config.Accounts
	WeekStarts time.Weekday
	// Days are the summaries the ledger holds of each local day from the
	// first it holds to Now's, oldest first.
	Days []ledger.Summary
	// Doc is the router's document, or probing's, and Now when it's all
	// read: the days, the document, the sessions and the stream.
	Doc status.Document
	Now time.Time
	// Period is the period Accounts is shown over.
	Period views.AccountsPeriod
	// Sessions are those the router lists, and Stream what the request
	// stream told.
	Sessions []status.Session
	Stream   views.Stream
	// Page are the figures wanted of the page as a whole, and Want those of
	// its rows, by each account's id, "" for every account's: each by its
	// name, as Figure names them, written as a test reads it off.
	Page Figures
	Want map[string]Figures
}

// Figures are figures wanted of Accounts at a moment, by their names: those
// a moment doesn't name aren't what it's for.
type Figures map[string]string

// The names of the figures wanted of Accounts' page, its rows and the
// moment, each written as the views' tests read it off: of the page, whether
// its period records anything, "yes" or "no", the six weeks' first days, and
// how many of this week's days are to come; of a row, its requests, its
// sessions served, the sessions moved on and off, as "on 1 off 3", its limits
// by window, as "5h×2 7d×1", its minutes at a cap or a limit, "—" where
// none was read, and whether some days never read them, "partial"; its share
// left at its last reset, "—" where none ended, and the date its week ran
// out in, "" where it didn't; its plan's name and its price, as "Max 20x
// $200/mo", its worth, as Worth.Dollars says it, and against its price, as
// "4.0× of $46"; its six weeks' peaks, as "55 100 — 34…"; its verdict, as
// "ends most weeks with room 2/6 23%", its limits over its weeks, and its
// share left on average; of the moment, its week window's use, WEEK USED's,
// the sessions running on the account, by their shown ids, and its LATELY,
// each sentence in full, " | " between.
const (
	FigureRecorded  = "recorded"
	FigureWeeks     = "weeks"
	FigureToCome    = "to come"
	FigureRequests  = "requests"
	FigureSessions  = "sessions"
	FigureMoved     = "moved"
	FigureLimits    = "limits"
	FigureMinutes   = "minutes"
	FigurePartial   = "partial"
	FigureLeft      = "left"
	FigureRanOut    = "ran out"
	FigurePlan      = "plan"
	FigureWorth     = "worth"
	FigureAgainst   = "against"
	FigurePeaks     = "peaks"
	FigureVerdict   = "verdict"
	FigureWeekUsed  = "week used"
	FigureRunning   = "running"
	FigureLately    = "lately"
	FigurePlansOf   = "plans known"
	FigureMostLeft  = "most left"
	FigureLeastLeft = "least left"
)

// AccountsMoments are the moments Accounts has to get right: a week at its
// limit reset before the router reads it again, at 20:59:59, at 21:00:01
// before the reread, once it's reread, the reset filed, and reread before the
// days file it; fewer than
// seven weeks recorded; a fresh install; an account added part-way through
// the period; ten accounts, extra usage on for one; six sessions on the
// account shown, then the one selected ended; today at midnight, and this
// week's first day at midnight; sessions moved both ways, limits in two
// windows, a week run out and one used up with no limit told; no plan, two
// of three plans, worth part unpriced and wholly unpriced; a summary of
// version 1 among the days; and probing, with no sessions and no events.
func AccountsMoments() []AccountsMoment {
	return slices.Concat(weekResetBeforeTheReread(), []AccountsMoment{
		fewerThanSevenWeeks(), freshInstall(), addedPartWay(), tenAccounts(),
	}, sixSessions(), atMidnight(), []AccountsMoment{
		movesLimitsAndRunsOut(), plansAndUnpriced(), versionOneAmongTheDays(), probing(),
	})
}

// The accounts the moments configure: work and side on Max 20x, personal,
// the primary, on Max 20x too, and spare on Max 5x.
var (
	workMax    = config.Account{ID: work, Label: work, Plan: "max20x"}
	personalMx = config.Account{ID: personal, Label: personal, Primary: true, Plan: "max20x"}
	sideMax    = config.Account{ID: side, Label: side, Plan: "max20x"}
	spareFive  = config.Account{ID: "spare", Label: "spare", Plan: "max5x"}
)

// in2026 is the local time in 2026, on the day of the month given, at the
// clock given.
func in2026(month time.Month, day, hour, minute, second int) time.Time {
	return time.Date(2026, month, day, hour, minute, second, 0, time.Local)
}

// weekResetBeforeTheReread is the v0.1.6 case on Monday 5 October: work's
// week, which resets each Monday at 21:00, at its limit since Friday 2
// October, resets at 21:00, and the router reads it again at 21:00:30; side's
// resets each Monday at 10:00. At 20:59:59, work's week still runs, placed
// under this week, and the one before shows none of its. At 21:00:01, before
// the reread, WEEK USED reads none, and its left at its last reset, its
// peaks and its verdict change with it: the week that reset closed at its
// use, 100%, under 28 September, a whole week, so its verdict now counts it.
// Once reread, the days hold the reset, and every figure is as it was at
// 21:00:01; and so it is reread over days read before 21:00, as a page held
// since then has them.
func weekResetBeforeTheReread() []AccountsMoment {
	accounts := config.Accounts{workMax, sideMax}
	asRead, reread := in2026(time.October, 5, 20, 59, 30), in2026(time.October, 5, 21, 0, 30)
	before := mondayWeeks(false)
	readBefore := readDoc(asRead,
		withLimit(readAccount(work, asRead, window(fiveHour, 0.2, in2026(time.October, 5, 23, 30, 0)), spent(week, 1, in2026(time.October, 5, 21, 0, 0))),
			[]string{week}, in2026(time.October, 5, 21, 0, 0)),
		readAccount(side, asRead, window(fiveHour, 0.1, in2026(time.October, 6, 1, 0, 0)), window(week, 0.04, in2026(time.October, 12, 10, 0, 0))))
	readAgain := readDoc(reread,
		readAccount(work, reread, window(fiveHour, 0.2, in2026(time.October, 5, 23, 30, 0)), window(week, 0, in2026(time.October, 12, 21, 0, 0))),
		readAccount(side, reread, window(fiveHour, 0.1, in2026(time.October, 6, 1, 0, 0)), window(week, 0.04, in2026(time.October, 12, 10, 0, 0))))
	sixWeeks := Figures{FigureWeeks: "2026-08-31 2026-09-07 2026-09-14 2026-09-21 2026-09-28 2026-10-05"}
	afterTheReset := map[string]Figures{
		work: {FigureWeekUsed: "0%", FigureLeft: "0%", FigureRanOut: "2026-10-02", FigurePeaks: "55 100 66 74 100 0…",
			FigureVerdict: views.HitsItsLimit + " 3/7 19%"},
		side: {FigureLeft: "50%", FigurePeaks: "40 35 20 45 50 4…", FigureVerdict: views.PlentyToSpare + " 0/7 65%"},
		"":   {FigurePeaks: "48 68 43 60 75 2…", FigureVerdict: views.RoomToSpare + " 3/7 42%", FigureMostLeft: side, FigureLeastLeft: work},
	}
	return []AccountsMoment{
		{
			Name: "a week at its limit at 20:59:59, a second before it resets", Accounts: accounts, WeekStarts: time.Monday,
			Days: before, Doc: readBefore, Now: in2026(time.October, 5, 20, 59, 59), Page: sixWeeks,
			Want: map[string]Figures{
				work: {FigureWeekUsed: "100%", FigureLeft: "26%", FigureRanOut: "", FigurePeaks: "55 100 66 74 — 100…",
					FigureVerdict: views.EndsWithRoom + " 2/6 22%"},
				side: {FigureLeft: "50%", FigurePeaks: "40 35 20 45 50 4…", FigureVerdict: views.PlentyToSpare + " 0/7 65%"},
				"":   {FigurePeaks: "48 68 43 60 50 52…", FigureVerdict: views.RoomToSpare + " 2/6 45%", FigureMostLeft: side, FigureLeastLeft: work},
			},
		},
		{
			Name: "a week reset at 21:00, at 21:00:01, before the router reads it again", Accounts: accounts, WeekStarts: time.Monday,
			Days: before, Doc: readBefore, Now: in2026(time.October, 5, 21, 0, 1), Page: sixWeeks, Want: afterTheReset,
		},
		{
			Name: "a week reset at 21:00, reread at 21:00:30, the reset filed", Accounts: accounts, WeekStarts: time.Monday,
			Days: mondayWeeks(true), Doc: readAgain, Now: in2026(time.October, 5, 21, 0, 31), Page: sixWeeks, Want: afterTheReset,
		},
		{
			Name: "a week reset at 21:00, reread at 21:00:30, the days read before it", Accounts: accounts, WeekStarts: time.Monday,
			Days: before, Doc: readAgain, Now: in2026(time.October, 5, 21, 0, 31), Page: sixWeeks, Want: afterTheReset,
		},
	}
}

// mondayWeeks are the days from Monday 10 August to Monday 5 October of
// work, whose week resets each Monday at 21:00, and side, whose week resets
// each Monday at 10:00, today's as summarised before 21:00, or, where filed
// says, once work's week has reset at 21:00. Work's weeks from 17 August
// peaked at 100%, 70%, 55%, 100%, 66% and 74%, reaching its week's limit in
// the first and the fourth, and its week from 28 September at 100% on Friday
// 2 October at 16:00; side's from 17 August at 30%, 25%, 40%, 35%, 20%, 45%
// and 50%, its week since 10:00 today at 4%.
func mondayWeeks(filed bool) []ledger.Summary {
	f := filing{}
	f.weekly(work, in2026(time.August, 17, 21, 0, 0), 0.6, 1, 0.7, 0.55, 1, 0.66, 0.74)
	f.weekly(side, in2026(time.August, 17, 10, 0, 0), 0.2, 0.3, 0.25, 0.4, 0.35, 0.2, 0.45, 0.5)
	f.ranOut(work, in2026(time.August, 21, 15, 0, 0), in2026(time.September, 11, 12, 0, 0), in2026(time.October, 2, 16, 0, 0))
	for day, use := range map[int]float64{29: 0.3, 30: 0.6} {
		f.on(work, in2026(time.September, day, 12, 0, 0)).Highest[week] = use
	}
	for day := 1; day <= 5; day++ {
		f.on(work, in2026(time.October, day, 12, 0, 0)).Highest[week] = min(1, 0.6+0.2*float64(day))
	}
	today := f.on(work, in2026(time.October, 5, 12, 0, 0))
	today.Rise[week] = 0
	if filed {
		today.Resets = append(today.Resets, ledger.Reset{Window: week, At: in2026(time.October, 5, 21, 0, 0).UTC(), Before: 1})
	}
	sideToday := f.on(side, in2026(time.October, 5, 12, 0, 0))
	f.on(side, in2026(time.October, 4, 12, 0, 0)).Highest[week] = 0.48
	sideToday.Highest[week], sideToday.Rise[week] = 0.5, 0.06
	return f.summaries(in2026(time.August, 10, 0, 0, 0), in2026(time.October, 5, 21, 0, 0))
}

// fewerThanSevenWeeks is Thursday 1 October, the ledger begun on Monday 14
// September: work's weeks, from Sunday at 21:00, and side's, from Wednesday
// at 10:00, have ended in two whole weeks alone, which their verdicts are
// over. Work's first, from the 13th, began nearest the ledger's first week,
// so counts; side's first, from the 9th, nearest the week before it, so
// doesn't.
func fewerThanSevenWeeks() AccountsMoment {
	f := filing{}
	f.weekly(work, in2026(time.September, 20, 21, 0, 0), 0.4, 0.6)
	f.weekly(side, in2026(time.September, 16, 10, 0, 0), 0.25, 0.3, 0.35)
	for day, use := range map[int]float64{28: 0.2, 29: 0.25, 30: 0.3} {
		f.on(work, in2026(time.September, day, 12, 0, 0)).Highest[week] = use
	}
	f.on(work, in2026(time.October, 1, 12, 0, 0)).Highest[week] = 0.34
	now := At(14, 42, 7)
	return AccountsMoment{
		Name: "fewer than seven weeks recorded", Accounts: config.Accounts{workMax, sideMax}, WeekStarts: time.Monday,
		Days: f.summaries(in2026(time.September, 14, 0, 0, 0), now), Doc: readDoc(now), Now: now,
		Want: map[string]Figures{
			work: {FigureLeft: "40%", FigurePeaks: "— — — 40 60 34…", FigureVerdict: views.PlentyToSpare + " 0/2 50%"},
			side: {FigureLeft: "65%", FigurePeaks: "— — — 30 35 —", FigureVerdict: views.PlentyToSpare + " 0/2 68%"},
			"":   {FigureVerdict: views.RoomToSpare + " 0/2 59%", FigureMostLeft: side, FigureLeastLeft: work},
		},
	}
}

// freshInstall is Thursday 1 October, the day switchboard was installed: the
// router has read work's and side's windows, but nothing is recorded, so
// every figure is unknown.
func freshInstall() AccountsMoment {
	f := filing{}
	f.on(work, At(9, 0, 0))
	f.on(side, At(9, 0, 0))
	now := At(14, 42, 7)
	nothing := Figures{FigureRequests: "0", FigureWorth: "—", FigureLeft: "—", FigurePeaks: "— — — — — —", FigureVerdict: "0 weeks"}
	return AccountsMoment{
		Name: "a fresh install", Accounts: config.Accounts{workMax, sideMax}, WeekStarts: time.Monday,
		Days: f.summaries(At(0, 0, 0), now), Doc: readDoc(now), Now: now,
		Page: Figures{FigureRecorded: "no"},
		Want: map[string]Figures{work: nothing, side: nothing, "": {FigureRequests: "0", FigureWorth: "—", FigureVerdict: "0 weeks", FigureMostLeft: ""}},
	}
}

// addedPartWay is the last 30 days to Thursday 1 October, spare added on
// Monday 21 September: it shows what was recorded of it, priced against the
// whole 30 days.
func addedPartWay() AccountsMoment {
	f := filing{}
	for day := 2; day <= 30; day++ {
		f.serve(work, in2026(time.September, day, 11, 0, 0), opus, 1, "w"+dayfile.DateOf(in2026(time.September, day, 0, 0, 0)))
	}
	for day := 21; day <= 30; day++ {
		f.serve("spare", in2026(time.September, day, 11, 0, 0), haiku, 1, "s"+dayfile.DateOf(in2026(time.September, day, 0, 0, 0)))
	}
	now := At(14, 42, 7)
	return AccountsMoment{
		Name: "an account added part-way through the period", Accounts: config.Accounts{workMax, spareFive}, WeekStarts: time.Monday,
		Days: f.summaries(in2026(time.September, 2, 0, 0, 0), now), Doc: readDoc(now), Now: now, Period: views.AccountsThirtyDays,
		Want: map[string]Figures{
			work:    {FigureRequests: "29", FigureWorth: "$116", FigurePlan: "Max 20x $200/mo", FigureAgainst: "0.6× of $200"},
			"spare": {FigureRequests: "10", FigureWorth: "$10", FigurePlan: "Max 5x $100/mo", FigureAgainst: "0.1× of $100"},
			"":      {FigureRequests: "39", FigureSessions: "39", FigurePlan: "$300/mo", FigurePlansOf: "2 of 2", FigureAgainst: "0.4× of $300"},
		},
	}
}

// tenAccounts is Thursday 1 October, this week, of ten accounts, nine on Max
// 20x and spare on Max 5x, each with a request a day, extra usage on for
// spare alone.
func tenAccounts() AccountsMoment {
	ids := []string{work, personal, side, "client", "spare", "lab", "team", "extra", "ops", "studio"}
	var accounts config.Accounts
	var read []status.Account
	f := filing{}
	now := At(14, 42, 7)
	for _, id := range ids {
		a := config.Account{ID: id, Label: id, Plan: "max20x"}
		if id == "spare" {
			a.Plan = "max5x"
		}
		accounts = append(accounts, a)
		for day := 28; day <= 30; day++ {
			f.serve(id, in2026(time.September, day, 10, 0, 0), sonnet, 1, id+"-session")
		}
		account := readAccount(id, now, window(fiveHour, 0.1, At(18, 0, 0)), window(week, 0.3, in2026(time.October, 4, 21, 0, 0)))
		if id == "spare" {
			account.Extra = quota.ExtraUsage{Status: quota.StatusAllowed}
		}
		read = append(read, account)
	}
	return AccountsMoment{
		Name: "ten accounts, extra usage on for one", Accounts: accounts, WeekStarts: time.Monday,
		Days: f.summaries(in2026(time.September, 28, 0, 0, 0), now), Doc: readDoc(now, read...), Now: now,
		Want: map[string]Figures{
			"spare": {FigureRequests: "3", FigurePlan: "Max 5x $100/mo"},
			"":      {FigureRequests: "30", FigureSessions: "10", FigurePlan: "$1,900/mo", FigurePlansOf: "10 of 10"},
		},
	}
}

// sixSessions are six sessions running on side, the account Detail shows, at
// 14:42:07, one more on work, and side's d28c, the one selected, ended a
// minute on, the router no longer listing it.
func sixSessions() []AccountsMoment {
	ids := []string{D28C, C61B, "7f3a1c9e", "db8a71e3", "9c2e4b10", "41e0aa57"}
	listed := func(now time.Time, ended string) []status.Session {
		sessions := []status.Session{session("5b19c0de", seat(Opus, work, now.Add(-2*time.Minute), ""))}
		for i, id := range ids {
			if id != ended {
				sessions = append(sessions, session(id, seat(Opus, side, now.Add(-time.Duration(i)*time.Minute), "")))
			}
		}
		return sessions
	}
	accounts := config.Accounts{workMax, sideMax}
	doc := func(at time.Time) status.Document {
		return readDoc(at, readAccount(side, at, window(fiveHour, 0.13, At(18, 50, 0))), readAccount(work, at, window(fiveHour, 0.5, At(17, 10, 0))))
	}
	later := At(14, 43, 7)
	return []AccountsMoment{
		{
			Name: "six sessions on the account shown", Accounts: accounts, WeekStarts: time.Monday,
			Days: filing{}.summaries(At(0, 0, 0), now), Doc: doc(now), Now: now, Sessions: listed(now, ""),
			Want: map[string]Figures{side: {FigureRunning: "d28c c61b 7f3a db8a 9c2e 41e0"}, work: {FigureRunning: "5b19"}},
		},
		{
			Name: "the session selected ended", Accounts: accounts, WeekStarts: time.Monday,
			Days: filing{}.summaries(At(0, 0, 0), later), Doc: doc(later), Now: later, Sessions: listed(later, D28C),
			Want: map[string]Figures{side: {FigureRunning: "c61b 7f3a db8a 9c2e 41e0"}},
		},
	}
}

// atMidnight are today at a second to midnight on Wednesday 30 September and
// a second after; and this week, from Monday, at a second to midnight on
// Sunday 4 October and a second after, on its first day: a period just begun
// is priced whole, and records nothing yet. Work serves a request at 10:00
// each day from Monday 28 September.
func atMidnight() []AccountsMoment {
	accounts := config.Accounts{workMax}
	moment := func(name string, now time.Time, period views.AccountsPeriod, recorded string, want Figures) AccountsMoment {
		f := filing{}
		for at := in2026(time.September, 28, 10, 0, 0); at.Before(now); at = at.AddDate(0, 0, 1) {
			f.serve(work, at, opus, 1, "w-session")
		}
		return AccountsMoment{
			Name: name, Accounts: accounts, WeekStarts: time.Monday, Days: f.summaries(in2026(time.September, 28, 0, 0, 0), now),
			Doc: readDoc(now), Now: now, Period: period, Page: Figures{FigureRecorded: recorded}, Want: map[string]Figures{work: want},
		}
	}
	return []AccountsMoment{
		moment("today at a second to midnight", in2026(time.September, 30, 23, 59, 59), views.AccountsToday, "yes",
			Figures{FigureRequests: "1", FigureAgainst: "0.6× of $7"}),
		moment("today at a second after midnight", in2026(time.October, 1, 0, 0, 1), views.AccountsToday, "no",
			Figures{FigureRequests: "0", FigureAgainst: "0.0× of $7"}),
		moment("this week at a second to midnight on its last day", in2026(time.October, 4, 23, 59, 59), views.AccountsThisWeek, "yes",
			Figures{FigureRequests: "7", FigureAgainst: "0.6× of $46"}),
		moment("this week on its first day, a second after midnight", in2026(time.October, 5, 0, 0, 1), views.AccountsThisWeek, "no",
			Figures{FigureRequests: "0", FigureAgainst: "0.0× of $46"}),
	}
}

// movesLimitsAndRunsOut is Thursday 1 October, this week: three sessions
// moved off work to side on Tuesday, one off personal onto work on
// Wednesday, and one off personal onto side today; work reached its 5-hour
// limit on Monday and Tuesday and its week's on Wednesday. Work's last week
// ended at its limit, reached on Friday 25 September; personal's ended used
// up with no limit told; side's ended with 40% left. The router's events
// tell of work's cap and the sessions it moved, a session starting on side,
// the router's best, personal under pressure, a pin of personal and side,
// work primed, and a restart falling due.
func movesLimitsAndRunsOut() AccountsMoment {
	f := filing{}
	f.weekly(work, in2026(time.September, 27, 21, 0, 0), 1)
	f.weekly(personal, in2026(time.September, 27, 21, 0, 0), 1)
	f.weekly(side, in2026(time.September, 28, 10, 0, 0), 0.6)
	f.ranOut(work, in2026(time.September, 25, 11, 0, 0))
	f.serve(work, in2026(time.September, 28, 9, 0, 0), opus, 2, "w1", "w2")
	f.serve(personal, in2026(time.September, 28, 9, 0, 0), sonnet, 2, "p1", "p2")
	f.serve(side, in2026(time.September, 28, 9, 0, 0), haiku, 1, "s1")
	f.moved(work, side, in2026(time.September, 29, 14, 0, 0), 3)
	f.moved(personal, work, in2026(time.September, 30, 14, 0, 0), 1)
	f.moved(personal, side, At(11, 0, 0), 1)
	f.limited(work, fiveHour, in2026(time.September, 28, 15, 0, 0), in2026(time.September, 29, 16, 0, 0))
	f.limited(work, week, in2026(time.September, 30, 18, 0, 0))
	now := At(14, 42, 7)
	doc := readDoc(now,
		readAccount(work, now, window(fiveHour, 0.5, At(17, 10, 0)), window(week, 0.34, in2026(time.October, 4, 21, 0, 0))),
		readAccount(personal, now, window(fiveHour, 0.4, At(16, 30, 0)), window(week, 0.66, in2026(time.October, 4, 21, 0, 0))),
		readAccount(side, now, window(fiveHour, 0.13, At(18, 50, 0)), window(week, 0.78, in2026(time.October, 5, 10, 0, 0))))
	doc.Best = side
	doc.Events = numbered(
		status.Event{At: At(14, 41, 11), Kind: status.EventStarted, Account: side, Session: C61B, Reason: status.ReasonNew},
		status.Event{At: At(14, 40, 0), Kind: status.EventRestart, Reason: "the config changed"},
		status.Event{At: At(14, 38, 0), Kind: status.EventPressure, Account: personal, Windows: []string{fiveHour}, Until: At(16, 5, 0)},
		status.Event{At: At(14, 31, 40), Kind: status.EventMoved, Session: "db8a71e3", From: work, To: side, Reason: "moved: work is at its reserve", ForcedBy: 3},
		status.Event{At: At(14, 31, 20), Kind: status.EventMoved, Session: "7f3a1c9e", From: work, To: side, Reason: "moved: work is at its reserve", ForcedBy: 3},
		status.Event{At: At(14, 31, 3), Kind: status.EventCap, Account: work, Windows: []string{fiveHour}, Reserve: 0.05, Count: 2, To: side},
		status.Event{At: At(13, 40, 0), Kind: status.EventPin, Account: personal, Accounts: []string{personal, side}},
		status.Event{At: At(12, 10, 0), Kind: status.EventPrimed, Account: work, Windows: []string{fiveHour}, Until: At(17, 10, 0)},
	)
	return AccountsMoment{
		Name: "moves both ways, limits in two windows, a week run out and one used up", Accounts: config.Accounts{workMax, personalMx, sideMax},
		WeekStarts: time.Monday, Days: f.summaries(in2026(time.September, 21, 0, 0, 0), now), Doc: doc, Now: now,
		Want: map[string]Figures{
			work: {FigureMoved: "on 1 off 3", FigureLimits: "5h×2 7d×1", FigureLeft: "0%", FigureRanOut: "2026-09-25",
				FigureLately: "reached its cap: 2 sessions moved to side | primed: its 5-hour window started"},
			personal: {FigureMoved: "on 0 off 2", FigureLimits: "", FigureLeft: "0%", FigureRanOut: "",
				FigureLately: "came under pressure: out ~16:05 at this pace | pinned with side, by hand"},
			side: {FigureMoved: "on 4 off 0", FigureLeft: "40%", FigureRanOut: "",
				FigureLately: "c61b started here, the best | 2 sessions came from work: work reached its cap | pinned with personal, by hand"},
			"": {FigureMoved: "on 5 off 5", FigureLimits: "5h×2 7d×1", FigureLeft: "—",
				FigureLately: "c61b started on side, the best | personal came under pressure: its 5-hour runs out ~16:05 | work reached its cap: 2 sessions moved to side"},
		},
	}
}

// plansAndUnpriced is Thursday 1 October, this week: work on Max 20x, its
// requests priced; side on Pro, one of its requests of a model the price
// table doesn't know; and spare, with no plan, its requests all of that
// model.
func plansAndUnpriced() AccountsMoment {
	f := filing{}
	f.serve(work, in2026(time.September, 29, 10, 0, 0), opus, 2, "w1")
	f.serve(side, in2026(time.September, 29, 10, 0, 0), opus, 1, "s1")
	f.serve(side, in2026(time.September, 29, 11, 0, 0), unknownModel, 1, "s1")
	f.serve("spare", in2026(time.September, 30, 10, 0, 0), unknownModel, 2, "x1")
	now := At(14, 42, 7)
	return AccountsMoment{
		Name:     "no plan, two of three plans, worth part and wholly unpriced",
		Accounts: config.Accounts{workMax, {ID: side, Label: side, Plan: "pro"}, {ID: "spare", Label: "spare"}}, WeekStarts: time.Monday,
		Days: f.summaries(in2026(time.September, 28, 0, 0, 0), now), Doc: readDoc(now), Now: now,
		Want: map[string]Figures{
			work:    {FigureWorth: "$8", FigurePlan: "Max 20x $200/mo", FigureAgainst: "0.2× of $46"},
			side:    {FigureWorth: "$4+", FigurePlan: "Pro $20/mo", FigureAgainst: "0.9× of $5"},
			"spare": {FigureWorth: "unpriced", FigurePlan: "—", FigureAgainst: "—"},
			"":      {FigureWorth: "$12+", FigurePlan: "$220/mo", FigurePlansOf: "2 of 3", FigureAgainst: "0.2× of $51"},
		},
	}
}

// versionOneAmongTheDays is Thursday 1 October, this week, Monday summarised
// by a router of version 1, which read no minutes: work's minutes at its cap
// and at a limit are those the days since read, partly known.
func versionOneAmongTheDays() AccountsMoment {
	f := filing{}
	f.serve(work, in2026(time.September, 29, 10, 0, 0), opus, 1, "w1")
	f.on(work, in2026(time.September, 29, 10, 0, 0)).MinutesAtCap = new(40)
	f.on(work, in2026(time.September, 30, 10, 0, 0)).MinutesAtLimit = new(15)
	now := At(14, 42, 7)
	days := f.summaries(in2026(time.September, 28, 0, 0, 0), now)
	days[0] = ledger.Summary{Version: 1, Day: days[0].Day, Accounts: []ledger.AccountDay{
		{Account: work, Models: []ledger.ModelDay{modelDay(opus, 2)}, Sessions: 2},
	}}
	return AccountsMoment{
		Name: "a summary of version 1 among the days", Accounts: config.Accounts{workMax}, WeekStarts: time.Monday,
		Days: days, Doc: readDoc(now), Now: now,
		Want: map[string]Figures{
			work: {FigureRequests: "3", FigureSessions: "3", FigureMinutes: "55", FigurePartial: "partial"},
			"":   {FigureMinutes: "55", FigurePartial: "partial"},
		},
	}
}

// probing is Thursday 1 October with no router, the dashboard probing the
// accounts itself: no sessions listed, no events told.
func probing() AccountsMoment {
	now := At(14, 42, 7)
	doc := readDoc(now, readAccount(work, now, window(fiveHour, 0.5, At(17, 10, 0))), readAccount(side, now, window(fiveHour, 0.13, At(18, 50, 0))))
	doc.Source, doc.Router = status.SourceProbe, status.Health{}
	return AccountsMoment{
		Name: "probing, no sessions and no events", Accounts: config.Accounts{workMax, sideMax}, WeekStarts: time.Monday,
		Days: filing{}.summaries(At(0, 0, 0), now), Doc: doc, Now: now,
		Want: map[string]Figures{work: {FigureRunning: "", FigureLately: ""}, side: {FigureRunning: "", FigureLately: ""}, "": {FigureLately: ""}},
	}
}

// unknownModel is a model the price table doesn't know, so its requests'
// worth is unpriced.
const unknownModel = "claude-opus-9"

// filing files accounts' days as the ledger summarises them: by the local
// day's date, then by the account's id.
type filing map[string]map[string]*ledger.AccountDay

// on is the day of the account with the given id on the local day t falls
// on, filed afresh, with no request yet, its sessions named and its windows'
// and minutes read, where it isn't yet.
func (f filing) on(id string, t time.Time) *ledger.AccountDay {
	date := dayfile.DateOf(t)
	if f[date] == nil {
		f[date] = make(map[string]*ledger.AccountDay)
	}
	if f[date][id] == nil {
		f[date][id] = &ledger.AccountDay{Account: id, SessionIDs: []string{}, Highest: map[string]float64{}, Rise: map[string]float64{},
			Resets: []ledger.Reset{}, MinutesAtCap: new(0), MinutesAtLimit: new(0)}
	}
	return f[date][id]
}

// serve files n requests of the model given, on the account with the given
// id, at t, of the sessions given.
func (f filing) serve(id string, t time.Time, model string, n int, sessions ...string) {
	d := f.on(id, t)
	d.Models = append(d.Models, modelDay(model, n))
	for _, s := range sessions {
		if !slices.Contains(d.SessionIDs, s) {
			d.SessionIDs = append(d.SessionIDs, s)
		}
	}
	d.Sessions = len(d.SessionIDs)
}

// modelDay is n requests of the model given, each a million tokens of
// input, as a day's model's requests went upstream.
func modelDay(model string, n int) ledger.ModelDay {
	usage, _ := json.Marshal(map[string]int{"input_tokens": n * 1_000_000})
	return ledger.ModelDay{Model: model, Upstream: n, Usage: usage}
}

// weekly files the resets of the week of the account with the given id,
// one a week from the first given, each its use just before it as befores
// gives, oldest first: each on its day, its highest that day.
func (f filing) weekly(id string, first time.Time, befores ...float64) {
	for i, before := range befores {
		at := first.AddDate(0, 0, 7*i)
		d := f.on(id, at)
		d.Resets = append(d.Resets, ledger.Reset{Window: week, At: at.UTC(), Before: before})
		d.Highest[week] = max(d.Highest[week], before)
	}
}

// ranOut files the account's week's limits, each reached at a time given.
func (f filing) ranOut(id string, at ...time.Time) {
	f.limited(id, week, at...)
}

// limited files the limits of the account's window with the given key,
// each reached at a time given.
func (f filing) limited(id, key string, at ...time.Time) {
	for _, t := range at {
		d := f.on(id, t)
		d.Limits = append(d.Limits, ledger.Limit{Window: key, At: t.UTC()})
	}
}

// moved files n sessions moved off one account onto another at t, as the
// requests that moved them count them.
func (f filing) moved(from, to string, t time.Time, n int) {
	f.on(from, t).MovedOff += n
	f.on(to, t).MovedOn += n
}

// summaries are the summaries of each local day from first's to last's,
// oldest first, of the days filed: each of version 2 or later, which names
// its sessions and reads its minutes, its accounts in the order of their
// ids.
func (f filing) summaries(first, last time.Time) []ledger.Summary {
	var days []ledger.Summary
	for _, date := range dayfile.Span(first, last) {
		s := ledger.Summary{Version: 2, Day: date}
		for _, id := range slices.Sorted(maps.Keys(f[date])) {
			s.Accounts = append(s.Accounts, *f[date][id])
		}
		days = append(days, s)
	}
	return days
}

// readDoc is the router's document, built at at, of the accounts given, as
// the router judges them then, new sessions going to the first.
func readDoc(at time.Time, accounts ...status.Account) status.Document {
	doc := status.Document{Source: status.SourceRouter, GeneratedAt: at, Router: status.Health{Healthy: true}}
	for _, a := range accounts {
		doc.Accounts = append(doc.Accounts, a.AsOf(claude.Policy, at))
	}
	if len(accounts) > 0 {
		doc.Best = accounts[0].ID
	}
	return doc
}

// readAccount is the account with the given id, its token set, its windows
// read at at as given.
func readAccount(id string, at time.Time, windows ...quota.Window) status.Account {
	return status.Account{ID: id, Label: id, TokenSet: true, FetchedAt: at, Windows: windows}
}

// withLimit returns a, the router having seen it reach its limit in the
// windows given, holding until until.
func withLimit(a status.Account, windows []string, until time.Time) status.Account {
	a.Limit = status.Limit{ID: 1, Windows: windows, Until: until}
	return a
}

// window is the window with the given key, used as given, resetting at
// resets.
func window(key string, used float64, resets time.Time) quota.Window {
	return quota.Window{Key: key, Label: claude.WindowLabel(key), Utilization: used, ResetsAt: resets}
}

// spent is the window with the given key at its limit, used as given,
// resetting at resets.
func spent(key string, used float64, resets time.Time) quota.Window {
	w := window(key, used, resets)
	w.Status = quota.StatusRejected
	return w
}

// numbered are the events given, newest first, numbered from the oldest, as
// the router's document gives them.
func numbered(events ...status.Event) []status.Event {
	for i := range events {
		events[i].ID = len(events) - i
	}
	return events
}
