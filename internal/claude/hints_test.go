package claude_test

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/ledger"
)

// promptID is the prompt the hints' tests' requests serve, as Claude Code
// names one.
const promptID = "6f1d2c3b-4a5e-4f60-8a7b-9c0d1e2f3a4b"

func TestProviderHints(t *testing.T) {
	tests := []struct {
		name   string
		header http.Header
		want   ledger.Hints
	}{
		{name: "none without the headers", header: header("X-Claude-Code-Session-Id", "0b5c6f2e-7d41-4a3b-9c8e-1f2a3b4c5d6e")},
		{
			name:   "a turn of the main conversation, its headers named in any case",
			header: header("x-claude-code-prompt-id", promptID, "X-CLAUDE-CODE-REQUEST-CLASS", "main"),
			want:   ledger.Hints{Prompt: promptID, Class: "main"},
		},
		{
			name: "a nested subagent's turn, after tool calls",
			header: header(
				"X-Claude-Code-Prompt-Id", promptID,
				"X-Claude-Code-Request-Class", "subagent",
				"X-Claude-Code-Agent-Id", "a1b2c3d4e5f6",
				"X-Claude-Code-Parent-Agent-Id", "f6e5d4c3b2a1",
				"X-Claude-Code-Agent-Type", "Explore",
				"X-Claude-Code-Prev-Tool-Durations", "Bash=742;Read=9",
			),
			want: ledger.Hints{Prompt: promptID, Class: "subagent", AgentID: "a1b2c3d4e5f6", ParentAgentID: "f6e5d4c3b2a1", AgentType: "Explore",
				ToolMS: []ledger.ToolTime{{Tool: "Bash", MS: 742}, {Tool: "Read", MS: 9}}},
		},
		{
			name:   "a compaction, started as the context ran out",
			header: header("X-Claude-Code-Prompt-Id", promptID, "X-Claude-Code-Request-Class", "compaction", "X-Claude-Code-Compaction", "auto"),
			want:   ledger.Hints{Prompt: promptID, Class: "compaction", Compaction: "auto"},
		},
		{
			name:   "the first request after a compaction made by hand",
			header: header("X-Claude-Code-Request-Class", "main", "X-Claude-Code-Context-Compacted", "manual"),
			want:   ledger.Hints{Class: "main", Compacted: "manual"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (claude.Provider{}).Hints(tt.header); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Hints() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestProviderHintsReadTheToolsTimes(t *testing.T) {
	// many are more tools' times than a line holds.
	many := make([]string, ledger.ListMost+8)
	for i := range many {
		many[i] = fmt.Sprintf("Tool%02d=%d", i, i)
	}
	firstMany := make([]ledger.ToolTime, ledger.ListMost)
	for i := range firstMany {
		firstMany[i] = ledger.ToolTime{Tool: fmt.Sprintf("Tool%02d", i), MS: int64(i)}
	}
	tests := []struct {
		name  string
		given string
		want  []ledger.ToolTime
	}{
		{name: "one", given: "Bash=742", want: []ledger.ToolTime{{Tool: "Bash", MS: 742}}},
		{
			name:  "each name decoded, a tool named as only an encoding allows",
			given: "mcp__notes__find%3Bv2=12;My%20Tool=3;caf%C3%A9%3D%25=0;a+b=1",
			want:  []ledger.ToolTime{{Tool: "mcp__notes__find;v2", MS: 12}, {Tool: "My Tool", MS: 3}, {Tool: "café=%", MS: 0}, {Tool: "a+b", MS: 1}},
		},
		{
			name:  "each entry that doesn't parse passed over",
			given: "Bash=742;Read;=5;Edit=;Grep=fast;Glob=-1;Bad%ZZ=4;Huge=99999999999999999999;;Write=1.5;Task=3",
			want:  []ledger.ToolTime{{Tool: "Bash", MS: 742}, {Tool: "Task", MS: 3}},
		},
		{name: "none that parses, none", given: "Read;=5;Edit="},
		{name: "as many as a line holds, the first", given: strings.Join(many, ";"), want: firstMany},
		{name: "as many as a line holds, of those that parse", given: "Read;" + strings.Join(many, ";"), want: firstMany},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (claude.Provider{}).Hints(header("X-Claude-Code-Prev-Tool-Durations", tt.given)).ToolMS
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Hints() of the tools' times %q = %+v, want %+v", tt.given, got, tt.want)
			}
		})
	}
}

func TestProviderHintsAllocateNothingButTheToolsTimes(t *testing.T) {
	tests := []struct {
		name   string
		header http.Header
		want   float64
	}{
		{name: "without them", header: header("X-Claude-Code-Session-Id", "0b5c6f2e-7d41-4a3b-9c8e-1f2a3b4c5d6e"), want: 0},
		{
			name:   "with them, but for the list of the tools' times",
			header: header("X-Claude-Code-Prompt-Id", promptID, "X-Claude-Code-Request-Class", "main", "X-Claude-Code-Prev-Tool-Durations", "Bash=742;Read=9"),
			want:   1,
		},
	}
	for _, tt := range tests {
		if got := testing.AllocsPerRun(100, func() { (claude.Provider{}).Hints(tt.header) }); got != tt.want {
			t.Errorf("Hints() of a request %s allocates %v times, want %v", tt.name, got, tt.want)
		}
	}
}
