package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// The runs of the router in the events' tests: one started on the 6th, and
// one after a restart on the 7th, whose ids start again from 1.
var (
	runOne = october(6, 8, 0, 0)
	runTwo = october(7, 12, 20, 0)
)

// filed is a version of an event, as the router filed it under the local day
// with the given date.
type filed struct {
	date string
	line events.Line
}

// eventLine is the version of the event with the given id, of the run given,
// as e has it.
func eventLine(run time.Time, id int, e status.Event) events.Line {
	e.ID = id
	return events.Line{Event: e, Run: run}
}

// The events of the events' tests, as the router filed them: personal's
// limit, filed again past midnight as it counts the session it moved; and
// today's, the last of them told by the router started again.
var (
	limitTold  = eventLine(runOne, 1, status.Event{At: october(6, 22, 0, 0), Kind: status.EventLimit, Account: "personal", Windows: []string{"5h"}, Until: october(6, 23, 58, 0)})
	limitMoved = eventLine(runOne, 1, status.Event{At: october(6, 22, 0, 0), Kind: status.EventLimit, Account: "personal", Windows: []string{"5h"}, Until: october(6, 23, 58, 0),
		Count: 1, To: "side"})
	sessionMoved = eventLine(runOne, 2, status.Event{At: october(6, 22, 5, 0), Kind: status.EventMoved, Session: sessionA, Model: "claude-opus-5-5",
		From: "personal", To: "side", Reason: "moved: personal hit its limit", Limit: 1, ForcedBy: 1})
	sessionStarted = eventLine(runOne, 3, status.Event{At: october(7, 9, 0, 0), Kind: status.EventStarted, Session: sessionC, Model: "claude-haiku-4-5",
		Account: "work", Reason: status.ReasonNew})
	workPressed   = eventLine(runOne, 4, status.Event{At: october(7, 10, 0, 0), Kind: status.EventPressure, Account: "work", Windows: []string{"5h"}, Until: october(7, 12, 30, 0)})
	pinned        = eventLine(runOne, 5, status.Event{At: october(7, 11, 0, 0), Kind: status.EventPin, Account: "work", Accounts: []string{"work", "side"}, By: "cli"})
	restartDue    = eventLine(runOne, 6, status.Event{At: october(7, 12, 0, 0), Kind: status.EventRestart, Reason: status.RestartForConfig})
	routerUnwell  = eventLine(runTwo, 1, status.Event{At: october(7, 12, 30, 0), Kind: status.EventHealth, Reason: "most requests failing"})
	fixtureEvents = []filed{
		{date: "2026-10-06", line: limitTold},
		{date: "2026-10-06", line: sessionMoved},
		{date: "2026-10-07", line: limitMoved},
		{date: "2026-10-07", line: sessionStarted},
		{date: "2026-10-07", line: workPressed},
		{date: "2026-10-07", line: pinned},
		{date: "2026-10-07", line: restartDue},
		{date: "2026-10-07", line: routerUnwell},
	}
)

// eventsDeps are deps whose state directory holds the events' tests' files,
// a line among today's that isn't an event, by a clock stopped at ledgerNow.
func eventsDeps(t *testing.T) cli.Deps {
	t.Helper()
	deps := testDeps(nil, t.TempDir())
	deps.Now = func() time.Time { return ledgerNow }
	for _, f := range fixtureEvents {
		fileEvent(t, deps, f.date, f.line)
	}
	appendStateFile(t, ledger.Dir(stateDir(t, deps)), "events-2026-10-07.jsonl", []byte("not an event\n"))
	return deps
}

// fileEvent appends line to the router's events' file of the local day with
// the given date, as the router files it, for commands run with deps: never
// rewriting the file, as a follower reads on from where it last ended.
func fileEvent(t *testing.T, deps cli.Deps, date string, line events.Line) {
	t.Helper()
	fileLines(t, deps, date, eventJSON(t, line))
}

// fileLines appends lines, each with its line ending, to the router's events'
// file of the local day with the given date, as fileEvent does.
func fileLines(t *testing.T, deps cli.Deps, date, lines string) {
	t.Helper()
	dir := ledger.Dir(stateDir(t, deps))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	appendTo(t, filepath.Join(dir, "events-"+date+".jsonl"), lines)
}

// withALaterField is the JSON of lines, each a line, with a field no release
// before a later one knows added to each.
func withALaterField(lines string) string {
	return strings.ReplaceAll("\n"+lines, "\n{", `
{"a_later_field":{"kept":true},`)[1:]
}

// eventJSON is line's JSON, as the router files it, its times in UTC, and a
// line ending.
func eventJSON(t *testing.T, lines ...events.Line) string {
	t.Helper()
	var b strings.Builder
	for _, line := range lines {
		line.At, line.Until, line.Run = line.At.UTC(), line.Until.UTC(), line.Run.UTC()
		data, err := json.Marshal(line)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(append(data, '\n'))
	}
	return b.String()
}

// todaysEvents is what events prints of today's events.
const todaysEvents = `Wed 7 Oct 2026, so far
  09:00:00  18bb2c41  started   work
  10:00:00            pressure  work        its 5-hour to run out at 12:30
  11:00:00            pin       work, side  new sessions go there, set from the command line
  12:00:00            restart               due: the config changed
  12:30:00            health                unhealthy: most requests failing
`

// eventsFromThe6th is what events prints of the 6th's events and today's.
const eventsFromThe6th = `Tue 6 Oct 2026
  22:00:00            limit  personal         its 5-hour window, till 23:58: its session moves to side
  22:05:00  5b0e7c1a  moved  personal → side  personal reached its limit

` + todaysEvents

func TestEventsPrintsEachEventOnceAsItLastStoodOldestFirst(t *testing.T) {
	deps := eventsDeps(t)
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "today's, unless told otherwise", want: todaysEvents},
		{name: "from the start of a day, under each day's", args: []string{"--since", "2026-10-06"}, want: eventsFromThe6th},
		{name: "from a time today", args: []string{"--since", "12:00"}, want: `Wed 7 Oct 2026, so far
  12:00:00    restart    due: the config changed
  12:30:00    health     unhealthy: most requests failing
`},
		{name: "from hours ago", args: []string{"--since", "3h"}, want: `Wed 7 Oct 2026, so far
  11:00:00    pin      work, side  new sessions go there, set from the command line
  12:00:00    restart              due: the config changed
  12:30:00    health               unhealthy: most requests failing
`},
		{name: "from days ago", args: []string{"--since", "1d"}, want: eventsFromThe6th},
		{name: "none", args: []string{"--since", "12:45"}, want: "no events since Wed 7 Oct 2026 12:45\n"},
	}
	for _, tt := range tests {
		for _, form := range prettyForms() {
			t.Run(tt.name+", "+form.name, func(t *testing.T) {
				got := form.run(t, deps, append([]string{"events"}, tt.args...)...)
				if got.stdout != tt.want || got.code != 0 {
					t.Errorf("switchboard events %s %s printed\n%s(%d, %s)\nwant\n%s",
						strings.Join(tt.args, " "), strings.Join(form.args, " "), got.stdout, got.code, got.stderr, tt.want)
				}
			})
		}
	}
}

func TestEventsJSONPrintsEachEventAsTheRouterFilesIt(t *testing.T) {
	deps := eventsDeps(t)
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "today's, oldest first", want: eventJSON(t, sessionStarted, workPressed, pinned, restartDue, routerUnwell)},
		{name: "from a day, each as it last stood", args: []string{"--since", "2026-10-06"},
			want: eventJSON(t, limitMoved, sessionMoved, sessionStarted, workPressed, pinned, restartDue, routerUnwell)},
		{name: "none", args: []string{"--since", "12:45"}},
	}
	for _, tt := range tests {
		for _, form := range jsonForms() {
			t.Run(tt.name+", "+form.name, func(t *testing.T) {
				got := form.run(t, deps, append([]string{"events"}, tt.args...)...)
				if got.stdout != tt.want || got.code != 0 {
					t.Errorf("switchboard events %s %s printed\n%s(%d, %s)\nwant\n%s",
						strings.Join(tt.args, " "), strings.Join(form.args, " "), got.stdout, got.code, got.stderr, tt.want)
				}
			})
		}
	}
}

func TestEventsWithoutAnyFilesSaysThereAreNone(t *testing.T) {
	deps := testDeps(nil, t.TempDir())
	deps.Now = func() time.Time { return ledgerNow }

	got := onATerminal.run(t, deps, "events")
	if want := (result{stdout: "no events since Wed 7 Oct 2026 00:00\n"}); got != want {
		t.Errorf("switchboard events with no events filed = %+v, want %+v", got, want)
	}
}

// follow runs switchboard with args, which follow the events' files, with
// deps, looking for lines every few milliseconds, its output a terminal
// where form says, and returns what it prints, and what interrupts it and
// returns its exit status and what it printed on stderr.
func follow(t *testing.T, deps cli.Deps, form printForm, args ...string) (*syncBuffer, func() (int, string)) {
	t.Helper()
	deps.FollowEvery = 5 * time.Millisecond
	form.on(&deps)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	root := cli.NewRootCommand(deps)
	root.SetContext(ctx)
	root.SetArgs(append(args, form.args...))
	stdout := &syncBuffer{}
	var stderr bytes.Buffer
	root.SetOut(stdout)
	root.SetErr(&stderr)
	code := make(chan int, 1)
	go func() { code <- cli.Execute(root) }()
	return stdout, func() (int, string) {
		cancel()
		return <-code, stderr.String()
	}
}

func TestEventsJSONKeepsTheFieldsALaterReleaseAdded(t *testing.T) {
	deps := testDeps(nil, t.TempDir())
	deps.Now = func() time.Time { return ledgerNow }
	listed := withALaterField(eventJSON(t, sessionStarted, workPressed))
	fileLines(t, deps, "2026-10-07", listed)
	stdout, interrupt := follow(t, deps, offATerminal, "events", "-f")

	stdout.waitFor(t, listed)
	followed := withALaterField(eventJSON(t, pinned))
	fileLines(t, deps, "2026-10-07", followed)
	stdout.waitFor(t, listed+followed)

	if code, stderr := interrupt(); code != 0 || stderr != "" {
		t.Errorf("switchboard events -f exited %d, printing %q on stderr, once interrupted; want 0 and nothing", code, stderr)
	}
}

func TestEventsFollowPrintsEachLineAsItsFiled(t *testing.T) {
	deps := eventsDeps(t)
	stdout, interrupt := follow(t, deps, offATerminal, "events", "--since", "12:00", "-f")

	listed := eventJSON(t, restartDue, routerUnwell)
	stdout.waitFor(t, listed)
	primed := eventLine(runTwo, 2, status.Event{At: october(7, 13, 10, 0), Kind: status.EventPrimed, Account: "side", Windows: []string{"5h"}, Until: october(7, 18, 10, 0)})
	fileEvent(t, deps, "2026-10-07", primed)
	stdout.waitFor(t, listed+eventJSON(t, primed))
	pressedAgain := workPressed
	pressedAgain.Until = october(7, 12, 45, 0)
	fileEvent(t, deps, "2026-10-07", pressedAgain)
	stdout.waitFor(t, listed+eventJSON(t, primed, pressedAgain))

	if code, stderr := interrupt(); code != 0 || stderr != "" {
		t.Errorf("switchboard events -f exited %d, printing %q on stderr, once interrupted; want 0 and nothing", code, stderr)
	}
}

func TestEventsFollowPrintsAChangedEventAgainWholeAlignedToTheListing(t *testing.T) {
	deps := eventsDeps(t)
	stdout, interrupt := follow(t, deps, onATerminal, "events", "-f")

	stdout.waitFor(t, todaysEvents)
	limitLifted := limitMoved
	limitLifted.Count, limitLifted.To = 2, ""
	fileEvent(t, deps, "2026-10-07", limitLifted)
	followed := todaysEvents + `
Tue 6 Oct 2026
  22:00:00            limit     personal    its 5-hour window, till 23:58: its 2 sessions move to other accounts
`
	stdout.waitFor(t, followed)
	fileEvent(t, deps, "2026-10-07", eventLine(runTwo, 2, status.Event{At: october(7, 13, 10, 0), Kind: status.EventMoved, Session: sessionB,
		From: "work", To: "personal", Reason: status.ReasonMovedByPin}))
	stdout.waitFor(t, followed+`
Wed 7 Oct 2026, so far
  13:10:00  5b0e9d33  moved     work → personal  pinned to personal
`)

	if code, stderr := interrupt(); code != 0 || stderr != "" {
		t.Errorf("switchboard events -f exited %d, printing %q on stderr, once interrupted; want 0 and nothing", code, stderr)
	}
}

func TestEventsFollowPrintsNothingAgainAsTheDayIsCompressed(t *testing.T) {
	deps := eventsDeps(t)
	stdout, interrupt := follow(t, deps, offATerminal, "events", "--since", "12:00", "-f")

	listed := eventJSON(t, restartDue, routerUnwell)
	stdout.waitFor(t, listed)
	// The router filed a last line under the day, and the day was compressed,
	// before the follower looked again, as over a quiet weekend.
	dir := ledger.Dir(stateDir(t, deps))
	plain := filepath.Join(dir, "events-2026-10-07.jsonl")
	last := eventLine(runTwo, 2, status.Event{At: october(7, 13, 10, 0), Kind: status.EventHealth})
	appendTo(t, plain, eventJSON(t, last))
	held, err := os.ReadFile(plain)
	if err != nil {
		t.Fatal(err)
	}
	writeStateFile(t, dir, "events-2026-10-07.jsonl.gz.tmp", gzipOf(t, held))
	if err := os.Rename(plain+".gz.tmp", plain+".gz"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(plain); err != nil {
		t.Fatal(err)
	}
	stdout.waitFor(t, listed+eventJSON(t, last))
	// Long enough for a few more looks, which find nothing new.
	time.Sleep(50 * time.Millisecond)

	if code, stderr := interrupt(); code != 0 || stderr != "" || stdout.String() != listed+eventJSON(t, last) {
		t.Errorf("switchboard events -f exited %d, printing\n%s\nand %q on stderr, once interrupted; want 0, the day's lines each once, and nothing",
			code, stdout.String(), stderr)
	}
}

func TestEventsFollowMovesOnADayAtATime(t *testing.T) {
	deps := eventsDeps(t)
	var clock atomic.Int64
	clock.Store(october(7, 23, 59, 0).UnixNano())
	deps.Now = func() time.Time { return time.Unix(0, clock.Load()).Local() }
	stdout, interrupt := follow(t, deps, onATerminal, "events", "--since", "12:30", "-f")

	printed := `Wed 7 Oct 2026, so far
  12:30:00    health    unhealthy: most requests failing
`
	stdout.waitFor(t, printed)
	// Past midnight, the day before is followed until the router files under
	// the next.
	clock.Store(october(8, 0, 0, 10).UnixNano())
	fileEvent(t, deps, "2026-10-07", eventLine(runTwo, 2, status.Event{At: october(7, 23, 59, 50), Kind: status.EventRoom, Account: "personal", Windows: []string{"5h"}}))
	printed += "  23:59:50    open    personal  its 5-hour window reset\n"
	stdout.waitFor(t, printed)
	// A day later, as after a follower was suspended, it reads the rest of
	// the day it followed, then the day between, then today's.
	clock.Store(october(9, 0, 0, 20).UnixNano())
	fileEvent(t, deps, "2026-10-07", eventLine(runTwo, 3, status.Event{At: october(7, 23, 59, 55), Kind: status.EventHealth}))
	fileEvent(t, deps, "2026-10-08", eventLine(runTwo, 4, status.Event{At: october(8, 0, 0, 5), Kind: status.EventStarted, Session: sessionB, Account: "side",
		Reason: status.ReasonNew}))
	printed += `  23:59:55    health            healthy again

Thu 8 Oct 2026
  00:00:05  5b0e9d33  started  side
`
	stdout.waitFor(t, printed)
	fileEvent(t, deps, "2026-10-09", eventLine(runTwo, 5, status.Event{At: october(9, 0, 0, 15), Kind: status.EventAuto, Accounts: []string{"work", "side"},
		By: "dashboard"}))
	printed += `
Fri 9 Oct 2026, so far
  00:00:15            auto               new sessions back from work and side to the router's choice, set from the dashboard
`
	stdout.waitFor(t, printed)
	fileEvent(t, deps, "2026-10-09", eventLine(runTwo, 6, status.Event{At: october(9, 0, 0, 18), Kind: status.EventHealth}))
	stdout.waitFor(t, printed+"  00:00:18            health             healthy again\n")

	if code, stderr := interrupt(); code != 0 || stderr != "" {
		t.Errorf("switchboard events -f exited %d, printing %q on stderr, once interrupted; want 0 and nothing", code, stderr)
	}
}
