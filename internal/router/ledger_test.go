package router_test

import (
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/claude/claudetest"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/router"
)

func TestRunKeepsALineOfEachRoutedRequestInTheLedger(t *testing.T) {
	up := newUpstream(t, answerOK)
	cfg := runConfig(t, up.URL)
	stop := runRouter(t, cfg)
	// The request carries what the ledger never keeps: its messages, its
	// system prompt, the ids its metadata gives, its keys, and fields
	// switchboard doesn't know, one holding an MCP server's token, which is
	// shaped like a token, though it's none.
	const (
		prompt    = "a system prompt kept to itself"
		message   = "a message kept to itself"
		device    = "device-kept-to-itself"
		apiKey    = "test-api-key-kept-to-itself"
		mcpToken  = "sk-ant-oat01-fake_mcp-token-shaped"
		container = "container-kept-to-itself"
	)
	body := `{"model":"claude-opus-5-5","max_tokens":32000,"system":"` + prompt + `","messages":[{"role":"user","content":"` + message + `"}],` +
		`"metadata":{"user_id":"user_` + device + `_account_` + sessionID + `"},"stream":true,` +
		`"mcp_servers":[{"name":"notes","authorization_token":"` + mcpToken + `"}],"container":"` + container + `"}`
	header := with(claudeCode(workToken), "X-Api-Key", apiKey)

	readAll(t, send(t, http.MethodPost, "http://"+cfg.Listen+"/v1/messages", header, strings.NewReader(body)))
	if err := stop(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	dir := filepath.Join(cfg.StateDir, "ledger")
	file := filepath.Join(dir, "requests-"+now.Local().Format(time.DateOnly)+".jsonl")
	for path, want := range map[string]fs.FileMode{dir: fs.ModeDir | 0o700, file: 0o600} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode() != want {
			t.Errorf("%s has mode %v, want %v", path, info.Mode(), want)
		}
	}
	lines := router.LedgerLines(t, dir)
	if len(lines) != 1 {
		t.Fatalf("the ledger holds %d lines, want the request's", len(lines))
	}
	if got := lines[0]; got.Kind != ledger.KindMessage || got.Session != sessionID || got.Model != opus || got.Account != "work" || got.Status != http.StatusOK || got.Attempts != 1 {
		t.Errorf("the ledger holds %+v, want the request, answered on work", got)
	}
	shape := ledger.Shape{Bytes: len(body), Messages: 1, System: 1, MaxTokens: new(int64(32000)), Stream: new(true)}
	if !reflect.DeepEqual(lines[0].Shape, shape) {
		t.Errorf("the ledger holds the request's shape as %+v, want %+v", lines[0].Shape, shape)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, kept := range []string{prompt, message, device, workToken, apiKey, mcpToken, container, "sk-ant-"} {
		if strings.Contains(string(data), kept) {
			t.Errorf("the ledger holds\n%s\nwant nothing of %q", data, kept)
		}
	}
}

func TestTheLedgerHoldsTheDirectoryARequestsSessionWasStartedIn(t *testing.T) {
	tests := []struct {
		name string
		// sent is the request's directory header, "" for none.
		sent string
		want string
	}{
		{name: "none"},
		{name: "as it was sent", sent: "~/Code/my project", want: "~/Code/my project"},
		{name: "encoded", sent: "~/Code/caf%C3%A9%0A100%25%20", want: "~/Code/café\n100% "},
		{name: "one that doesn't decode, passed over", sent: "~/Code/100%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			up := newUpstream(t, answerOK)
			cfg := runConfig(t, up.URL)
			stop := runRouter(t, cfg)

			readAll(t, send(t, http.MethodPost, "http://"+cfg.Listen+"/v1/messages", with(claudeCode(workToken), router.DirHeader, tt.sent), strings.NewReader(messages)))
			if err := stop(); err != nil {
				t.Fatalf("Run() = %v", err)
			}
			lines := router.LedgerLines(t, filepath.Join(cfg.StateDir, "ledger"))
			if len(lines) != 1 || lines[0].Dir != tt.want {
				t.Errorf("the ledger holds %+v, want the request's line, its directory %q", lines, tt.want)
			}
		})
	}
}

func TestTheLedgerHoldsTheShapeOfALongSessionsRequest(t *testing.T) {
	up := newUpstream(t, answerOK)
	cfg := runConfig(t, up.URL)
	stop := runRouter(t, cfg)
	body := longSession(t)

	readAll(t, send(t, http.MethodPost, "http://"+cfg.Listen+"/v1/messages", claudeCode(workToken), strings.NewReader(body)))
	if err := stop(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	lines := router.LedgerLines(t, filepath.Join(cfg.StateDir, "ledger"))
	if len(lines) != 1 {
		t.Fatalf("the ledger holds %d lines, want the request's", len(lines))
	}
	want := ledger.Shape{Bytes: len(body), Messages: 201, System: 3, Tools: 30, MaxTokens: new(int64(32000)),
		Thinking: ledger.Thinking{Type: "enabled", BudgetTokens: new(int64(31999))}, Stream: new(true)}
	if !reflect.DeepEqual(lines[0].Shape, want) {
		t.Errorf("the ledger holds the shape of a long session's request as %+v, want %+v", lines[0].Shape, want)
	}
}

func TestTheLedgerHoldsTheLineOfARequestFinishedAsTheRouterStops(t *testing.T) {
	upstream, arrived, release := holdingUpstream(t)
	cfg := runConfig(t, upstream)
	stop := runRouter(t, cfg)
	answered := make(chan string, 1)
	go func() { answered <- post("http://" + cfg.Listen + "/v1/messages") }()
	<-arrived

	stopped := make(chan error, 1)
	go func() { stopped <- stop() }()
	waitUntil(t, "the proxy stops taking requests", func() bool {
		conn, err := net.Dial("tcp", cfg.Listen)
		if err == nil {
			_ = conn.Close()
		}
		return err != nil
	})
	release()
	if got := <-answered; got != `200 {"type":"message"}` {
		t.Errorf("the request in flight was answered %q, want 200 and its body", got)
	}
	if err := <-stopped; err != nil {
		t.Errorf("Run() = %v, want nil", err)
	}
	lines := router.LedgerLines(t, filepath.Join(cfg.StateDir, "ledger"))
	if len(lines) != 1 || lines[0].Status != http.StatusOK {
		t.Errorf("once the router stopped, the ledger holds %+v, want the line of the request it finished", lines)
	}
}

func TestTheLedgerHoldsTheLineOfARequestTheRouterCutsOffAsItStops(t *testing.T) {
	log := logstest.Capture(t)
	// The upstream begins its answer, and holds the rest until the request is
	// cut off, once the drain's time has passed. Counting the answer then
	// takes a while to finish, as on a loaded machine, so the request is
	// still unwinding as the drain ends.
	up := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, claudetest.MessageStart)
		_ = http.NewResponseController(w).Flush()
		<-r.Context().Done()
	})
	cfg := runConfig(t, up.URL)
	cfg.Provider, cfg.DrainFor = slowToFinish{}, 50*time.Millisecond
	stop := runRouter(t, cfg)
	if resp := send(t, http.MethodPost, "http://"+cfg.Listen+"/v1/messages", claudeCode(workToken), strings.NewReader(messages)); resp.StatusCode != http.StatusOK {
		t.Fatalf("the answer began %d, want 200", resp.StatusCode)
	}

	if err := stop(); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	lines := router.LedgerLines(t, filepath.Join(cfg.StateDir, "ledger"))
	if len(lines) != 1 || lines[0].Status != http.StatusOK || !lines[0].CutOff || lines[0].Canceled {
		t.Errorf("once the router stopped, the ledger holds %+v, want the line of the request it cut off, not canceled: its client stayed", lines)
	}
	if !log.Has("level=INFO", "msg=routed", "status=200", "cut_off=true") || log.Has("canceled=true") {
		t.Errorf("log reads\n%s\nwant the request routed, and cut off, not canceled", log)
	}
}

// slowToFinish is Claude's provider, but for taking a while to finish counting
// an answer once its body has ended.
type slowToFinish struct {
	claude.Provider
}

func (p slowToFinish) Count(h http.Header, body io.Reader, chars func(int)) (*quota.Tokens, ledger.Reply) {
	tokens, reply := p.Provider.Count(h, body, chars)
	time.Sleep(200 * time.Millisecond)
	return tokens, reply
}
