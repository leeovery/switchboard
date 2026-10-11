package capture

import (
	"bytes"
	"cmp"
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// madeTraffic is the Log's traffic as log1 made it, drawing from Python's
// random numbers: testdata/README.md says how.
//
//go:embed testdata/traffic.jsonl
var madeTraffic []byte

// madeLine is a line of the Log's traffic as it's kept: when its request
// came, on the day before today's where day says -1, to the second; its
// session, by its short id, or a quota check's own, whole; the model as the
// frames name it; the account it went to; its kind, a message where none's
// given; its status, 200 where none's given; how long its answer took to
// start and to end, in milliseconds; and its usage, 3 tokens in where none's
// given, its writes to the cache lasting an hour; and how its answer stopped,
// and the tool it called.
type madeLine struct {
	Day     int    `json:"day"`
	At      string `json:"at"`
	Session string `json:"session"`
	Model   string `json:"model"`
	Account string `json:"on"`
	Kind    string `json:"kind"`
	Status  int    `json:"status"`
	First   int64  `json:"first"`
	Took    int64  `json:"took"`
	In      int    `json:"in"`
	Read    int    `json:"read"`
	Wrote   int    `json:"wrote"`
	Out     int    `json:"out"`
	Stop    string `json:"stop"`
	Tool    string `json:"tool"`
}

// trafficSession is a session of the traffic's: its whole id, and the
// directory it runs in, as log1.DIRS gives it.
type trafficSession struct {
	id, dir string
}

// trafficSessions are the traffic's sessions, by their short ids.
var trafficSessions = map[string]trafficSession{
	"41e0": {id41E0, "~/Code/docs"}, "7f3a": {id7F3A, "~/Code/api"}, "db8a": {idDB8A, "~/Code/infra"}, "c61b": {idC61B, "~/Code/mobile"},
	"5b19": {id5B19, "~/Code/mobile"}, "3d7a": {id3D7A, "~/Code/docs"}, "e0c4": {idE0C4, "~/Code/infra"},
	"a8f1": {idA8F1, "~/Code/website"}, "2b6d": {id2B6D, "~/Code/api"}, "5e7d": {id5E7D, "~/Code/api"},
	"4b0e": {id4B0E, "~/Code/infra"}, "8a2c": {id8A2C, "~/Code/docs"},
}

// reasonUnsessioned is the router's reason for a request of no session's.
const reasonUnsessioned = "unsessioned"

// traffic is the Log's traffic of the day now falls on, and of the day before
// where its sessions began, oldest first, as log1.traffic made it, for the
// sessions whose lines the session pages' model doesn't make: each asking as
// Claude Code does, those the Log's tail and the frames around work's cap
// draw as they draw them; and Claude Code's quota checks, three of them
// failed, and its counts of tokens.
func traffic(now time.Time) []ledger.Line {
	var lines []ledger.Line
	route := newRouting()
	for kept := range bytes.Lines(madeTraffic) {
		var made madeLine
		if err := json.Unmarshal(kept, &made); err != nil {
			panic(fmt.Sprintf("capture: the traffic's line %q: %v", kept, err))
		}
		l := made.line(now)
		l.Request = requestID(l.Session, len(lines))
		route.request(&l)
		lines = append(lines, l)
	}
	return lines
}

// line is the made line as the ledger holds it, at its time on the day before
// now's or on now's, of the session it names, but its route.
func (m madeLine) line(now time.Time) ledger.Line {
	clock, err := time.Parse(time.TimeOnly, m.At)
	if err != nil {
		panic(fmt.Sprintf("capture: the traffic's time %q: %v", m.At, err))
	}
	l := ledger.Line{
		At: clockAt(now, m.Day, clock.Hour(), clock.Minute(), clock.Second()).UTC(), Kind: cmp.Or(m.Kind, ledger.KindMessage),
		Model: "claude-" + m.Model, Account: m.Account, Status: cmp.Or(m.Status, 200), Attempts: 1, TotalMS: m.Took, Agent: claudeCodeAgent,
	}
	l.Session, l.Dir = m.session()
	if m.First > 0 {
		l.FirstMS = &m.First
	}
	l.Answer.Stop = m.Stop
	if m.Tool != "" {
		l.Answer.Tools = []string{m.Tool}
	}
	if l.Kind == ledger.KindMessage {
		l.Class = classMain
		if strings.HasPrefix(m.Model, "haiku") {
			l.Class = classAuxiliary
		}
	}
	if l.Kind != ledger.KindCount && l.Status < 400 {
		l.Usage = usageTokens{input: cmp.Or(m.In, sampleInput), read: m.Read, written: m.Wrote, out: m.Out}.usage()
	}
	return l
}

// session is the whole id of the line's session, and the directory it runs
// in: a quota check's own id, whole, and no directory; none for a count.
func (m madeLine) session() (string, string) {
	if s, ok := trafficSessions[m.Session]; ok {
		return s.id, s.dir
	}
	return m.Session, ""
}

// routing routes the traffic's requests as the router does: each session's
// models apart, a check chosen afresh as a new session, a count of no session
// unsessioned. It holds each session's models' lines last routed, and the
// prompt each session's own conversation is on.
type routing struct {
	last    map[[2]string]ledger.Line
	prompts map[string]int
}

func newRouting() routing {
	return routing{last: make(map[[2]string]ledger.Line), prompts: make(map[string]int)}
}

// request routes l, saying why it went where it did: new, for its session's
// model's first; moved, where its session left the account its model went to
// last, as movedWhy says why, an account left at its limit tried first;
// rescored, where it came after its cache there went cold; else sticky. A
// request of a session's own conversation, or a side request for it, serves
// the prompt it's on, and one that ends its turn ends that prompt.
func (r routing) request(l *ledger.Line) {
	switch {
	case l.Kind == ledger.KindCheck:
		l.Reason = status.ReasonNew
		return
	case l.Session == "":
		l.Reason = reasonUnsessioned
		return
	}
	key := [2]string{l.Session, l.Model}
	last, ok := r.last[key]
	switch {
	case !ok:
		l.Reason = status.ReasonNew
	case last.Account != l.Account:
		l.From, l.Reason = last.Account, movedWhy(last, *l)
		if l.Reason == reasonLimited {
			l.Tried, l.Attempts = []ledger.Tried{{Account: last.Account, Why: status.WhyLimit}}, 2
		}
	case l.At.Sub(last.At) > cacheLife:
		l.Reason = status.Rescored(status.Countdown(last.At, l.At))
	default:
		l.Reason = status.ReasonSticky
	}
	r.last[key] = *l
	l.Prompt = fmt.Sprintf("%s-prompt-%d", l.Session[:8], r.prompts[l.Session]+1)
	if l.Class == classMain && l.Answer.Stop == "end_turn" {
		r.prompts[l.Session]++
	}
}

// movedWhy is why the router moved a session's model off the account its line
// last went to, onto l's, as its moved events say: off work at its cap; off
// personal at its limit; else chosen afresh as it idled, passing personal
// over under pressure.
func movedWhy(last, l ledger.Line) string {
	switch {
	case last.Account == "work":
		return reasonCapped
	case l.Account == "side":
		return reasonLimited
	}
	return status.Rescored(status.Countdown(last.At, l.At)) + ", " + last.Account + " under pressure"
}

// cacheLife is how long a session's cache lasts unused, Claude Code's writes
// to it lasting an hour.
const cacheLife = time.Hour

// sessionsLines are every line the sessions' world holds, oldest first:
// today's, the Log's traffic and d28c's and 9c2e's, as their pages draw them;
// and yesterday's, the traffic's sessions' that began then, and b9c4's.
func sessionsLines(now time.Time) []ledger.Line {
	lines := slices.Concat(traffic(now), d28cPlan(now).lines(), quietPlan(now).lines(), endedPlan(now).lines())
	slices.SortStableFunc(lines, func(a, b ledger.Line) int { return a.At.Compare(b.At) })
	return lines
}

// sessionsRecords are the sessions' world's records: every line it holds, and
// the router's events.
func sessionsRecords(now time.Time) *records {
	return &records{lines: heldAsWritten(sessionsLines(now)), events: routerEvents(now)}
}
