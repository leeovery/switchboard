package capture

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// The projects the worlds' sessions run in, by their paths, as their lines
// name them: the six of this week's, each running a session now; and two of
// the year's that ran none since the summer.
const (
	switchboardProject = "~/Code/switchboard"
	websiteProject     = quietDir
	infraProject       = "~/Code/infra"
	apiProject         = "~/Code/api"
	docsProject        = "~/Code/docs"
	mobileProject      = "~/Code/mobile"
	kitProject         = "~/Code/kit"
	oldSiteProject     = "~/Code/old-site"
)

// projectWeek is a project's week, from Monday to now, as Projects lists it:
// its requests, and their worth, in cents.
type projectWeek struct {
	project  string
	requests int
	cents    int64
}

// projectWeeks are the six projects' weeks as the design's world has them,
// summing to Accounts' every account this week, 5,410 requests and $635.
// The world keeps them but docs' and mobile's, each of whose sessions this
// week ran yesterday or today, as the lines tell them whole: what the lines
// don't give of those two goes to the other four, as weekLeft shares it.
var projectWeeks = []projectWeek{
	{switchboardProject, 1874, 22140}, {websiteProject, 1102, 13980}, {infraProject, 846, 9810},
	{apiProject, 702, 8730}, {docsProject, 611, 6120}, {mobileProject, 275, 2720},
}

// projectPlace is where a session's requests run: its directory, the project
// its lines name, "" as a line from before projects names none, and the
// model they ask for.
type projectPlace struct {
	dir, project, model string
}

// placeIn is a session's place in the project given, at its root, asking
// for the model given.
func placeIn(project, model string) projectPlace {
	return projectPlace{dir: project, project: project, model: model}
}

// worktreeOf is the place of a session in switchboard's worktree with the
// given name, asking for the model given.
func worktreeOf(name, model string) projectPlace {
	return projectPlace{dir: switchboardProject + "/.claude/worktrees/" + name, project: switchboardProject, model: model}
}

// beforeProjects is the place as a line from before projects names it: its
// directory alone.
func (p projectPlace) beforeProjects() projectPlace {
	p.project = ""
	return p
}

// weekPlaces are the places of the sessions each of the three ran from
// Monday to yesterday that no lines tell of, in the order of their ids, as
// sessionThisWeek gives them: of switchboard's eight this week six, one in
// its worktree fix-login, two in pools, and one in release, its lines from
// before projects; of website's five three; of infra's five two; and of
// api's four one.
var weekPlaces = map[string][]projectPlace{
	"work": {worktreeOf("release", opus55).beforeProjects()},
	"personal": {
		worktreeOf("fix-login", opus55), placeIn(switchboardProject, opus55), placeIn(websiteProject, sonnet55),
		placeIn(websiteProject, opus55), placeIn(infraProject, opus5),
	},
	"side": {
		worktreeOf("pools", opus55), worktreeOf("pools", sonnet55), placeIn(switchboardProject, opus55),
		placeIn(websiteProject, sonnet55), placeIn(infraProject, opus55), placeIn(apiProject, opus55),
	},
}

// yearProject is a project of the year's sessions before this week, and how
// many it ran.
type yearProject struct {
	project  string
	sessions int
}

// yearProjects are the projects of the year's sessions before this week, as
// the design's last 12 months have them, less this week's: each running
// some all year but kit and old-site.
var yearProjects = []yearProject{
	{switchboardProject, 498}, {websiteProject, 299}, {infraProject, 248}, {apiProject, 215}, {docsProject, 132}, {mobileProject, 66},
}

// kit's and old-site's sessions, and the day of the year each ran its last
// on: kit's on Saturday 13 June, as the design's Sunday 14 June is a day of
// no requests, as empty has it, and old-site's on Monday 2 March.
const (
	kitSessions     = 152
	kitLast         = 257
	oldSiteSessions = 51
	oldSiteLast     = 154
)

// readsADollar are the tokens read from the cache a dollar buys at the
// frames' prices, as drawnPrices has them: $0.50 a million.
const readsADollar = 2_000_000

// projectRun is a session's requests of a day of the year, in its place, on
// the account given: how many, from when to when, and their worth, as
// tokens read from the cache.
type projectRun struct {
	session, account string
	place            projectPlace
	d                int
	from, to         time.Time
	requests         int
	reads            int
}

// lines are the run's requests as the ledger's lines keep them, evenly from
// when it began to when it ended, its worth all on its first, the rest
// spending nothing.
func (r projectRun) lines() []ledger.Line {
	step := r.to.Sub(r.from) / time.Duration(max(r.requests-1, 1))
	lines := make([]ledger.Line, r.requests)
	for i := range lines {
		usage := json.RawMessage(`{"input_tokens":0,"output_tokens":0}`)
		if i == 0 {
			usage = fmt.Appendf(nil, `{"cache_read_input_tokens":%d}`, r.reads)
		}
		lines[i] = ledger.Line{
			At: r.from.Add(time.Duration(i) * step).UTC(), Request: fmt.Sprintf("%s-%s-%d-%d", r.session[:8], r.account, r.d, i), Kind: ledger.KindMessage,
			Session: r.session, Dir: r.place.dir, Project: r.place.project, Model: r.place.model, Account: r.account,
			Status: 200, Attempts: 1, Class: ledger.ClassMain, Agent: claudeCodeAgent, Usage: usage,
		}
	}
	return lines
}

// runOn is the session's run of day d of the year, of the requests and
// reads given, on the account given, in its place: from a time between
// 09:00 and 15:59 drawn from its id and the day, the same every run, a
// request each 40 seconds, ending by 23:30.
func (y year) runOn(d int, session, account string, place projectPlace, requests, reads int) projectRun {
	at := drawn(fmt.Appendf(nil, "%s/%s/run", dateOf(d), session))
	from := dayAt(y.now, d, 9+int(at%7), int(at/7%60), 0)
	to := score.Earlier(from.Add(time.Duration(requests)*40*time.Second), dayAt(y.now, d, 23, 30, 0))
	return projectRun{session: session, account: account, place: place, d: d, from: from, to: to, requests: requests, reads: reads}
}

// projectRuns are the runs of the sessions of the world's accounts among the
// three that no lines tell of, by the date of their day: the year's, as
// yearRuns has them, and this week's, as weekRuns has them, of the lines
// given.
func (y year) projectRuns(accounts config.Accounts, lines []filed) map[string][]projectRun {
	runs := make(map[string][]projectRun)
	for _, r := range slices.Concat(y.yearRuns(), y.weekRuns(lines)) {
		if slices.ContainsFunc(accounts, func(a config.Account) bool { return a.ID == r.account }) {
			runs[dateOf(r.d)] = append(runs[dateOf(r.d)], r)
		}
	}
	return runs
}

// yearRuns are the runs of the three's sessions of the last 12 months before
// this week: each of an account's day sharing its requests and worth as
// runWeights weighs them, a request at least, in the project yearPlaced places it, asking for one of the day's versions,
// drawn as their shares are.
func (y year) yearRuns() []projectRun {
	projects := y.yearPlaced()
	var runs []projectRun
	for d := lastTwelveMonths; d < thisWeek; d++ {
		for _, id := range three {
			use := y.days[id][d]
			if use.requests == 0 {
				continue
			}
			weights := runWeights(d, use.sessions)
			requests := atLeastOne(use.requests, weights)
			reads := apportion(int(math.Round(use.worth*readsADollar)), weights)
			for i, s := range use.sessions {
				runs = append(runs, y.runOn(d, s, id, placeIn(projects[s], versionDrawn(use.versions, s, d)), requests[i], reads[i]))
			}
		}
	}
	return runs
}

// runWeights weigh the runs of the sessions with the ids given on day d of
// the year: each by a weight drawn from its id and the day, the same every
// run, from half to one and a half.
func runWeights(d int, sessions []string) []float64 {
	weights := make([]float64, len(sessions))
	for i, s := range sessions {
		weights[i] = 0.5 + float64(drawn(fmt.Appendf(nil, "%s/%s/share", dateOf(d), s)))/math.MaxUint64
	}
	return weights
}

// atLeastOne shares n among parts as apportion does, but one to each first,
// where n gives each one.
func atLeastOne(n int, weights []float64) []int {
	if n < len(weights) {
		return apportion(n, weights)
	}
	shares := apportion(n-len(weights), weights)
	for i := range shares {
		shares[i]++
	}
	return shares
}

// versionDrawn is one of the versions given, drawn from the session's id and
// day d of the year, the same every run, as their shares are.
func versionDrawn(versions map[string]float64, session string, d int) string {
	at := float64(drawn(fmt.Appendf(nil, "%s/%s/version", dateOf(d), session))) / math.MaxUint64
	held := ids(versions)
	for _, v := range held {
		if at -= versions[v]; at < 0 {
			return v
		}
	}
	return held[len(held)-1]
}

// yearPlaced is the project of each of the three's sessions of the last 12
// months before this week, by its id: in the order each first ran, old-site's
// spread evenly to its last on oldSiteLast, kit's to its last on kitLast, and
// the rest's among the others, each as far along as its share of them.
func (y year) yearPlaced() map[string]string {
	var order []string
	first := make(map[string]int)
	for d := lastTwelveMonths; d < thisWeek; d++ {
		for _, id := range three {
			for _, s := range y.days[id][d].sessions {
				if _, ok := first[s]; !ok {
					first[s] = d
					order = append(order, s)
				}
			}
		}
	}
	lastBy := func(d int) int {
		return slices.IndexFunc(order, func(s string) bool { return first[s] > d }) - 1
	}
	placed := make([]string, len(order))
	spreadTo(placed, oldSiteProject, oldSiteSessions, lastBy(oldSiteLast))
	spreadTo(placed, kitProject, kitSessions, lastBy(kitLast))
	shared(placed, yearProjects)
	projects := make(map[string]string, len(order))
	for i, s := range order {
		projects[s] = placed[i]
	}
	return projects
}

// spreadTo places n sessions in the project given, evenly among those not
// yet placed from the first to the one at last, which it places there.
func spreadTo(placed []string, project string, n, last int) {
	var free []int
	for i := range placed[:last+1] {
		if placed[i] == "" {
			free = append(free, i)
		}
	}
	for k := range n {
		placed[free[int(math.Round(float64(k*(len(free)-1))/float64(n-1)))]] = project
	}
}

// shared places the sessions not yet placed among the projects given, each
// next going to the one furthest behind its share of them.
func shared(placed []string, projects []yearProject) {
	total := 0
	for _, p := range projects {
		total += p.sessions
	}
	given := make([]int, len(projects))
	filled := 0
	for i := range placed {
		if placed[i] != "" {
			continue
		}
		filled++
		behind := func(j int) float64 { return float64(projects[j].sessions*filled)/float64(total) - float64(given[j]) }
		j := slices.MaxFunc(indexes(len(projects)), func(a, b int) int { return cmp.Compare(behind(a), behind(b)) })
		placed[i] = projects[j].project
		given[j]++
	}
}

// weekRuns are the runs of this week's sessions before today that no lines
// tell of, as weekPlaces places them, each running Monday to yesterday,
// each project's week, with the lines given, as weekLeft leaves it.
// Yesterday's and Monday's requests the lines leave of the three's are
// shared among the projects with such sessions, as what's left them is;
// Tuesday's are what's left of each; and each project's of a day among its
// sessions, as runWeights weighs them.
func (y year) weekRuns(lines []filed) []projectRun {
	runners := y.weekRunners()
	left, ofLines := weekLeft(lines, y.now, runners)
	var of []int
	for p := range runners {
		if len(runners[p]) > 0 {
			of = append(of, p)
		}
	}
	var runs []projectRun
	for _, d := range []int{today - 1, thisWeek} {
		requests := apportion(y.dayRequests(d)-ofLines[d].requests, weightsOf(of, func(p int) float64 { return float64(left[p].requests) }))
		reads := apportion(y.dayReads(d)-ofLines[d].reads, weightsOf(of, func(p int) float64 { return float64(left[p].reads) }))
		for i, p := range of {
			runs = append(runs, y.runsOf(runners[p], d, requests[i], reads[i])...)
			left[p] = projectUse{requests: left[p].requests - requests[i], reads: left[p].reads - reads[i]}
		}
	}
	for _, p := range of {
		runs = append(runs, y.runsOf(runners[p], thisWeek+1, left[p].requests, left[p].reads)...)
	}
	return runs
}

// weekRunner is a session running this week that no lines tell of, on the
// account given, in its place.
type weekRunner struct {
	id, account string
	place       projectPlace
}

// weekRunners are the sessions of each of projectWeeks's projects running
// this week that no lines tell of, in its order: the three's that
// sessionThisWeek gives Monday.
func (y year) weekRunners() [][]weekRunner {
	runners := make([][]weekRunner, len(projectWeeks))
	for _, id := range three {
		for i, s := range y.days[id][thisWeek].sessions {
			p := projectWeekOf(weekPlaces[id][i].project, weekPlaces[id][i].dir)
			runners[p] = append(runners[p], weekRunner{id: s, account: id, place: weekPlaces[id][i]})
		}
	}
	return runners
}

// projectWeekOf is the place in projectWeeks of the project a session in
// dir of the project given is of, as status.ProjectOf reads it.
func projectWeekOf(project, dir string) int {
	p := slices.IndexFunc(projectWeeks, func(w projectWeek) bool { return w.project == status.ProjectOf(project, dir) })
	if p < 0 {
		panic(fmt.Sprintf("capture: no week of the project of %q in %q", project, dir))
	}
	return p
}

// runsOf are the runs of the project's sessions given on day d of the year,
// sharing the requests and reads given.
func (y year) runsOf(runners []weekRunner, d, requests, reads int) []projectRun {
	ids := make([]string, len(runners))
	for i, r := range runners {
		ids[i] = r.id
	}
	weights := runWeights(d, ids)
	counts, worths := atLeastOne(requests, weights), apportion(reads, weights)
	runs := make([]projectRun, len(runners))
	for i, r := range runners {
		runs[i] = y.runOn(d, r.id, r.account, r.place, counts[i], worths[i])
	}
	return runs
}

// projectUse is requests, and their worth, as tokens read from the cache.
type projectUse struct {
	requests, reads int
}

// weekLeft are what projectWeeks leaves of each project's week, in its
// order, once the lines given that arrived this week by now are taken from
// it; and what the lines hold of each of this week's days of the year. What
// it leaves of a project with no session among runners, as docs' and
// mobile's, all of whose sessions this week the lines tell of, goes to
// those with one, shared as what it leaves them is, so the week still sums
// to Accounts'.
func weekLeft(lines []filed, now time.Time, runners [][]weekRunner) ([]projectUse, map[int]projectUse) {
	left := make([]projectUse, len(projectWeeks))
	for p, w := range projectWeeks {
		left[p] = projectUse{requests: w.requests, reads: int(w.cents) * readsADollar / 100}
	}
	ofLines := make(map[int]projectUse)
	for _, f := range lines {
		d := dayOf(f.At.In(now.Location()))
		if d < thisWeek || f.At.After(now) || !f.Counted() || f.Session == "" {
			continue
		}
		p := projectWeekOf(f.Project, f.Dir)
		worth, _ := drawnPrices.SummedSpend(f.Model, f.Shape.InferenceGeo, ledger.SpendOf(&f.Line), f.date)
		reads := int(worth.Cost) / (1_000_000_000_000 / readsADollar)
		left[p] = projectUse{requests: left[p].requests - 1, reads: left[p].reads - reads}
		ofLines[d] = projectUse{requests: ofLines[d].requests + 1, reads: ofLines[d].reads + reads}
	}
	var unrun projectUse
	var run []int
	for p := range left {
		if len(runners[p]) == 0 {
			unrun = projectUse{requests: unrun.requests + left[p].requests, reads: unrun.reads + left[p].reads}
			left[p] = projectUse{}
		} else {
			run = append(run, p)
		}
	}
	requests := apportion(unrun.requests, weightsOf(run, func(p int) float64 { return float64(left[p].requests) }))
	reads := apportion(unrun.reads, weightsOf(run, func(p int) float64 { return float64(left[p].reads) }))
	for i, p := range run {
		left[p] = projectUse{requests: left[p].requests + requests[i], reads: left[p].reads + reads[i]}
	}
	return left, ofLines
}

// dayRequests are the three's requests on day d of the year.
func (y year) dayRequests(d int) int {
	n := 0
	for _, id := range three {
		n += y.days[id][d].requests
	}
	return n
}

// dayReads are the three's worth on day d of the year, as tokens read from
// the cache.
func (y year) dayReads(d int) int {
	worth := 0.0
	for _, id := range three {
		worth += y.days[id][d].worth
	}
	return int(math.Round(worth * readsADollar))
}

// runsOn are the lines of the records' runs of the days with the given
// dates that arrived by now, as the ledger files them, each day's made once
// and kept: each after the records' own lines, in its day's order, so a
// reader orders them as it orders its own.
func (r *records) runsOn(dates []string, now time.Time) []filed {
	var lines []filed
	for _, date := range dates {
		for _, f := range r.runLines(date) {
			if !f.At.After(now) {
				lines = append(lines, f)
			}
		}
	}
	return lines
}

// runLines are the lines of the records' runs of the day with the given
// date, as runsOn has them, made once.
func (r *records) runLines(date string) []filed {
	runs := r.runs[date]
	if len(runs) == 0 {
		return nil
	}
	r.made.mu.Lock()
	defer r.made.mu.Unlock()
	if lines, ok := r.made.lines[date]; ok {
		return lines
	}
	var day []ledger.Line
	for _, run := range runs {
		day = append(day, run.lines()...)
	}
	slices.SortStableFunc(day, func(a, b ledger.Line) int { return a.At.Compare(b.At) })
	first := len(r.lines) + runs[0].d*maxRunLinesADay
	lines := make([]filed, len(day))
	for i, l := range day {
		lines[i] = filed{Line: l, date: date, place: first + i}
	}
	if r.made.lines == nil {
		r.made.lines = make(map[string][]filed)
	}
	r.made.lines[date] = lines
	return lines
}

// maxRunLinesADay is more lines than the runs of any day of the year make,
// so each day's are placed after the day's before.
const maxRunLinesADay = 1_000_000

// firstRun is the date of the first day the records' runs ran on, none
// where they ran on none.
func (r *records) firstRun() []string {
	if len(r.runs) == 0 {
		return nil
	}
	return []string{slices.Min(slices.Collect(maps.Keys(r.runs)))}
}

// datesOfRuns are the dates of the days each session of the runs given ran
// on, by its id, oldest first.
func datesOfRuns(runs map[string][]projectRun) map[string][]string {
	dates := make(map[string][]string)
	for date, day := range runs {
		for _, run := range day {
			if !slices.Contains(dates[run.session], date) {
				dates[run.session] = append(dates[run.session], date)
			}
		}
	}
	for _, d := range dates {
		slices.Sort(d)
	}
	return dates
}

// side is the side of the day with the given date, as the views' Digests
// build it of its lines, as lines gives them, the day's runs' among them,
// as a finished day's is built of its lines alone, made once and kept;
// none of a day of no runs, whose lines are read in its place alike.
func (r *records) side(date string, lines func() []ledger.Held) (json.RawMessage, bool) {
	if len(r.runs[date]) == 0 {
		return nil, false
	}
	r.made.mu.Lock()
	side, ok := r.made.sides[date]
	r.made.mu.Unlock()
	if ok {
		return side, true
	}
	build := views.Digests{}.Day()
	for _, h := range lines() {
		build.Take(h.Line)
	}
	built, err := build.Built()
	if err != nil {
		panic(fmt.Sprintf("capture: the side of %s: %v", date, err))
	}
	r.made.mu.Lock()
	defer r.made.mu.Unlock()
	if r.made.sides == nil {
		r.made.sides = make(map[string]json.RawMessage)
	}
	r.made.sides[date] = built.Side
	return built.Side, true
}
