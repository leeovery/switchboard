package viewstest

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// RunwayMoment is the router's document, the readings history and the
// ledger's days at a moment Runway has to get right, the view it's shown in,
// and what's wanted of its lanes then.
type RunwayMoment struct {
	Name string
	// Doc is the router's document, or probing's, worked out as the dashboard
	// is given it, and Now when it's read.
	Doc status.Document
	Now time.Time
	// Readings are the readings history's, oldest first, each window's run
	// before the timeline among them, and those the dashboard read itself;
	// Days the ledger's summaries of the days the view shows, oldest first,
	// with any before them it carries in; Kept when the first day the
	// readings history holds starts, zero where it holds every day the view
	// shows; and Seen when the readings the dashboard read itself begin, zero
	// where there are none.
	Readings []readings.Reading
	Days     []ledger.Summary
	Kept     time.Time
	Seen     time.Time
	// Window is the view; Day the day it's stepped back to, as dayfile.Civil
	// gives one, zero where it's live; WeekStarts the day weeks start on; and
	// Columns how many columns its lanes are.
	Window     views.RunwayWindow
	Day        time.Time
	WeekStarts time.Weekday
	Columns    int
	// Want are the figures wanted of each lane, by its account's id, and of
	// the pool's lane and the view as a whole under "": each by its name, as
	// the Runway figures name them, written as a test reads it off.
	Want map[string]Figures
}

// The names of the figures wanted of Runway, each written as the views'
// tests read it off, its times as "15:04", with ":05" where the seconds
// aren't none, a day other than Now's led by its weekday, as "Sun 21:00",
// and "—" where a time is zero. Of the view: now's column, "none" where the
// timeline has no now; the pool's count of accounts and its columns' counts
// with room; what the lanes draw, as the legend keys it; and COMING UP. Of a
// lane: its cells, its stretches without room, its resets, its room and its
// use.
//
//   - cells and the pool's columns are runs of like columns, ", " between,
//     each as "room 0-29", or "room 7" for one: a lane's by its cell, "blank",
//     "room", "runs out", "capped", "limited", "refused" or "no token"; the
//     pool's by its count with room, "blank" where none is known.
//   - stretches are "; " between, each its cell, start, "~" before it where
//     it's projected, and back, as "capped 14:31:03 – 17:10", then ", as it
//     resets" where it's back as the view's window resets, then "@" its
//     column, its columns, as "@5 5-20", and ", over" where it's over.
//   - resets are ", " between, each its column and time, then " back" where
//     a stretch ends at it, as "21 17:10 back".
//   - room is "yes" or "no".
//   - use is its window's key and its share, as "5h 34%", "stale, read 14:20"
//     after where it's stale; "5h highest 62%" stepped back, "5h highest —"
//     where none was read; else "not read yet", "no token" or "—".
//   - drawn are the cells, ", " between, then "│" where any lane marks a
//     reset.
//   - coming is "; " between, each its time, account and kind, then its
//     window, as "16:30 personal reset 5h", then " cap" where it's a cap's.
const (
	FigureNow       = "now"
	FigureOf        = "of"
	FigurePool      = "pool"
	FigureDrawn     = "drawn"
	FigureComing    = "coming"
	FigureCells     = "cells"
	FigureStretches = "stretches"
	FigureResets    = "resets"
	FigureRoom      = "room"
	FigureUse       = "use"
)

// RunwayMoments are the moments Runway has to get right: the v0.1.6 case in
// both views; just after a 5-hour reset, before the reread; a first read
// failed, and a failed read after an earlier one; a refused token, a refusal
// lifted early and a family's refusal; a hold with no event, probed, told by
// the readings alone, and told by neither; a cap the 5-hour began and the
// week took over, as its event tells and as the readings alone do; a cap
// outlasting a limit and a refusal; a hold begun before the span of the readings history the tail
// reads, and before its keep; the router not answering for 10 minutes; a
// pool of one, a pin naming an account at its limit, and one account;
// stepped back to a day with nothing read, a day older than the readings'
// keep, today across midnight, a 23- and a 25-hour day and a week across a
// clock change; a 5-hour window restarted by hand; a cap turning to a limit,
// and two limits too close; a run-out reaching the cap first; a model's own
// window at its limit; and the frames' moments.
func RunwayMoments() []RunwayMoment {
	return slices.Concat(
		weekResetBeforeRunwayRereads(), justAfterAFiveHourReset(), failedReads(), refusals(), holdsNoEventTells(), holdsEventsTell(), holdsTakenOver(), holdsChanging(), holdsAhead(),
		holdsOutlasted(), holdsCarriedIn(), routerSilent(), pools(), steppedBack(), liftedByHand(),
		[]RunwayMoment{liftedAsTheWeekResets()},
		[]RunwayMoment{tokenless(), restartedByHand(), capTurningToALimit(), twoLimitsClose(), runOutReachingTheCap(), modelsOwnLimit()},
		framesMoments(),
	)
}

// The view most moments are shown in: the 5-hour, 136 columns of 10 minutes,
// at 14:42:07 on Thursday 1 October 2026, from 13:40, so column c runs from
// 13:40 and 10c minutes, its moment 5 minutes in, and now's is 6.
const lanes = 136

// onThu is the time on Thursday 1 October 2026, in UTC, at the clock given;
// onWed the Wednesday before.
func onThu(hour, minute, second int) time.Time {
	return time.Date(2026, 10, 1, hour, minute, second, 0, time.UTC)
}

// onWed is the time on Wednesday 30 September 2026, in UTC, at the clock
// given.
func onWed(hour, minute, second int) time.Time {
	return time.Date(2026, 9, 30, hour, minute, second, 0, time.UTC)
}

// thursday is when most moments are shown: 14:42:07 on Thursday 1 October.
var thursday = onThu(14, 42, 7)

// readEach are a reading of each of the windows given of the account with
// the given id, read at at, as the readings history holds them.
func readEach(id string, at time.Time, windows ...quota.Window) []readings.Reading {
	return readings.Of(id, windows, at, readings.FromAnswer)
}

// routed is the router's document read at at, of the accounts given, its
// events those given, worked out as the dashboard is given it.
func routed(at time.Time, events []status.Event, accounts ...status.Account) status.Document {
	doc := readDoc(at, accounts...)
	doc.Events = numbered(events...)
	return doc.WorkedOut(claude.Policy())
}

// pinned is doc with the global pin naming the accounts given, set at at,
// worked out again.
func pinned(doc status.Document, at time.Time, ids ...string) status.Document {
	doc.Pin = status.Pin{Accounts: ids, Since: at}
	return doc.WorkedOut(claude.Policy())
}

// reserving is a with a reserve of the share given.
func reserving(a status.Account, reserve float64) status.Account {
	a.Reserve = reserve
	return a
}

// rated is a, the router having seen its window with the given key used at
// the rate given, a share of it an hour, over the half hour before at.
func rated(a status.Account, key string, rate float64, at time.Time) status.Account {
	a.Rates = append(slices.Clip(a.Rates), status.Rate{Window: key, Rate: rate, Since: at.Add(-30 * time.Minute)})
	return a
}

// sideOn is side, read at at, with room: its 5-hour a fifth used, resetting
// at 16:50, and its week 30%, resetting Monday 5 October at 10:00; and
// sideRead is the same read at 13:00.
func sideOn(at time.Time) status.Account {
	return readAccount(side, at, window(fiveHour, 0.2, onThu(16, 50, 0)), window(week, 0.3, mondayTen))
}

// mondayTen is side's week's reset: Monday 5 October at 10:00.
var mondayTen = time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)

// sideRead are side's readings: at 13:00, its 5-hour a tenth used, and as
// sideOn reads it at 14:41.
func sideRead() []readings.Reading {
	return slices.Concat(
		readEach(side, onThu(13, 0, 0), window(fiveHour, 0.1, onThu(16, 50, 0)), window(week, 0.3, mondayTen)),
		readEach(side, onThu(14, 41, 0), sideOn(onThu(14, 41, 0)).Windows...),
	)
}

// fiveHourAt is the 5-hour view at thursday, of the document and readings
// given, wanting what's given.
func fiveHourAt(name string, doc status.Document, read []readings.Reading, want map[string]Figures) RunwayMoment {
	return RunwayMoment{Window: views.RunwayFiveHour, Name: name, Doc: doc, Now: thursday, Readings: read, Columns: lanes, Want: want}
}

// sorted are the readings given, oldest first.
func sorted(read ...[]readings.Reading) []readings.Reading {
	all := slices.Concat(read...)
	slices.SortStableFunc(all, func(a, b readings.Reading) int { return a.At.Compare(b.At) })
	return all
}

// weekResetBeforeRunwayRereads is the v0.1.6 case: work's week, at its limit
// since Wednesday 10:00, resets at 21:00 on Thursday, and is read again at
// 21:00:30. Drawn at 22:50 in the week view, from Wednesday 22:50, every
// column before 21:00 is at its limit, and every one after it has room; so in
// the 5-hour view at 21:30, from 20:30.
func weekResetBeforeRunwayRereads() []RunwayMoment {
	thuNine := onThu(21, 0, 0)
	nextWeek := thuNine.AddDate(0, 0, 7)
	reread := onThu(21, 0, 30)
	workAt := readAccount(work, reread, window(fiveHour, 0.1, time.Date(2026, 10, 2, 1, 0, 0, 0, time.UTC)), window(week, 0, nextWeek))
	read := sorted(
		readEach(work, onWed(9, 0, 0), window(week, 0.9, thuNine)),
		readEach(work, onWed(10, 0, 0), spent(week, 1, thuNine)),
		readEach(work, reread, workAt.Windows...),
		readEach(side, onWed(9, 0, 0), window(week, 0.2, mondayTen)),
	)
	at := func(now time.Time) status.Document {
		return routed(now.Add(-30*time.Second), nil, workAt, readAccount(side, now.Add(-30*time.Second), window(week, 0.2, mondayTen)))
	}
	return []RunwayMoment{
		{
			Name: "the v0.1.6 case, the week view at 22:50", Doc: at(onThu(22, 50, 0)), Now: onThu(22, 50, 0), Readings: read,
			Window: views.RunwayWeek, Columns: lanes,
			Want: map[string]Figures{
				"": {FigureNow: "19", FigureOf: "2", FigurePool: "1 0-17, 2 18-135"},
				work: {
					FigureCells: "limited 0-17, room 18-135", FigureStretches: "limited Wed 10:00 – 21:00, as it resets @0 0-17, over",
					FigureResets: "", FigureRoom: "yes", FigureUse: "7d 0%",
				},
			},
		},
		{Window: views.RunwayFiveHour,
			Name: "the v0.1.6 case, the 5-hour view at 21:30", Doc: at(onThu(21, 30, 0)), Now: onThu(21, 30, 0), Readings: read, Columns: lanes,
			Want: map[string]Figures{
				"": {FigureNow: "6"},
				work: {
					FigureCells: "limited 0-2, room 3-135", FigureStretches: "limited Wed 10:00 – 21:00 @0 0-2, over",
					FigureResets: "27 Fri 01:00", FigureRoom: "yes", FigureUse: "5h 10%",
				},
			},
		},
	}
}

// justAfterAFiveHourReset has work's 5-hour, read at its limit at 16:50,
// reset at 17:10, the router not yet reading it again: at 17:09:59, held till
// 17:10, its │ there; at 17:10:01, room from now's column, its use none, the
// │ gone, its stretch over, the pool all with room again, and COMING UP
// without its back, all on the one second.
func justAfterAFiveHourReset() []RunwayMoment {
	reset := onThu(17, 10, 0)
	workAt := readAccount(work, onThu(16, 50, 0), spent(fiveHour, 1, reset), window(week, 0.5, mondayTen.Add(11*time.Hour)))
	sideAt := readAccount(side, onThu(16, 50, 0), window(fiveHour, 0.2, onThu(19, 0, 0)), window(week, 0.3, mondayTen))
	read := sorted(
		readEach(work, onThu(15, 0, 0), window(fiveHour, 0.6, reset), window(week, 0.5, mondayTen.Add(11*time.Hour))),
		readEach(work, onThu(16, 50, 0), workAt.Windows...),
		readEach(side, onThu(15, 0, 0), sideAt.Windows...),
	)
	doc := routed(onThu(17, 9, 30), nil, workAt, sideAt)
	return []RunwayMoment{
		{Window: views.RunwayFiveHour,
			Name: "just before a 5-hour reset", Doc: doc, Now: onThu(17, 9, 59), Readings: read, Columns: lanes,
			Want: map[string]Figures{
				"": {
					FigureNow: "6", FigurePool: "2 0-4, 1 5-6, 2 7-135",
					FigureComing: "17:10 work back; 19:00 side reset 5h",
				},
				work: {
					FigureCells: "room 0-4, limited 5-6, room 7-135", FigureStretches: "limited 16:50 – 17:10, as it resets @5 5-6",
					FigureResets: "7 17:10 back", FigureRoom: "no", FigureUse: "5h 100%",
				},
			},
		},
		{Window: views.RunwayFiveHour,
			Name: "just after a 5-hour reset, before the reread", Doc: doc, Now: onThu(17, 10, 1), Readings: read, Columns: lanes,
			Days: []ledger.Summary{{Version: 2, Day: "2026-10-01", Accounts: []ledger.AccountDay{{Account: work, Resets: []ledger.Reset{{Window: fiveHour, At: reset, Before: 1}}}}}},
			Want: map[string]Figures{
				"": {
					FigureNow: "6", FigurePool: "2 0-3, 1 4-5, 2 6-135",
					FigureComing: "19:00 side reset 5h",
				},
				work: {
					FigureCells: "room 0-3, limited 4-5, room 6-135", FigureStretches: "limited 16:50 – 17:10, as it resets @4 4-5, over",
					FigureResets: "", FigureRoom: "yes", FigureUse: "5h 0%",
				},
			},
		},
	}
}

// failedReads has work's first read fail, nothing read of it before, so its
// lane is blank, counted without room; its read fail after one at 14:20,
// its lane drawn as read, its use stale; its 5-hour not reported, its week
// alone read, so it has no use to show; and its windows listed, nothing read
// of them yet.
func failedReads() []RunwayMoment {
	unread := status.Account{ID: work, Label: work, TokenSet: true, Error: "HTTP 529 · Overloaded"}
	stale := readAccount(work, onThu(14, 20, 0), window(fiveHour, 0.4, onThu(16, 30, 0)), window(week, 0.3, mondayTen))
	stale.Error = "HTTP 529 · Overloaded"
	weekOnly := readAccount(work, onThu(14, 41, 0), window(week, 0.3, mondayTen))
	listed := readAccount(work, time.Time{}, window(fiveHour, 0, onThu(16, 30, 0)), window(week, 0, mondayTen))
	sideAt := sideOn(onThu(14, 41, 0))
	return []RunwayMoment{
		fiveHourAt("its first read failed", routed(onThu(14, 41, 0), nil, unread, sideAt), sideRead(), map[string]Figures{
			"":   {FigurePool: "1 0-135"},
			work: {FigureCells: "blank 0-135", FigureStretches: "", FigureResets: "", FigureRoom: "no", FigureUse: "not read yet"},
		}),
		fiveHourAt("a failed read after an earlier one", routed(onThu(14, 41, 0), nil, stale, sideAt), sorted(
			readEach(work, onThu(13, 0, 0), window(fiveHour, 0.3, onThu(16, 30, 0)), window(week, 0.3, mondayTen)),
			readEach(work, onThu(14, 20, 0), stale.Windows...),
			sideRead(),
		), map[string]Figures{
			"":   {FigurePool: "2 0-135"},
			work: {FigureCells: "room 0-135", FigureResets: "17 16:30", FigureRoom: "yes", FigureUse: "5h 40% stale, read 14:20"},
		}),
		fiveHourAt("a window its account hasn't reported", routed(onThu(14, 41, 0), nil, weekOnly, sideAt), sorted(
			readEach(work, onThu(13, 0, 0), weekOnly.Windows...),
			sideRead(),
		), map[string]Figures{
			work: {FigureCells: "room 0-135", FigureResets: "", FigureRoom: "yes", FigureUse: "—"},
		}),
		fiveHourAt("its windows listed, nothing read of them yet", routed(onThu(14, 41, 0), nil, listed, sideAt), sideRead(), map[string]Figures{
			work: {FigureCells: "blank 0-135", FigureRoom: "no", FigureUse: "not read yet"},
		}),
	}
}

// refusals have work's token refused at 14:12 till 15:00; that refusal
// lifted early, at 14:30, over and so unsaid live, and said on today stepped
// back to; and its Opus requests refused alone, which its lane doesn't draw,
// as it doesn't a model's own window.
func refusals() []RunwayMoment {
	refusedAt, refusal := onThu(14, 12, 0), status.Refusal{Until: onThu(15, 0, 0), Status: 401}
	read := sorted(readEach(work, onThu(13, 0, 0), window(fiveHour, 0.3, onThu(16, 30, 0)), window(week, 0.5, mondayTen)), sideRead())
	workAt := func(r status.Refusal) status.Account {
		a := readAccount(work, refusedAt, window(fiveHour, 0.3, onThu(16, 30, 0)), window(week, 0.5, mondayTen))
		a.Refused = r
		return a
	}
	refused := status.Event{At: refusedAt, Kind: status.EventRefused, Account: work, Until: refusal.Until, Status: 401}
	lifted := refused
	lifted.Until = onThu(14, 30, 0)
	family := status.Refusal{Until: refusal.Until, Status: 404, Family: "opus"}
	familyEvent := status.Event{At: refusedAt, Kind: status.EventRefused, Account: work, Until: refusal.Until, Status: 404, Family: "opus"}
	sideAt := sideOn(onThu(14, 41, 0))
	liftedToday := fiveHourAt("its token's refusal lifted early, today stepped back to", routed(onThu(14, 41, 0), []status.Event{lifted}, workAt(status.Refusal{}), sideAt), read, map[string]Figures{
		work: {FigureCells: "blank 0-73, room 74-79, refused 80-81, room 82-135", FigureStretches: "refused 14:12 – 14:30 @80 80-81"},
	})
	liftedToday.Day = civilDay(2026, time.October, 1)
	return []RunwayMoment{
		fiveHourAt("its token refused", routed(onThu(14, 41, 0), []status.Event{refused}, workAt(refusal), sideAt), read, map[string]Figures{
			"": {FigurePool: "2 0-2, 1 3-7, 2 8-135", FigureDrawn: "room, refused, │"},
			work: {
				FigureCells: "room 0-2, refused 3-7, room 8-135", FigureStretches: "refused 14:12 – 15:00 @3 3-7",
				FigureRoom: "no", FigureUse: "5h 30% stale, read 14:12",
			},
		}),
		fiveHourAt("its token's refusal lifted early", routed(onThu(14, 41, 0), []status.Event{lifted}, workAt(status.Refusal{}), sideAt), read, map[string]Figures{
			work: {
				FigureCells: "room 0-2, refused 3-4, room 5-135", FigureStretches: "refused 14:12 – 14:30 @3 3-4, over",
				FigureRoom: "yes", FigureUse: "5h 30%",
			},
		}),
		liftedToday,
		fiveHourAt("its Opus requests refused alone", routed(onThu(14, 41, 0), []status.Event{familyEvent}, workAt(family), sideAt), read, map[string]Figures{
			"":   {FigureDrawn: "room, │"},
			work: {FigureCells: "room 0-135", FigureStretches: "", FigureRoom: "yes"},
		}),
	}
}

// holdsNoEventTells have work's 5-hour read spent from 14:05 till 15:30, as
// probing reads it, no event telling, the reading drawn from the column
// whose moment it's read at; team's spent from 13:58, the router telling no
// event of it, as the ten world has it, its words from 13:58's column, a
// column before its first drawn; work held by a limit naming no window that
// neither an event nor a reading tells the start of, from before the
// timeline, but not on a day gone, a week before, read with room all day, as
// such a hold is known only to hold now; work read spent by probing at
// 14:41, in now's column; and work read spent as the dashboard, probing,
// first read it itself at 14:41, its 5-hour last read the day before, so
// nothing tells when its limit began.
func holdsNoEventTells() []RunwayMoment {
	back := onThu(15, 30, 0)
	probe := status.Document{GeneratedAt: onThu(14, 41, 0), Source: status.SourceProbe, Fallback: status.Fallback{Router: status.RouterNotRunning}}
	probed := readAccount(work, onThu(14, 41, 0), spent(fiveHour, 1, back), window(week, 0.5, mondayTen))
	probe.Accounts = []status.Account{probed.AsOf(claude.Policy(), probe.GeneratedAt), sideOn(onThu(14, 41, 0)).AsOf(claude.Policy(), probe.GeneratedAt)}
	probe.Best = side
	team := readAccount("team", onThu(14, 41, 0), spent(fiveHour, 1, back), window(week, 0.4, mondayTen))
	unnamed := readAccount(work, onThu(14, 41, 0), window(fiveHour, 0.5, onThu(16, 30, 0)), window(week, 0.5, mondayTen))
	unnamed.Limit = status.Limit{ID: 3, Until: back}
	weekOn := thursday.AddDate(0, 0, 7)
	unnamedWeekOn := readAccount(work, weekOn.Add(-time.Minute), window(fiveHour, 0.5, weekOn.Add(time.Hour)), window(week, 0.5, weekOn.Add(72*time.Hour)))
	unnamedWeekOn.Limit = status.Limit{ID: 3, Until: weekOn.Add(20 * time.Minute)}
	firstSeen := fiveHourAt("a hold the dashboard first read itself, probed, nothing read before it since its window began", probedAt(onThu(14, 41, 0), probed), sorted(
		readEach(work, onWed(20, 0, 0), window(fiveHour, 0.3, onWed(23, 0, 0)), window(week, 0.5, mondayTen)),
		readEach(work, onThu(14, 41, 0), probed.Windows...),
		sideRead(),
	), map[string]Figures{
		work: {FigureCells: "room 0-5, limited 6-10, room 11-135", FigureStretches: "limited — – 15:30, as it resets @6 6-10"},
	})
	firstSeen.Seen = onThu(14, 41, 0)
	var weekBefore []readings.Reading
	for at := onWed(23, 0, 0); at.Before(onThu(24, 1, 0)); at = at.Add(30 * time.Minute) {
		weekBefore = append(weekBefore, readEach(work, at, window(fiveHour, 0.2, at.Add(3*time.Hour)), window(week, 0.2, mondayTen))...)
	}
	return []RunwayMoment{
		fiveHourAt("a hold no event tells of, probed", probe.WorkedOut(claude.Policy()), sorted(
			readEach(work, onThu(13, 30, 0), window(fiveHour, 0.8, back), window(week, 0.5, mondayTen)),
			readEach(work, onThu(14, 5, 0), spent(fiveHour, 1, back)),
			readEach(work, onThu(14, 41, 0), probed.Windows...),
			sideRead(),
		), map[string]Figures{
			work: {FigureCells: "room 0-1, limited 2-10, room 11-135", FigureStretches: "limited 14:05 – 15:30, as it resets @2 2-10"},
		}),
		fiveHourAt("a hold no event tells of, the router's", routed(onThu(14, 41, 0), nil, team, sideOn(onThu(14, 41, 0))), sorted(
			readEach(team.ID, onThu(13, 30, 0), window(fiveHour, 0.9, back), window(week, 0.4, mondayTen)),
			readEach(team.ID, onThu(13, 58, 0), spent(fiveHour, 1, back)),
			sideRead(),
		), map[string]Figures{
			team.ID: {FigureCells: "room 0-1, limited 2-10, room 11-135", FigureStretches: "limited 13:58 – 15:30, as it resets @1 2-10"},
		}),
		fiveHourAt("a hold neither an event nor a reading tells the start of", routed(onThu(14, 41, 0), nil, unnamed, sideOn(onThu(14, 41, 0))), sorted(
			readEach(work, onThu(13, 0, 0), unnamed.Windows...),
			sideRead(),
		), map[string]Figures{
			work: {FigureCells: "limited 0-10, room 11-135", FigureStretches: "limited — – 15:30 @0 0-10"},
		}),
		{Window: views.RunwayFiveHour,
			Name: "a hold neither an event nor a reading tells the start of, a day a week before stepped back to",
			Doc:  routed(weekOn.Add(-time.Minute), nil, unnamedWeekOn, sideOn(weekOn.Add(-time.Minute))), Now: weekOn, Readings: weekBefore,
			Day: civilDay(2026, time.October, 1), Columns: lanes,
			Want: map[string]Figures{work: {FigureCells: "room 0-135", FigureStretches: "", FigureRoom: "yes"}},
		},
		fiveHourAt("a hold begun in now's column, no event telling", probedAt(onThu(14, 41, 0), probed), sorted(
			readEach(work, onThu(13, 30, 0), window(fiveHour, 0.8, back), window(week, 0.5, mondayTen)),
			readEach(work, onThu(14, 41, 0), probed.Windows...),
			sideRead(),
		), map[string]Figures{
			work: {FigureCells: "room 0-5, limited 6-10, room 11-135", FigureStretches: "limited 14:41 – 15:30, as it resets @6 6-10"},
		}),
		firstSeen,
	}
}

// holdsEventsTell have the router's event of a hold tell when it began,
// before the first reading that shows it: work's cap at 14:31:03, read at
// 14:31:30, or with the readings history holding none of it, its words
// starting in the column the event tells though the lane's blank there; its
// limit at 14:03:30, read spent at 14:04, as the event of the router's limit
// tells; as an event naming its window, of no limit the router still holds,
// tells; and each, the cap and the limit, as its event tells on today stepped
// back to once it's lifted, the router keeping the event.
func holdsEventsTell() []RunwayMoment {
	back, capBack := onThu(15, 30, 0), onThu(17, 10, 0)
	capped := reserving(readAccount(work, onThu(14, 41, 0), window(fiveHour, 0.96, capBack), window(week, 0.4, mondayTen)), 0.05)
	capEvent := status.Event{At: onThu(14, 31, 3), Kind: status.EventCap, Account: work, Windows: []string{fiveHour}, Until: capBack, Reserve: 0.05}
	limited := withLimit(readAccount(work, onThu(14, 41, 0), spent(fiveHour, 1, back), window(week, 0.4, mondayTen)), []string{fiveHour}, back)
	limitEvent := status.Event{At: onThu(14, 3, 30), Kind: status.EventLimit, Account: work, Windows: []string{fiveHour}, Until: back, Limit: 1}
	untold := limitEvent
	untold.Limit = 0
	spentRead := sorted(
		readEach(work, onThu(13, 30, 0), window(fiveHour, 0.8, back), window(week, 0.4, mondayTen)),
		readEach(work, onThu(14, 4, 0), spent(fiveHour, 1, back)),
		sideRead(),
	)
	limitWant := map[string]Figures{work: {FigureCells: "room 0-1, limited 2-10, room 11-135", FigureStretches: "limited 14:03:30 – 15:30, as it resets @2 2-10"}}
	lifted := func(name string, at time.Time, events []status.Event, read []readings.Reading, want map[string]Figures) RunwayMoment {
		fresh := reserving(readAccount(work, at, window(fiveHour, 0.02, at.Add(5*time.Hour)), window(week, 0.4, mondayTen)), 0.05)
		return RunwayMoment{Window: views.RunwayFiveHour,
			Name: name, Doc: routed(at, events, fresh, sideOn(at)), Now: at.Add(time.Minute + 7*time.Second),
			Readings: sorted(read, readEach(work, at, fresh.Windows...)), Day: civilDay(2026, time.October, 1), Columns: lanes, Want: want,
		}
	}
	return []RunwayMoment{
		lifted("a cap its event tells the start of, read later, today stepped back to once it's lifted", onThu(17, 29, 0), []status.Event{capEvent}, sorted(
			readEach(work, onThu(13, 30, 0), window(fiveHour, 0.8, capBack), window(week, 0.4, mondayTen)),
			readEach(work, onThu(14, 31, 30), window(fiveHour, 0.95, capBack)),
			readEach(work, onThu(14, 41, 0), capped.Windows...),
			sideRead(),
		), map[string]Figures{
			work: {FigureStretches: "capped 14:31:03 – 17:10, as it resets @82 82-96"},
		}),
		lifted("a limit its event tells the start of, read later, today stepped back to once it's lifted", onThu(15, 59, 0), []status.Event{limitEvent}, spentRead, map[string]Figures{
			work: {FigureStretches: "limited 14:03:30 – 15:30, as it resets @79 80-87"},
		}),
		fiveHourAt("a cap its event tells the start of, read later", routed(onThu(14, 41, 0), []status.Event{capEvent}, capped, sideOn(onThu(14, 41, 0))), sorted(
			readEach(work, onThu(13, 30, 0), window(fiveHour, 0.8, capBack), window(week, 0.4, mondayTen)),
			readEach(work, onThu(14, 31, 30), window(fiveHour, 0.95, capBack)),
			readEach(work, onThu(14, 41, 0), capped.Windows...),
			sideRead(),
		), map[string]Figures{
			work: {FigureCells: "room 0-4, capped 5-20, room 21-135", FigureStretches: "capped 14:31:03 – 17:10, as it resets @5 5-20"},
		}),
		fiveHourAt("a cap its event tells the start of, the readings history holding none of it", routed(onThu(14, 41, 0), []status.Event{capEvent}, capped, sideOn(onThu(14, 41, 0))), sideRead(), map[string]Figures{
			work: {FigureCells: "blank 0-5, capped 6-20, room 21-135", FigureStretches: "capped 14:31:03 – 17:10, as it resets @5 6-20"},
		}),
		fiveHourAt("a limit its event tells the start of, read later", routed(onThu(14, 41, 0), []status.Event{limitEvent}, limited, sideOn(onThu(14, 41, 0))), spentRead, limitWant),
		fiveHourAt("a limit an event naming its window tells the start of", routed(onThu(14, 41, 0), []status.Event{untold}, readAccount(work, onThu(14, 41, 0), limited.Windows...), sideOn(onThu(14, 41, 0))), spentRead, limitWant),
	}
}

// holdsTakenOver have work, its reserve 5%, reach its cap on its 5-hour at
// 10:00, its week joining the cap at 11:00, and the 5-hour reset at 12:00,
// below its reserve since: one cap, unbroken since 10:00, as the router's
// one event of it tells, though the window that began it holds it no
// longer, its words saying 10:00 in either view; and, no event telling of
// it, as the readings alone tell it, the 5-hour's run among them.
func holdsTakenOver() []RunwayMoment {
	at := onThu(14, 41, 0)
	capped := reserving(readAccount(work, at, window(fiveHour, 0.5, onThu(17, 0, 0)), window(week, 0.96, mondayTen)), 0.05)
	capEvent := status.Event{
		At: onThu(10, 0, 0), Kind: status.EventCap, Account: work, Windows: []string{fiveHour, week},
		Joined: map[string]time.Time{week: onThu(11, 0, 0)}, Until: mondayTen, Reserve: 0.05,
	}
	read := sorted(
		readEach(work, onThu(9, 0, 0), window(fiveHour, 0.9, onThu(12, 0, 0)), window(week, 0.9, mondayTen)),
		readEach(work, onThu(10, 0, 0), window(fiveHour, 0.96, onThu(12, 0, 0))),
		readEach(work, onThu(11, 0, 0), window(fiveHour, 0.97, onThu(12, 0, 0)), window(week, 0.96, mondayTen)),
		readEach(work, onThu(13, 0, 0), window(fiveHour, 0.3, onThu(17, 0, 0))),
		readEach(work, at, capped.Windows...),
		sideRead(),
	)
	weekView := fiveHourAt("a cap the 5-hour began and the week took over, the week view", routed(at, []status.Event{capEvent}, capped, sideOn(at)), read, map[string]Figures{
		work: {FigureCells: "blank 0-14, room 15, capped 16-92, room 93-135", FigureStretches: "capped 10:00 – Mon 10:00, as it resets @15 16-92"},
	})
	weekView.Window, weekView.WeekStarts = views.RunwayWeek, time.Monday
	return []RunwayMoment{
		fiveHourAt("a cap the 5-hour began and the week took over", routed(at, []status.Event{capEvent}, capped, sideOn(at)), read, map[string]Figures{
			work: {FigureCells: "capped 0-135", FigureStretches: "capped 10:00 – Mon 10:00 @0 0-135"},
		}),
		weekView,
		fiveHourAt("a cap the 5-hour began and the week took over, no event telling of it", routed(at, nil, capped, sideOn(at)), read, map[string]Figures{
			work: {FigureCells: "capped 0-135", FigureStretches: "capped 10:00 – Mon 10:00 @0 0-135"},
		}),
	}
}

// holdsChanging have work, its reserve 5%, reach its cap at 14:20 and its
// limit at 14:33, till 17:10, a column at each, two stretches; and the router's
// document, read at 14:30 with work at its cap, which no event tells the
// start of, as the readings history has it spent since 14:41: now's column
// is the document's cap, from no start the readings tell.
func holdsChanging() []RunwayMoment {
	back := onThu(17, 10, 0)
	limited := reserving(readAccount(work, onThu(14, 41, 0), spent(fiveHour, 1, back), window(week, 0.4, mondayTen)), 0.05)
	capped := reserving(readAccount(work, onThu(14, 30, 0), window(fiveHour, 0.96, back), window(week, 0.4, mondayTen)), 0.05)
	return []RunwayMoment{
		fiveHourAt("a cap turning to a limit, a column at each", routed(onThu(14, 41, 0), nil, limited, sideOn(onThu(14, 41, 0))), sorted(
			readEach(work, onThu(13, 30, 0), window(fiveHour, 0.8, back), window(week, 0.4, mondayTen)),
			readEach(work, onThu(14, 20, 0), window(fiveHour, 0.95, back)),
			readEach(work, onThu(14, 33, 0), spent(fiveHour, 1, back)),
			sideRead(),
		), map[string]Figures{work: {
			FigureCells:     "room 0-3, capped 4, limited 5-20, room 21-135",
			FigureStretches: "capped 14:20 – 17:10, lifts 17:10, as it resets @4 4, over; limited 14:33 – 17:10, as it resets @5 5-20",
		}}),
		fiveHourAt("a hold at now the readings tell otherwise", routed(onThu(14, 30, 0), nil, capped, sideOn(onThu(14, 30, 0))), sorted(
			readEach(work, onThu(13, 30, 0), window(fiveHour, 0.8, back), window(week, 0.4, mondayTen)),
			readEach(work, onThu(14, 41, 0), spent(fiveHour, 1, back)),
			sideRead(),
		), map[string]Figures{work: {
			FigureCells:     "room 0-5, capped 6-20, room 21-135",
			FigureStretches: "capped — – 17:10, as it resets @6 6-20",
		}}),
	}
}

// probedAt is the document probing builds at at, of the accounts given and
// side, as sideOn reads it then, the router not running.
func probedAt(at time.Time, accounts ...status.Account) status.Document {
	doc := status.Document{GeneratedAt: at, Source: status.SourceProbe, Fallback: status.Fallback{Router: status.RouterNotRunning}, Best: side}
	for _, a := range append(accounts, sideOn(at)) {
		doc.Accounts = append(doc.Accounts, a.AsOf(claude.Policy(), at))
	}
	return doc.WorkedOut(claude.Policy())
}

// tokenless has work and spare without a usable token, work read before it
// lost it, spare never: each has no room all along.
func tokenless() RunwayMoment {
	noToken := status.Account{ID: work, Label: work, FetchedAt: onThu(14, 41, 0), Windows: []quota.Window{window(fiveHour, 0.3, onThu(16, 30, 0))}}
	never := status.Account{ID: spare, Label: spare}
	return fiveHourAt("no usable token", routed(onThu(14, 41, 0), nil, noToken, sideOn(onThu(14, 41, 0)), never), sorted(
		readEach(work, onThu(13, 0, 0), noToken.Windows...),
		sideRead(),
	), map[string]Figures{
		"":    {FigurePool: "1 0-135", FigureDrawn: "room, no token, │"},
		work:  {FigureCells: "no token 0-135", FigureStretches: "no token — – — @0 0-135", FigureResets: "", FigureRoom: "no", FigureUse: "no token"},
		spare: {FigureCells: "no token 0-135", FigureUse: "no token"},
	})
}

// spare is an account most moments don't have.
const spare = "spare"

// holdsAhead are what holds side back from now on: its 5-hour's limit, from
// 14:00 till 16:30, as its week runs out, at 16:00, to its limit, one
// stretch till its week resets, as the 5-hour view leaves out a week running
// out, but not a week at its limit; and to its cap, its reserve 5%, the
// limit drawn till it lifts, then the cap, the limit lifting at 16:30 and
// back as the cap is; its 5-hour's limit with no known back, held all
// along; a limit naming no window from 14:12 till 15:00, its
// 5-hour running out once it lifts, at 15:50; and its Fable week running out,
// which holds back Fable's requests alone, not drawn.
func holdsAhead() []RunwayMoment {
	at, back, weekBack := onThu(14, 41, 0), onThu(16, 30, 0), mondayTen.Add(11*time.Hour)
	workAt := readAccount(work, at, window(fiveHour, 0.2, onThu(17, 10, 0)), window(week, 0.4, mondayTen))
	read := func(more ...[]readings.Reading) []readings.Reading {
		return sorted(slices.Concat(more...), readEach(work, onThu(13, 0, 0), workAt.Windows...))
	}
	weekOut := rated(readAccount(side, at, spent(fiveHour, 1, back), window(week, 0.9, weekBack)), week, 0.1/(79*time.Minute).Hours(), at)
	weekCapped := rated(reserving(readAccount(side, at, spent(fiveHour, 1, back), window(week, 0.9, weekBack)), 0.05), week, 0.05/(79*time.Minute).Hours(), at)
	spentRead := slices.Concat(
		readEach(side, onThu(13, 0, 0), window(fiveHour, 0.8, back), window(week, 0.85, weekBack)),
		readEach(side, onThu(14, 0, 0), spent(fiveHour, 1, back)),
	)
	noBack := readAccount(side, at, spent(fiveHour, 1, time.Time{}), window(week, 0.4, mondayTen))
	unnamed := readAccount(side, at, window(fiveHour, 0.7, onThu(17, 0, 0)), window(week, 0.4, mondayTen))
	unnamed.Limit = status.Limit{ID: 3, Until: onThu(15, 0, 0)}
	unnamedEvent := status.Event{At: onThu(14, 12, 0), Kind: status.EventLimit, Account: side, Until: onThu(15, 0, 0), Limit: 3}
	fableOut := rated(readAccount(side, at, window(fiveHour, 0.2, onThu(16, 50, 0)), window(week, 0.3, mondayTen), window(fable, 0.9, mondayTen)), fable, 0.1/(79*time.Minute).Hours(), at)
	return []RunwayMoment{
		fiveHourAt("a limit holding as its week runs out to its limit", routed(at, nil, workAt, weekOut), read(spentRead, readEach(side, at, weekOut.Windows...)), map[string]Figures{
			side: {FigureCells: "room 0-1, limited 2-135", FigureStretches: "limited 14:00 – Mon 21:00 @2 2-135"},
		}),
		fiveHourAt("a limit holding as its week runs out to its cap", routed(at, nil, workAt, weekCapped), read(spentRead, readEach(side, at, weekCapped.Windows...)), map[string]Figures{
			side: {FigureCells: "room 0-1, limited 2-16, capped 17-135", FigureStretches: "limited 14:00 – Mon 21:00, lifts 16:30, as it resets @2 2-16; capped ~16:00 – Mon 21:00 @14 17-135"},
		}),
		fiveHourAt("a limit with no known back", routed(at, nil, workAt, noBack), read(
			readEach(side, onThu(13, 30, 0), window(fiveHour, 0.8, back), window(week, 0.4, mondayTen)),
			readEach(side, onThu(14, 3, 0), spent(fiveHour, 1, time.Time{})),
		), map[string]Figures{
			side: {FigureCells: "room 0-1, limited 2-135", FigureStretches: "limited 14:03 – — @2 2-135", FigureRoom: "no"},
		}),
		fiveHourAt("a limit naming no window, its 5-hour running out once it lifts", routed(at, []status.Event{unnamedEvent}, workAt, unnamed), read(
			readEach(side, onThu(13, 0, 0), unnamed.Windows...),
		), map[string]Figures{
			side: {
				FigureCells:     "room 0-2, limited 3-7, runs out 8-12, limited 13-19, room 20-135",
				FigureStretches: "limited 14:12 – 15:00 @3 3-7; limited ~15:50 – 17:00, as it resets @13 13-19",
			},
		}),
		fiveHourAt("a model's own window running out", routed(at, nil, workAt, fableOut), read(readEach(side, onThu(13, 0, 0), fableOut.Windows...)), map[string]Figures{
			side: {FigureCells: "room 0-135", FigureStretches: "", FigureRoom: "yes"},
		}),
	}
}

// holdsOutlasted have work's cap, its reserve 5%, outlast what holds it back
// first, till its week resets on Monday at 21:00: its 5-hour read spent at
// 14:00 till 15:30 as its week reaches its cap, the limit drawn till it
// lifts, then the cap, at 14:42:07 and at 16:00:07, the limit over; and its
// token refused till 14:52, which no event tells the start of, as its week
// reaches its cap, the refusal drawn till it lifts, then the cap. Each
// stretch starts as it began, its cap at 14:00 as the readings show it, and
// is back as work next has room, on Monday, the limit and the refusal each
// lifting as its own hold does.
func holdsOutlasted() []RunwayMoment {
	weekBack := mondayTen.Add(11 * time.Hour)
	limited := reserving(readAccount(work, onThu(14, 0, 0), spent(fiveHour, 1, onThu(15, 30, 0)), window(week, 0.96, weekBack)), 0.05)
	limitedRead := sorted(
		readEach(work, onThu(12, 0, 0), window(fiveHour, 0.5, onThu(15, 30, 0)), window(week, 0.9, weekBack)),
		readEach(work, onThu(14, 0, 0), limited.Windows...),
		sideRead(),
	)
	refused := reserving(readAccount(work, onThu(14, 0, 0), window(fiveHour, 0.3, onThu(17, 0, 0)), window(week, 0.96, weekBack)), 0.05)
	refused.Refused = status.Refusal{Until: onThu(14, 52, 0), Status: 401}
	limitOver := fiveHourAt("a cap outlasting a limit, the limit over", routed(onThu(14, 41, 0), nil, limited, sideOn(onThu(14, 41, 0))), limitedRead, map[string]Figures{
		work: {
			FigureCells:     "limited 0-2, capped 3-135",
			FigureStretches: "limited 14:00 – Mon 21:00, lifts 15:30, as it resets @0 0-2, over; capped 14:00 – Mon 21:00 @0 3-135",
		},
	})
	limitOver.Now = onThu(16, 0, 7)
	return []RunwayMoment{
		limitOver,
		fiveHourAt("a cap outlasting a limit", routed(onThu(14, 41, 0), nil, limited, sideOn(onThu(14, 41, 0))), limitedRead, map[string]Figures{
			"": {FigurePool: "2 0-1, 1 2-135"},
			work: {
				FigureCells:     "room 0-1, limited 2-10, capped 11-135",
				FigureStretches: "limited 14:00 – Mon 21:00, lifts 15:30, as it resets @2 2-10; capped 14:00 – Mon 21:00 @2 11-135",
				FigureResets:    "11 15:30 back", FigureRoom: "no",
			},
		}),
		fiveHourAt("a cap outlasting a refusal", routed(onThu(14, 41, 0), nil, refused, sideOn(onThu(14, 41, 0))), sorted(
			readEach(work, onThu(14, 0, 0), refused.Windows...),
			sideRead(),
		), map[string]Figures{
			"": {FigurePool: "1 0-135"},
			work: {
				FigureCells:     "refused 0-6, capped 7-135",
				FigureStretches: "refused — – Mon 21:00, lifts 14:52 @0 0-6; capped 14:00 – Mon 21:00 @2 7-135",
				FigureResets:    "20 17:00", FigureRoom: "no",
			},
		}),
	}
}

// holdsCarriedIn are holds begun before the span of the readings history
// Runway's live tail reads, from Monday 28 September, which carries each in,
// probed, so no event tells when it began: work's week at its limit from
// 15:00 on Saturday 26 September, resetting Monday 5 October at 10:00, read
// every half hour, live in either view and on Tuesday stepped back to; the
// same limit read once, then again on Sunday at noon, as a restarted
// router's first read is; and read at it from Tuesday, the first day the
// readings history holds, its week begun before, with no summary telling
// when the limit began, and with Saturday's telling it.
func holdsCarriedIn() []RunwayMoment {
	saturday := time.Date(2026, 9, 26, 15, 0, 0, 0, time.UTC)
	sunday := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	tuesday := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	limited := readAccount(work, onThu(14, 41, 0), window(fiveHour, 0.3, onThu(17, 0, 0)), spent(week, 1, mondayTen))
	doc := probedAt(onThu(14, 41, 0), limited)
	var halfHourly []readings.Reading
	for at := saturday.Add(-time.Hour); at.Before(onThu(14, 41, 0)); at = at.Add(30 * time.Minute) {
		weekRead := spent(week, 1, mondayTen)
		if at.Before(saturday) {
			weekRead = window(week, 0.97, mondayTen)
		}
		halfHourly = append(halfHourly, readEach(work, at, window(fiveHour, 0.3, at.Add(3*time.Hour)), weekRead)...)
	}
	halfHourly = sorted(halfHourly, readEach(work, onThu(14, 41, 0), limited.Windows...), sideRead())
	restarted := sorted(
		readEach(work, saturday.Add(-time.Hour), window(week, 0.97, mondayTen)),
		readEach(work, saturday, spent(week, 1, mondayTen)),
		readEach(work, sunday, spent(week, 1, mondayTen)),
		readEach(work, onThu(14, 41, 0), limited.Windows...),
		sideRead(),
	)
	fromTuesday := sorted(
		readEach(work, tuesday.Add(5*time.Minute), spent(week, 1, mondayTen)),
		readEach(work, onThu(14, 41, 0), limited.Windows...),
		sideRead(),
	)
	limitedDay := func(day string, limits ...ledger.Limit) ledger.Summary {
		return ledger.Summary{Version: 2, Day: day, Accounts: []ledger.AccountDay{{Account: work, Highest: map[string]float64{week: 1}, Limits: limits}}}
	}
	saturdays := []ledger.Summary{
		limitedDay("2026-09-26", ledger.Limit{Window: week, At: saturday, ResetsAt: mondayTen}), limitedDay("2026-09-27"), limitedDay("2026-09-28"),
	}
	sinceSaturday := map[string]Figures{
		work: {FigureCells: "limited 0-135", FigureStretches: "limited Sat 15:00 – Mon 10:00 @0 0-135"},
	}
	weekView := fiveHourAt("a week at its limit since before the live tail, read every half hour, the week view", doc, halfHourly, map[string]Figures{
		work: {FigureStretches: "limited Sat 15:00 – Mon 10:00, as it resets @0 0-92"},
	})
	weekView.Window, weekView.WeekStarts = views.RunwayWeek, time.Monday
	keptWith := fiveHourAt("a week at its limit since before the readings history's keep, a summary telling when", doc, fromTuesday, sinceSaturday)
	keptWith.Days, keptWith.Kept = saturdays, tuesday
	keptWithout := fiveHourAt("a week at its limit since before the readings history's keep", doc, fromTuesday, map[string]Figures{
		work: {FigureStretches: "limited — – Mon 10:00 @0 0-135"},
	})
	keptWithout.Kept = tuesday
	return []RunwayMoment{
		fiveHourAt("a week at its limit since before the live tail, read every half hour", doc, halfHourly, sinceSaturday),
		weekView,
		{Window: views.RunwayFiveHour,
			Name: "a week at its limit since before the live tail, read every half hour, Tuesday stepped back to", Doc: doc, Now: thursday,
			Readings: halfHourly, Day: civilDay(2026, time.September, 29), WeekStarts: time.Monday, Columns: lanes,
			Want: map[string]Figures{work: {FigureStretches: "limited Sat 15:00 – Mon 10:00 @0 0-135"}},
		},
		fiveHourAt("a week at its limit since before the live tail, read again by a restarted router", doc, restarted, sinceSaturday),
		keptWith,
		keptWithout,
	}
}

// routerSilent has the router's last document read at 14:32:07, side's
// 5-hour at 85% heading to run out about 15:04 before it resets at 16:30: at
// 14:32:07, and at 14:42:07, the router not answering for 10 minutes, its
// run-out where the document put it, now's column moved on.
func routerSilent() []RunwayMoment {
	built := onThu(14, 32, 7)
	sideAt := readAccount(side, built, window(fiveHour, 0.85, onThu(16, 30, 0)), window(week, 0.3, mondayTen))
	workAt := readAccount(work, built, window(fiveHour, 0.3, onThu(17, 10, 0)), window(week, 0.4, mondayTen))
	doc := routed(built, nil, workAt, sideAt)
	read := sorted(
		readEach(side, onThu(13, 0, 0), window(fiveHour, 0.5, onThu(16, 30, 0)), window(week, 0.3, mondayTen)),
		readEach(side, built, sideAt.Windows...),
		readEach(work, onThu(13, 0, 0), workAt.Windows...),
		readEach(work, built, workAt.Windows...),
	)
	return []RunwayMoment{
		{Window: views.RunwayFiveHour,
			Name: "the router's document as it's read", Doc: doc, Now: built, Readings: read, Columns: lanes,
			Want: map[string]Figures{
				side: {FigureCells: "room 0-5, runs out 6-8, limited 9-17, room 18-135", FigureStretches: "limited ~15:04:15 – 16:30, as it resets @9 9-17"},
			},
		},
		{Window: views.RunwayFiveHour,
			Name: "the router not answering for 10 minutes", Doc: doc, Now: built.Add(10 * time.Minute), Readings: read, Columns: lanes,
			Want: map[string]Figures{
				"":   {FigureNow: "6"},
				side: {FigureCells: "room 0-5, runs out 6-7, limited 8-16, room 17-135", FigureStretches: "limited ~15:04:15 – 16:30, as it resets @8 8-16"},
			},
		},
	}
}

// pools have the global pin naming side, a pool of one; naming work, at its
// 5-hour limit from 14:20 till 15:05, a pool of one with no room till then,
// room in the column whose moment 15:05 is; and work alone, which has no
// pool's lane.
func pools() []RunwayMoment {
	open := sorted(readEach(work, onThu(13, 0, 0), window(fiveHour, 0.4, onThu(17, 10, 0)), window(week, 0.4, mondayTen)), sideRead())
	workOpen := readAccount(work, onThu(14, 41, 0), window(fiveHour, 0.4, onThu(17, 10, 0)), window(week, 0.4, mondayTen))
	back := onThu(15, 5, 0)
	limited := withLimit(readAccount(work, onThu(14, 41, 0), spent(fiveHour, 1, back), window(week, 0.4, mondayTen)), []string{fiveHour}, back)
	limitedEvent := status.Event{At: onThu(14, 20, 0), Kind: status.EventLimit, Account: work, Windows: []string{fiveHour}, Until: back, Limit: 1}
	return []RunwayMoment{
		fiveHourAt("the global pin naming one account, a pool of one",
			pinned(routed(onThu(14, 41, 0), nil, workOpen, sideOn(onThu(14, 41, 0))), onThu(14, 0, 0), side), open,
			map[string]Figures{"": {FigureOf: "1", FigurePool: "1 0-135"}}),
		fiveHourAt("the global pin naming an account at its limit",
			pinned(routed(onThu(14, 41, 0), []status.Event{limitedEvent}, limited, sideOn(onThu(14, 41, 0))), onThu(14, 30, 0), work), sorted(
				readEach(work, onThu(13, 0, 0), window(fiveHour, 0.7, back), window(week, 0.4, mondayTen)),
				readEach(work, onThu(14, 20, 0), spent(fiveHour, 1, back)),
				sideRead(),
			),
			map[string]Figures{
				"":   {FigureOf: "1", FigurePool: "1 0-3, 0 4-7, 1 8-135"},
				work: {FigureCells: "room 0-3, limited 4-7, room 8-135", FigureStretches: "limited 14:20 – 15:05, as it resets @4 4-7"},
			}),
		fiveHourAt("one account", routed(onThu(14, 41, 0), nil, workOpen), readEach(work, onThu(13, 0, 0), workOpen.Windows...),
			map[string]Figures{"": {FigureOf: "0", FigurePool: ""}}),
	}
}

// london is London's time, whose clocks go forward an hour at 01:00 on
// Sunday 29 March 2026, and back an hour at 02:00 on Sunday 25 October.
var london = zone("Europe/London")

// steppedBack are the views stepped back to: Tuesday 29 September, when
// nothing was read; Monday 28 September, older than the readings history's
// keep, drawn from its summary's limits and resets, and Sunday's, a limit
// reached at 23:00 holding till 01:00; today, at its limit from 11:00 till
// 13:00, at 23:59:59, holding now, and at 00:00:01, its date kept, holding
// none, with no room all day either way; and the days and the week the
// clocks change in.
func steppedBack() []RunwayMoment {
	doc := routed(onThu(14, 41, 0), nil, readAccount(work, onThu(14, 41, 0), window(fiveHour, 0.3, onThu(17, 10, 0))), sideOn(onThu(14, 41, 0)))
	fromWednesday := sorted(
		readEach(work, onWed(9, 0, 0), window(fiveHour, 0.1, onWed(13, 0, 0)), window(week, 0.2, mondayTen)),
		readEach(side, onWed(9, 0, 0), window(fiveHour, 0.1, onWed(13, 0, 0)), window(week, 0.2, mondayTen)),
	)
	monday := func(hour, minute int) time.Time { return time.Date(2026, 9, 28, hour, minute, 0, 0, time.UTC) }
	keptFrom := []ledger.Summary{
		{Version: 2, Day: "2026-09-27", Accounts: []ledger.AccountDay{{
			Account: work, Highest: map[string]float64{fiveHour: 1, week: 0.4},
			Resets: []ledger.Reset{{Window: fiveHour, At: monday(-8, 0), Before: 0.3}},
			Limits: []ledger.Limit{{Window: fiveHour, At: monday(-1, 0), ResetsAt: monday(1, 0)}},
		}}},
		{Version: 2, Day: "2026-09-28", Accounts: []ledger.AccountDay{{
			Account: work, Highest: map[string]float64{fiveHour: 1, week: 0.5},
			Resets: []ledger.Reset{
				{Window: week, At: monday(9, 0), Before: 0.6}, {Window: fiveHour, At: monday(1, 0), Before: 1}, {Window: fiveHour, At: monday(12, 0), Before: 1}, {Window: fiveHour, At: monday(17, 0), Before: 0.4},
			},
			Limits: []ledger.Limit{{Window: fiveHour, At: monday(10, 15), ResetsAt: monday(12, 0)}},
		}}},
	}
	today := sorted(
		readEach(work, onThu(10, 0, 0), window(fiveHour, 0.5, onThu(13, 0, 0)), window(week, 0.4, mondayTen)),
		readEach(work, onThu(11, 0, 0), spent(fiveHour, 1, onThu(13, 0, 0))),
		readEach(work, onThu(22, 0, 0), window(fiveHour, 0.2, time.Date(2026, 10, 2, 3, 0, 0, 0, time.UTC))),
	)
	lastRead := readAccount(work, onThu(23, 59, 30), window(fiveHour, 0.2, time.Date(2026, 10, 2, 3, 0, 0, 0, time.UTC)), window(week, 0.4, mondayTen))
	thursdays := []ledger.Summary{{Version: 2, Day: "2026-10-01", Accounts: []ledger.AccountDay{{Account: work, Highest: map[string]float64{fiveHour: 0.5, week: 0.4}}}}}
	return slices.Concat([]RunwayMoment{
		{Window: views.RunwayFiveHour,
			Name: "stepped back to a day with nothing read", Doc: doc, Now: thursday, Readings: fromWednesday,
			Days: []ledger.Summary{{Version: 2, Day: "2026-09-29"}}, Day: civilDay(2026, time.September, 29), Columns: lanes,
			Want: map[string]Figures{
				"":   {FigureNow: "none", FigurePool: "blank 0-135", FigureDrawn: ""},
				work: {FigureCells: "blank 0-135", FigureStretches: "", FigureResets: "", FigureRoom: "no", FigureUse: "5h highest —"},
			},
		},
		{Window: views.RunwayFiveHour,
			Name: "stepped back to a day older than the readings history's keep", Doc: doc, Now: thursday, Readings: fromWednesday,
			Days: keptFrom, Kept: onWed(0, 0, 0), Day: civilDay(2026, time.September, 28), Columns: lanes,
			Want: map[string]Figures{
				"": {FigureNow: "none"},
				work: {
					FigureCells:     "limited 0-5, room 6-57, limited 58-67, room 68-135",
					FigureStretches: "limited Sun 23:00 – Mon 01:00, as it resets @0 0-5; limited Mon 10:15 – Mon 12:00, as it resets @58 58-67",
					FigureResets:    "5 Mon 01:00 back, 68 Mon 12:00 back, 96 Mon 17:00", FigureRoom: "no", FigureUse: "5h highest 100%",
				},
			},
		},
		{Window: views.RunwayFiveHour,
			Name: "stepped back to a day older than the readings history's keep, a limit lifted early by hand", Doc: doc, Now: thursday,
			Readings: fromWednesday, Kept: onWed(0, 0, 0), Day: civilDay(2026, time.September, 28), Columns: lanes,
			Days: []ledger.Summary{{Version: 2, Day: "2026-09-28", Accounts: []ledger.AccountDay{{
				Account: work, Highest: map[string]float64{fiveHour: 1},
				Resets: []ledger.Reset{{Window: fiveHour, At: monday(11, 30), Before: 1}},
				Limits: []ledger.Limit{{Window: fiveHour, At: monday(10, 15), ResetsAt: monday(12, 0)}},
			}}}},
			Want: map[string]Figures{work: {FigureCells: "room 0-57, limited 58-64, room 65-135"}},
		},
		{Window: views.RunwayFiveHour,
			Name: "stepped back to today", Doc: doc, Now: thursday, Readings: fromWednesday, Day: civilDay(2026, time.October, 1), Columns: lanes,
			Days: []ledger.Summary{{Version: 2, Day: "2026-10-01", Accounts: []ledger.AccountDay{{
				Account: work, Highest: map[string]float64{fiveHour: 0.3}, Resets: []ledger.Reset{{Window: fiveHour, At: onThu(12, 10, 0)}},
			}}}},
			Want: map[string]Figures{"": {FigureNow: "83"}, work: {FigureResets: "68 12:10, 97 17:10", FigureUse: "5h highest 30%"}},
		},
		{
			Name: "stepped back to a week its account's week reset in", Doc: doc, Now: thursday, Days: weekHighs(work, week, 0.9, 0.93, 0.96, 0.98, 0.2, 0.4, 0.6),
			Window: views.RunwayWeek, Day: civilDay(2026, time.September, 24), WeekStarts: time.Monday, Columns: lanes,
			Want: map[string]Figures{work: {FigureUse: "7d highest 98%"}},
		},
		{Window: views.RunwayFiveHour,
			Name: "stepped back to today, a second before midnight", Doc: routed(onThu(23, 59, 30), nil, lastRead),
			Now: onThu(23, 59, 59), Readings: today, Days: thursdays, Day: civilDay(2026, time.October, 1), Columns: lanes,
			Want: map[string]Figures{"": {FigureNow: "135"}, work: {FigureUse: "5h highest 50%", FigureRoom: "no"}},
		},
		{Window: views.RunwayFiveHour,
			Name: "stepped back to today, a second after midnight, its date kept", Doc: routed(onThu(23, 59, 30), nil, lastRead),
			Now: time.Date(2026, 10, 2, 0, 0, 1, 0, time.UTC), Readings: today, Days: thursdays, Day: civilDay(2026, time.October, 1), Columns: lanes,
			Want: map[string]Figures{"": {FigureNow: "none"}, work: {FigureUse: "5h highest 50%", FigureRoom: "no"}},
		},
	}, clocksChanging())
}

// clocksChanging are the views stepped back to days and a week the clocks
// change in, in London: Sunday 29 March, of 23 hours, work at its 5-hour
// limit from 12:00 till 13:00, 11 hours in; Sunday 25 October, of 25, work at
// it from 12:00 till 13:00, 13 hours in; and the week from Monday 19 October,
// of 169 hours, work's week at its limit from 12:00 on its Sunday.
func clocksChanging() []RunwayMoment {
	in := func(month time.Month, day, hour int) time.Time {
		return time.Date(2026, month, day, hour, 0, 0, 0, london)
	}
	now := in(time.November, 2, 12)
	doc := routed(now, nil, readAccount(work, now, window(fiveHour, 0.1, in(time.November, 2, 16)), window(week, 0.2, in(time.November, 9, 10))))
	limitedOn := func(day time.Time) []readings.Reading {
		before := day.AddDate(0, 0, -1)
		return sorted(
			readEach(work, before.Add(12*time.Hour), window(fiveHour, 0.2, before.Add(17*time.Hour))),
			readEach(work, dayAt(day, 12), spent(fiveHour, 1, dayAt(day, 13))),
		)
	}
	weekBack := in(time.October, 26, 10)
	return []RunwayMoment{
		{Window: views.RunwayFiveHour,
			Name: "stepped back to a day of 23 hours", Doc: doc, Now: now, Readings: limitedOn(in(time.March, 29, 0)),
			Day: civilDay(2026, time.March, 29), Columns: lanes,
			Want: map[string]Figures{work: {FigureCells: "room 0-64, limited 65-70, room 71-135", FigureStretches: "limited Sun 12:00 – Sun 13:00, as it resets @65 65-70"}},
		},
		{Window: views.RunwayFiveHour,
			Name: "stepped back to a day of 25 hours", Doc: doc, Now: now, Readings: limitedOn(in(time.October, 25, 0)),
			Day: civilDay(2026, time.October, 25), Columns: lanes,
			Want: map[string]Figures{work: {FigureCells: "room 0-70, limited 71-75, room 76-135", FigureStretches: "limited Sun 12:00 – Sun 13:00, as it resets @70 71-75"}},
		},
		{
			Name: "stepped back to a week the clocks go back in", Doc: doc, Now: now, Readings: sorted(
				readEach(work, in(time.October, 18, 12), window(week, 0.5, weekBack)),
				readEach(work, in(time.October, 25, 12), spent(week, 1, weekBack)),
			),
			Window: views.RunwayWeek, Day: civilDay(2026, time.October, 25), WeekStarts: time.Monday, Columns: lanes,
			Want: map[string]Figures{work: {FigureCells: "room 0-125, limited 126-135", FigureStretches: "limited Sun 12:00 – Mon 10:00, as it resets @126 126-135"}},
		},
	}
}

// weekHighs are the summaries of the week from Monday 21 September, a day
// each, the account with the given id's window with the given key at its
// highest each day as given, in order.
func weekHighs(id, key string, highs ...float64) []ledger.Summary {
	days := make([]ledger.Summary, len(highs))
	for i, high := range highs {
		days[i] = ledger.Summary{Version: 2, Day: civilDay(2026, time.September, 21+i).Format(time.DateOnly), Accounts: []ledger.AccountDay{
			{Account: id, Highest: map[string]float64{key: high}},
		}}
	}
	return days
}

// dayAt is the hour given of the day day starts, as its clocks read it.
func dayAt(day time.Time, hour int) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), hour, 0, 0, 0, day.Location())
}

// civilDay is the day given, as dayfile.Civil gives one.
func civilDay(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// restartedByHand has work's 5-hour, read at 60% at 13:00 to reset at 16:30,
// restarted by hand at 14:40, to reset at 19:40: its │ there.
func restartedByHand() RunwayMoment {
	restarted := quota.Window{Key: fiveHour, Label: claude.WindowLabel(fiveHour), Utilization: 0.05, ResetsAt: onThu(19, 40, 0), RestartedAt: onThu(14, 40, 0)}
	workAt := readAccount(work, onThu(14, 41, 0), restarted, window(week, 0.4, mondayTen))
	return fiveHourAt("a 5-hour window restarted by hand", routed(onThu(14, 41, 0), nil, workAt, sideOn(onThu(14, 41, 0))), sorted(
		readEach(work, onThu(13, 0, 0), window(fiveHour, 0.6, onThu(16, 30, 0)), window(week, 0.4, mondayTen)),
		readEach(work, onThu(14, 41, 0), workAt.Windows...),
		sideRead(),
	), map[string]Figures{work: {FigureCells: "room 0-135", FigureResets: "36 19:40", FigureRoom: "yes", FigureUse: "5h 5%"}})
}

// capTurningToALimit is Wednesday stepped back to: work, its reserve 5%,
// reaching its cap at 14:58 and its limit at 15:02, till 17:10. No column's
// moment falls in its cap, which is neither drawn nor said, nor counted in
// the pool's columns.
func capTurningToALimit() RunwayMoment {
	reset := onWed(17, 10, 0)
	workAt := reserving(readAccount(work, onThu(14, 41, 0), window(fiveHour, 0.6, onThu(17, 10, 0)), window(week, 0.34, mondayTen)), 0.05)
	return RunwayMoment{Window: views.RunwayFiveHour,
		Name: "stepped back to a cap turning to a limit", Doc: routed(onThu(14, 41, 0), nil, workAt, sideOn(onThu(14, 41, 0))), Now: thursday,
		Readings: sorted(
			readEach(work, onWed(-1, 0, 0), window(fiveHour, 0, onWed(3, 0, 0)), window(week, 0.2, mondayTen)),
			readEach(work, onWed(12, 10, 0), window(fiveHour, 0, reset)),
			readEach(work, onWed(14, 58, 0), window(fiveHour, 0.95, reset)),
			readEach(work, onWed(15, 2, 0), spent(fiveHour, 1, reset)),
			readEach(side, onWed(-1, 0, 0), window(fiveHour, 0.1, onWed(3, 0, 0)), window(week, 0.2, mondayTen)),
		),
		Days: []ledger.Summary{{Version: 2, Day: "2026-09-30", Accounts: []ledger.AccountDay{
			{Account: side, Highest: map[string]float64{fiveHour: 0.1, week: 0.2}, Resets: []ledger.Reset{{Window: fiveHour, At: onWed(3, 0, 0), Before: 0.1}}},
			{
				Account: work, Highest: map[string]float64{fiveHour: 1, week: 0.2},
				Resets: []ledger.Reset{{Window: fiveHour, At: onWed(3, 0, 0)}, {Window: fiveHour, At: reset, Before: 1}},
				Limits: []ledger.Limit{{Window: fiveHour, At: onWed(15, 2, 0), ResetsAt: reset}},
			},
		}}},
		Day: civilDay(2026, time.September, 30), Columns: lanes,
		Want: map[string]Figures{
			"": {FigurePool: "2 0-84, 1 85-96, 2 97-135", FigureDrawn: "room, limited, │"},
			work: {
				FigureCells: "room 0-84, limited 85-96, room 97-135", FigureStretches: "limited Wed 15:02 – Wed 17:10, as it resets @85 85-96",
				FigureResets: "17 Wed 03:00, 97 Wed 17:10 back", FigureUse: "5h highest 100%",
			},
		},
	}
}

// liftedAsTheWeekResets is the week from Monday 21 September stepped back
// to: work, its reserve 5%, at its cap from Friday at 12:00 till its week
// resets on Saturday at 21:00, its 5-hour at its limit from Saturday at 20:00
// till Sunday at 01:00 holding it on past the cap's lift. The cap's stretch
// lifts as the week resets, which its │ marks as a stretch's end, so the
// reset isn't said again; the limit's is back as its own hold lifts.
func liftedAsTheWeekResets() RunwayMoment {
	in := func(day, hour, minute int) time.Time {
		return time.Date(2026, time.September, day, hour, minute, 0, 0, time.UTC)
	}
	weekReset := in(26, 21, 0)
	workAt := reserving(readAccount(work, onThu(14, 41, 0), window(fiveHour, 0.6, onThu(17, 10, 0)), window(week, 0.34, mondayTen)), 0.05)
	return RunwayMoment{
		Name: "stepped back to a cap lifting as its week resets, a limit holding it on", Now: thursday,
		Doc: routed(onThu(14, 41, 0), nil, workAt, sideOn(onThu(14, 41, 0))),
		Readings: sorted(
			readEach(work, in(24, 12, 0), window(fiveHour, 0.1, in(24, 17, 0)), window(week, 0.5, weekReset)),
			readEach(work, in(25, 12, 0), window(fiveHour, 0.3, in(25, 17, 0)), window(week, 0.96, weekReset)),
			readEach(work, in(26, 20, 0), spent(fiveHour, 1, in(27, 1, 0)), window(week, 0.97, weekReset)),
			readEach(work, in(26, 21, 30), window(week, 0.01, weekReset.AddDate(0, 0, 7))),
			readEach(work, in(27, 1, 30), window(fiveHour, 0.05, in(27, 6, 30))),
		),
		Days: []ledger.Summary{{Version: 2, Day: "2026-09-26", Accounts: []ledger.AccountDay{{
			Account: work, Highest: map[string]float64{fiveHour: 1, week: 0.97},
			Resets: []ledger.Reset{{Window: week, At: weekReset, Before: 0.97}},
		}}}},
		Window: views.RunwayWeek, Day: civilDay(2026, time.September, 24), WeekStarts: time.Monday, Columns: lanes,
		Want: map[string]Figures{work: {
			FigureStretches: "capped Fri 12:00 – Sun 01:00, lifts Sat 21:00, as it resets @87 87-112; limited Sat 20:00 – Sun 01:00 @113 113-116",
			FigureResets:    "114 Sat 21:00 back",
		}},
	}
}

// liftedByHand are Wednesday stepped back to: work, its reserve 5%, at its
// cap, or at its limit, from 14:00, its 5-hour reset by hand at 15:30, as
// claude.ai's banked reset does, its use dropped and its reset, 17:10, kept.
// Its hold lifts at the reading that shows the reset, as the window started
// again then, which the day's summary has as one of its resets: back 15:30,
// a cap's as the window resets, not at 17:10.
func liftedByHand() []RunwayMoment {
	reset := onWed(17, 10, 0)
	workAt := reserving(readAccount(work, onThu(14, 41, 0), window(fiveHour, 0.6, onThu(17, 10, 0)), window(week, 0.34, mondayTen)), 0.05)
	moment := func(name string, held quota.Window) RunwayMoment {
		return RunwayMoment{Window: views.RunwayFiveHour,
			Name: name, Doc: routed(onThu(14, 41, 0), nil, workAt, sideOn(onThu(14, 41, 0))), Now: thursday,
			Readings: sorted(
				readEach(work, onWed(-1, 0, 0), window(fiveHour, 0, onWed(3, 0, 0)), window(week, 0.2, mondayTen)),
				readEach(work, onWed(12, 10, 0), window(fiveHour, 0, reset)),
				readEach(work, onWed(14, 0, 0), held),
				readEach(work, onWed(15, 30, 0), window(fiveHour, 0.05, reset)),
				readEach(side, onWed(-1, 0, 0), window(fiveHour, 0.1, onWed(3, 0, 0)), window(week, 0.2, mondayTen)),
			),
			Days: []ledger.Summary{{Version: 2, Day: "2026-09-30", Accounts: []ledger.AccountDay{{
				Account: work, Highest: map[string]float64{fiveHour: held.Utilization, week: 0.2},
				Resets: []ledger.Reset{
					{Window: fiveHour, At: onWed(3, 0, 0)}, {Window: fiveHour, At: onWed(15, 30, 0), Before: held.Utilization},
					{Window: fiveHour, At: reset, Before: 0.05},
				},
			}}}},
			Day: civilDay(2026, time.September, 30), Columns: lanes,
		}
	}
	capped := moment("stepped back to a cap lifted early by a reset made by hand", window(fiveHour, 0.96, reset))
	capped.Want = map[string]Figures{work: {
		FigureCells: "room 0-78, capped 79-87, room 88-135", FigureStretches: "capped Wed 14:00 – Wed 15:30, as it resets @79 79-87",
		FigureResets: "17 Wed 03:00, 87 Wed 15:30 back, 97 Wed 17:10",
	}}
	limited := moment("stepped back to a limit lifted early by a reset made by hand", spent(fiveHour, 1, reset))
	limited.Want = map[string]Figures{work: {
		FigureCells: "room 0-78, limited 79-87, room 88-135", FigureStretches: "limited Wed 14:00 – Wed 15:30, as it resets @79 79-87",
		FigureResets: "17 Wed 03:00, 87 Wed 15:30 back, 97 Wed 17:10",
	}}
	return []RunwayMoment{capped, limited}
}

// twoLimitsClose has work at its 5-hour limit from 13:52 till it resets at
// 14:10, then at its week's from 14:12, two stretches a column apart.
func twoLimitsClose() RunwayMoment {
	weekBack := mondayTen.Add(11 * time.Hour)
	workAt := withLimit(readAccount(work, onThu(14, 41, 0), window(fiveHour, 0.06, onThu(19, 12, 0)), spent(week, 1, weekBack)), []string{week}, weekBack)
	limit := status.Event{At: onThu(14, 12, 0), Kind: status.EventLimit, Account: work, Windows: []string{week}, Until: weekBack, Limit: 1}
	return fiveHourAt("two limits a column apart", routed(onThu(14, 41, 0), []status.Event{limit}, workAt, sideOn(onThu(14, 41, 0))), sorted(
		readEach(work, onThu(13, 0, 0), window(fiveHour, 0.8, onThu(14, 10, 0)), window(week, 0.9, weekBack)),
		readEach(work, onThu(13, 52, 0), spent(fiveHour, 1, onThu(14, 10, 0))),
		readEach(work, onThu(14, 12, 0), window(fiveHour, 0.05, onThu(19, 12, 0)), spent(week, 1, weekBack)),
		sideRead(),
	), map[string]Figures{work: {
		FigureCells:     "room 0, limited 1-135",
		FigureStretches: "limited 13:52 – 14:10, as it resets @1 1-2, over; limited 14:12 – Mon 21:00 @3 3-135",
		FigureResets:    "33 19:12",
	}})
}

// runOutReachingTheCap has work, its reserve 5%, its 5-hour at 80% at 14:41,
// heading to reach its cap about 15:17, before it resets at 16:30: room
// running out till then, then its cap.
func runOutReachingTheCap() RunwayMoment {
	workAt := reserving(readAccount(work, onThu(14, 41, 0), window(fiveHour, 0.8, onThu(16, 30, 0)), window(week, 0.4, mondayTen)), 0.05)
	return fiveHourAt("a run-out reaching the cap first", routed(onThu(14, 41, 0), nil, workAt, sideOn(onThu(14, 41, 0))), sorted(
		readEach(work, onThu(13, 0, 0), window(fiveHour, 0.5, onThu(16, 30, 0)), window(week, 0.4, mondayTen)),
		readEach(work, onThu(14, 41, 0), workAt.Windows...),
		sideRead(),
	), map[string]Figures{
		"": {FigureDrawn: "room, runs out, capped, │"},
		work: {
			FigureCells: "room 0-5, runs out 6-9, capped 10-16, room 17-135", FigureStretches: "capped ~15:16:49 – 16:30, as it resets @9 10-16",
			FigureResets: "17 16:30 back", FigureRoom: "no",
		},
	})
}

// modelsOwnLimit has work's Fable week at its limit, which holds back Fable's
// requests alone: its lane isn't held, the pool counts it with room, and
// COMING UP lists no back of it.
func modelsOwnLimit() RunwayMoment {
	fableBack := time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)
	workAt := withLimit(readAccount(work, onThu(14, 41, 0), window(fiveHour, 0.3, onThu(16, 30, 0)), window(week, 0.4, mondayTen), spent(fable, 1, fableBack)), []string{fable}, fableBack)
	limit := status.Event{At: onThu(13, 20, 0), Kind: status.EventLimit, Account: work, Windows: []string{fable}, Until: fableBack, Limit: 1}
	return fiveHourAt("a model's own window at its limit", routed(onThu(14, 41, 0), []status.Event{limit}, workAt, sideOn(onThu(14, 41, 0))), sorted(
		readEach(work, onThu(13, 0, 0), window(fiveHour, 0.2, onThu(16, 30, 0)), window(week, 0.4, mondayTen), window(fable, 0.9, fableBack)),
		readEach(work, onThu(14, 41, 0), workAt.Windows...),
		sideRead(),
	), map[string]Figures{
		"":   {FigurePool: "2 0-135", FigureDrawn: "room, │", FigureComing: "16:30 work reset 5h; 16:50 side reset 5h"},
		work: {FigureCells: "room 0-135", FigureStretches: "", FigureRoom: "yes"},
	})
}

// fable is the Fable week's key: a model's own window.
const fable = "7d_oi"

// framesMoments are the frames' moments, as the auto and ten worlds have
// them, built by hand from what they read: at 14:42:07 on Thursday 1 October,
// in the 5-hour view and the week's; Wednesday stepped back to; and the ten
// accounts' 5-hour view. The reference tests hold the frames themselves.
func framesMoments() []RunwayMoment {
	auto, ten := autoWorld(), tenWorld()
	return []RunwayMoment{
		{Window: views.RunwayFiveHour,
			Name: "the auto world's 5-hour view", Doc: auto.doc(), Now: thursday, Readings: auto.read, Columns: lanes,
			Want: map[string]Figures{
				"": {
					FigureOf: "3", FigurePool: "3 0-4, 2 5-20, 3 21-135", FigureDrawn: "room, capped, │",
					FigureComing: "16:30 personal reset 5h; 17:10 work back cap; 18:50 side reset 5h",
				},
				work: {
					FigureCells: "room 0-4, capped 5-20, room 21-135", FigureStretches: "capped 14:31:03 – 17:10, as it resets @5 5-20",
					FigureResets: "21 17:10 back", FigureRoom: "no", FigureUse: "5h 95%",
				},
				personal: {FigureCells: "room 0-135", FigureResets: "17 16:30", FigureRoom: "yes", FigureUse: "5h 39%"},
				side:     {FigureCells: "room 0-135", FigureResets: "31 18:50", FigureRoom: "yes", FigureUse: "5h 13%"},
			},
		},
		{
			Name: "the auto world's week view", Doc: auto.doc(), Now: thursday, Readings: auto.read, Window: views.RunwayWeek, Columns: lanes,
			Want: map[string]Figures{
				"": {FigureNow: "19"},
				work: {
					FigureCells:     "limited 0-1, room 2-18, capped 19-20, room 21-135",
					FigureStretches: "limited Wed 15:02 – Wed 17:10 @0 0-1, over; capped 14:31:03 – 17:10 @19 19-20",
					FigureResets:    "82 Sun 21:00", FigureUse: "7d 34%",
				},
				personal: {
					FigureCells:     "room 0-3, limited 4-7, room 8-18, runs out 19-50, limited 51-66, room 67-135",
					FigureStretches: "limited Wed 19:12:40 – Wed 23:58 @3 4-7, over; limited ~Sat 06:00 – Sun 02:00, as it resets @51 51-66",
					FigureResets:    "67 Sun 02:00 back", FigureUse: "7d 66%",
				},
			},
		},
		{Window: views.RunwayFiveHour,
			Name: "the auto world, Wednesday stepped back to", Doc: auto.doc(), Now: thursday, Readings: auto.read, Days: autoWednesday(),
			Day: civilDay(2026, time.September, 30), Columns: lanes,
			Want: map[string]Figures{
				"": {
					FigureNow: "none", FigurePool: "3 0-84, 2 85-96, 3 97-108, 2 109-135",
				},
				work:     {FigureCells: "room 0-84, limited 85-96, room 97-135", FigureStretches: "limited Wed 15:02 – Wed 17:10, as it resets @85 85-96", FigureUse: "5h highest 100%"},
				personal: {FigureCells: "room 0-108, limited 109-135", FigureStretches: "limited Wed 19:12:40 – Wed 23:58, as it resets @108 109-135", FigureUse: "5h highest 100%"},
				side:     {FigureCells: "room 0-135", FigureStretches: "", FigureUse: "5h highest 62%"},
			},
		},
		{Window: views.RunwayFiveHour,
			Name: "the ten world's 5-hour view", Doc: ten.doc(), Now: thursday, Readings: ten.read, Columns: lanes,
			Want: map[string]Figures{
				"": {
					FigureOf: "10", FigurePool: "10 0-1, 9 2-4, 8 5-10, 9 11-13, 8 14-18, 9 19-20, 10 21-135",
				},
				"team": {FigureCells: "room 0-1, limited 2-10, room 11-135", FigureStretches: "limited 13:58 – 15:30, as it resets @1 2-10"},
				"lab":  {FigureCells: "room 0-5, runs out 6-13, limited 14-18, room 19-135", FigureStretches: "limited ~16:04 – 16:55, as it resets @14 14-18"},
			},
		},
	}
}

// world is a world's accounts, as the router read them at 14:41 on Thursday,
// what it told of, and the readings history.
type world struct {
	accounts []status.Account
	events   []status.Event
	read     []readings.Reading
}

// doc is the router's document of the world, read at 14:41 on Thursday.
func (w world) doc() status.Document {
	return routed(onThu(14, 41, 0), w.events, w.accounts...)
}

// The resets of the auto world's weeks: personal's on Sunday at 02:00, and
// work's on Sunday at 21:00.
var (
	sunTwo  = time.Date(2026, 10, 4, 2, 0, 0, 0, time.UTC)
	sunNine = time.Date(2026, 10, 4, 21, 0, 0, 0, time.UTC)
)

// pressed is a, the router having seen its 5-hour, its pressure window, used
// at the rate given, a share of it an hour, over the half hour before at, as
// it projects that window by.
func pressed(a status.Account, rate float64, at time.Time) status.Account {
	since := at.Add(-30 * time.Minute)
	a.Rates = append(slices.Clip(a.Rates), status.Rate{Window: fiveHour, Rate: rate, Since: since})
	a.Pressure = status.Pressure{Window: fiveHour, Rate: rate, Recent: true, Since: since}
	return a
}

// autoWorld is the auto world at 14:41, each account's 5-hour used slowly of
// late, and its readings since Wednesday: work, its reserve 5%, its 5-hour
// primed at 12:10 reaching its cap at 14:31:03 till 17:10, its week at 34%
// till Sunday 21:00, and on Wednesday its 5-hour at its cap at 14:58 and its
// limit at 15:02 till 17:10; personal, the primary, its 5-hour primed at
// 11:30 at 39% till 16:30, its week at 66% till Sunday 02:00, at a pace that
// runs it out on Saturday at 06:00, and on Wednesday its 5-hour at its limit
// from 19:12:40 till 23:58; and side, its 5-hour primed at 13:50 at 13% till
// 18:50, its week at 78% till Monday 10:00, and on Wednesday its 5-hour at
// 62%.
func autoWorld() world {
	at := onThu(14, 41, 0)
	workAt := pressed(reserving(readAccount(work, at, window(fiveHour, 0.95, onThu(17, 10, 0)), window(week, 0.34, sunNine)), 0.05), 0.02, at)
	personalAt := rated(pressed(readAccount(personal, at, window(fiveHour, 0.39, onThu(16, 30, 0)), window(week, 0.66, sunTwo)), 0.05, at),
		week, 0.34/(39*time.Hour+19*time.Minute).Hours(), at)
	personalAt.Primary = true
	sideAt := pressed(readAccount(side, at, window(fiveHour, 0.13, onThu(18, 50, 0)), window(week, 0.78, mondayTen)), 0.05, at)
	return world{
		accounts: []status.Account{workAt, personalAt, sideAt},
		events:   []status.Event{{At: onThu(14, 31, 3), Kind: status.EventCap, Account: work, Windows: []string{fiveHour}, Until: onThu(17, 10, 0), Reserve: 0.05}},
		read: sorted(
			readEach(work, onWed(-1, 0, 0), window(fiveHour, 0, onWed(3, 0, 0)), window(week, 0.2, sunNine)),
			readEach(work, onWed(12, 10, 0), window(fiveHour, 0, onWed(17, 10, 0))),
			readEach(work, onWed(14, 58, 0), window(fiveHour, 0.95, onWed(17, 10, 0))),
			readEach(work, onWed(15, 2, 0), spent(fiveHour, 1, onWed(17, 10, 0))),
			readEach(work, onThu(12, 10, 0), window(fiveHour, 0, onThu(17, 10, 0))),
			readEach(work, onThu(14, 20, 0), window(fiveHour, 0.9, onThu(17, 10, 0))),
			readEach(work, onThu(14, 31, 3), window(fiveHour, 0.95, onThu(17, 10, 0)), window(week, 0.34, sunNine)),
			readEach(personal, onWed(-1, 0, 0), window(fiveHour, 0, onWed(3, 0, 0)), window(week, 0.5, sunTwo)),
			readEach(personal, onWed(18, 58, 0), window(fiveHour, 0, onWed(23, 58, 0))),
			readEach(personal, onWed(19, 12, 40), spent(fiveHour, 1, onWed(23, 58, 0))),
			readEach(personal, onThu(11, 30, 0), window(fiveHour, 0, onThu(16, 30, 0))),
			readEach(personal, onThu(14, 20, 0), window(week, 0.66, sunTwo)),
			readEach(personal, onThu(14, 40, 0), window(fiveHour, 0.39, onThu(16, 30, 0))),
			readEach(side, onWed(-1, 0, 0), window(fiveHour, 0, onWed(3, 0, 0)), window(week, 0.7, mondayTen)),
			readEach(side, onWed(19, 30, 0), window(fiveHour, 0, onThu(0, 30, 0))),
			readEach(side, onWed(23, 0, 0), window(fiveHour, 0.62, onThu(0, 30, 0))),
			readEach(side, onThu(13, 50, 0), window(fiveHour, 0, onThu(18, 50, 0))),
			readEach(side, onThu(14, 40, 0), window(fiveHour, 0.13, onThu(18, 50, 0)), window(week, 0.78, mondayTen)),
		),
	}
}

// autoWednesday is the auto world's Wednesday, as the ledger summarises it.
func autoWednesday() []ledger.Summary {
	return []ledger.Summary{{Version: 2, Day: "2026-09-30", Accounts: []ledger.AccountDay{
		{
			Account: personal, Highest: map[string]float64{fiveHour: 1, week: 0.5},
			Resets: []ledger.Reset{{Window: fiveHour, At: onWed(3, 0, 0)}, {Window: fiveHour, At: onWed(23, 58, 0), Before: 1}},
			Limits: []ledger.Limit{{Window: fiveHour, At: onWed(19, 12, 40), ResetsAt: onWed(23, 58, 0)}},
		},
		{Account: side, Highest: map[string]float64{fiveHour: 0.62, week: 0.7}, Resets: []ledger.Reset{{Window: fiveHour, At: onWed(3, 0, 0)}}},
		{
			Account: work, Highest: map[string]float64{fiveHour: 1, week: 0.2},
			Resets: []ledger.Reset{{Window: fiveHour, At: onWed(3, 0, 0)}, {Window: fiveHour, At: onWed(17, 10, 0), Before: 1}},
			Limits: []ledger.Limit{{Window: fiveHour, At: onWed(15, 2, 0), ResetsAt: onWed(17, 10, 0)}},
		},
	}}}
}

// tenWorld is the ten world at 14:41: the auto world's three, and client,
// spare, lab, under pressure, running out about 16:04 before its 5-hour
// resets at 16:55, team, at its 5-hour limit since 13:58 till 15:30, as the
// readings tell and no event does, extra, ops and studio, each read at 13:00
// as at 14:41, their weeks resetting on Friday at 10:00.
func tenWorld() world {
	at, fridayTen, teamBack := onThu(14, 41, 0), time.Date(2026, 10, 2, 10, 0, 0, 0, time.UTC), onThu(15, 30, 0)
	steady := func(id string, used float64, resets time.Time, weekUsed float64) status.Account {
		return pressed(readAccount(id, at, window(fiveHour, used, resets), window(week, weekUsed, fridayTen)), 0.05, at)
	}
	team := readAccount("team", at, spent(fiveHour, 1, teamBack), window(week, 0.83, fridayTen))
	team.Limit = status.Limit{ID: 9, Windows: []string{fiveHour}, Until: teamBack}
	w := autoWorld()
	w.accounts = append(w.accounts,
		steady("client", 0.52, onThu(17, 40, 0), 0.47),
		steady("spare", 0.04, onThu(19, 35, 0), 0.05),
		pressed(readAccount("lab", at, window(fiveHour, 0.71, onThu(16, 55, 0)), window(week, 0.58, fridayTen)), 0.29/(83*time.Minute).Hours(), at),
		team,
		steady("extra", 0.22, onThu(17, 55, 0), 0.12),
		steady("ops", 0.06, onThu(19, 10, 0), 0.29),
		steady("studio", 0.33, onThu(16, 20, 0), 0.41),
	)
	read := slices.Concat(
		readEach(team.ID, onThu(13, 30, 0), window(fiveHour, 0.9, teamBack), window(week, 0.83, fridayTen)),
		readEach(team.ID, onThu(13, 58, 0), spent(fiveHour, 1, teamBack)),
	)
	for _, a := range w.accounts[3:] {
		if a.ID != team.ID {
			read = append(read, readEach(a.ID, onThu(13, 0, 0), a.Windows...)...)
		}
	}
	w.read = sorted(w.read, read)
	return w
}
