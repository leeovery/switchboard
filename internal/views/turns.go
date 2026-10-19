package views

import (
	"cmp"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
)

// Why an answer stopped, as a line's answer gives it, where it stopped to
// call a tool, its conversation going on: pause_turn, as a server tool's
// long run pauses it, as well as tool_use.
const (
	stopToolUse   = "tool_use"
	stopPauseTurn = "pause_turn"
)

// turnsOf returns the turns of a session, newest first, as TURNS has them:
// of their tallies, the first started first, as pageTally tallies them,
// inNone saying a request in none is in flight, each worth what its requests
// spent, as priced prices it. A turn goes on while it's open, as open says;
// and, running saying the session runs, not having ended, its newest goes on
// while its last answer of its own conversation stopped to call a tool, or
// while a request of the session is in flight that neither its lines nor the
// stream place in a turn: one the stream told of naming no prompt, or, as
// unplaced says, one the router lists in flight that the stream hasn't told
// of.
func turnsOf(tallies []*turnTally, inNone, running, unplaced bool, priced pricing) []Turn {
	turns := make([]Turn, len(tallies))
	for i, t := range tallies {
		newest := i == len(tallies)-1
		turns[len(tallies)-1-i] = t.turn(i+1, t.open() || running && newest && (t.calling() || unplaced || inNone), priced)
	}
	return turns
}

// sent is a session's request as its turn takes it in: as its line tells it,
// once that has come; else as the request stream told of it, from when it
// was sent. It names a prompt, a class and an agent, each "" where it names
// none, asks for a model, and went to an account, or went out on it last
// while in flight. A turn is the requests of one prompt, as Claude Code
// names it; a subagent's, nested ones too, count in the turn of the prompt
// its first request names, whatever prompts come meanwhile; and a request
// that names no prompt is in none.
type sent struct {
	prompt, class, agentID string
	at                     time.Time
	model, account         string
	// line is its line, nil before that has come; and told what the stream
	// told of it till then, nil once its line has come.
	line *heldRequest
	told *Told
	// end is when it ended, as its line gives it, or the stream told, or,
	// lost, when the stream last told of it: zero while it's in flight.
	end time.Time
	// pending is set while its answer is yet to be read from its line: in
	// flight, or ended with its line to come, as the stream told; never once
	// it's lost, which ended it untold.
	pending bool
}

// sentOf returns the requests of a session as its turns take them in, oldest
// first: requests, its lines, in order, as inOrder has it; and of told, the
// requests the stream told of, the first sent first, those whose lines
// haven't come, as lined says, each from when it was sent, a line first
// where one came as another was sent.
func sentOf(requests []heldRequest, told []Told, lined func(Told) bool) []sent {
	all := make([]sent, 0, len(requests)+len(told))
	next := 0
	for _, t := range told {
		if lined(t) {
			continue
		}
		for ; next < len(requests) && !requests[next].arrived().After(t.Sent); next++ {
			all = append(all, lineSent(&requests[next]))
		}
		all = append(all, toldSent(t))
	}
	for ; next < len(requests); next++ {
		all = append(all, lineSent(&requests[next]))
	}
	return all
}

// lineSent is the request r as its line tells it.
func lineSent(r *heldRequest) sent {
	return sent{
		prompt: r.prompt.Value(), class: r.class.Value(), agentID: r.agentID.Value(), at: r.arrived(), model: r.model.Value(), account: r.account.Value(),
		line: r, end: r.ended(),
	}
}

// toldSent is the request t as the stream told of it, before its line has
// come.
func toldSent(t Told) sent {
	s := sent{prompt: t.Prompt, class: t.Class, agentID: t.AgentID, at: t.Sent, model: t.Model, account: t.Account, told: &t, pending: !t.Lost}
	switch {
	case !t.Ended.IsZero():
		s.end = t.Ended
	case t.Lost:
		s.end = t.Heard
	}
	return s
}

// turnTally is a turn as its requests, oldest first, are taken in: the
// prompt it serves; when its first came; how its last, and its last of its
// own conversation, ended; how many there are; how many times each tool was
// called; the tokens they read from the cache, wrote to it and put out;
// what they spent; the accounts they went to, and those its own conversation's
// went to, each as withAccount lists them; and whether any of them, and any
// of its own conversation's, is pending.
type turnTally struct {
	prompt                 string
	started                time.Time
	last, lastMain         turnEnd
	requests               int
	tools                  map[string]int
	read, written, out     int
	spent                  ledger.Spent
	accounts, mainAccounts []string
	pending, mainPending   bool
}

// turnEnd is how a turn's request ended: when, zero while it's in flight;
// whether its line has come, and why its answer stopped, as its line gives
// it. It's set once a request is taken in.
type turnEnd struct {
	end   time.Time
	lined bool
	stop  string
	set   bool
}

// endOf is how r ended.
func endOf(r *sent) turnEnd {
	e := turnEnd{end: r.end, lined: r.line != nil, set: true}
	if r.line != nil {
		e.stop = r.line.stop.Value()
	}
	return e
}

// cloned is a copy of t for a request to be taken into, sharing nothing it
// changes with t.
func (t *turnTally) cloned() *turnTally {
	c := *t
	c.tools = maps.Clone(t.tools)
	c.accounts, c.mainAccounts = slices.Clip(t.accounts), slices.Clip(t.mainAccounts)
	return &c
}

func (t *turnTally) add(r *sent) {
	end := endOf(r)
	t.last, t.accounts, t.pending = end, withAccount(t.accounts, r.account), t.pending || r.pending
	if r.class == ledger.ClassMain {
		t.lastMain, t.mainAccounts, t.mainPending = end, withAccount(t.mainAccounts, r.account), t.mainPending || r.pending
	}
	t.requests++
	if r.line == nil {
		return
	}
	for tool := range r.line.calls() {
		t.tools[tool]++
	}
	tokens, _ := r.line.spend.Tokens()
	t.read, t.written, t.out = t.read+tokens.CacheRead, t.written+tokens.CacheWrite, t.out+tokens.Output
	r.line.spentInto(&t.spent)
}

func withAccount(accounts []string, account string) []string {
	if account == "" || len(accounts) > 0 && accounts[len(accounts)-1] == account {
		return accounts
	}
	return append(accounts, account)
}

// open reports whether the turn is open, following its own conversation: a
// request of it pending; or, where it has none of it, any of its requests
// pending. A subagent still running doesn't keep it open.
func (t *turnTally) open() bool {
	if !t.lastMain.set {
		return t.pending
	}
	return t.mainPending
}

// calling reports whether the turn's last answer of its own conversation, as
// its line gives it, stopped to call a tool, so the turn goes on past it.
func (t *turnTally) calling() bool {
	stop := t.lastMain.stop
	return t.lastMain.lined && (stop == stopToolUse || stop == stopPauseTurn)
}

// turn returns the turn, numbered n, following its own conversation: on the
// accounts its requests went to, and ended with the end of its last one,
// unless it's going; or, where it has none of them, following its requests;
// worth what its requests spent, as priced prices it.
func (t *turnTally) turn(n int, going bool, priced pricing) Turn {
	turn := Turn{
		Turn: n, Started: t.started.UTC(), Requests: t.requests, Tools: toolsCalled(t.tools), Read: t.read, Written: t.written, Out: t.out,
		Worth: priced.worth(t.spent), Accounts: t.accounts,
	}
	ended := t.last
	if t.lastMain.set {
		turn.Accounts, ended = t.mainAccounts, t.lastMain
	}
	if !going {
		turn.Ended = ended.end.UTC()
	}
	return turn
}

// toolsCalled returns the tools calls counts the calls of, by their names,
// the most called first, those called as often by their names.
func toolsCalled(calls map[string]int) []ToolCalls {
	tools := make([]ToolCalls, 0, len(calls))
	for name, times := range calls {
		tools = append(tools, ToolCalls{Tool: name, Times: times})
	}
	slices.SortFunc(tools, func(a, b ToolCalls) int {
		return cmp.Or(cmp.Compare(b.Times, a.Times), strings.Compare(a.Tool, b.Tool))
	})
	return tools
}
