package router_test

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude/claudetest"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/router"
)

func TestClientStream(t *testing.T) {
	texts := []string{"Hello, ", "world"}
	up := newUpstream(t, answerStreamed(claudetest.AnswerPieces(texts...)...))
	rt := newRouter(t, up.URL)
	ctx, stop := context.WithTimeout(t.Context(), 10*time.Second)
	defer stop()
	events, err := router.NewClient(serveControl(t, rt)).Stream(ctx)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	readAll(t, send(t, http.MethodPost, serveProxy(t, rt)+"/v1/messages", claudeCode(workToken), strings.NewReader(messages)))
	var got []router.StreamEvent
	for e := range events {
		if e.Kind != router.StreamProgress {
			got = append(got, e)
		}
		if e.Kind == router.StreamDone {
			break
		}
	}
	if len(got) == 0 {
		t.Fatal("the stream closed before telling of the request")
	}
	// Nothing has been read of any account, so the request goes out on the
	// client's own.
	request := router.StreamEvent{At: now, Request: got[0].Request, Attempt: 1, Session: sessionID, Model: opus, Account: "work"}
	want := []router.StreamEvent{as(request, router.StreamSent), as(request, router.StreamFirst), as(request, router.StreamDone)}
	want[2].Status, want[2].Chars, want[2].Tokens = http.StatusOK, claudetest.AnswerChars(texts...), &claudetest.AnswerTokens
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Stream() told of\n%+v\nwant\n%+v", got, want)
	}

	stop()
	select {
	case _, open := <-events:
		if open {
			t.Error("Stream() told of more once its context ended, want its events closed")
		}
	case <-time.After(5 * time.Second):
		t.Error("Stream()'s events are still open once its context ended, want them closed")
	}
}

func TestClientStreamOpensWithTheRequestsInFlight(t *testing.T) {
	upstream, arrived, release := holdingUpstream(t)
	defer release()
	rt := newRouter(t, upstream)
	client := router.NewClient(serveControl(t, rt))
	answered := make(chan string, 1)
	go func() { answered <- post(serveProxy(t, rt) + "/v1/messages") }()
	<-arrived

	events, err := client.Stream(t.Context())
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	opening := <-events
	release()
	want := router.StreamEvent{At: now, Kind: router.StreamInFlight, Request: opening.Request, Attempt: 1, Session: sessionID, Model: opus, Account: "work", SentAt: now}
	if !reflect.DeepEqual(opening, want) {
		t.Errorf("Stream() opened with %+v, want %+v", opening, want)
	}
	if got := <-answered; got != `200 {"type":"message"}` {
		t.Errorf("the request in flight was answered %q, want 200 and its body", got)
	}
}

func TestASessionOrModelTooLongIsCutShortAndCutsNoReaderOff(t *testing.T) {
	up := newUpstream(t, answerOK)
	rt := newRouter(t, up.URL)
	ctx, stop := context.WithTimeout(t.Context(), 10*time.Second)
	defer stop()
	events, err := router.NewClient(serveControl(t, rt)).Stream(ctx)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	// A session's id of 90 KB, of characters of three bytes each, and a
	// model's of 100 KB.
	session, model := strings.Repeat("☃", 30_000), strings.Repeat("m", 100_000)
	body := `{"model":"` + model + `","max_tokens":1,"messages":[{"role":"user","content":"hello"}]}`

	readAll(t, send(t, http.MethodPost, serveProxy(t, rt)+"/v1/messages", with(claudeCode(workToken), "X-Claude-Code-Session-Id", session), strings.NewReader(body)))
	cut := router.StreamEvent{Session: strings.Repeat("☃", 66), Model: strings.Repeat("m", 200)}
	done := false
	for e := range events {
		if e.Session != cut.Session || e.Model != cut.Model {
			t.Errorf("the stream told of %s's session and model as %d and %d bytes, want them cut to 200 bytes at most, at the end of a character", e.Kind, len(e.Session), len(e.Model))
		}
		if done = e.Kind == router.StreamDone; done {
			break
		}
	}
	if !done {
		t.Fatal("the stream ended before telling of the request done, want it read on")
	}
	started := rt.Status().Events
	if len(started) != 1 || started[0].Session != cut.Session || started[0].Model != cut.Model {
		t.Errorf("the events are %+v, want the session started, its id and model cut short", started)
	}
}

func TestClientStreamReadsALineLongerThanTheControlAPITakes(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "control.sock")
	long := router.StreamEvent{Kind: router.StreamMoved, Request: "1", Reason: strings.Repeat("x", 100<<10)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /stream", func(w http.ResponseWriter, _ *http.Request) {
		lines := json.NewEncoder(w)
		_ = lines.Encode(long)
		_ = lines.Encode(router.StreamEvent{Kind: router.StreamDone, Request: "1"})
	})
	serveOn(t, path, mux)

	events, err := router.NewClient(path).Stream(t.Context())
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	var got []string
	for e := range events {
		got = append(got, e.Kind)
	}
	if want := []string{router.StreamMoved, router.StreamDone}; !slices.Equal(got, want) {
		t.Errorf("Stream() told of %q, want %q: a line of 100 KiB read", got, want)
	}
}

func TestClientStreamOfARouterFromBeforeIt(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "control.sock")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `{"ok": true}`) })
	serveOn(t, path, mux)

	if _, err := router.NewClient(path).Stream(t.Context()); !errors.Is(err, router.ErrNoStream) {
		t.Errorf("Stream() error = %v, want ErrNoStream", err)
	}
}

func TestTheRequestStreamLeavesTheAnswerAsItCame(t *testing.T) {
	plain := strings.Join(claudetest.AnswerPieces(slices.Repeat([]string{"some text of an answer "}, 2000)...), "")
	tests := []struct {
		name     string
		encoding string
		body     []byte
		// reading is how the stream is read as the answer comes, if it is.
		reading func(t *testing.T, control string)
	}{
		{name: "without a reader", body: []byte(plain)},
		{name: "with a reader", body: []byte(plain), reading: readAlong},
		{name: "with a reader that never reads", body: []byte(plain), reading: neverRead},
		{name: "encoded, without a reader", encoding: "gzip", body: gzipBody(t, plain)},
		{name: "encoded, with a reader", encoding: "gzip", body: gzipBody(t, plain), reading: readAlong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			up := newUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				if tt.encoding != "" {
					w.Header().Set("Content-Encoding", tt.encoding)
				}
				for piece := range slices.Chunk(tt.body, 1000) {
					_, _ = w.Write(piece)
					_ = http.NewResponseController(w).Flush()
				}
			})
			rt := newRouter(t, up.URL)
			if tt.reading != nil {
				tt.reading(t, serveControl(t, rt))
			}

			resp := send(t, http.MethodPost, serveProxy(t, rt)+"/v1/messages", claudeCode(workToken), strings.NewReader(messages))
			got := readAll(t, resp)
			if got != string(tt.body) || resp.Header.Get("Content-Encoding") != tt.encoding || resp.Header.Get("Content-Type") != "text/event-stream" {
				t.Errorf("the client got %d bytes, %q encoded %q, that differ from the %d the upstream sent, %q encoded %q",
					len(got), resp.Header.Get("Content-Type"), resp.Header.Get("Content-Encoding"), len(tt.body), "text/event-stream", tt.encoding)
			}
		})
	}
}

func TestARoutedRequestAsksForAnAnswerTheRouterCanCount(t *testing.T) {
	gzipped := gzipBody(t, strings.Join(claudetest.AnswerPieces("Hello"), ""))
	up := newUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(gzipped)
	})
	rt := newRouter(t, up.URL)
	ctx, stop := context.WithTimeout(t.Context(), 10*time.Second)
	defer stop()
	events, err := router.NewClient(serveControl(t, rt)).Stream(ctx)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	proxy := serveProxy(t, rt)
	offered := with(claudeCode(workToken), "Accept-Encoding", "gzip, deflate, br, zstd")

	resp := send(t, http.MethodPost, proxy+"/v1/messages", offered, strings.NewReader(messages))
	if got := readAll(t, resp); got != string(gzipped) || resp.Header.Get("Content-Encoding") != "gzip" {
		t.Errorf("the client got %d bytes encoded %q, want the %d the upstream sent, encoded gzip", len(got), resp.Header.Get("Content-Encoding"), len(gzipped))
	}
	readAll(t, send(t, http.MethodPost, proxy+"/v1/files", offered, strings.NewReader("a file")))
	sent := up.all()
	if len(sent) != 2 {
		t.Fatalf("%d requests reached the upstream, want 2", len(sent))
	}
	if got := sent[0].header.Get("Accept-Encoding"); got != "gzip, deflate" {
		t.Errorf("the routed request went upstream accepting %q, want gzip, deflate: those of its encodings the router can count an answer in", got)
	}
	if got := sent[1].header.Get("Accept-Encoding"); got != "gzip, deflate, br, zstd" {
		t.Errorf("the request passed through went upstream accepting %q, want it as it came", got)
	}
	for e := range events {
		if e.Kind == router.StreamDone {
			if e.Chars != claudetest.AnswerChars("Hello") {
				t.Errorf("done told of %d characters, want the %d of the answer, counted", e.Chars, claudetest.AnswerChars("Hello"))
			}
			break
		}
	}
}

func TestTheRequestStreamLeavesAConnectionSwitchedToAnotherProtocolAlone(t *testing.T) {
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
	rt := newRouter(t, up.URL)
	readAlong(t, serveControl(t, rt))
	header := with(with(claudeCode(workToken), "Connection", "Upgrade"), "Upgrade", "websocket")

	resp := send(t, http.MethodPost, serveProxy(t, rt)+"/v1/messages", header, strings.NewReader(messages))
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status = %d, want 101", resp.StatusCode)
	}
	tunnel, ok := resp.Body.(io.ReadWriteCloser)
	if !ok {
		t.Fatalf("a 101's body is a %T, want the connection", resp.Body)
	}
	if _, err := io.WriteString(tunnel, "ping\n"); err != nil {
		t.Fatalf("write through the switched connection: %v", err)
	}
	if got, err := bufio.NewReader(tunnel).ReadString('\n'); err != nil || got != "echo ping\n" {
		t.Errorf("read %q (%v) through the switched connection, want %q", got, err, "echo ping\n")
	}
}

func TestAReaderThatNeverReadsIsDroppedAndHoldsNothingUp(t *testing.T) {
	log := logstest.Capture(t)
	pieces := claudetest.AnswerPieces("Hello")
	up := newUpstream(t, answerStreamed(pieces...))
	rt := newRouter(t, up.URL)
	control := serveControl(t, rt)
	neverRead(t, control)
	events, err := router.NewClient(control).Stream(t.Context())
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	proxy := serveProxy(t, rt)

	// Each request is told of three times at least: as it's sent, its first
	// byte and done. The reader that never reads takes what its connection
	// holds, then falls behind.
	const requests = 300
	done := make(chan struct{}, requests)
	go func() {
		for e := range events {
			if e.Kind == router.StreamDone {
				done <- struct{}{}
			}
		}
	}()
	for range requests {
		if got := readAll(t, send(t, http.MethodPost, proxy+"/v1/messages", claudeCode(workToken), strings.NewReader(messages))); got != strings.Join(pieces, "") {
			t.Fatalf("a request was answered %q, want the answer as it came", got)
		}
	}
	for i := range requests {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("the reader that reads along was told of %d requests done, want all %d", i, requests)
		}
	}
	waitForLine(t, log, "level=INFO", `msg="dropped a reader of the request stream, as it fell behind"`)
	if strings.Count(log.String(), "dropped a reader") != 1 {
		t.Errorf("log reads\n%s\nwant the reader that never reads dropped once, and the one that reads along kept", log)
	}
}

func TestTheRequestStreamTellsOfTheRequestsARestartFinishes(t *testing.T) {
	log := logstest.Capture(t)
	s := newSelfWatching(t, true)
	var arrived <-chan struct{}
	var release func()
	s.cfg.Upstream, arrived, release = holdingUpstream(t)
	defer release()
	execs := replacing(&s.cfg)
	r := startRouter(t, s.cfg)
	client := router.NewClient(router.SocketPath(s.cfg.StateDir))
	events, err := client.Stream(t.Context())
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	answered := make(chan string, 1)
	go func() { answered <- post("http://" + s.cfg.Listen + "/v1/messages") }()
	<-arrived
	if e := <-events; e.Kind != router.StreamSent {
		t.Fatalf("the stream told of %+v, want the request sent", e)
	}

	if _, err := client.Restart(t.Context()); err != nil {
		t.Fatalf("Restart() error = %v", err)
	}
	waitForLine(t, log, "level=INFO", "msg=stopping")
	// A reader joining as the router finishes its requests is told of the
	// one in flight.
	joining, err := client.Stream(t.Context())
	if err != nil {
		t.Fatalf("Stream() as the router restarts: %v, want the requests it finishes", err)
	}
	if e := <-joining; e.Kind != router.StreamInFlight {
		t.Errorf("a reader joining as the router restarts was told of %+v, want the request in flight", e)
	}
	release()
	var told []string
	closed := time.After(10 * time.Second)
	for open := true; open; {
		select {
		case e, ok := <-events:
			if open = ok; ok {
				told = append(told, e.Kind)
			}
		case <-closed:
			t.Fatal("the stream is still open once the router has finished its requests, want it closed as its control API goes")
		}
	}
	if !slices.Contains(told, router.StreamDone) {
		t.Errorf("as the router restarted, the stream told of %q, want the request it finished done, before it closed", told)
	}
	if got := <-answered; got != `200 {"type":"message"}` {
		t.Errorf("the request in flight was answered %q, want 200 and its body", got)
	}
	r.waitForExit(t)
	execs.only(t)
}

// as is the event of the kind given telling of request.
func as(request router.StreamEvent, kind string) router.StreamEvent {
	request.Kind = kind
	return request
}

// answerStreamed answers with a stream of events, its pieces, each flushed as
// it's written.
func answerStreamed(pieces ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, piece := range pieces {
			_, _ = io.WriteString(w, piece)
			_ = http.NewResponseController(w).Flush()
		}
	}
}

// readAlong reads the request stream of the router whose control socket is at
// control, as it comes, until the test ends.
func readAlong(t *testing.T, control string) {
	t.Helper()
	events, err := router.NewClient(control).Stream(t.Context())
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	go func() {
		for range events {
		}
	}()
}

// neverRead opens the request stream of the router whose control socket is
// at control, and never reads it.
func neverRead(t *testing.T, control string) {
	t.Helper()
	conn, err := net.Dial("unix", control)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := io.WriteString(conn, "GET /stream HTTP/1.1\r\nHost: switchboard\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
}

// gzipBody is s compressed with gzip.
func gzipBody(t *testing.T, s string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err := io.WriteString(w, s); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
