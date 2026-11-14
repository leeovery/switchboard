package viewstest

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// ProjectsWorld is the design's world of Projects, at Thursday 1 October
// 2026, 14:42:07: six projects whose sessions this week, from Monday, sum to
// Accounts' every account this week, 27 sessions, 5,410 requests and
// $635.00, each running one session now; switchboard's eight, three of them
// in its worktrees fix-login, pools and release, one of release's from a
// line before projects; and, back in the last 12 months, two old projects,
// kit, last active on 14 June, and old-site, on 2 March. Every request is
// priced at ledger.Pricing's prices on that Thursday, each session's use of
// a model its worth to the cent.
type ProjectsWorld struct {
	Now        time.Time
	WeekStarts time.Weekday
	Prices     ledger.Table
	// Lines are every request of the world, oldest first.
	Lines []ledger.Line
	// List is today's List as the router lists the six running sessions,
	// each as the List has it while it runs.
	List views.SessionList
}

// The projects of the world, by their paths, the home shown as ~.
const (
	Switchboard = "~/Code/switchboard"
	Website     = "~/Code/website"
	Infra       = "~/Code/infra"
	API         = "~/Code/api"
	Docs        = "~/Code/docs"
	Mobile      = "~/Code/mobile"
	Kit         = "~/Code/kit"
	OldSite     = "~/Code/old-site"
)

// The models the world's sessions use beside Opus and Haiku.
const (
	sonnet55 = "claude-sonnet-5-5"
	opus5    = "claude-opus-5"
	sonnet5  = "claude-sonnet-5"
	fable51  = "claude-fable-5-1"
)

// plannedSession is a session of the world: its id, the directory it ran in
// and the project its lines name, "" as a line from before projects names
// none; the account it ran on; its day, as days from Monday 28 September,
// negative before, and when it ran that day, from and to, in hours and
// minutes; what it used; and, where it runs now, what it's doing, since
// when.
type plannedSession struct {
	id, dir, project, account string
	day                       int
	from, to                  [2]int
	uses                      []plannedUse
	running                   *views.Activity
}

// plannedUse is a session's use of a model: how many requests, and their
// worth, in cents.
type plannedUse struct {
	model    string
	requests int
	cents    int64
}

// projectsNow is the world's moment.
var projectsNow = time.Date(2026, time.October, 1, 14, 42, 7, 0, time.Local)

// shortID is a whole session's id of the short one given.
func shortID(short string) string {
	return short + "0000-0000-4000-8000-000000000000"
}

// worktree is the directory of switchboard's worktree with the given name.
func worktree(name string) string {
	return Switchboard + "/.claude/worktrees/" + name
}

// runningSince is a running session's activity: doing what it's doing,
// since the time given today.
func runningSince(doing views.Doing, hour, minute, second int) *views.Activity {
	since := time.Date(2026, time.October, 1, hour, minute, second, 0, time.Local)
	return &views.Activity{Doing: doing, Since: since, LastSeen: since}
}

// plannedSessions are the world's sessions: this week's as the design's
// world has them, Saturday's 71bd of switchboard's on Monday evening, as
// the frames' weeks start on Monday; and those of kit and old-site.
func plannedSessions() []plannedSession {
	return []plannedSession{
		{id: D28C, dir: Switchboard, project: Switchboard, account: "side", day: 3, from: [2]int{12, 2}, to: [2]int{14, 41},
			uses: []plannedUse{{Opus, 400, 4620}, {Haiku, 45, 200}}, running: runningSince(views.Asking, 14, 42, 0)},
		{id: shortID("a4f2"), dir: worktree("fix-login"), project: Switchboard, account: "personal", day: 3, from: [2]int{9, 12}, to: [2]int{11, 40},
			uses: []plannedUse{{Opus, 170, 2110}, {Haiku, 18, 100}}},
		{id: shortID("9b31"), dir: Switchboard, project: Switchboard, account: "work", day: 2, from: [2]int{14, 5}, to: [2]int{18, 20},
			uses: []plannedUse{{Opus, 340, 3837}, {sonnet55, 40, 853}, {Haiku, 22, 100}}},
		{id: shortID("c7e0"), dir: worktree("pools"), project: Switchboard, account: "side", day: 2, from: [2]int{9, 30}, to: [2]int{13, 2},
			uses: []plannedUse{{Opus, 290, 3530}, {Haiku, 21, 150}}},
		{id: shortID("5d19"), dir: worktree("pools"), project: Switchboard, account: "side", day: 1, from: [2]int{15, 10}, to: [2]int{17, 44},
			uses: []plannedUse{{sonnet55, 166, 1140}}},
		{id: shortID("e2a8"), dir: Switchboard, project: Switchboard, account: "work", day: 1, from: [2]int{8, 55}, to: [2]int{12, 31},
			uses: []plannedUse{{Opus, 200, 2696}, {Haiku, 14, 64}}},
		{id: shortID("0f6c"), dir: worktree("release"), account: "work", day: 0, from: [2]int{14, 40}, to: [2]int{16, 2},
			uses: []plannedUse{{Opus, 97, 1310}}},
		{id: shortID("71bd"), dir: Switchboard, project: Switchboard, account: "personal", day: 0, from: [2]int{19, 48}, to: [2]int{22, 15},
			uses: []plannedUse{{Opus, 51, 1430}}},

		{id: shortID("9c2e"), dir: Website, project: Website, account: "personal", day: 3, from: [2]int{11, 55}, to: [2]int{14, 33},
			uses: []plannedUse{{sonnet55, 280, 3000}}, running: runningSince(views.Idle, 14, 33, 7)},
		{id: shortID("a8f1"), dir: Website, project: Website, account: "personal", day: 3, from: [2]int{8, 31}, to: [2]int{9, 28},
			uses: []plannedUse{{sonnet55, 110, 1000}}},
		{id: shortID("3e71"), dir: Website, project: Website, account: "personal", day: 2, from: [2]int{10, 0}, to: [2]int{15, 30},
			uses: []plannedUse{{Opus, 200, 3200}, {Haiku, 40, 210}}},
		{id: shortID("4c90"), dir: Website, project: Website, account: "personal", day: 1, from: [2]int{9, 0}, to: [2]int{13, 0},
			uses: []plannedUse{{sonnet55, 230, 2376}}},
		{id: shortID("6a52"), dir: Website, project: Website, account: "side", day: 0, from: [2]int{9, 30}, to: [2]int{12, 45},
			uses: []plannedUse{{Opus, 150, 1833}, {sonnet55, 72, 2152}, {Haiku, 20, 209}}},

		{id: shortID("db8a"), dir: Infra, project: Infra, account: "side", day: 3, from: [2]int{13, 22}, to: [2]int{14, 41},
			uses: []plannedUse{{Opus, 200, 2500}}, running: runningSince(views.Idle, 14, 41, 17)},
		{id: shortID("e0c4"), dir: Infra, project: Infra, account: "work", day: 3, from: [2]int{9, 12}, to: [2]int{10, 2},
			uses: []plannedUse{{Opus, 60, 800}}},
		{id: shortID("8d14"), dir: Infra, project: Infra, account: "work", day: 2, from: [2]int{9, 0}, to: [2]int{12, 0},
			uses: []plannedUse{{opus5, 150, 1962}, {sonnet5, 60, 785}}},
		{id: shortID("2f6b"), dir: Infra, project: Infra, account: "work", day: 1, from: [2]int{13, 0}, to: [2]int{17, 0},
			uses: []plannedUse{{Opus, 180, 2143}}},
		{id: shortID("b5e3"), dir: Infra, project: Infra, account: "side", day: 0, from: [2]int{10, 0}, to: [2]int{14, 0},
			uses: []plannedUse{{Opus, 196, 1620}}},

		{id: shortID("7f3a"), dir: API, project: API, account: "side", day: 3, from: [2]int{14, 12}, to: [2]int{14, 41},
			uses: []plannedUse{{Opus, 120, 2000}, {fable51, 30, 524}}, running: runningSince(views.Answering, 14, 42, 1)},
		{id: shortID("2b6d"), dir: API, project: API, account: "work", day: 3, from: [2]int{7, 58}, to: [2]int{9, 5},
			uses: []plannedUse{{sonnet55, 110, 1419}}},
		{id: shortID("c48e"), dir: API, project: API, account: "side", day: 2, from: [2]int{11, 0}, to: [2]int{16, 0},
			uses: []plannedUse{{Opus, 200, 3238}}},
		{id: shortID("91d7"), dir: API, project: API, account: "work", day: 1, from: [2]int{10, 0}, to: [2]int{14, 30},
			uses: []plannedUse{{Opus, 142, 349}, {sonnet55, 100, 1200}}},

		{id: shortID("41e0"), dir: Docs, project: Docs, account: "personal", day: 3, from: [2]int{12, 47}, to: [2]int{14, 41},
			uses: []plannedUse{{sonnet55, 250, 3000}}, running: runningSince(views.Idle, 14, 41, 33)},
		{id: shortID("3d7a"), dir: Docs, project: Docs, account: "side", day: 3, from: [2]int{9, 40}, to: [2]int{10, 15},
			uses: []plannedUse{{Opus, 100, 1591}}},
		{id: shortID("7b2c"), dir: Docs, project: Docs, account: "personal", day: 1, from: [2]int{14, 0}, to: [2]int{18, 0},
			uses: []plannedUse{{sonnet55, 61, 366}, {Opus, 200, 1163}}},

		{id: C61B, dir: Mobile, project: Mobile, account: "side", day: 3, from: [2]int{14, 21}, to: [2]int{14, 41},
			uses: []plannedUse{{Opus, 150, 2203}}, running: runningSince(views.Answering, 14, 41, 53)},
		{id: shortID("5b19"), dir: Mobile, project: Mobile, account: "personal", day: 3, from: [2]int{10, 2}, to: [2]int{11, 21},
			uses: []plannedUse{{Opus, 100, 299}, {Haiku, 25, 218}}},

		{id: shortID("4a61"), dir: Kit, project: Kit, account: "work", day: -112, from: [2]int{10, 0}, to: [2]int{16, 0},
			uses: []plannedUse{{Opus, 300, 4200}}},
		{id: shortID("6e08"), dir: Kit, project: Kit, account: "side", day: -106, from: [2]int{9, 0}, to: [2]int{12, 30},
			uses: []plannedUse{{Opus, 180, 2600}, {Haiku, 30, 120}}},
		{id: shortID("0b9d"), dir: OldSite, project: OldSite, account: "personal", day: -210, from: [2]int{13, 0}, to: [2]int{15, 0},
			uses: []plannedUse{{sonnet55, 90, 950}}},
	}
}

// NewProjectsWorld is the design's world of Projects, as ProjectsWorld has
// it.
func NewProjectsWorld() ProjectsWorld {
	w := ProjectsWorld{Now: projectsNow, WeekStarts: time.Monday, Prices: ledger.Pricing}
	w.List = views.SessionList{GeneratedAt: projectsNow.UTC(), PricesAsOf: w.Prices.AsOf, Sessions: []views.ListedSession{}}
	for _, s := range plannedSessions() {
		w.Lines = append(w.Lines, s.lines(w.Prices)...)
		if s.running != nil {
			w.List.Sessions = append(w.List.Sessions, s.listed())
		}
	}
	slices.SortStableFunc(w.Lines, func(a, b ledger.Line) int { return a.At.Compare(b.At) })
	return w
}

// lines are the session's requests, as the ledger's lines keep them, its
// uses' requests one after another evenly from when it began to when it
// ended, each use's worth all on its first, the rest spending nothing.
func (s plannedSession) lines(prices ledger.Table) []ledger.Line {
	day := time.Date(2026, time.September, 28+s.day, 0, 0, 0, 0, time.Local)
	from := time.Date(day.Year(), day.Month(), day.Day(), s.from[0], s.from[1], 0, 0, time.Local)
	to := time.Date(day.Year(), day.Month(), day.Day(), s.to[0], s.to[1], 0, 0, time.Local)
	n := 0
	for _, u := range s.uses {
		n += u.requests
	}
	step := to.Sub(from) / time.Duration(max(n-1, 1))
	var lines []ledger.Line
	for _, u := range s.uses {
		class := ledger.ClassMain
		if u.model == Haiku {
			class = "auxiliary"
		}
		for i := range u.requests {
			at := from.Add(time.Duration(len(lines)) * step)
			usage := json.RawMessage(`{"input_tokens":0,"output_tokens":0}`)
			if i == 0 {
				usage = usageOfCents(prices, u.model, u.cents)
			}
			lines = append(lines, ledger.Line{
				At: at.UTC(), Request: fmt.Sprintf("%s-%d", s.id[:4], len(lines)), Kind: ledger.KindMessage, Session: s.id, Dir: s.dir,
				Project: s.project, Model: u.model, Account: s.account, Reason: "sticky", Status: 200, Attempts: 1, Class: class, Usage: usage,
			})
		}
	}
	return lines
}

// listed is the running session as today's List has it: running on its
// account, of its main model, doing what it's doing, and its requests today.
func (s plannedSession) listed() views.ListedSession {
	listed := views.ListedSession{
		Session: s.id, Dir: s.dir, Project: s.project, Account: s.account, Label: s.account,
		Model: s.uses[0].model, Running: true, LastSeen: s.running.LastSeen.UTC(), Activity: *s.running,
		Started: time.Date(2026, time.October, 1, s.from[0], s.from[1], 0, 0, time.Local).UTC(),
		Models:  []views.SessionModel{{Model: s.uses[0].model, Account: s.account, Reason: "sticky"}},
	}
	switch s.running.Doing {
	case views.Asking:
		listed.State = status.Asking
	case views.Answering:
		listed.State = status.Answering
	}
	for _, u := range s.uses {
		listed.Requests += u.requests
	}
	return listed
}

// usageOfCents is the usage of a request of the model given worth the cents
// given, exactly, at prices on the world's Thursday: reads from the cache,
// and as many web searches, a cent each, as the reads' price leaves over.
func usageOfCents(prices ledger.Table, model string, cents int64) json.RawMessage {
	million := ledger.Line{Kind: ledger.KindMessage, Model: model, Usage: json.RawMessage(`{"cache_read_input_tokens":1000000}`)}
	w, _ := prices.SummedSpend(model, "", ledger.SpendOf(&million), dayfile.DateOf(projectsNow))
	perToken := int64(w.Cost) / 1_000_000
	const aCent = 10_000_000_000
	searches := int64(0)
	for (cents-searches)*aCent%perToken != 0 {
		searches++
	}
	reads := (cents - searches) * aCent / perToken
	return fmt.Appendf(nil, `{"cache_read_input_tokens":%d,"server_tool_use":{"web_search_requests":%d}}`, reads, searches)
}

// worldLedger is a ledger holding the world's lines, each day's filed under
// it, and no day's head: each day's sessions are read of its lines.
type worldLedger struct {
	ledger.Empty
	days map[string][]ledger.Held
}

func (l worldLedger) Day(date string) []ledger.Held {
	return l.days[date]
}

// Ledger is a ledger holding the world's lines, as the views read it.
func (w ProjectsWorld) Ledger() views.Ledger {
	l := worldLedger{days: make(map[string][]ledger.Held)}
	for _, line := range w.Lines {
		date := dayfile.DateOf(line.At)
		l.days[date] = append(l.days[date], ledger.Held{Line: line})
	}
	return l
}

// Days are the world's sessions of each day of the last 12 months, as
// ReadSessionDays reads them, and today's as the tail gives them, every one
// of today's.
func (w ProjectsWorld) Days() views.SessionDays {
	l := w.Ledger()
	return views.ReadSessionDays(l, w.Now.AddDate(-1, 0, 0), w.Now).Tailed(l.Day(dayfile.DateOf(w.Now)), true, nil, nil, w.Now)
}

// Input is what the world's page is built of over the period given.
func (w ProjectsWorld) Input(period views.Period) views.PeriodInput {
	return views.PeriodInput{
		Period: period, List: w.List, Names: status.Names{}, Prices: w.Prices, WeekStarts: w.WeekStarts, Family: ledger.FamilyOf, Now: w.Now,
	}
}
