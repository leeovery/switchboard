package router_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/claude/claudetest"
	"github.com/leeovery/switchboard/internal/quota"
)

func BenchmarkProxyStreamed(b *testing.B) {
	pieces := claudetest.AnswerPieces("Hello, ", "world")
	benchmarkProxy(b, answerStreamed(pieces...), strings.Join(pieces, ""))
}

func BenchmarkProxyJSON(b *testing.B) {
	benchmarkProxy(b, answerMessage, claudetest.Message)
}

// benchmarkProxy times a messages request, as Claude Code sends one on a
// session under way, going through a running router's proxy to an upstream
// that answers it as answer does, reporting the account's usage in its
// headers as the API does, and back to a client that reads the answer whole,
// which is to be body.
//
// The router's clock stands still, so no request's result ever leaves its
// health's window, and each request scans them all: a request takes longer
// the more came before it. Compare runs of the same -benchtime count, such as
// 2000x.
func benchmarkProxy(b *testing.B, answer http.HandlerFunc, body string) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reportWindows(w.Header(), []quota.Window{session, week})
		answer(w, r)
	}))
	b.Cleanup(up.Close)
	cfg := runConfig(b, up.URL)
	cfg.Prober = readingEvery(session, week)
	runRouter(b, cfg)
	proxy := "http://" + cfg.Listen + "/v1/messages"
	// The session's first request, which chooses its account, goes untimed.
	post(proxy)
	want := "200 " + body
	b.ReportAllocs()
	for b.Loop() {
		if got := post(proxy); got != want {
			b.Fatalf("the proxy answered %q, want %q", got, want)
		}
	}
}

// answerMessage answers with a message whole, not streamed.
func answerMessage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, claudetest.Message)
}
