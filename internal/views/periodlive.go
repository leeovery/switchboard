package views

import (
	"maps"
	"slices"
)

// Live is the page, and whether it changed, telling what the sessions list
// lists running are doing, where it tells otherwise of any than the List
// the page was built of, as doingAlike has it: each row's activity, as
// doingIn has it, and the List each project's page is built of, as Project
// builds it; nothing else built afresh, so list must list running the
// sessions the page was built of, where they were and on what. A page of a
// period gone tells nothing afresh, as it lists none of the List, as lists
// has it.
func (p ProjectsPage) Live(list SessionList) (ProjectsPage, bool) {
	if !p.in.lists() || doingAlike(p.in.List, list) {
		return p, false
	}
	p.in.List, p.Rows = list, slices.Clone(p.Rows)
	for i, r := range p.Rows {
		p.Rows[i].Activity = p.tallies[r.Path].doingIn(list)
	}
	return p, true
}

// ProjectLive is the project's page given, of a page Live told afresh as
// p: its row's activity p's row's, and its sessions running doing as p's
// List tells, as liveRows has it.
func (p ProjectsPage) ProjectLive(page ProjectPage) ProjectPage {
	if i := slices.IndexFunc(p.Rows, func(r ProjectRow) bool { return r.Path == page.Path }); i >= 0 {
		page.Activity = p.Rows[i].Activity
	}
	page.Sessions = liveRows(page.Sessions, p.in.List)
	return page
}

// Live is the log, and whether it changed, telling what the sessions list
// lists running are doing, where it tells otherwise of any than the List
// the log was built of, as doingAlike has it: their rows', as liveRows has
// them; nothing else built afresh, so list must list running the sessions
// the log was built of, where they were and on what. A log of a period gone
// tells nothing afresh, as Projects' page doesn't.
func (l SessionLog) Live(list SessionList) (SessionLog, bool) {
	if !l.in.lists() || doingAlike(l.in.List, list) {
		return l, false
	}
	l.in.List, l.Sessions = list, liveRows(l.Sessions, list)
	return l, true
}

// rowDoing is what a session running is doing, as a row of Projects or the
// List draws it: its activity, and its mark, which shows it, as broughtBy
// marks it.
type rowDoing struct {
	activity Activity
	mark     RowMark
}

// alike reports whether d and o draw alike: doing the same, since the same
// time, held by the same answer, alike as last known, last seen at work at
// the same time, and marked the same, however far an answer has streamed,
// which no such row says.
func (d rowDoing) alike(o rowDoing) bool {
	a, b := d.activity, o.activity
	return a.Doing == b.Doing && a.Since.Equal(b.Since) && a.Status == b.Status && a.Stale == b.Stale && a.LastSeen.Equal(b.LastSeen) && d.mark == o.mark
}

// liveRows are the rows given, afresh, each of a session list lists running
// doing as list tells, as rowDoing has it.
func liveRows(rows []PeriodSession, list SessionList) []PeriodSession {
	doing := runningDoing(list)
	rows = slices.Clone(rows)
	for i, r := range rows {
		if d, ok := doing[r.Session]; ok && r.Running {
			rows[i].Activity, rows[i].Mark = d.activity, d.mark
		}
	}
	return rows
}

// runningDoing are what the sessions list lists running are doing, as
// rowDoing has it, by their ids.
func runningDoing(list SessionList) map[string]rowDoing {
	doing := make(map[string]rowDoing)
	for _, s := range list.Sessions {
		if s.Running {
			doing[s.Session] = rowDoing{activity: s.Activity, mark: s.Mark}
		}
	}
	return doing
}

// doingAlike reports whether the Lists given tell alike of the sessions
// they list running, as a row of Projects or the List draws them: the same
// sessions, each doing alike, as rowDoing's alike has it.
func doingAlike(a, b SessionList) bool {
	return maps.EqualFunc(runningDoing(a), runningDoing(b), rowDoing.alike)
}
