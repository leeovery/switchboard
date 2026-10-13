package views

import (
	"cmp"
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
// of its requests, oldest first, and those the request stream told of, told,
// the first sent first, as talliesOf tallies them. A turn goes on while it's
// open, as open says; and, running saying the session runs, not having
// ended, its newest goes on while its last answer of its own conversation
// stopped to call a tool, or while unplaced says a request of the session is
// in flight that neither its lines nor the stream place in a turn, as one
// the router lists in flight that the stream hasn't told of.
func turnsOf(requests []request, told []Told, running, unplaced bool) []Turn {
	tallies := talliesOf(requests, told)
	turns := make([]Turn, len(tallies))
	for i, t := range tallies {
		newest := i == len(tallies)-1
		turns[len(tallies)-1-i] = t.turn(i+1, t.open() || running && newest && (t.calling() || unplaced))
	}
	return turns
}

// talliesOf returns the tallies of a session's turns, the first started
// first, of its requests, oldest first, and those the request stream told
// of, told, the first sent first, each counted in its turn as it's sent, as
// sentOf takes them in. A turn is the requests of one prompt, as Claude Code
// names it; a subagent's, nested ones too, count in the turn of the prompt
// its first request names, whatever prompts come meanwhile; and a request
// that names no prompt is in none.
func talliesOf(requests []request, told []Told) []*turnTally {
	var tallies []*turnTally
	byPrompt := make(map[string]*turnTally)
	agents := make(map[string]string)
	sent := sentOf(requests, told)
	for i := range sent {
		r := &sent[i]
		prompt := promptOf(r.agentID, r.prompt, agents)
		if prompt == "" {
			continue
		}
		t, ok := byPrompt[prompt]
		if !ok {
			t = &turnTally{started: r.at, tools: make(map[string]int), accounts: []string{}, mainAccounts: []string{}}
			byPrompt[prompt], tallies = t, append(tallies, t)
		}
		t.add(r)
	}
	return tallies
}

// promptOf returns the prompt whose turn a request counts in, of the agent
// with the given id, naming the prompt given: a subagent's, the prompt its
// agent's first request named, as agents holds them, by the agents' ids,
// noting it there where this is that first request; else the one it names.
func promptOf(agentID, prompt string, agents map[string]string) string {
	if agentID == "" {
		return prompt
	}
	first, ok := agents[agentID]
	if !ok {
		first = prompt
		agents[agentID] = first
	}
	return first
}

// sent is a session's request as its turn takes it in: as its line tells it,
// once that has come; else as the request stream told of it, from when it
// was sent. It names a prompt, a class and an agent, each "" where it names
// none, asks for a model, and went to an account, or went out on it last
// while in flight.
type sent struct {
	prompt, class, agentID string
	at                     time.Time
	model, account         string
	// line is its line, nil before that has come; and told what the stream
	// told of it till then, nil once its line has come.
	line *request
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
// first: its requests, oldest first, by their lines; and of told, the
// requests the stream told of, the first sent first, those whose lines
// haven't come, as filedAs has it, each from when it was sent, a line first
// where one came as another was sent.
func sentOf(requests []request, told []Told) []sent {
	filed := make(map[string][]*ledger.Line, len(requests))
	for _, r := range requests {
		filed[r.Request] = append(filed[r.Request], r.Line)
	}
	all := make([]sent, 0, len(requests)+len(told))
	next := 0
	for _, t := range told {
		if slices.ContainsFunc(filed[t.Request], t.filedAs) {
			continue
		}
		for ; next < len(requests) && !requests[next].At.After(t.Sent); next++ {
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
func lineSent(r *request) sent {
	return sent{prompt: r.Prompt, class: r.Class, agentID: r.AgentID, at: r.At, model: r.Model, account: r.Account, line: r, end: ended(r.Line)}
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

// turnTally is a turn as its requests, oldest first, are taken in: when its
// first came; each of them, its last, and its last of its own conversation;
// how many there are; how many times each tool was called; the tokens they
// read from the cache, wrote to it and put out; their worth; the accounts
// they went to, and those its own conversation's went to, each as
// withAccount lists them; and whether any of them, and any of its own
// conversation's, is pending.
type turnTally struct {
	started                time.Time
	members                []*sent
	last, lastMain         *sent
	requests               int
	tools                  map[string]int
	read, written, out     int
	worth                  Worth
	accounts, mainAccounts []string
	pending, mainPending   bool
}

func (t *turnTally) add(r *sent) {
	t.members = append(t.members, r)
	t.last, t.accounts, t.pending = r, withAccount(t.accounts, r.account), t.pending || r.pending
	if r.class == ledger.ClassMain {
		t.lastMain, t.mainAccounts, t.mainPending = r, withAccount(t.mainAccounts, r.account), t.mainPending || r.pending
	}
	t.requests++
	if r.line == nil {
		return
	}
	for _, tool := range r.line.Answer.Tools {
		t.tools[tool]++
	}
	tokens := r.line.tokens
	t.read, t.written, t.out = t.read+tokens.CacheRead, t.written+tokens.CacheWrite, t.out+tokens.Output
	t.worth.add(r.line.worth)
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
	if t.lastMain == nil {
		return t.pending
	}
	return t.mainPending
}

// calling reports whether the turn's last answer of its own conversation, as
// its line gives it, stopped to call a tool, so the turn goes on past it.
func (t *turnTally) calling() bool {
	if t.lastMain == nil || t.lastMain.line == nil {
		return false
	}
	stop := t.lastMain.line.Answer.Stop
	return stop == stopToolUse || stop == stopPauseTurn
}

// turn returns the turn, numbered n, following its own conversation: on the
// accounts its requests went to, and ended with the end of its last one,
// unless it's going; or, where it has none of them, following its requests.
func (t *turnTally) turn(n int, going bool) Turn {
	turn := Turn{
		Turn: n, Started: t.started.UTC(), Requests: t.requests, Tools: toolsCalled(t.tools), Read: t.read, Written: t.written, Out: t.out,
		Worth: t.worth, Accounts: t.accounts,
	}
	if t.lastMain != nil {
		turn.Accounts = t.mainAccounts
	}
	if !going {
		turn.Ended = cmp.Or(t.lastMain, t.last).end.UTC()
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
