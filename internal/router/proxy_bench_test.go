package router_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/claude/claudetest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/router"
)

func BenchmarkProxyStreamed(b *testing.B) {
	pieces := claudetest.AnswerPieces("Hello, ", "world")
	benchmarkProxy(b, messages, answerStreamed(pieces...), strings.Join(pieces, ""))
}

func BenchmarkProxyJSON(b *testing.B) {
	benchmarkProxy(b, messages, answerMessage, claudetest.Message)
}

func BenchmarkProxyLongSession(b *testing.B) {
	pieces := claudetest.AnswerPieces("Hello, ", "world")
	benchmarkProxy(b, longSession(b), answerStreamed(pieces...), strings.Join(pieces, ""))
}

// benchmarkProxy times a messages request whose body is asked, as Claude
// Code sends one on a session under way, started by run in a directory, after
// tool calls of the prompt it serves, going through a running router's proxy
// to an upstream that reads it whole and answers it as answer does, reporting
// the account's usage in its headers as the API does, and back to a client
// that reads the answer whole, which is to be body.
//
// The router's clock stands still, so no request's result ever leaves its
// health's window, and each request scans them all: a request takes longer
// the more came before it. Compare runs of the same -benchtime count, such as
// 2000x.
func benchmarkProxy(b *testing.B, asked string, answer http.HandlerFunc, body string) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		reportWindows(w.Header(), []quota.Window{session, week})
		answer(w, r)
	}))
	b.Cleanup(up.Close)
	cfg := runConfig(b, up.URL)
	cfg.Prober = readingEvery(session, week)
	runRouter(b, cfg)
	proxy := "http://" + cfg.Listen + "/v1/messages"
	header := hinted(with(claudeCode(workToken), router.DirHeader, router.EncodeDir("~/Code/project")),
		map[string]string{claude.PromptHeader: promptID, claude.ClassHeader: "main", claude.ToolTimesHeader: "Bash=742;Read=9"})
	// The session's first request, which chooses its account, goes untimed.
	postAsking(proxy, asked, header)
	want := "200 " + body
	b.ReportAllocs()
	for b.Loop() {
		if got := postAsking(proxy, asked, header); got != want {
			b.Fatalf("the proxy answered %q, want %q", got, want)
		}
	}
}

// answerMessage answers with a message whole, not streamed.
func answerMessage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, claudetest.Message)
}

// longSession is the body of a messages request as Claude Code sends one deep
// into a long session, the same every time: some 500 KB, of a system prompt
// in three blocks, 30 tools' definitions and 200 messages, the assistant's
// thinking, text and tool calls in turn with the user's tool results, asking
// for 32,000 tokens at most, with thinking, streamed.
func longSession(t testing.TB) string {
	type object = map[string]any
	tools := make([]object, 30)
	for i := range tools {
		tools[i] = object{
			"name":        fmt.Sprintf("Tool%02d", i),
			"description": strings.Repeat(fmt.Sprintf("What tool %d does, and when to reach for it rather than another. ", i), 24),
			"input_schema": object{
				"type": "object",
				"properties": object{
					"path":    object{"type": "string", "description": "The absolute path of the file to work on."},
					"content": object{"type": "string", "description": "What to write, in full."},
					"limit":   object{"type": "integer", "description": "How many lines to read at most."},
				},
				"required": []string{"path"},
			},
		}
	}
	system := []object{
		{"type": "text", "text": "You are Claude Code, Anthropic's official CLI for Claude."},
		{"type": "text", "text": strings.Repeat("How to work in this repository, and what to leave alone. ", 220), "cache_control": object{"type": "ephemeral"}},
		{"type": "text", "text": strings.Repeat("The environment: a working directory, a platform and a date. ", 50)},
	}
	messages := []object{{"role": "user", "content": "Read the notes and tidy them up."}}
	for turn := range 100 {
		id := fmt.Sprintf("toolu_%04d", turn)
		messages = append(messages,
			object{"role": "assistant", "content": []object{
				{"type": "thinking", "thinking": "", "signature": strings.Repeat("c2lnbmVk", 48)},
				{"type": "text", "text": fmt.Sprintf("Step %d: reading the next file, then changing what it needs. ", turn)},
				{"type": "tool_use", "id": id, "name": fmt.Sprintf("Tool%02d", turn%30), "input": object{"path": fmt.Sprintf("/work/notes/%03d.md", turn), "limit": 400}},
			}},
			object{"role": "user", "content": []object{
				{"type": "tool_result", "tool_use_id": id, "content": strings.Repeat(fmt.Sprintf("%4d\tfunc note() string { return \"a line of file %d, \\\"quoted\\\" ☃\" }\n", turn, turn), 46)},
			}},
		)
	}
	body, err := json.Marshal(struct {
		Model     string   `json:"model"`
		Messages  []object `json:"messages"`
		System    []object `json:"system"`
		Tools     []object `json:"tools"`
		Metadata  object   `json:"metadata"`
		MaxTokens int      `json:"max_tokens"`
		Thinking  object   `json:"thinking"`
		Stream    bool     `json:"stream"`
	}{
		Model:     opus,
		Messages:  messages,
		System:    system,
		Tools:     tools,
		Metadata:  object{"user_id": "user_" + strings.Repeat("0123456789abcdef", 4) + "_account_" + sessionID + "_session_" + sessionID},
		MaxTokens: 32000,
		Thinking:  object{"type": "enabled", "budget_tokens": 31999},
		Stream:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
