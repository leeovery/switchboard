package views

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// ProjectPage is the project chosen on Projects, as its page under the list
// shows it over the period: its row; its share of every project's worth,
// nil where either is none; its tokens; its worth in buckets, WORTH BY; each
// model version's share of its worth, BY MODEL; each account's, ON
// ACCOUNTS; and its sessions, SESSIONS.
type ProjectPage struct {
	ProjectRow
	Share  *float64
	Tokens Tokens
	// WorthBy is its worth by the bucket the period has it in, as
	// worthBuckets has them: of the last 12 weeks, by day, and WorthByWeek
	// by week, for the drawing to take whichever fits; nil of any other
	// period.
	WorthBy     WorthBy
	WorthByWeek *WorthBy
	// ByModel are each model version's share of its worth, as Accounts'
	// BY MODEL reckons a row's, the greatest first, those whose share
	// rounds to none, or isn't known, left out.
	ByModel []VersionShare
	// OnAccounts are each account its requests went to, the one it was
	// worth most on first.
	OnAccounts []AccountShare
	// Sessions are its sessions over the period, in the List's order: those
	// running first, the oldest started first, then those ended, the latest
	// ended first.
	Sessions []ProjectSession
}

// VersionShare is a model version's share of a project's worth: the
// version, as the version table names and shades it, and its share.
type VersionShare struct {
	Version ledger.Version
	ModelShare
}

// AccountShare is an account's share of a project's worth: the account, by
// its id, as names name it; what the project's requests on it were worth;
// and that worth's share of the project's, nil where either is none.
type AccountShare struct {
	Account string
	status.AccountName
	Worth
	Share *float64
}

// ProjectSession is a session of a project as SESSIONS rows it, in the
// List's form, so it's drawn in the List's columns: its requests and their
// worth over the period. EndedOn is the date of the local day it ended on,
// as its rule groups it, "" where it hasn't ended.
type ProjectSession struct {
	ListedSession
	EndedOn string
}

// BucketUnit is what a bucket of WORTH BY spans: an hour, a day or a week.
type BucketUnit int

const (
	// ByHour is an hour of today, by the local clock.
	ByHour BucketUnit = iota
	// ByDay is a local day.
	ByDay
	// ByWeek is a calendar week, from the day weeks start on.
	ByWeek
)

// WorthBy is a project's worth in buckets, oldest first, as WORTH BY draws
// it: what each spans; each bucket; the index of the one worth most, the
// first of any that tie, -1 where none is worth anything; and how many of
// the last are still to come, as a week's days after today are.
type WorthBy struct {
	Unit    BucketUnit
	Buckets []WorthBucket
	Peak    int
	ToCome  int
}

// WorthBucket is a bucket of WORTH BY: when it starts, as dayfile.Civil
// gives a day, or of an hour's, as byHour has it; its name, as "14", "Wed"
// or "17 Aug"; what its axis names it, as Axis has it; and what the
// project's requests in it were worth.
type WorthBucket struct {
	Start time.Time
	Label string
	Axis  string
	Worth
}

// PeakNamed names WORTH BY's peak bucket, as the line under it says it: an
// hour by its start, "14:00"; a day by its name, "Wed"; a week by its first
// day, "week of 17 Aug"; "" where there's no peak.
func (by WorthBy) PeakNamed() string {
	if by.Peak < 0 {
		return ""
	}
	b := by.Buckets[by.Peak]
	switch by.Unit {
	case ByHour:
		return prose.TimeOfDay(b.Start, b.Start)
	case ByWeek:
		return "week of " + b.Label
	}
	return b.Label
}

// Now is the index of the bucket now falls in, the last of those not still
// to come, as the axis picks its name out: -1 where all are to come.
func (by WorthBy) Now() int {
	return len(by.Buckets) - 1 - by.ToCome
}

// page is the project's page, its row given, every project's totals given,
// the page built of in.
func (p *projectTally) page(row ProjectRow, all ProjectTotals, in ProjectsInput) ProjectPage {
	v := versions{table: in.Prices, family: in.Family}
	page := ProjectPage{ProjectRow: row, Share: shareOf(p.worth, all.amount()), Tokens: p.tokens, ByModel: p.byModel(v), OnAccounts: p.onAccounts(in.Names)}
	page.WorthBy, page.WorthByWeek = p.worthBy(in)
	page.Sessions = p.rows(in)
	return page
}

// byModel are each version's share of the project's worth, as sharesOf
// reckons them, the greatest first, those of the version table's order
// first where they tie, those whose share rounds to none or isn't known left
// out, each named as v names it.
func (p *projectTally) byModel(v versions) []VersionShare {
	shares := slices.DeleteFunc(sharesOf(p.models, p.worth.amount()), func(s ModelShare) bool {
		return s.Share == nil || prose.WholePercent(*s.Share) == 0
	})
	slices.SortStableFunc(shares, func(a, b ModelShare) int { return cmp.Compare(b.amount(), a.amount()) })
	named := v.named(shares)
	byModel := make([]VersionShare, len(shares))
	for i, s := range shares {
		byModel[i] = VersionShare{Version: named[i], ModelShare: s}
	}
	return byModel
}

// onAccounts are each account the project's requests went to, its share of
// the worth of those that went to one, so the shares make the whole of it,
// the one worth most first, then by id, each named as names name it.
func (p *projectTally) onAccounts(names status.Namer) []AccountShare {
	var named Worth
	for _, w := range p.accounts {
		named.add(w)
	}
	on := make([]AccountShare, 0, len(p.accounts))
	for id, w := range p.accounts {
		on = append(on, AccountShare{Account: id, AccountName: namedOf(names, id), Worth: w, Share: shareOf(w, named.amount())})
	}
	slices.SortFunc(on, func(a, b AccountShare) int {
		return cmp.Or(cmp.Compare(b.amount(), a.amount()), strings.Compare(a.Account, b.Account))
	})
	return on
}

// worthBy is the project's worth in the buckets in's period has it in, as
// WORTH BY draws them: today's by hour; this week's and the last 30 days' by
// day; the last 12 weeks' by day, and by week beside them; and the last 12
// months' by week.
func (p *projectTally) worthBy(in ProjectsInput) (WorthBy, *WorthBy) {
	today := dayfile.CivilOf(in.Now)
	first, last := in.Period.first(today, in.WeekStarts), in.Period.last(today, in.WeekStarts)
	switch in.Period {
	case PeriodToday:
		return p.byHour(in.Now), nil
	case PeriodTwelveWeeks:
		weeks := p.byWeek(first, last, today, in.WeekStarts)
		return p.byDay(first, last, today, ByDay), &weeks
	case PeriodTwelveMonths:
		return p.byWeek(first, last, today, in.WeekStarts), nil
	}
	return p.byDay(first, last, today, ByDay), nil
}

// byHour is the project's worth today by the local hour, now given, as
// WORTH BY HOUR names them, on the axis too, "00" to "23", each starting as today's clocks
// first read it, or went forward over it, as dayfile.TimeOfDay has it, so
// an hour a clock change repeats is one bucket, and one it skips starts with
// the next.
func (p *projectTally) byHour(now time.Time) WorthBy {
	buckets := make([]WorthBucket, len(p.hours))
	for h, w := range p.hours {
		named := prose.HourNamed(h)
		buckets[h] = WorthBucket{Start: dayfile.TimeOfDay(now.Local(), h, 0), Label: named, Axis: named, Worth: w}
	}
	return peaked(WorthBy{Unit: ByHour, Buckets: buckets, ToCome: len(buckets) - 1 - now.Local().Hour()})
}

// byDay is the project's worth by day from first to last, each as
// dayfile.Civil gives a day, today given: each named, on the axis too, by
// its weekday where they're a week or fewer, else by its date, as "21 Sep".
func (p *projectTally) byDay(first, last, today time.Time, unit BucketUnit) WorthBy {
	named := prose.DayMonth
	if last.Sub(first) < 7*24*time.Hour {
		named = func(d time.Time) string { return prose.Weekday(d.Weekday()) }
	}
	by := WorthBy{Unit: unit}
	for d := first; !d.After(last); d = d.AddDate(0, 0, 1) {
		by.Buckets = append(by.Buckets, WorthBucket{Start: d, Label: named(d), Axis: named(d), Worth: p.days[d.Format(time.DateOnly)]})
		if d.After(today) {
			by.ToCome++
		}
	}
	return peaked(by)
}

// byWeek is the project's worth by calendar week from the one first falls
// in to the one last does, each as dayfile.Civil gives a day, weeks
// starting on the day given, today given: each named by its first day's
// date, as "17 Aug", the axis naming the month of the first of its days
// from first, as "Aug", where it's another than the week before's, its
// worth that of its days from first to last alone.
func (p *projectTally) byWeek(first, last, today time.Time, starts time.Weekday) WorthBy {
	by := WorthBy{Unit: ByWeek}
	month := ""
	for week := weekOf(first, starts); !week.After(last); week = week.AddDate(0, 0, 7) {
		b := WorthBucket{Start: week, Label: prose.DayMonth(week)}
		if named := prose.Month(score.Later(week, first), false); named != month {
			b.Axis, month = named, named
		}
		for d := score.Later(week, first); d.Before(week.AddDate(0, 0, 7)) && !d.After(last); d = d.AddDate(0, 0, 1) {
			b.add(p.days[d.Format(time.DateOnly)])
		}
		by.Buckets = append(by.Buckets, b)
		if week.After(today) {
			by.ToCome++
		}
	}
	return peaked(by)
}

// peaked is by with its peak found: the first of the buckets worth most,
// -1 where none is worth anything.
func peaked(by WorthBy) WorthBy {
	by.Peak = -1
	for i, b := range by.Buckets {
		if b.amount() > 0 && (by.Peak < 0 || b.amount() > by.Buckets[by.Peak].amount()) {
			by.Peak = i
		}
	}
	return by
}

// rows are the project's sessions as SESSIONS rows them, in the List's
// order: each of in's List as the List has it, its requests and worth those
// of the period; each other as listedEnded has it.
func (p *projectTally) rows(in ProjectsInput) []ProjectSession {
	listed := make(map[string]ListedSession, len(in.List.Sessions))
	for _, s := range in.List.Sessions {
		listed[s.Session] = s
	}
	price := pricing{prices: in.Prices, date: dayfile.DateOf(in.Now)}
	rows := make([]ProjectSession, 0, len(p.sessions))
	for _, st := range p.sessions {
		s, ok := listed[st.id]
		if ok {
			s.Requests, s.Worth = st.requests, st.worth
			s.Started = cmp.Or(s.Started, st.firstRequest)
		} else {
			s = st.listedEnded(price)
		}
		row := ProjectSession{ListedSession: s.named(in.Names)}
		if s.Activity.Doing == Ended {
			row.EndedOn = dayfile.DateOf(s.Ended)
		}
		rows = append(rows, row)
	}
	slices.SortFunc(rows, func(a, b ProjectSession) int { return inListOrder(a.ListedSession, b.ListedSession) })
	return rows
}

// listedEnded is the session as the List lists one that ended, of the days
// it ran on in the period, as their records tell it: in the directory, and
// of the project, it last named; its requests and their worth; started at
// its first request and ended at its last, or of none, its last line; named
// as names has it of what names it over its days, as no router lists it;
// and brought to its account, as the List brings one, by its model's latest
// move, on its last day, where that was onto it, as movedOnto has it, its
// cost priced as price prices it, marked ▸ where a move brought it there.
func (st *sessionTally) listedEnded(price pricing) ListedSession {
	ended := cmp.Or(st.lastRequest, st.lastLine)
	s := ListedSession{
		Session: st.id, Dir: st.dir, Project: st.project, Requests: st.requests, Worth: st.worth,
		Started: st.firstRequest, Ended: ended, Activity: Activity{Doing: Ended, Since: ended, LastSeen: ended},
	}
	s.Account, s.Model = st.naming.names(nil)
	if m := movedOnto(st.moves, s.Model, s.Account, func(m *dayMove) (string, string) { return m.model, m.to }); m != nil {
		s.broughtBy(m.brought(price))
	} else {
		s.broughtBy(nil, RoutingSaid{})
	}
	return s
}

// brought is the move as the List gives one that brought a session to its
// account, and how the router brought it there, as routing says it: what
// writing the session's context again cost priced as price prices it, as the
// story's rewriteCost prices it, nil where none was written, or the table
// can't price all of it.
func (m dayMove) brought(price pricing) (*Move, RoutingSaid) {
	var cost *ledger.Picodollars
	if m.rewrite != nil {
		if w, priced := price.prices.SummedSpent(*m.rewrite, price.date); priced && len(w.Unpriced) == 0 {
			cost = &w.Cost
		}
	}
	moved := &Move{At: m.at, From: m.from, Reason: m.reason, Cost: cost}
	return moved, movedRouting(m.tried, m.reason, status.Facts{From: m.from, To: m.to}, cost, m.rewrite != nil)
}
