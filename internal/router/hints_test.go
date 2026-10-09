package router_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/router"
)

const (
	// promptID is the prompt the hints' tests' requests serve, and agentID
	// the subagent that sends them, as Claude Code names them.
	promptID = "6f1d2c3b-4a5e-4f60-8a7b-9c0d1e2f3a4b"
	agentID  = "a1b2c3d4e5f6"
	// hintToken is shaped like a Claude token, though it's none.
	hintToken = "sk-ant-oat01-fake_hint-token-shaped"
)

// everyHint is each of the headers Claude Code tells of a request by, beside
// its session.
var everyHint = map[string]string{
	claude.PromptHeader:      promptID,
	claude.ClassHeader:       "subagent",
	claude.AgentHeader:       agentID,
	claude.ParentAgentHeader: "f6e5d4c3b2a1",
	claude.AgentTypeHeader:   "Explore",
	claude.CompactionHeader:  "auto",
	claude.CompactedHeader:   "manual",
	claude.ToolTimesHeader:   "Bash=742;Read=9",
}

// hinted returns h with each of hints set, by its name, to its value.
func hinted(h http.Header, hints map[string]string) http.Header {
	h = h.Clone()
	for name, value := range hints {
		h.Set(name, value)
	}
	return h
}

func TestTheLedgerHoldsWhatClaudeCodesHeadersSayOfARequest(t *testing.T) {
	long := strings.Repeat("☃", 100)
	// many are more tools' times than a line holds.
	many := make([]string, ledger.ListMost+8)
	for i := range many {
		many[i] = fmt.Sprintf("Tool%02d=%d", i, i)
	}
	firstMany := []ledger.ToolTime{{Tool: "My Tool", MS: 3}}
	for i := range ledger.ListMost - 1 {
		firstMany = append(firstMany, ledger.ToolTime{Tool: fmt.Sprintf("Tool%02d", i), MS: int64(i)})
	}
	tests := []struct {
		name string
		sent map[string]string
		want ledger.Hints
	}{
		{name: "none"},
		{
			name: "each, as sent",
			sent: everyHint,
			want: ledger.Hints{Prompt: promptID, Class: "subagent", AgentID: agentID, ParentAgentID: "f6e5d4c3b2a1", AgentType: "Explore",
				Compaction: "auto", Compacted: "manual", ToolMS: []ledger.ToolTime{{Tool: "Bash", MS: 742}, {Tool: "Read", MS: 9}}},
		},
		{
			name: "a token hidden, and a value too long cut",
			sent: map[string]string{claude.PromptHeader: hintToken, claude.ClassHeader: "main " + hintToken, claude.AgentHeader: long, claude.ToolTimesHeader: hintToken + "=5"},
			want: ledger.Hints{Prompt: redact.Placeholder, Class: "main " + redact.Placeholder, AgentID: strings.Repeat("☃", 66),
				ToolMS: []ledger.ToolTime{{Tool: redact.Placeholder, MS: 5}}},
		},
		{
			name: "the tools' times, a name decoded, an entry that doesn't parse passed over, as many as a line holds",
			sent: map[string]string{claude.ToolTimesHeader: "My%20Tool=3;Read=fast;" + strings.Join(many, ";")},
			want: ledger.Hints{ToolMS: firstMany},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			up := newUpstream(t, answerOK)
			cfg := runConfig(t, up.URL)
			stop := runRouter(t, cfg)

			readAll(t, send(t, http.MethodPost, "http://"+cfg.Listen+"/v1/messages", hinted(claudeCode(workToken), tt.sent), strings.NewReader(messages)))
			if err := stop(); err != nil {
				t.Fatalf("Run() = %v", err)
			}
			lines := router.LedgerLines(t, filepath.Join(cfg.StateDir, "ledger"))
			if len(lines) != 1 {
				t.Fatalf("the ledger holds %d lines, want the request's", len(lines))
			}
			if !reflect.DeepEqual(lines[0].Hints, tt.want) {
				t.Errorf("the ledger holds the request's hints as %+v, want %+v", lines[0].Hints, tt.want)
			}
		})
	}
}

func TestTheRequestStreamTellsARequestsPromptClassAndAgentAsItsLineGivesThem(t *testing.T) {
	long := strings.Repeat("☃", 100)
	tests := []struct {
		name string
		sent map[string]string
		// want are the prompt, class and agent every event tells of.
		want [3]string
	}{
		{name: "none"},
		{name: "as sent", sent: everyHint, want: [3]string{promptID, "subagent", agentID}},
		{
			name: "a token hidden, and a value too long cut",
			sent: map[string]string{claude.PromptHeader: hintToken, claude.ClassHeader: "main", claude.AgentHeader: long},
			want: [3]string{redact.Placeholder, "main", strings.Repeat("☃", 66)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream, arrived, release := holdingUpstream(t)
			cfg := runConfig(t, upstream)
			stop := runRouter(t, cfg)
			client := router.NewClient(router.SocketPath(cfg.StateDir))
			answered := make(chan string, 1)
			go func() {
				answered <- postAsking("http://"+cfg.Listen+"/v1/messages", messages, hinted(claudeCode(workToken), tt.sent))
			}()
			<-arrived

			told := toldOfARequest(t, client, release)
			<-answered
			if len(told) == 0 || told[0].Kind != router.StreamInFlight {
				t.Fatalf("the stream told of %+v, want it to open with the request in flight", told)
			}
			for _, e := range told {
				if got := [3]string{e.Prompt, e.Class, e.AgentID}; got != tt.want {
					t.Errorf("the stream told of %s with the prompt, class and agent %q, want %q", e.Kind, got, tt.want)
				}
				data, err := json.Marshal(e)
				if err != nil {
					t.Fatal(err)
				}
				for i, field := range []string{`"prompt"`, `"class"`, `"agent_id"`} {
					if strings.Contains(string(data), field) != (tt.want[i] != "") {
						t.Errorf("the stream told of %s as %s, want %s given only where the request carries it", e.Kind, data, field)
					}
				}
			}
			if err := stop(); err != nil {
				t.Fatalf("Run() = %v", err)
			}
			lines := router.LedgerLines(t, filepath.Join(cfg.StateDir, "ledger"))
			if len(lines) != 1 || [3]string{lines[0].Prompt, lines[0].Class, lines[0].AgentID} != tt.want {
				t.Errorf("the ledger holds %+v, want the request's line, its prompt, class and agent %q, as the stream told", lines, tt.want)
			}
		})
	}
}

func TestClaudeCodesHeadersGoUpstreamUnchanged(t *testing.T) {
	up := newUpstream(t, answerOK)
	proxy := serveProxy(t, newRouter(t, up.URL))
	// Each as Claude Code sends it, a token and a value longer than a line
	// holds among them, which only the ledger hides and cuts.
	sent := map[string]string{
		claude.PromptHeader:    hintToken,
		claude.ClassHeader:     "subagent",
		claude.AgentHeader:     strings.Repeat("a", 300),
		claude.AgentTypeHeader: "Explore",
		claude.ToolTimesHeader: "My%20Tool=3;Read=fast;mcp__notes__find%3Bv2=12",
	}

	readAll(t, send(t, http.MethodPost, proxy+"/v1/messages", hinted(claudeCode(workToken), sent), strings.NewReader(messages)))
	checkHeader(t, up.only(t), hinted(routedOn(workToken), sent))
}
