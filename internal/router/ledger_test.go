package router_test

import (
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
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
