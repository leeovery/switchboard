package views

import (
	"cmp"
	"maps"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// PeriodInput is what Projects' page and Sessions' log are built of beside
// the sessions' days: the period they count over; today's List, as
// HeldSessions' List gives it, its sessions running and those of today, as
// SESSIONS and the log row them at now, as listed has it; Hours, the fold of
// a day gone the period is, as ReadDayFold reads it, for its pages to chart
// it by the hour, as HoursFold picks it, the zero DayFold where none is read;
// what names the accounts, configured and removed; the prices worth is
// priced by, at now's day's prices, as Past and the List price theirs, a
// period gone's too, and the version table that names the models; the day
// weeks start on; how a model's family is read from its id, for one the
// table doesn't name; now; and Began, the start of the first day of the work
// the days are of, a pool's first day of work of a pool's, as PoolRead's
// Start tells it, for a page's charts to begin there, as worthCharts has it,
// and the days whose sessions are no longer kept to be told from those before
// it, as unkept has them: zero where it's unknown; Keep, how long the ledger
// keeps its lines, as [ledger] keep gives it, for the days no longer kept to
// be the Log's, as OldestKept has them, none as a config that doesn't say;
// and how a session's project is read, as the List reads it.
type PeriodInput struct {
	Period     Period
	List       SessionList
	Hours      DayFold
	Names      status.Namer
	Prices     ledger.Table
	WeekStarts time.Weekday
	Family     func(model string) string
	Now        time.Time
	Began      time.Time
	Keep       time.Duration
	Projects   status.Projects
}

// span is the period's span at in's Now, weeks starting on in's day.
func (in PeriodInput) span() periodSpan {
	return in.Period.spanAt(dayfile.CivilOf(in.Now), in.WeekStarts)
}

// lists reports whether a page of in's period lists in's List, today's: at
// now; never of a period gone, as none of its sessions runs, and today's are
// none of its, so nothing it rows, counts or tells afresh is of now.
func (in PeriodInput) lists() bool {
	return !in.span().gone
}

// listed is in's List as a page of its period lists it, as lists has it:
// none of a period gone.
func (in PeriodInput) listed() SessionList {
	if !in.lists() {
		return SessionList{}
	}
	return in.List
}

// ProjectsPage is Projects' list, as the dashboard draws it: a row for each
// project with a session of the period, as inPeriod has it, by its path,
// the sessions with no directory one row of their own, and those of
// temporary folders another, the one worth most first; and every project's
// together. A session's project is the one in's Projects reads of what its
// lines named, and of what the router lists, of a session running. Project
// gives each one's page; Most, the most rows any one's page takes; and
// Unkept, the days of a period gone whose sessions are no longer kept.
type ProjectsPage struct {
	Period Period
	Rows   []ProjectRow
	All    ProjectTotals
	Most   ProjectsMost
	Unkept UnkeptDays
	// PricesAsOf is the date of the day the price table worth is priced by
	// was read, as help gives it.
	PricesAsOf string

	tallies map[string]*projectTally
	in      PeriodInput
}

// UnkeptDays tell of the days of a period gone whose sessions are no longer
// kept, as the ledger keeps a day's sessions only as long as its lines, as
// OldestDay's kept has it: KeptFrom, the first day whose sessions are kept,
// as dayfile.Civil gives a day, the oldest day whose lines the ledger keeps
// at the page's now, where it began as the days were read and its keep
// applied at now, where a day of the period from the day the work it counts
// began is before it; zero where none is, as of every period at now; and
// Every, set where every such day is, so the period says so in place of
// saying it had no session. Where only some are, the List and a project's
// SESSIONS say so once, after their rows.
type UnkeptDays struct {
	KeptFrom time.Time
	Every    bool
}

// Said tells of the days no longer kept at now, as the List and a project's
// SESSIONS say it: "sessions no longer kept" where every day is, in place of
// saying the period had no session; "sessions before 14 Sep no longer kept"
// where only some are, by the first day kept, its year where it isn't now's,
// as prose.DayMonthAt dates it; "" where none is.
func (u UnkeptDays) Said(now time.Time) string {
	switch {
	case u.KeptFrom.IsZero():
		return ""
	case u.Every:
		return "sessions no longer kept"
	}
	return "sessions before " + prose.DayMonthAt(now, u.KeptFrom) + " no longer kept"
}

// ProjectsMost are the most of a page's rows any project of the list has:
// BY MODEL's versions and ON ACCOUNTS' accounts, as its page has them; and
// the cells of the widest name BY MODEL gives a version among them; so the
// page lays out alike whichever project is chosen.
type ProjectsMost struct {
	Versions, Accounts int
	VersionCells       int
}

// ProjectTotals are a project's totals over the period, or every project's:
// its sessions running now, none of a period gone; its sessions, each once,
// however many days it ran on, those running among them; its requests; and
// what they were worth at API prices, as Worth sums it.
type ProjectTotals struct {
	Live, Sessions, Requests int
	Worth
}

// ProjectRow is a project's row of the list: its path, as status.Projects
// reads it, "" of the sessions with no directory, status.TemporaryFolders of
// those of temporary folders; its name, by its path's last part, and its
// parents' too where two share it, as status.ProjectNames names them, uncut,
// "" of either of those; its totals; its share of every project's worth,
// nil where either is none; its tokens; its worth by model family, in the
// version table's order of families, as BY MONTH splits a month's; when its
// last request in the period arrived, in UTC, zero where none did; and what
// its sessions running are doing, the busiest's, as busiest has it, Idle
// where none runs, as in a period gone, so its last request reads as now
// while one of them is busy, as Activity's Busy has it.
type ProjectRow struct {
	Path, Name string
	ProjectTotals
	Share    *float64
	Tokens   Tokens
	ByFamily []FamilyWorth
	Last     time.Time
	Activity Activity
}

// NoDirectory reports whether the row is of the sessions with no directory.
func (r ProjectRow) NoDirectory() bool {
	return r.Path == ""
}

// Temporary reports whether the row is of the sessions of temporary folders.
func (r ProjectRow) Temporary() bool {
	return r.Path == status.TemporaryFolders
}

// Pathless reports whether the row is of no project's path: of the sessions
// with no directory, or of those of temporary folders.
func (r ProjectRow) Pathless() bool {
	return r.NoDirectory() || r.Temporary()
}

// Projects is Projects' page over in's period, as ProjectsPage has it, of
// the days d holds at in's Now, each project tallied as tallied has it.
func (d SessionDays) Projects(in PeriodInput) ProjectsPage {
	return d.tallied(in).page(in)
}

// SessionLog is Sessions' List over the period, as the dashboard draws it:
// every session of the period, as inPeriod has it, as SESSIONS rows a
// project's, those running first, or of a period gone those that went on
// after it, then those ended, by the day they ended, the latest first, as
// periodRows orders them; what they come to together, as every project's
// together on Projects; whether the period was gone as it was built, as
// Dial's Gone has it; and the days of a period gone whose sessions are no
// longer kept, as Projects has them. in is what it was built of, its List
// as Live tells it afresh.
type SessionLog struct {
	Period   Period
	Sessions []PeriodSession
	All      ProjectTotals
	Gone     bool
	Unkept   UnkeptDays

	in PeriodInput
}

// CountsTold reports whether the List's period line tells what the period's
// sessions come to beside its name: not where every day of the period gone
// is no longer kept, as the List says so in place of its rows, and there's
// nothing to count, so the line names the period alone.
func (l SessionLog) CountsTold() bool {
	return !l.Unkept.Every
}

// SessionLog is Sessions' List over in's period, as SessionLog has it, of
// the days d holds at in's Now, each project tallied as tallied has it, so
// it rows every session alike with Projects' pages, each named by its
// project as Projects names its row.
func (d SessionDays) SessionLog(in PeriodInput) SessionLog {
	t := d.tallied(in)
	names := t.names()
	sessions := make(map[string]*sessionTally)
	projectNamed := make(map[string]string)
	for _, p := range t.all {
		maps.Copy(sessions, p.sessions)
		for id := range p.sessions {
			projectNamed[id] = names[p.path]
		}
	}
	return SessionLog{
		Period: in.Period, Sessions: periodRows(sessions, in, projectNamed), All: t.totals(), Gone: in.span().gone, Unkept: t.unkept, in: in,
	}
}

// tallied are the projects of in's period, each tallied of the days d holds
// at in's Now, as each has it, from its first day to its last, or today at
// now; what each spent by the hour of the period's day, of the fold of it
// HoursFold picks of in's Hours and d's fold of the tail; with the sessions
// in's List lists running, at now alone, as listed has it; each session of
// the period alone, as inPeriod has it. Each session is one project's, the
// one of where it last ran in the period, as sessionPlaces has it, and
// started at its first request wherever it fell, as startedBefore has it of
// the days before the period's. Of a period gone, its days whose sessions
// are no longer kept are told, as unkept has them, and those told are the
// days none of whose sessions is counted, as keptOf has it; and those that
// went on after it are told, as wentOnAfter has them.
func (d SessionDays) tallied(in PeriodInput) projectTallies {
	span := in.span()
	told := unkept(span, in.Began, d.lines.Oldest(in.Keep, in.Now))
	counted := span.keptOf(told)
	first := counted.first.Format(time.DateOnly)
	hours := in.Period.HoursFold(in.Now, in.WeekStarts, in.Hours, d.TodayFold())
	t := projectTallies{
		all: make(map[string]*projectTally), price: pricing{prices: in.Prices, date: dayfile.DateOf(in.Now)}, v: versions{table: in.Prices, family: in.Family},
		projects: in.Projects, hoursOn: hours.hoursOn(), unkept: told,
	}
	listed := in.listed()
	places := d.sessionPlaces(counted, listed)
	d.each(counted, func(date string, s *foldedSession) {
		p := t.of(places[s.ID])
		p.take(date, s.DaySession, t.price, t.v)
		if date != t.hoursOn {
			return
		}
		if folded, ok := hours.held.held[s.ID]; ok {
			p.hourly(folded.hours, t.price)
		}
	})
	running := make(map[string]bool)
	for _, s := range listed.Sessions {
		if s.Running {
			t.of(places[s.Session]).run(s)
			running[s.Session] = true
		}
	}
	t.ofPeriod(running)
	t.started(d.startedBefore(first))
	if span.gone {
		t.wentOn(d.wentOnAfter(span, in.List))
	}
	return t
}

// sessionPlaces are where each session of the span s ran last, by its id,
// as sessionPlace's after has it of its days in their order, as each has
// them, then of list's listing of it, where it runs.
func (d SessionDays) sessionPlaces(s periodSpan, list SessionList) map[string]sessionPlace {
	places := make(map[string]sessionPlace)
	ranIn := func(id string, q sessionPlace) { places[id] = places[id].after(q) }
	d.each(s, func(_ string, s *foldedSession) { ranIn(s.ID, s.place()) })
	for _, s := range list.Sessions {
		if s.Running {
			ranIn(s.Session, s.place())
		}
	}
	return places
}

// wentOnAfter are the ids of the sessions that went on after the span gone
// given: those that made a request on a day after its last that d holds, to
// today, as each has them, and those list lists running now.
func (d SessionDays) wentOnAfter(s periodSpan, list SessionList) map[string]bool {
	on := make(map[string]bool)
	d.each(s.after(), func(_ string, s *foldedSession) {
		if !s.LastRequest.IsZero() {
			on[s.ID] = true
		}
	})
	for _, s := range list.Sessions {
		if s.Running {
			on[s.Session] = true
		}
	}
	return on
}

// unkept tells of the days of the span gone given whose sessions are no
// longer kept, as UnkeptDays has them, the days from its first, or the day
// the work began at began where that's later, to its last, each kept or not
// as OldestDay's kept has it of oldest, the oldest day the ledger keeps lines
// of. None of a span at now, nor of one lying before the work began, nor
// where the day it began isn't known, as no day can then be told from one
// before it.
func unkept(s periodSpan, began time.Time, oldest OldestDay) UnkeptDays {
	first, last := begunFrom(s.first, began), s.last
	if !s.gone || began.IsZero() || first.After(last) || oldest.kept(first.Format(time.DateOnly)) {
		return UnkeptDays{}
	}
	return UnkeptDays{KeptFrom: dayfile.CivilOf(oldest.Day.Start), Every: !oldest.kept(last.Format(time.DateOnly))}
}

// keptOf is the span from the first day whose sessions are kept, as unkept
// tells it, where that's later than its first: the days of it whose
// sessions are kept, none where none is; the span as it is where unkept
// tells of none, as at now, so the days dropped are those told of alone.
func (s periodSpan) keptOf(unkept UnkeptDays) periodSpan {
	if unkept.KeptFrom.After(s.first) {
		s.first = unkept.KeptFrom
	}
	return s
}

// each hands f each session of each day of the span s d holds, oldest
// first, with its day's date, from its first day through its last, or
// today, as through has it: of the days gone, as their records hold them;
// and of the day d's fold of the tail is of, today's or, where the day
// turned since the tail was last read, the day it turned from, as the fold
// holds them, what they spent by the hour too.
func (d SessionDays) each(s periodSpan, f func(date string, s *foldedSession)) {
	first, through, today := s.first.Format(time.DateOnly), s.through().Format(time.DateOnly), s.today.Format(time.DateOnly)
	for _, day := range d.days {
		if day.date >= first && day.date <= through && day.date < today && day.date != d.today.Date {
			for _, s := range day.sessions {
				f(day.date, &foldedSession{DaySession: s})
			}
		}
	}
	if d.today.Date >= first && d.today.Date <= through {
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
	return p.tallies[path].page(p.Rows[i], p.in, p.Unkept), true
}

// projectTallies are each project's tally, by its path, as the page's days
// are taken in, priced and named as price and v have them, each session's
// project read as projects reads it; hoursOn, the date of the day whose
// hours they take, as tallied has it, "" where none; and unkept, the days of
// a period gone whose sessions are no longer kept.
type projectTallies struct {
	all      map[string]*projectTally
	price    pricing
	v        versions
	projects status.Projects
	hoursOn  string
	unkept   UnkeptDays
}

// of is the tally of the project a session that ran where given is of, as
// the tallies' projects reads it: begun where none is yet.
func (t projectTallies) of(ran sessionPlace) *projectTally {
	path := t.projects.Of(ran.project, ran.dir)
	p, ok := t.all[path]
	if !ok {
		p = &projectTally{path: path, sessions: make(map[string]*sessionTally), families: familySums{}, models: modelTally{},
			accounts: make(map[string]Worth), days: dayWorths{}, hours: hourWorths{date: t.hoursOn}}
		t.all[path] = p
	}
	return p
}

// wentOn tells each session the tallies hold whose id is among those given
// as one that went on after the period, as wentOnAfter has them.
func (t projectTallies) wentOn(on map[string]bool) {
	for _, p := range t.all {
		for id, st := range p.sessions {
			st.wentOn = on[id]
		}
	}
}

// ofPeriod lets go of each session the tallies hold that isn't of the
// period, as inPeriod has it, those running given by their ids, and of each
// project left with none.
func (t projectTallies) ofPeriod(running map[string]bool) {
	for path, p := range t.all {
		maps.DeleteFunc(p.sessions, func(id string, st *sessionTally) bool { return !st.inPeriod(running[id]) })
		if len(p.sessions) == 0 {
			delete(t.all, path)
		}
	}
}

// inPeriod reports whether the session is one of the period, as Projects,
// a project's SESSIONS and the List count and row them: running now, as
// running says, or with a request in it. One that doesn't run, whose lines
// in it are all token counts, isn't.
func (st *sessionTally) inPeriod(running bool) bool {
	return running || st.requests > 0
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

// names name each project tallied, by its path, as status.ProjectNames names
// projects shown together, uncut: none of temporary folders, nor of the
// sessions with no directory.
func (t projectTallies) names() map[string]string {
	paths := slices.DeleteFunc(slices.Collect(maps.Keys(t.all)), func(p string) bool { return p == status.TemporaryFolders })
	return status.ProjectNames(paths, math.MaxInt)
}

// page is the list of the tallies, as Projects has it.
func (t projectTallies) page(in PeriodInput) ProjectsPage {
	names := t.names()
	page := ProjectsPage{Period: in.Period, All: t.totals(), Unkept: t.unkept, PricesAsOf: in.Prices.AsOf, tallies: t.all, in: in}
	listed := in.listed()
	for _, p := range t.all {
		page.Rows = append(page.Rows, p.row(names[p.path], listed, t.v, page.All))
		byModel := p.byModel(t.v)
		page.Most.Versions = max(page.Most.Versions, len(byModel))
		page.Most.Accounts = max(page.Most.Accounts, len(p.accounts))
		for _, v := range byModel {
			page.Most.VersionCells = max(page.Most.VersionCells, ansi.StringWidth(v.Version.Name))
		}
	}
	slices.SortFunc(page.Rows, func(a, b ProjectRow) int {
		return cmp.Or(cmp.Compare(b.amount(), a.amount()), strings.Compare(a.Name, b.Name), strings.Compare(a.Path, b.Path))
	})
	return page
}

// totals are every project's tallied together, each session once.
func (t projectTallies) totals() ProjectTotals {
	var all ProjectTotals
	sessions := make(map[string]bool)
	for _, p := range t.all {
		all.Live += p.live
		all.Requests += p.requests
		all.add(p.worth)
		for id := range p.sessions {
			sessions[id] = true
		}
	}
	all.Sessions = len(sessions)
	return all
}

// projectTally is a project's use over the period as it's taken in: its
// path; its sessions, by their ids; how many of them run now; its requests,
// their worth and tokens; its worth by family, by model version and by the
// account it ran on; and by day, by date, and by the local hour of the day
// the tallies take the hours of, as hoursOn has it; and when its last
// request arrived.
type projectTally struct {
	path     string
	sessions map[string]*sessionTally
	live     int
	requests int
	worth    Worth
	tokens   Tokens
	families familySums
	models   modelTally
	accounts map[string]Worth
	days     dayWorths
	hours    hourWorths
	last     time.Time
}

// sessionTally is a session of a project over the period as its days are
// taken in, oldest first: where it ran, as its last day to name a directory
// or a project named them; its requests and their worth; when its first
// request arrived, wherever it fell, as started has it, its last, and its
// last line, of those counted toward it; what
// names it over its days; its last day, by date, and the moves of each of
// its models that day; and, of a period gone, whether it went on after it,
// as wentOnAfter has it.
type sessionTally struct {
	id string
	sessionPlace
	requests                  int
	worth                     Worth
	firstRequest, lastRequest time.Time
	lastLine                  time.Time
	naming                    naming
	lastDay                   string
	moves                     []dayMove
	wentOn                    bool
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
	st.sessionPlace = st.after(s.place())
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

// hourly takes in what a session of the project's spent by the local hour of
// the day the tallies take the hours of, on every account, as hours holds
// it, priced as price prices it.
func (p *projectTally) hourly(hours accountHours, price pricing) {
	for _, on := range hours {
		for h := range on {
			p.hours.worth[h].add(price.worth(on[h]))
		}
	}
}

// run takes in s, a session of today's List the router lists running.
func (p *projectTally) run(s ListedSession) {
	p.live++
	p.session(s.Session)
}

// row is the project's row, named as given, its sessions running doing as
// list tells, as doingIn has it, its share of every project's totals given.
func (p *projectTally) row(name string, list SessionList, v versions, all ProjectTotals) ProjectRow {
	return ProjectRow{
		Path: p.path, Name: name,
		Live: p.live, Sessions: len(p.sessions), Requests: p.requests, Worth: p.worth,
		Share: shareOf(p.worth, all.amount()), Tokens: p.tokens,
		ByFamily: v.byFamily(p.families), Last: p.last, Activity: p.doingIn(list),
	}
}

// doingIn is what the project's sessions list lists running are doing, the
// busiest's, as with has it, taken in list's order: Idle where none runs.
func (p *projectTally) doingIn(list SessionList) Activity {
	var doing Activity
	for _, s := range list.Sessions {
		if _, ours := p.sessions[s.Session]; ours && s.Running {
			doing = doing.with(s.Activity)
		}
	}
	return doing
}
