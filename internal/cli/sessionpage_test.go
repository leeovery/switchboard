package cli_test

import (
	"bytes"
	"encoding/json"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// The sessions of sessions <id>'s tests, beside sessions' own.
const (
	// paging started yesterday on work, and came back this morning, its
	// conversation rescored onto side, its subagent on work.
	paging = "5e6f9d33-0004-4a00-8000-000000000004"
	// gone ran two days ago, and ended then.
	gone = "3c4d5e6f-0005-4a00-8000-000000000005"
)

// limitsOf are an answer's usage headers reading the session window at
// fiveHour of its use, resetting at fiveHourReset, and the week at week,
// resetting on Monday 12 October at 10:00.
func limitsOf(fiveHour string, fiveHourReset time.Time, week string) map[string]string {
	unix := func(t time.Time) string { return strconv.FormatInt(t.Unix(), 10) }
	return map[string]string{"5h-utilization": fiveHour, "5h-reset": unix(fiveHourReset), "7d-utilization": week, "7d-reset": unix(october(12, 10, 0, 0))}
}

// pagedLine is a request of paging's, its conversation's unless agent names
// the subagent that sent it, serving prompt, of model on account, routed for
// reason, arriving at at and taking ms, its answer stopping as stop says,
// calling tools, and reading the windows as limits gives them.
type pagedLine struct {
	request, prompt, agent, model, account, reason, from, stop string
	at                                                         time.Time
	ms                                                         int64
	tools                                                      []string
	limits                                                     map[string]string
}

// line is the line the ledger holds of p.
func (p pagedLine) line() ledger.Line {
	l := ledger.Line{At: p.at, Request: p.request, Kind: ledger.KindMessage, Session: paging, Dir: "~/Code/web", Model: p.model, Account: p.account,
		Reason: p.reason, From: p.from, Status: 200, Attempts: 1, TotalMS: p.ms, Usage: json.RawMessage(hourUsage)}
	l.Prompt, l.Class = p.prompt, ledger.ClassMain
	if p.agent != "" {
		l.Class, l.AgentID, l.Usage = "subagent", p.agent, json.RawMessage(checkUsage)
	}
	l.Answer, l.Limits = ledger.Answer{Stop: p.stop, Tools: p.tools}, p.limits
	return l
}

// pageLines are sessions <id>'s tests' lines, by the day each arrived on:
// sessions' own, then paging's and gone's, and another of asking's, which
// shares work's rises with paging.
func pageLines() map[string][]ledger.Line {
	days := maps.Clone(sessionLines)
	days["2026-10-05"] = []ledger.Line{{At: october(5, 10, 0, 0), Request: "g1", Kind: ledger.KindMessage, Session: gone, Dir: "~/Code/old",
		Model: "claude-opus-5-5", Account: "work", Reason: "new", Status: 200, Attempts: 1, TotalMS: 9000, Usage: json.RawMessage(hourUsage)}}
	workReset, sideReset := october(6, 20, 0, 0), october(7, 14, 12, 0)
	asked := ledger.Line{At: october(6, 17, 0, 0), Request: "o1", Kind: ledger.KindMessage, Session: asking, Model: "claude-opus-5-5", Account: "work",
		Reason: "new", Status: 200, Attempts: 1, TotalMS: 9000, Usage: json.RawMessage(hourUsage), Limits: limitsOf("0.26", workReset, "0.40")}
	days["2026-10-06"] = append([]ledger.Line{
		pagedLine{request: "p1", prompt: "first", model: "claude-opus-5-5", account: "work", reason: "new", stop: "tool_use", at: october(6, 16, 40, 0),
			ms: 60000, tools: []string{"Bash", "Bash"}, limits: limitsOf("0.20", workReset, "0.40")}.line(),
		asked,
		pagedLine{request: "p2", prompt: "first", model: "claude-opus-5-5", account: "work", reason: "sticky", stop: "end_turn", at: october(6, 18, 5, 0),
			ms: 60000, limits: limitsOf("0.30", workReset, "0.41")}.line(),
	}, days["2026-10-06"]...)
	days["2026-10-07"] = append(days["2026-10-07"],
		pagedLine{request: "p3", prompt: "second", model: "claude-opus-5-5", account: "side", reason: "rescored after 15h 7m idle", from: "work",
			stop: "tool_use", at: october(7, 9, 12, 0), ms: 60000, tools: []string{"Read"}, limits: limitsOf("0.05", sideReset, "0.78")}.line(),
		pagedLine{request: "p4", prompt: "second", agent: "a1", model: "claude-haiku-4-5", account: "work", reason: "sticky", stop: "end_turn",
			at: october(7, 9, 13, 0), ms: 30000, tools: []string{"Read", "Grep"}}.line(),
		pagedLine{request: "p5", prompt: "second", model: "claude-opus-5-5", account: "side", reason: "sticky", stop: "tool_use", at: october(7, 9, 20, 0),
			ms: 60000, tools: []string{"Edit", "Read"}, limits: limitsOf("0.07", sideReset, "0.78")}.line(),
	)
	return days
}

// pagingRouted is paging as the router lists it: its Claude Opus 5.5 rescored
// onto side, its Claude Haiku 4.5 on work.
var pagingRouted = status.Session{ID: paging, Assignments: []status.Assignment{
	{Model: "claude-opus-5-5", Family: "opus", Account: "side", Dir: "~/Code/web", Reason: "rescored after 15h 7m idle",
		AssignedAt: october(7, 9, 12, 0).UTC(), LastSeen: october(7, 13, 10, 0).UTC()},
	{Model: "claude-haiku-4-5", Family: "haiku", Account: "work", Reason: "sticky", AssignedAt: october(6, 22, 0, 0).UTC(), LastSeen: october(7, 9, 13, 0).UTC()},
}}

// pageDeps are sessionsDeps holding sessions <id>'s tests' lines, and the
// router, where routed says, listing sessions' own running and paging.
func pageDeps(t *testing.T, routed bool) cli.Deps {
	t.Helper()
	deps := sessionsDepsOf(t, pageLines())
	if routed {
		serveSessions(t, deps, slices.Concat(routedSessions, []status.Session{pagingRouted}))
	}
	return deps
}

// pageText is what sessions prints of paging's page, the router listing it.
const pageText = `session 5e6f9d33  ~/Code/web
idle 2m  ·  on side since 09:12, rescored after 15h 7m idle: side had the most room  ·  started yesterday 16:40 · resumed 09:12
opus-5-5 → side   haiku-4-5 → work

work  yesterday 16:40 – 09:13  3 requests  ≈ 4 points of its 5-hour · 1 of its week  $0.08
side  09:12 – now              2 requests  ≈ 2 points of its 5-hour · 0 of its week  $0.08

2 turns  ·  5 requests  ·  99% from cache  ·  $0.15 at API prices
#  started          took   requests  tools                      read  written  out   worth  on
2  09:12            240m…  3         Read ×3, Edit ×1, Grep ×1  200k  2.0k     1.0k  $0.08  side
1  yesterday 16:40  86m    2         Bash ×2                    200k  2.0k     1.0k  $0.08  work
`

func TestSessionsShowsTheSessionGivenAsMuchOfItsIdAsIsUnique(t *testing.T) {
	deps := pageDeps(t, true)
	for _, form := range prettyForms() {
		for _, given := range []string{"5e6f9", paging} {
			t.Run(form.name+", "+given, func(t *testing.T) {
				if got := form.run(t, deps, "sessions", given); got != (result{stdout: pageText}) {
					t.Errorf("switchboard sessions %s %s = %+v, want it to print\n%s", given, strings.Join(form.args, " "), got, pageText)
				}
			})
		}
	}
}

func TestASessionsPageJSONHoldsItsShape(t *testing.T) {
	deps := pageDeps(t, true)
	at := func(t time.Time) string { return `"` + t.UTC().Format(time.RFC3339) + `"` }
	// Paging's every request read 100,000 tokens from the cache, but its
	// subagent's, which sent 8 tokens of input: of 4 × 101,010 + 8 sent.
	fromCache := strconv.FormatFloat(400000.0/404048, 'f', -1, 64)
	want := `{"session":"` + paging + `","dir":"~/Code/web","running":true,"last_seen":` + at(october(7, 13, 10, 0)) + `,` +
		`"models":[{"model":"claude-opus-5-5","account":"side","reason":"rescored after 15h 7m idle"},{"model":"claude-haiku-4-5","account":"work","reason":"sticky"}],` +
		`"move_cost":0.80808,"started":` + at(october(6, 16, 40, 0)) + `,"resumed":` + at(october(7, 9, 12, 0)) + `,` +
		`"accounts":[{"account":"work","from":` + at(october(6, 16, 40, 0)) + `,"to":` + at(october(7, 9, 13, 0)) + `,"requests":3,"worth":0.076093,"points":{"5h":4,"7d":1}},` +
		`{"account":"side","from":` + at(october(7, 9, 12, 0)) + `,"to":` + at(october(7, 9, 20, 0)) + `,"requests":2,"worth":0.07608,"points":{"5h":2,"7d":0}}],` +
		`"moves":[{"at":` + at(october(7, 9, 12, 0)) + `,"from":"work","to":"side","reason":"rescored after 15h 7m idle","written":1000}],` +
		`"turns":[{"turn":2,"started":` + at(october(7, 9, 12, 0)) + `,"requests":3,` +
		`"tools":[{"tool":"Read","times":3},{"tool":"Edit","times":1},{"tool":"Grep","times":1}],"read":200000,"written":2000,"out":1001,"worth":0.076093,"accounts":["side"]},` +
		`{"turn":1,"started":` + at(october(6, 16, 40, 0)) + `,"ended":` + at(october(6, 18, 6, 0)) + `,"requests":2,"tools":[{"tool":"Bash","times":2}],` +
		`"read":200000,"written":2000,"out":1000,"worth":0.07608,"accounts":["work"]}],` +
		`"totals":{"turns":2,"requests":5,"from_cache":` + fromCache + `,"worth":0.152173}}`
	for _, form := range jsonForms() {
		t.Run(form.name, func(t *testing.T) {
			got := form.run(t, deps, "sessions", "5e6f9")
			var compact bytes.Buffer
			if err := json.Compact(&compact, []byte(got.stdout)); err != nil || got.code != 0 || got.stderr != "" ||
				!strings.HasPrefix(got.stdout, "{\n  \"session\"") {
				t.Fatalf("switchboard sessions 5e6f9 %s = %+v (%v), want indented JSON", strings.Join(form.args, " "), got, err)
			}
			if compact.String() != want {
				t.Errorf("switchboard sessions 5e6f9 %s printed\n%s\nwant\n%s", strings.Join(form.args, " "), compact.String(), want)
			}
		})
	}
}

func TestSessionsFindsASessionOfAnEarlierDayByItsWholeIdAlone(t *testing.T) {
	deps := pageDeps(t, true)
	const want = `session 3c4d5e6f  ~/Code/old
ended Mon 10:00  ·  ran on work  ·  started Mon 10:00 · ran 0m
kept until 9 Nov 2027

work  Mon 10:00 – Mon 10:00  1 request    $0.04

0 turns  ·  1 request  ·  99% from cache  ·  $0.04 at API prices
`
	if got := run(t, deps, "sessions", gone, "--pretty"); got != (result{stdout: want}) {
		t.Errorf("switchboard sessions %s --pretty = %+v, want it to print\n%s", gone, got, want)
	}
	const unknown = "Error: no session 3c4d in the ledger, or among the router's sessions\n"
	if got := run(t, deps, "sessions", "3c4d", "--pretty"); got != (result{stderr: unknown, code: 1}) {
		t.Errorf("switchboard sessions 3c4d --pretty = %+v, want it to fail, saying\n%s", got, unknown)
	}
}

func TestSessionsRefusesAnIdStartingSeveralSessionsListingThem(t *testing.T) {
	deps := pageDeps(t, true)
	const want = "Error: session 5e6f could be any of these, so give more of its id:\n  " + paging + "\n  " + moving + "\n"
	if got := run(t, deps, "sessions", "5e6f", "--json"); got != (result{stderr: want, code: 1}) {
		t.Errorf("switchboard sessions 5e6f --json = %+v, want it to fail, saying\n%s", got, want)
	}
}

func TestASessionsPageWithoutTheRouterIsItsLinesAloneAsEnded(t *testing.T) {
	deps := pageDeps(t, false)
	const notice = "switchboard: the router isn't running — showing the session from the ledger alone, as not running\n"
	const want = `session 5e6f9d33  ~/Code/web
ended 09:20  ·  ran on work, then side from 09:12: rescored after 15h 7m idle  ·  started yesterday 16:40 · ran 16h 40m
kept until 10 Nov 2027

work  yesterday 16:40 – 09:13  3 requests  ≈ 4 points of its 5-hour · 1 of its week  $0.08
side  09:12 – 09:20            2 requests  ≈ 2 points of its 5-hour · 0 of its week  $0.08

2 turns  ·  5 requests  ·  99% from cache  ·  $0.15 at API prices
#  started          took  requests  tools                      read  written  out   worth  on
2  09:12            9m    3         Read ×3, Edit ×1, Grep ×1  200k  2.0k     1.0k  $0.08  side
1  yesterday 16:40  86m   2         Bash ×2                    200k  2.0k     1.0k  $0.08  work
`
	if got := run(t, deps, "sessions", "5e6f9", "--pretty"); got != (result{stdout: want, stderr: notice}) {
		t.Errorf("switchboard sessions 5e6f9 --pretty = %+v, want it to print\n%s\nand say on stderr\n%s", got, want, notice)
	}

	got := run(t, deps, "sessions", "5e6f9", "--json")
	var page struct {
		Running   json.RawMessage `json:"running"`
		LastSeen  json.RawMessage `json:"last_seen"`
		Models    json.RawMessage `json:"models"`
		MoveCost  json.RawMessage `json:"move_cost"`
		Ended     string          `json:"ended"`
		KeptUntil string          `json:"kept_until"`
	}
	if err := json.Unmarshal([]byte(got.stdout), &page); err != nil || got.code != 0 || got.stderr != notice {
		t.Fatalf("switchboard sessions 5e6f9 --json = %+v (%v), want its page, and the notice", got, err)
	}
	if page.Running != nil || page.LastSeen != nil || page.Models != nil || page.MoveCost != nil || page.Ended == "" || page.KeptUntil != "2027-11-10" {
		t.Errorf("without the router, its page is %+v, want it ended, kept until 2027-11-10, with nothing the router says", page)
	}
}

func TestATurnWhoseConversationMovedReadsAsItsMove(t *testing.T) {
	lines := []ledger.Line{
		pagedLine{request: "m1", prompt: "moving", model: "claude-opus-5-5", account: "work", reason: "new", stop: "tool_use", at: october(7, 11, 0, 0), ms: 60000}.line(),
		pagedLine{request: "m2", prompt: "moving", agent: "a1", model: "claude-haiku-4-5", account: "personal", reason: "new", stop: "end_turn",
			at: october(7, 11, 1, 0), ms: 30000}.line(),
		pagedLine{request: "m3", prompt: "moving", model: "claude-opus-5-5", account: "side", reason: "moved: work hit its limit", from: "work", stop: "end_turn",
			at: october(7, 11, 2, 0), ms: 60000}.line(),
	}
	deps := sessionsDepsOf(t, map[string][]ledger.Line{"2026-10-07": lines})
	got := run(t, deps, "sessions", paging, "--pretty")
	const row = "1  11:00    3m    3                200k  2.0k     1.0k  $0.08  work → side\n"
	if got.code != 0 || !strings.Contains(got.stdout, row) {
		t.Errorf("switchboard sessions %s --pretty = %+v, want its turn's row, on its conversation's accounts alone\n%s", paging, got, row)
	}
}

func TestSessionsTakesOneSessionAtMost(t *testing.T) {
	deps := pageDeps(t, true)
	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"sessions", "5e6f9", paging}, want: "Error: give one session's id, or none to list today's sessions\n"},
		{args: []string{"sessions", ""}, want: "Error: give a session's id, or none to list today's sessions\n"},
	}
	for _, tt := range tests {
		if got := run(t, deps, tt.args...); got.code != 1 || !strings.HasPrefix(got.stderr, tt.want) {
			t.Errorf("switchboard %q = %+v, want it refused, saying %q", tt.args, got, tt.want)
		}
	}
}
