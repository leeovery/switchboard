package views

import (
	"cmp"
	"maps"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// ProjectsInput is what Projects' page is built of beside the sessions'
// days: the period it counts over; today's List, as HeldSessions' List gives
// it, its sessions running and those of today, as SESSIONS rows them; what
// names the accounts, configured and removed; the prices worth is priced by,
// at now's day's prices, as Past and the List price theirs, and the version
// table that names the models; the day weeks start on; how a model's family
// is read from its id, for one the table doesn't name; and now.
type ProjectsInput struct {
	Period     Period
	List       SessionList
	Names      status.Namer
	Prices     ledger.Table
	WeekStarts time.Weekday
	Family     func(model string) string
	Now        time.Time
}

// ProjectsPage is Projects' list, as the dashboard draws it and a verb gives
// it: a row for each project with a session in the period, by its path,
// the sessions with no directory one row of their own, the one worth most
// first; and every project's together. A session's project is the one
// status.ProjectOf reads of what its lines named, and of what the router
// lists, of a session running. Project gives each one's page.
type ProjectsPage struct {
	Period Period
	Rows   []ProjectRow
	All    ProjectTotals
	// PricesAsOf is the date of the day the price table worth is priced by
	// was read, as help gives it.
	PricesAsOf string

	tallies map[string]*projectTally
	in      ProjectsInput
}

// ProjectTotals are a project's totals over the period, or every project's:
// its sessions running now; its sessions, each once, however many days it
// ran on, those running among them; its requests; and what they were worth
// at API prices, as Worth sums it.
type ProjectTotals struct {
	Live, Sessions, Requests int
	Worth
}

// ProjectRow is a project's row of the list: its path, as status.ProjectOf
// gives it, "" of the sessions with no directory; its name, by its path's
// last part, and its parents' too where two share it, as
// status.ProjectNames names them, uncut, "" of no directory; its totals; its
// worth by model family, in the version table's order of families, as BY
// MONTH splits a month's; when its last request arrived, in UTC, zero
// where none did; and what its sessions running are doing, the busiest's,
// as busiest has it, Idle where none runs, so its last request reads as now
// while one of them is busy, as Activity's Busy has it.
type ProjectRow struct {
	Path, Name string
	ProjectTotals
	ByFamily []FamilyWorth
	Last     time.Time
	Activity Activity
}

// NoDirectory reports whether the row is of the sessions with no directory.
func (r ProjectRow) NoDirectory() bool {
	return r.Path == ""
}

// Projects is Projects' page over in's period, as ProjectsPage has it, of
// the days d holds at in's Now, as each has it: the days of the period
// gone, and the day d's fold of the tail is of, today's or, where the day
// turned since the tail was last read, the day it turned from; with the
// sessions in's List lists running. Each session is one project's, the one
// of where it last ran, as sessionPlaces has it, and started at its first
// request wherever it fell, as startedBefore has it of the days before the
// period's.
func (d SessionDays) Projects(in ProjectsInput) ProjectsPage {
	today := dayfile.CivilOf(in.Now)
	first := in.Period.first(today, in.WeekStarts).Format(time.DateOnly)
	todayDate := today.Format(time.DateOnly)
	t := projectTallies{all: make(map[string]*projectTally), price: pricing{prices: in.Prices, date: todayDate}, v: versions{table: in.Prices, family: in.Family}}
	places := d.sessionPlaces(first, todayDate, in.List)
	d.each(first, todayDate, func(date string, s *foldedSession) {
		p := t.of(places[s.ID])
		p.take(date, s.DaySession, t.price, t.v)
		if date == todayDate {
			p.hourly(s.hours, t.price)
		}
	})
	for _, s := range in.List.Sessions {
		if s.Running {
			t.of(places[s.Session]).run(s)
		}
	}
	t.started(d.startedBefore(first))
	return t.page(in)
}

// sessionPlace is where a session ran, its directory and its project, as
// placeAfter has them.
type sessionPlace struct {
	dir, project string
}

// sessionPlaces are where each session of the period from the date first to
// the date today ran last, by its id, as placeAfter has it of its days in
// their order, then of list's listing of it, where it runs.
func (d SessionDays) sessionPlaces(first, today string, list SessionList) map[string]sessionPlace {
	places := make(map[string]sessionPlace)
	ranIn := func(id, dir, project string) {
		was := places[id]
		was.dir, was.project = placeAfter(was.dir, was.project, dir, project)
		places[id] = was
	}
	d.each(first, today, func(_ string, s *foldedSession) { ranIn(s.ID, s.Dir, s.Project) })
	for _, s := range list.Sessions {
		if s.Running {
			ranIn(s.Session, s.Dir, s.Project)
		}
	}
	return places
}

// each hands f each session of each day of the period from the date first
// to the date today d holds, oldest first, with its day's date: of the days
// gone, as their records hold them; and of the day d's fold of the tail is
// of, as the fold holds them, what they spent by the hour too.
func (d SessionDays) each(first, today string, f func(date string, s *foldedSession)) {
	for _, day := range d.days {
		if day.date >= first && day.date < today && day.date != d.today.Date {
			for _, s := range day.sessions {
				f(day.date, &foldedSession{DaySession: s})
			}
		}
	}
	if d.today.Date >= first && d.today.Date <= today {
		for _, s := range d.held.held {
			f(d.today.Date, s)
		}
	}
}

// startedBefore are when the sessions d holds of the days before the date
// first made their first requests, by their ids: the earliest each made of
// those days d holds, or of the days before them ReadSessionDays walked back
// over, as begun has them.
func (d SessionDays) startedBefore(first string) map[string]time.Time {
	started := make(map[string]time.Time, len(d.begun))
	maps.Copy(started, d.begun)
	note := func(s *DaySession) {
		if was, ok := started[s.ID]; !s.FirstRequest.IsZero() && (!ok || s.FirstRequest.Before(was)) {
			started[s.ID] = s.FirstRequest
		}
	}
	for _, day := range d.days {
		if day.date < first {
			for i := range day.sessions {
				note(&day.sessions[i])
			}
		}
	}
	if d.today.Date < first {
		for _, s := range d.held.held {
			note(&s.DaySession)
		}
	}
	return started
}

// Project is the page of the project of the row with the given path, as
// ProjectPage has it, reporting false where the list has none.
func (p ProjectsPage) Project(path string) (ProjectPage, bool) {
	i := slices.IndexFunc(p.Rows, func(r ProjectRow) bool { return r.Path == path })
	if i < 0 {
		return ProjectPage{}, false
	}
	return p.tallies[path].page(p.Rows[i], p.All, p.in), true
}

// projectTallies are each project's tally, by its path, as the page's days
// are taken in, priced and named as price and v have them.
type projectTallies struct {
	all   map[string]*projectTally
	price pricing
	v     versions
}

// of is the tally of the project a session that ran where given is of, as
// status.ProjectOf reads it: begun where none is yet.
func (t projectTallies) of(ran sessionPlace) *projectTally {
	path := status.ProjectOf(ran.project, ran.dir)
	p, ok := t.all[path]
	if !ok {
		p = &projectTally{path: path, sessions: make(map[string]*sessionTally), families: familySums{}, models: modelTally{},
			accounts: make(map[string]Worth), days: dayWorths{}}
		t.all[path] = p
	}
	return p
}

// started has each session the tallies hold start at its first request
// before the period, where started gives one, by its id.
func (t projectTallies) started(started map[string]time.Time) {
	for _, p := range t.all {
		for id, st := range p.sessions {
			if at, ok := started[id]; ok && (st.firstRequest.IsZero() || at.Before(st.firstRequest)) {
				st.firstRequest = at
			}
		}
	}
}

// page is the list of the tallies, as Projects has it.
func (t projectTallies) page(in ProjectsInput) ProjectsPage {
	paths := slices.Collect(maps.Keys(t.all))
	names := status.ProjectNames(paths, math.MaxInt)
	page := ProjectsPage{Period: in.Period, PricesAsOf: in.Prices.AsOf, tallies: t.all, in: in}
	sessions := make(map[string]bool)
	for _, p := range t.all {
		row := p.row(names[p.path], t.v)
		page.Rows = append(page.Rows, row)
		page.All.Live += row.Live
		page.All.Requests += row.Requests
		page.All.add(row.Worth)
		for id := range p.sessions {
			sessions[id] = true
		}
	}
	page.All.Sessions = len(sessions)
	slices.SortFunc(page.Rows, func(a, b ProjectRow) int {
		return cmp.Or(cmp.Compare(b.amount(), a.amount()), strings.Compare(a.Name, b.Name), strings.Compare(a.Path, b.Path))
	})
	return page
}

// projectTally is a project's use over the period as it's taken in: its
// path; its sessions, by their ids; how many of them run now, and the
// busiest's activity; its requests,
// their worth and tokens; its worth by family, by model version and by the
// account it ran on; and by day, by date, and today by the local hour; and
// when its last request arrived.
type projectTally struct {
	path     string
	sessions map[string]*sessionTally
	live     int
	activity Activity
	requests int
	worth    Worth
	tokens   Tokens
	families familySums
	models   modelTally
	accounts map[string]Worth
	days     dayWorths
	hours    [24]Worth
	last     time.Time
}

// sessionTally is a session of a project over the period as its days are
// taken in, oldest first: where it ran, as its last day to name a directory
// or a project named them; its requests and their worth; when its first
// request arrived, wherever it fell, as started has it, its last, and its
// last line, of those counted toward it; what
// names it over its days; and its last day, by date, and the moves of each
// of its models that day.
type sessionTally struct {
	id, dir, project          string
	requests                  int
	worth                     Worth
	firstRequest, lastRequest time.Time
	lastLine                  time.Time
	naming                    naming
	lastDay                   string
	moves                     []dayMove
}

// session is the tally of the session with the given id: begun where none
// is yet.
func (p *projectTally) session(id string) *sessionTally {
	s, ok := p.sessions[id]
	if !ok {
		s = &sessionTally{id: id}
		p.sessions[id] = s
	}
	return s
}

// take takes in s, a session of the project's on the day with the given
// date, its uses priced as price prices them, named as v names them.
func (p *projectTally) take(date string, s DaySession, price pricing, v versions) {
	st := p.session(s.ID)
	st.dir, st.project = placeAfter(st.dir, st.project, s.Dir, s.Project)
	st.lastLine = score.Later(st.lastLine, s.Last)
	st.naming = st.naming.with(s.naming)
	if date >= st.lastDay {
		st.lastDay, st.moves = date, s.moves
	}
	if !s.FirstRequest.IsZero() && (st.firstRequest.IsZero() || s.FirstRequest.Before(st.firstRequest)) {
		st.firstRequest = s.FirstRequest
	}
	st.lastRequest = score.Later(st.lastRequest, s.LastRequest)
	p.last = score.Later(p.last, s.LastRequest)
	for _, u := range s.Uses {
		w := price.worth(u.Spent)
		st.requests += u.Requests
		st.worth.add(w)
		p.requests += u.Requests
		p.worth.add(w)
		p.tokens.count(u.Tokens)
		if u.Model != "" {
			p.families.add(v.familyOf(u.Model), w)
		}
		p.models.worth(v, u.Model, w, u.Tokens, u.Account)
		if u.Account != "" {
			on := p.accounts[u.Account]
			on.add(w)
			p.accounts[u.Account] = on
		}
		p.days.add(date, w)
	}
}

// hourly takes in what a session of the project's spent today by the local
// hour, on every account, as hours holds it, priced as price prices it.
func (p *projectTally) hourly(hours accountHours, price pricing) {
	for _, on := range hours {
		for h := range on {
			p.hours[h].add(price.worth(on[h]))
		}
	}
}

// run takes in s, a session of today's List the router lists running.
func (p *projectTally) run(s ListedSession) {
	p.live++
	p.activity = p.activity.with(s.Activity)
	p.session(s.Session)
}

// row is the project's row, named as given.
func (p *projectTally) row(name string, v versions) ProjectRow {
	return ProjectRow{
		Path: p.path, Name: name,
		Live: p.live, Sessions: len(p.sessions), Requests: p.requests, Worth: p.worth,
		ByFamily: v.byFamily(p.families), Last: p.last, Activity: p.activity,
	}
}
