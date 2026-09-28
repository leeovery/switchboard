package router_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

func TestRoutedRequestGoesOutOnTheChosenAccount(t *testing.T) {
	up := newUpstream(t, answerOK)
	rt := newRouter(t, up.URL)
	chooser := &fixedChooser{account: "side"}
	rt.SetChooser(chooser)
	proxy := serveProxy(t, rt)

	resp := send(t, http.MethodPost, proxy+"/v1/messages?beta=true", claudeCode(workToken), strings.NewReader(messages))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	got := up.only(t)
	if got.method != http.MethodPost || got.path != "/v1/messages" || got.query != "beta=true" {
		t.Errorf("upstream got %s %s?%s, want POST /v1/messages?beta=true", got.method, got.path, got.query)
	}
	checkHeader(t, got, with(claudeCode(workToken), "Authorization", "Bearer "+sideToken))
	want := []router.Request{{Session: sessionID, Model: opus, Client: "work"}}
	if asked := chooser.requests(); !reflect.DeepEqual(asked, want) {
		t.Errorf("chooser was asked %+v, want %+v", asked, want)
	}
}

func TestQueriesGoUpstreamAsSent(t *testing.T) {
	for _, query := range []string{"beta=true", "beta=true;x=1", "beta=%zz", "b=2&a=1&a=0"} {
		t.Run(query, func(t *testing.T) {
			up := newUpstream(t, answerOK)
			proxy := serveProxy(t, newRouter(t, up.URL))

			send(t, http.MethodPost, proxy+"/v1/messages?"+query, claudeCode(workToken), strings.NewReader(messages))
			if got := up.only(t).query; got != query {
				t.Errorf("upstream got query %q, want %q", got, query)
			}
		})
	}
}

func TestPins(t *testing.T) {
	tokens := map[string]string{"work": workToken, "side": sideToken}
	// The router has read no account's usage, so a request without a pin
	// keeps to the client's own account.
	const unpinned = `reason="no account has room"`
	tests := []struct {
		name        string
		pin         string
		wantAccount string
		wantReason  string
		// wantWarning is a line the log should have, when the pin is ignored.
		wantWarning []string
	}{
		{name: "none leaves the choice to the scheduler", wantAccount: "work", wantReason: unpinned},
		{name: "to another account moves the request there", pin: "side", wantAccount: "side", wantReason: "reason=pinned"},
		{name: "to the client's own account", pin: "work", wantAccount: "work", wantReason: "reason=pinned"},
		{
			name:        "to an account there isn't is ignored",
			pin:         "nope",
			wantAccount: "work",
			wantReason:  unpinned,
			wantWarning: []string{"level=WARN", `msg="pin ignored: no such account"`, "pin=nope"},
		},
		{
			name:        "to an account without a token is ignored",
			pin:         "personal",
			wantAccount: "work",
			wantReason:  unpinned,
			wantWarning: []string{"level=WARN", `msg="pin ignored: account has no token"`, "pin=personal"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			up := newUpstream(t, answerOK)
			proxy := serveProxy(t, newRouter(t, up.URL))

			readAll(t, send(t, http.MethodPost, proxy+"/v1/messages", with(claudeCode(workToken), "X-Switchboard-Account", tt.pin), strings.NewReader(messages)))
			checkHeader(t, up.only(t), with(claudeCode(workToken), "Authorization", "Bearer "+tokens[tt.wantAccount]))
			waitForLine(t, log, "msg=routed", "account="+tt.wantAccount, tt.wantReason)
			if warned := log.Has("level=WARN"); warned != (tt.wantWarning != nil) {
				t.Errorf("log reads\n%s\nwant a warning: %v", log, tt.wantWarning != nil)
			}
			if tt.wantWarning != nil && !log.Has(tt.wantWarning...) {
				t.Errorf("log reads\n%s\nwant a line with %q", log, tt.wantWarning)
			}
		})
	}
}

func TestRequestsNotRoutedPassThroughUntouched(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		header http.Header
		body   string
		// wantHeader is the header the upstream should get, when it isn't header.
		wantHeader http.Header
	}{
		{
			name:   "the connectivity check, which has no token",
			method: http.MethodHead,
			path:   "/api/hello",
			header: http.Header{"User-Agent": {"claude-cli/2.1.300 (external, cli)"}},
		},
		{
			name:   "an identity-bound path, which keeps the client's token",
			method: http.MethodGet,
			path:   "/v1/code/sessions",
			header: claudeCode(workToken),
		},
		{
			name:   "a request that asks for no encoding, which isn't made to ask for one",
			method: http.MethodGet,
			path:   "/v1/code/sessions",
			header: with(claudeCode(workToken), "Accept-Encoding", ""),
		},
		{
			name:   "a file upload, which keeps the client's token",
			method: http.MethodPost,
			path:   "/v1/files",
			header: with(claudeCode(sideToken), "Content-Type", "application/octet-stream"),
			body:   "\x00\x01 a file",
		},
		{
			name:       "an identity-bound path that's pinned, which isn't moved",
			method:     http.MethodGet,
			path:       "/v1/code/sessions",
			header:     with(claudeCode(workToken), "X-Switchboard-Account", "side"),
			wantHeader: claudeCode(workToken),
		},
		{
			name:       "a batch that's pinned, which isn't moved from the account that made it",
			method:     http.MethodGet,
			path:       "/v1/messages/batches/msgbatch_01HkcTjaV5uDC8jWR4ZsDV8d",
			header:     with(claudeCode(workToken), "X-Switchboard-Account", "side"),
			wantHeader: claudeCode(workToken),
		},
		{
			name:   "a messages request with a token no account has",
			method: http.MethodPost,
			path:   "/v1/messages",
			header: claudeCode("test-token-unknown"),
			body:   messages,
		},
		{
			name:   "a messages request without a token",
			method: http.MethodPost,
			path:   "/v1/messages",
			header: with(claudeCode(workToken), "Authorization", ""),
			body:   messages,
		},
		{
			name:   "a messages request with an account's token as a key, not a bearer token",
			method: http.MethodPost,
			path:   "/v1/messages",
			header: with(with(claudeCode(workToken), "Authorization", ""), "X-Api-Key", workToken),
			body:   messages,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			up := newUpstream(t, answerWith(http.StatusUnauthorized, session, week))
			rt := newRouter(t, up.URL)
			proxy := serveProxy(t, rt)

			resp := send(t, tt.method, proxy+tt.path, tt.header, strings.NewReader(tt.body))
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("status = %d, want the upstream's 401", resp.StatusCode)
			}
			got := up.only(t)
			if got.method != tt.method || got.path != tt.path || string(got.body) != tt.body {
				t.Errorf("upstream got %s %s with body %q, want %s %s with %q", got.method, got.path, got.body, tt.method, tt.path, tt.body)
			}
			want := tt.header
			if tt.wantHeader != nil {
				want = tt.wantHeader
			}
			checkHeader(t, got, want)
			for _, account := range rt.Status().Accounts {
				if len(account.Windows) > 0 {
					t.Errorf("account %s has windows %+v, want none read off a request not routed", account.ID, account.Windows)
				}
			}
		})
	}
}

func TestBodiesGoUpstreamByteForByte(t *testing.T) {
	odd := `{ "model" : "claude-opus-5-5",` + "\n\t" + `"messages":[{"role":"user","content":"caf\u00e9 or café, \"q\" \\ ☃"}], "temperature":1.0e0 }  `
	large := `{"model":"claude-opus-5-5","messages":[{"role":"user","content":"` + strings.Repeat("0123456789abcdef", 1<<16) + `"}]}`
	tests := []struct {
		name  string
		token string
		body  string
		// chunked sends the body without a length.
		chunked bool
	}{
		{name: "routed", token: workToken, body: odd},
		{name: "routed, without a length", token: workToken, body: odd, chunked: true},
		{name: "routed, a megabyte", token: workToken, body: large},
		{name: "passed through, without a length", token: "test-token-unknown", body: odd, chunked: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			up := newUpstream(t, answerOK)
			proxy := serveProxy(t, newRouter(t, up.URL))
			var body io.Reader = strings.NewReader(tt.body)
			if tt.chunked {
				body = io.MultiReader(body)
			}

			send(t, http.MethodPost, proxy+"/v1/messages", claudeCode(tt.token), body)
			if got := up.only(t).body; string(got) != tt.body {
				t.Errorf("upstream got a body of %d bytes that differs from the %d sent", len(got), len(tt.body))
			}
		})
	}
}

func TestStreamsPassThroughAsTheyArrive(t *testing.T) {
	const first, second = "event: message_start\ndata: {}\n\n", "event: message_stop\ndata: {}\n\n"
	read := make(chan struct{})
	up := newUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, first)
		_ = http.NewResponseController(w).Flush()
		select {
		case <-read:
		case <-time.After(5 * time.Second):
			t.Error("the client never got the first event while the stream was open: the proxy held it back")
		}
		_, _ = io.WriteString(w, second)
	})
	proxy := serveProxy(t, newRouter(t, up.URL))

	resp := send(t, http.MethodPost, proxy+"/v1/messages", claudeCode(workToken), strings.NewReader(messages))
	events := bufio.NewReader(resp.Body)
	if got := readEvent(t, events); got != first {
		t.Fatalf("first event = %q, want %q", got, first)
	}
	close(read)
	if rest, err := io.ReadAll(events); err != nil || string(rest) != second {
		t.Errorf("rest of the stream = %q (%v), want %q", rest, err, second)
	}
}

// readEvent reads one server-sent event, up to and including the blank line
// that ends it.
func readEvent(t *testing.T, r *bufio.Reader) string {
	t.Helper()
	var event strings.Builder
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("read event: %v, after %q", err, event.String()+line)
		}
		event.WriteString(line)
		if line == "\n" {
			return event.String()
		}
	}
}

func TestUsageIsReadOffResponses(t *testing.T) {
	spent := session
	spent.Utilization, spent.Status = 1, quota.StatusRejected
	tests := []struct {
		name    string
		status  int
		windows []quota.Window
	}{
		{name: "a 200", status: http.StatusOK, windows: []quota.Window{session, week}},
		{name: "a 429", status: http.StatusTooManyRequests, windows: []quota.Window{spent, week}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			up := newUpstream(t, answerWith(tt.status, tt.windows...))
			rt := newRouter(t, up.URL)
			rt.SetChooser(&fixedChooser{account: "side"})
			proxy := serveProxy(t, rt)

			resp := send(t, http.MethodPost, proxy+"/v1/messages", claudeCode(workToken), strings.NewReader(messages))
			if resp.StatusCode != tt.status {
				t.Errorf("status = %d, want the upstream's %d", resp.StatusCode, tt.status)
			}
			doc := rt.Status()
			side, _ := doc.Account("side")
			want := status.Account{ID: "side", Label: "Side", TokenSet: true, FetchedAt: now, Windows: tt.windows}
			if !reflect.DeepEqual(side, want) {
				t.Errorf("side, which served the request, reads\n%+v\nwant\n%+v", side, want)
			}
			if work, _ := doc.Account("work"); len(work.Windows) > 0 {
				t.Errorf("work, whose token the client sent, reads windows %+v, want none", work.Windows)
			}
		})
	}
}

func TestRefusedTokensAreNeverRelayed(t *testing.T) {
	const planted = "sk-ant-oat01-planted_fake"
	for _, refusal := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(refusal), func(t *testing.T) {
			log := logstest.Capture(t)
			up := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
				refuseWith(refusal, "token "+bearerOf(r)+" refused, as was "+planted)(w, r)
			})
			cfg := testConfig(up.URL)
			cfg.Prober = readingEvery(session, week)
			proxy := serveProxy(t, newRouterFrom(t, cfg))

			resp := send(t, http.MethodPost, proxy+"/v1/messages", with(claudeCode(workToken), "X-Switchboard-Account", "side"), strings.NewReader(messages))
			body := readAll(t, resp)
			want := fmt.Sprintf(`{"type":"error","error":{"type":"api_error","message":"switchboard: the upstream refused account work (HTTP %d)"}}`+"\n", refusal)
			if resp.StatusCode != http.StatusBadGateway || resp.Header.Get("Content-Type") != "application/json" || body != want {
				t.Errorf("answered %d (%s) %s\nwant 502 (application/json) %s", resp.StatusCode, resp.Header.Get("Content-Type"), body, want)
			}
			if got := resp.Header.Values("X-Should-Retry"); !slices.Equal(got, []string{"false"}) {
				t.Errorf("X-Should-Retry = %q, want false: the same token would only be refused again", got)
			}
			if got := up.accounts(); !slices.Equal(got, []string{"side", "work"}) {
				t.Errorf("the request went out on %q, want side, its pin, then work", got)
			}
			for _, id := range []string{"side", "work"} {
				waitForLine(t, log, "level=WARN", `msg="upstream refused the account's token"`, "account="+id,
					fmt.Sprintf("status=%d", refusal), `error="token [redacted] refused, as was [redacted]"`)
			}
			waitForLine(t, log, "level=INFO", "msg=replaying", "attempt=2", "from=side", "to=work", `why="was refused"`)
			waitForLine(t, log, "level=WARN", `msg="no account left to try"`, "attempts=2")
			waitForLine(t, log, "level=INFO", "msg=routed", "account=work", `reason="pin yields: side was refused"`, "status=502", "attempts=2")
			for _, token := range []string{workToken, sideToken, planted} {
				if strings.Contains(body+log.String(), token) {
					t.Errorf("the answer or the log shows a token:\n%s\n%s", body, log)
				}
			}
		})
	}
}

func TestARefusalsReasonIsLoggedCutShort(t *testing.T) {
	log := logstest.Capture(t)
	up := newUpstream(t, refuseWith(http.StatusForbidden, strings.Repeat("x", 300)))
	proxy := serveProxy(t, newRouter(t, up.URL))

	readAll(t, send(t, http.MethodPost, proxy+"/v1/messages", claudeCode(workToken), strings.NewReader(messages)))
	waitForLine(t, log, "level=WARN", `msg="upstream refused the account's token"`, "error="+strings.Repeat("x", 200))
	if strings.Contains(log.String(), strings.Repeat("x", 201)) {
		t.Errorf("log reads\n%s\nwant the upstream's reason cut to 200 characters", log)
	}
}

// refuseWith answers every request with status, and an API error giving
// message as the reason.
func refuseWith(status int, message string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = fmt.Fprintf(w, `{"type":"error","error":{"type":"permission_error","message":%q}}`, message)
	}
}

func TestUpstreamFailuresGive502(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		header  http.Header
		body    string
		wantLog []string
	}{
		{
			name:    "routed",
			method:  http.MethodPost,
			path:    "/v1/messages",
			header:  claudeCode(workToken),
			body:    messages,
			wantLog: []string{"level=ERROR", `msg="upstream request failed"`, "account=work", `error="dial tcp`},
		},
		{
			name:    "passed through",
			method:  http.MethodHead,
			path:    "/api/hello",
			wantLog: []string{"level=ERROR", `msg="upstream request failed"`, "method=HEAD", "path=/api/hello", `error="dial tcp`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			gone := httptest.NewServer(http.NotFoundHandler())
			gone.Close()
			proxy := serveProxy(t, newRouter(t, gone.URL))

			resp := send(t, tt.method, proxy+tt.path, tt.header, strings.NewReader(tt.body))
			if resp.StatusCode != http.StatusBadGateway {
				t.Errorf("status = %d, want 502", resp.StatusCode)
			}
			if got := resp.Header.Values("X-Should-Retry"); got != nil {
				t.Errorf("X-Should-Retry = %q, want none: another try may well get through", got)
			}
			if tt.method != http.MethodHead {
				checkAPIError(t, readAll(t, resp), "api_error", "switchboard: the request to the upstream failed: dial tcp ")
			}
			waitForLine(t, log, tt.wantLog...)
		})
	}
}

func TestBodyOverTheCapGives413(t *testing.T) {
	const limit = 64 << 20
	up := newUpstream(t, answerOK)
	proxy := serveProxy(t, newRouter(t, up.URL))
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, proxy+"/v1/messages", io.LimitReader(zeros{}, limit+1))
	if err != nil {
		t.Fatal(err)
	}
	req.ContentLength = limit + 1
	req.Header = claudeCode(workToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST a body of 64 MiB and a byte: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", resp.StatusCode)
	}
	checkAPIError(t, readAll(t, resp), "request_too_large", "switchboard: the request body is over its limit of 64 MiB")
	if n := up.count(); n > 0 {
		t.Errorf("upstream received %d requests, want none", n)
	}
}

// zeros reads as endless zero bytes.
type zeros struct{}

func (zeros) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

// checkAPIError checks body is an error as the API shapes them, of the type
// given, whose message starts as given.
func checkAPIError(t *testing.T, body, kind, message string) {
	t.Helper()
	var got struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("answer %q isn't JSON: %v", body, err)
	}
	if got.Type != "error" || got.Error.Type != kind || !strings.HasPrefix(got.Error.Message, message) {
		t.Errorf("answer = %s, want an error of type %s whose message starts %q", body, kind, message)
	}
}

func TestUpgradesPassThrough(t *testing.T) {
	up := newUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
		conn, rw, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Errorf("upstream: hijack: %v", err)
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n")
		_ = rw.Flush()
		line, err := rw.ReadString('\n')
		if err != nil {
			return
		}
		_, _ = rw.WriteString("echo " + line)
		_ = rw.Flush()
	})
	proxy := serveProxy(t, newRouter(t, up.URL))
	header := http.Header{"Authorization": {"Bearer " + workToken}, "Connection": {"Upgrade"}, "Upgrade": {"websocket"}}

	resp := send(t, http.MethodGet, proxy+"/v1/sessions/ws", header, nil)
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status = %d, want 101", resp.StatusCode)
	}
	tunnel, ok := resp.Body.(io.ReadWriteCloser)
	if !ok {
		t.Fatalf("a 101's body is a %T, want the connection", resp.Body)
	}
	if _, err := io.WriteString(tunnel, "ping\n"); err != nil {
		t.Fatalf("write through the upgraded connection: %v", err)
	}
	if got, err := bufio.NewReader(tunnel).ReadString('\n'); err != nil || got != "echo ping\n" {
		t.Errorf("read %q (%v) through the upgraded connection, want %q", got, err, "echo ping\n")
	}
	if got := up.only(t).header.Get("Authorization"); got != "Bearer "+workToken {
		t.Errorf("upstream got Authorization %q, want the client's own", got)
	}
}

func TestRoutedRequestsAreLogged(t *testing.T) {
	log := logstest.Capture(t)
	up := newUpstream(t, answerWith(http.StatusOK, session))
	proxy := serveProxy(t, newRouter(t, up.URL))

	readAll(t, send(t, http.MethodPost, proxy+"/v1/messages", with(claudeCode(workToken), "X-Switchboard-Account", "side"), strings.NewReader(messages)))
	readAll(t, send(t, http.MethodHead, proxy+"/api/hello", nil, nil))

	waitForLine(t, log, "level=INFO", "msg=routed component=router")
	line := regexp.MustCompile(`level=INFO msg=routed component=router pid=\d+ id=[0-9a-f]{8} session=0b5c6f2e model=claude-opus-5-5 account=side reason=pinned status=200 duration=\S+$`)
	if !slices.ContainsFunc(log.Lines(), line.MatchString) {
		t.Errorf("log reads\n%s\nwant a line matching %s", log, line)
	}
	waitForLine(t, log, "level=DEBUG", `msg="passed through" component=router`, "method=HEAD path=/api/hello status=200")
	for _, token := range []string{workToken, sideToken} {
		if strings.Contains(log.String(), token) {
			t.Errorf("log shows a token:\n%s", log)
		}
	}
	if strings.Contains(log.String(), "max_tokens") {
		t.Errorf("log shows a body:\n%s", log)
	}
}

func TestRequestsTheClientAbandonsAreLoggedAsCanceled(t *testing.T) {
	log := logstest.Capture(t)
	up := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message_start\ndata: {}\n\n")
		_ = http.NewResponseController(w).Flush()
		<-r.Context().Done()
	})
	proxy := serveProxy(t, newRouter(t, up.URL))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, proxy+"/v1/messages", strings.NewReader(messages))
	if err != nil {
		t.Fatal(err)
	}
	req.Header = claudeCode(workToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	readEvent(t, bufio.NewReader(resp.Body))
	cancel()
	_ = resp.Body.Close()

	waitForLine(t, log, "level=INFO", "msg=routed", "account=work", "status=200", "canceled=true")
}

func TestAnUpstreamFailingMidStreamIsLogged(t *testing.T) {
	log := logstest.Capture(t)
	up := newUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message_start\ndata: {}\n\n")
		_ = http.NewResponseController(w).Flush()
		conn, _, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Errorf("upstream: hijack: %v", err)
			return
		}
		_ = conn.Close()
	})
	proxy := serveProxy(t, newRouter(t, up.URL))

	resp := send(t, http.MethodPost, proxy+"/v1/messages", claudeCode(workToken), strings.NewReader(messages))
	if _, err := io.ReadAll(resp.Body); err == nil {
		t.Error("the stream read to its end, want it cut short")
	}
	waitForLine(t, log, "level=WARN", "ReverseProxy read error during body copy", "component=router")
	waitForLine(t, log, "level=INFO", "msg=routed", "account=work", "status=200")
	if log.Has("canceled=true") {
		t.Errorf("log reads\n%s\nwant no word of the client going away: the upstream did", log)
	}
}

func TestAChoiceTheRouterCantSendOnKeepsToTheClientsAccount(t *testing.T) {
	log := logstest.Capture(t)
	up := newUpstream(t, answerOK)
	rt := newRouter(t, up.URL)
	rt.SetChooser(&fixedChooser{account: "personal"})
	proxy := serveProxy(t, rt)

	send(t, http.MethodPost, proxy+"/v1/messages", claudeCode(workToken), strings.NewReader(messages))
	checkHeader(t, up.only(t), claudeCode(workToken))
	waitForLine(t, log, "level=ERROR", `msg="chooser picked an account it can't send on; keeping the client's"`, "picked=personal")
}

// fixedChooser picks one account for every request, and notes what it's asked.
type fixedChooser struct {
	account string

	mu    sync.Mutex
	asked []router.Request
}

func (c *fixedChooser) Choose(_ context.Context, req router.Request) router.Choice {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.asked = append(c.asked, req)
	return router.Choice{Account: c.account, Reason: "fixed"}
}

func (c *fixedChooser) requests() []router.Request {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.asked)
}

// waitForLine waits a few seconds at most for the log to have a line with
// every one of parts: a request is logged as its handler returns, which can be
// just after its client has read the whole response.
func waitForLine(t *testing.T, log *logstest.Log, parts ...string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !log.Has(parts...) {
		if time.Now().After(deadline) {
			t.Fatalf("log reads\n%s\nwant a line with %q", log, parts)
		}
		time.Sleep(time.Millisecond)
	}
}
