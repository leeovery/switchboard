package router_test

import (
	"io"
	"net/http"
	"testing"

	"github.com/leeovery/switchboard/internal/claude/claudetest"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

func TestASessionsAssignmentSaysWhatItsRequestInFlightIsDoing(t *testing.T) {
	// The upstream holds each answer back until the test sends on answer,
	// then holds its end back until it sends on finish.
	answer, finish := make(chan struct{}), make(chan struct{})
	pieces := claudetest.AnswerPieces("Hello")
	up := newUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
		<-answer
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, pieces[0])
		_ = http.NewResponseController(w).Flush()
		<-finish
		for _, piece := range pieces[1:] {
			_, _ = io.WriteString(w, piece)
		}
	})
	rt := newRouter(t, up.URL)
	proxy, client := serveProxy(t, rt)+"/v1/messages", router.NewClient(serveControl(t, rt))
	events, err := client.Stream(t.Context())
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	await := func(kind string) {
		t.Helper()
		for e := range events {
			if e.Kind == kind {
				return
			}
		}
		t.Fatalf("the stream ended before telling of %s", kind)
	}
	doing := func(when, want string) {
		t.Helper()
		s, err := client.Session(t.Context(), sessionID)
		if err != nil {
			t.Fatalf("Session() error = %v", err)
		}
		listed, err := client.Sessions(t.Context())
		if err != nil || len(listed) != 1 {
			t.Fatalf("Sessions() = %+v, %v, want the session", listed, err)
		}
		if got := s.Assignments[0].InFlight; got != want {
			t.Errorf("%s, Session() says its request in flight is %q, want %q", when, got, want)
		}
		if got := listed[0].Assignments[0].InFlight; got != want {
			t.Errorf("%s, Sessions() says its request in flight is %q, want %q", when, got, want)
		}
	}
	posted := make(chan string, 1)

	go func() { posted <- post(proxy) }()
	answer <- struct{}{}
	finish <- struct{}{}
	<-posted
	await(router.StreamDone)
	doing("with none in flight", "")

	go func() { posted <- post(proxy) }()
	await(router.StreamSent)
	doing("waiting for its answer", status.Asking)
	answer <- struct{}{}
	await(router.StreamFirst)
	doing("as its answer streams", status.Answering)
	finish <- struct{}{}
	await(router.StreamDone)
	doing("once it's done", "")
	<-posted
}
