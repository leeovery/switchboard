package router_test

import (
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
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
	// system prompt, the ids its metadata gives, and its keys.
	const (
		prompt  = "a system prompt kept to itself"
		message = "a message kept to itself"
		device  = "device-kept-to-itself"
		apiKey  = "test-api-key-kept-to-itself"
	)
	body := `{"model":"claude-opus-5-5","max_tokens":32000,"system":"` + prompt + `","messages":[{"role":"user","content":"` + message + `"}],` +
		`"metadata":{"user_id":"user_` + device + `_account_` + sessionID + `"},"stream":true}`
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
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, kept := range []string{prompt, message, device, workToken, apiKey} {
		if strings.Contains(string(data), kept) {
			t.Errorf("the ledger holds\n%s\nwant nothing of %q", data, kept)
		}
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
