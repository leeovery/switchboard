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
// shows it over the period: its row, which holds its share of every
// project's worth and its tokens too; its worth in buckets, WORTH BY; each
// model version's share of its worth, BY MODEL; each account's, ON
// ACCOUNTS; its sessions, SESSIONS; and the days of a period gone whose
// sessions are no longer kept, as Projects has them, which SESSIONS says.
type ProjectPage struct {
	ProjectRow
	// WorthBy is its worth in each bucket the period charts it in, as
	// worthCharts has them, for the drawing to take the finest that fits.
	WorthBy WorthCharts
	// ByModel are each model version's share of its worth, as Accounts'
	// BY MODEL reckons a row's, the greatest first, those whose share
	// rounds to none, or isn't known, left out.
	ByModel []VersionShare
	// OnAccounts are each account its requests went to, the one it was
	// worth most on first.
	OnAccounts []AccountShare
	// Sessions are its sessions over the period, in the List's order: those
	// running first, or of a period gone those that went on after it, the
	// oldest started first, then those ended, the latest ended first.
	Sessions []PeriodSession
	Unkept   UnkeptDays
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

// PeriodSession is a session of the period as the List and a project's
// SESSIONS row it, in the List's form, so it's drawn in the List's columns:
// its requests and their worth over the period. EndedOn is the date of the
// local day it ended on, as its rule groups it, by its last request; ""
// while it runs, or where it went on. WentOn is set, in a period gone, where
// it had requests in the period and went on after its last day, as
// wentOnAfter has it: it sits first, where one running would at now, its
// state that it went on in place of its end, as it didn't end in the period,
// and all it's listed by, its requests and worth among them, of the
// period's days alone, as listedEnded lists one. ProjectName names its
// project, on Sessions' List alone, as Projects names the project's row: ""
// on a project's SESSIONS, which the project heads, and of a session with no
// directory or of temporary folders. Twinned is set where another row of its
// list shows the same project and name, so ShownName gives its id too; and
// IDCells, where it shows its id beside one that would show alike, is how
// much of its id tells it apart, 0 for status.IDShown: each as toldApart has
// them.
type PeriodSession struct {
	ListedSession
	EndedOn     string
	WentOn      bool
	ProjectName string
	Twinned     bool
	IDCells     int
}

// page is the project's page, its row given, the page built of in, the days
// whose sessions are no longer kept given.
func (p *projectTally) page(row ProjectRow, in PeriodInput, unkept UnkeptDays) ProjectPage {
	v := versions{table: in.Prices, family: in.Family}
	page := ProjectPage{ProjectRow: row, ByModel: p.byModel(v), OnAccounts: p.onAccounts(in.Names), Unkept: unkept}
	page.WorthBy = worthCharts(in.Period, in.Now, in.WeekStarts, in.Began, p.days, &p.hours)
	page.Sessions = periodRows(p.sessions, in, nil)
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

// periodRows are the sessions given as the List and SESSIONS row them, in
// the order inPeriodOrder has: each of in's List, as listed has it, as the
// List has it, its requests and worth those of the period, started at the
// earlier of the List's start and its first request, as the days and the
// walk back before them found it; each other as listedEnded has it, of its
// days in the period alone, as every session of a period gone is, those
// that went on after it told so; each named by its project as projects
// names it, by its id, none for nil; and those that show alike told apart,
// as toldApart has it. It's the one way a session of the period is rowed, a
// project's or the List's.
func periodRows(sessions map[string]*sessionTally, in PeriodInput, projects map[string]string) []PeriodSession {
	list := in.listed()
	listed := make(map[string]ListedSession, len(list.Sessions))
	for _, s := range list.Sessions {
		listed[s.Session] = s
	}
	price := pricing{prices: in.Prices, date: dayfile.DateOf(in.Now)}
	rows := make([]PeriodSession, 0, len(sessions))
	for _, st := range sessions {
		s, ok := listed[st.id]
		if ok {
			s.Requests, s.Worth = st.requests, st.worth
			s.Started = earliestKnown(s.Started, st.firstRequest)
		} else {
			s = st.listedEnded(price, in.Projects)
		}
		row := PeriodSession{ListedSession: s.named(in.Names), WentOn: st.wentOn, ProjectName: projects[st.id]}
		if !row.leads() {
			row.EndedOn = dayfile.DateOf(cmp.Or(s.Ended, in.Now))
		}
		rows = append(rows, row)
	}
	slices.SortFunc(rows, inPeriodOrder)
	toldApart(rows)
	return rows
}

// leads reports whether the session sits first among the period's, as
// inPeriodOrder has it: running, or in a period gone, gone on after it.
func (s PeriodSession) leads() bool {
	return s.Running || s.WentOn
}

// inPeriodOrder orders the sessions of a period: those that lead first, as
// leads has them, as byStart orders them; then the rest, by the day each
// ended, the latest first, as its
// last request's day has it, as of one the stream told of before the router
// lists it, each day's as the List orders them.
func inPeriodOrder(a, b PeriodSession) int {
	switch {
	case a.leads() != b.leads():
		if a.leads() {
			return -1
		}
		return 1
	case a.leads():
		return byStart(a.ListedSession, b.ListedSession)
	}
	return cmp.Or(strings.Compare(b.EndedOn, a.EndedOn), inListOrder(a.ListedSession, b.ListedSession))
}

// earliestKnown is the earlier of a and b, either where the other is zero,
// as not known.
func earliestKnown(a, b time.Time) time.Time {
	switch {
	case a.IsZero():
		return b
	case b.IsZero():
		return a
	}
	return score.Earlier(a, b)
}

// listedEnded is the session as the List lists one that ended, of the days
// it ran on in the period, as their records tell it: in the directory, and
// of the project, it last named, as projects reads them together, and
// named by the terminal session it last named;
// its requests and their worth; started at its first request and ended at
// its last, or of none, its last line; named as names has it of what names
// it over its days, as no router lists it; and brought to its account, as
// the List brings one, by its model's latest move, on its last day, where
// that was onto it, as movedOnto has it, its cost priced as price prices
// it, marked ▸ where a move brought it there.
func (st *sessionTally) listedEnded(price pricing, projects status.Projects) ListedSession {
	ended := cmp.Or(st.lastRequest, st.lastLine)
	s := ListedSession{
		Session: st.id, Dir: st.dir, Project: projects.Of(st.project, st.dir), Name: st.name, Requests: st.requests, Worth: st.worth,
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
