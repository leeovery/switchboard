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

// turnsOf returns the turns requests, a session's, oldest first, tell, newest
// first, as TURNS has them: a turn is the requests of one prompt, as Claude
// Code names it; a subagent's, nested ones too, count in the turn of the
// prompt its first request names, whatever prompts come meanwhile; and a
// request that names no prompt is in none. running says the session runs, so
// its newest turn is still going while its last answer of its own
// conversation stopped to call a tool.
func turnsOf(requests []request, running bool) []Turn {
	var tallies []*turnTally
	byPrompt := make(map[string]*turnTally)
	agents := make(map[string]string)
	for _, r := range requests {
		prompt := promptOf(r.Line, agents)
		if prompt == "" {
			continue
		}
		t, ok := byPrompt[prompt]
		if !ok {
			t = &turnTally{started: r.At, tools: make(map[string]int), accounts: []string{}}
			byPrompt[prompt], tallies = t, append(tallies, t)
		}
		t.add(r)
	}
	turns := make([]Turn, len(tallies))
	for i, t := range tallies {
		newest := i == len(tallies)-1
		turns[len(tallies)-1-i] = t.turn(i+1, running && newest && t.calling())
	}
	return turns
}

// promptOf returns the prompt whose turn the request l counts in: a
// subagent's, the prompt its agent's first request named, as agents holds
// them, by the agents' ids, noting it there where l is that first request;
// else the one it names.
func promptOf(l *ledger.Line, agents map[string]string) string {
	if l.AgentID == "" {
		return l.Prompt
	}
	prompt, ok := agents[l.AgentID]
	if !ok {
		prompt = l.Prompt
		agents[l.AgentID] = prompt
	}
	return prompt
}

// turnTally is a turn as its requests, oldest first, are taken in: when its
// first came; its last, and its last of its own conversation; how many there
// are; how many times each tool was called; the tokens they read from the
// cache, wrote to it and put out; their worth; and the accounts they went
// to, in the order they first did.
type turnTally struct {
	started            time.Time
	last, lastMain     *ledger.Line
	requests           int
	tools              map[string]int
	read, written, out int
	worth              worthSum
	accounts           []string
}

// add takes in r, the turn's next request.
func (t *turnTally) add(r request) {
	t.last = r.Line
	if r.Class == ledger.ClassMain {
		t.lastMain = r.Line
	}
	t.requests++
	for _, tool := range r.Answer.Tools {
		t.tools[tool]++
	}
	t.read, t.written, t.out = t.read+r.tokens.CacheRead, t.written+r.tokens.CacheWrite, t.out+r.tokens.Output
	t.worth.add(r.worth)
	if r.Account != "" && !slices.Contains(t.accounts, r.Account) {
		t.accounts = append(t.accounts, r.Account)
	}
}

// calling reports whether the turn's last answer of its own conversation
// stopped to call a tool, so the turn goes on past it.
func (t *turnTally) calling() bool {
	return t.lastMain != nil && (t.lastMain.Answer.Stop == stopToolUse || t.lastMain.Answer.Stop == stopPauseTurn)
}

// turn returns the turn, numbered n: ended with the end of its last request
// of its own conversation, or of its last request where it has none of
// them, unless it's going, as a subagent still running doesn't keep it so.
func (t *turnTally) turn(n int, going bool) Turn {
	turn := Turn{
		Turn: n, Started: t.started.UTC(), Requests: t.requests, Tools: toolsCalled(t.tools), Read: t.read, Written: t.written, Out: t.out,
		Worth: t.worth.cost, Unpriced: t.worth.unpriced, Accounts: t.accounts,
	}
	if !going {
		turn.Ended = ended(cmp.Or(t.lastMain, t.last)).UTC()
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
