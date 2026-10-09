package claude

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/leeovery/switchboard/internal/ledger"
)

// The headers Claude Code tells of a request by, beside its session, as its
// gateway guide documents them, each left out where it has nothing to say: to
// a base URL, all but the agents' ids only with HintHeadersEnv set.
const (
	// PromptHeader names the prompt a request serves, by a random id, the
	// same on every request serving one, those of the subagents it starts
	// included.
	PromptHeader = "X-Claude-Code-Prompt-Id"
	// ClassHeader is the kind of request it is: main, subagent, workflow,
	// compaction or auxiliary.
	ClassHeader = "X-Claude-Code-Request-Class"
	// AgentHeader names the subagent that sent a request, and
	// ParentAgentHeader the agent that started that one, where agents nest.
	AgentHeader       = "X-Claude-Code-Agent-Id"
	ParentAgentHeader = "X-Claude-Code-Parent-Agent-Id"
	// AgentTypeHeader is the kind of subagent that sent a request, of a
	// subagent's own turns: a built-in agent's name, or custom, teammate or
	// fork, never a name the user chose.
	AgentTypeHeader = "X-Claude-Code-Agent-Type"
	// CompactionHeader is what started the compaction a request is: auto,
	// manual or reactive; and CompactedHeader the same, on the first request
	// of the main conversation after one.
	CompactionHeader = "X-Claude-Code-Compaction"
	CompactedHeader  = "X-Claude-Code-Context-Compacted"
	// ToolTimesHeader is how long each tool call whose result a request
	// carries ran, as <name>=<ms>;<name>=<ms>, each name percent-encoded.
	ToolTimesHeader = "X-Claude-Code-Prev-Tool-Durations"
)

// Hints returns what a request's header says of it beside its session, as
// the request ledger keeps it, each left out where it carries none: the
// prompt it serves, its class, the agent that sent it, the compaction it is
// or follows, and how long each tool call whose result it carries ran.
func (Provider) Hints(h http.Header) ledger.Hints {
	return ledger.Hints{
		Prompt:        h.Get(PromptHeader),
		Class:         h.Get(ClassHeader),
		AgentID:       h.Get(AgentHeader),
		ParentAgentID: h.Get(ParentAgentHeader),
		AgentType:     h.Get(AgentTypeHeader),
		Compaction:    h.Get(CompactionHeader),
		Compacted:     h.Get(CompactedHeader),
		ToolMS:        toolTimes(h.Get(ToolTimesHeader)),
	}
}

// toolTimes reads the tool calls' times ToolTimesHeader gives as value, in
// the order given, each tool's name decoded, as many as a line holds at most:
// nil for none. An entry that doesn't read as a name and whole milliseconds is
// passed over.
func toolTimes(value string) []ledger.ToolTime {
	var times []ledger.ToolTime
	for entry := range strings.SplitSeq(value, ";") {
		t, ok := toolTime(entry)
		if !ok {
			continue
		}
		if times == nil {
			times = make([]ledger.ToolTime, 0, min(strings.Count(value, ";")+1, ledger.ListMost))
		}
		if times = append(times, t); len(times) == ledger.ListMost {
			break
		}
	}
	return times
}

// toolTime reads an entry of ToolTimesHeader, <name>=<ms>, reporting false
// where it doesn't read as a name and whole milliseconds.
func toolTime(entry string) (ledger.ToolTime, bool) {
	name, given, ok := strings.Cut(entry, "=")
	if !ok || name == "" {
		return ledger.ToolTime{}, false
	}
	tool, err := url.PathUnescape(name)
	if err != nil {
		return ledger.ToolTime{}, false
	}
	ms, err := strconv.ParseInt(given, 10, 64)
	if err != nil || ms < 0 {
		return ledger.ToolTime{}, false
	}
	return ledger.ToolTime{Tool: tool, MS: ms}, true
}
