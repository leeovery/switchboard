package router

import (
	"bufio"
	"cmp"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/claude/claudetest"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
)

// The user agent and betas of the requests the ledger's tests send, as
// Claude Code sends them.
const (
	testAgent = "claude-cli/2.1.300 (external, cli)"
	testBetas = "oauth-2025-04-20, context-1m-2025-08-07"
)

func TestTheLedgerHoldsALineOfEachRoutedRequest(t *testing.T) {
	// quotaCheck is Claude Code's quota check, on Claude Haiku.
	const quotaCheck = `{"model":"` + haiku + `","max_tokens":1,"messages":[{"role":"user","content":"quota"}]}`
	asked := ledger.Line{At: start, Kind: ledger.KindMessage, Session: "one", Model: opus, Account: "work", Reason: reasonSticky,
		Status: http.StatusOK, Attempts: 1, Agent: testAgent, Betas: []string{"oauth-2025-04-20", "context-1m-2025-08-07"},
		Shape: ledger.Shape{Bytes: len(opusAsked), MaxTokens: new(int64(1))}}
	// answered is what the answer AnswerPieces streams tells of itself, and
	// its usage, which Message's is too.
	answered := ledger.Reply{Answer: ledger.Answer{Model: opus, Stop: "end_turn"},
		Usage: json.RawMessage(`{"cache_creation_input_tokens":512,"cache_read_input_tokens":40000,"input_tokens":3,"output_tokens":120}`)}
	// limited is an answer's header as the API gives it, its id and its usage
	// headers, of a stream of events; and the limits a line has of them.
	limited := http.Header{"Content-Type": {"text/event-stream"}, "Request-Id": {"req_011CTest"},
		"Anthropic-Ratelimit-Unified-Status": {"allowed"}, "Anthropic-Ratelimit-Unified-5h-Utilization": {"0.23"}}
	limits := map[string]string{"status": "allowed", "5h-utilization": "0.23"}
	// unreadable is an answer encoded as the router can't decode.
	const unreadable = "\x1b\x2c\x00\xf8 brotli, notionally"
	tests := []struct {
		name string
		// path is the request's, /v1/messages unless it says, and body its
		// body, opusAsked unless it says, of session.
		path, body, session string
		// work and side are the answers of each, in turn.
		work, side []answer
		// goneAfter is when the client goes, if it does.
		goneAfter time.Duration
		want      ledger.Line
		// wantAnswer is the answer the client gets, where it matters.
		wantAnswer string
	}{
		{
			name:    "a streamed answer, its first byte a second on, its end five after",
			session: "one",
			work:    []answer{streamed(time.Second, claudetest.AnswerPieces("Hello")...)},
			want:    edited(asked, func(l *ledger.Line) { l.FirstMS, l.TotalMS, l.Reply = ms(1000), 6000, answered }),
		},
		{
			name:    "a message answered whole",
			session: "one",
			work:    []answer{messageWhole},
			want: edited(asked, func(l *ledger.Line) {
				l.FirstMS, l.Reply = ms(0), answered
				l.Answer.Blocks = map[string]int{"text": 1}
			}),
		},
		{
			name:    "an answer whose header gives its id and limits, which called a tool",
			session: "one",
			work: []answer{answeredAs(limited, claudetest.MessageStart+claudetest.BlockStart("tool_use", "Bash")+
				claudetest.InputDelta(`{"command":"ls"}`)+claudetest.MessageEnd)},
			want: edited(asked, func(l *ledger.Line) {
				l.FirstMS, l.Reply = ms(0), answered
				l.Answer.ID, l.Answer.Blocks, l.Answer.Tools, l.Limits = "req_011CTest", map[string]int{"tool_use": 1}, []string{"Bash"}, limits
			}),
		},
		{
			name:       "an answer in an encoding the router can't read, its header read alone",
			session:    "one",
			work:       []answer{answeredAs(with(limited, "Content-Encoding", "br"), unreadable)},
			want:       edited(asked, func(l *ledger.Line) { l.FirstMS, l.Answer.ID, l.Limits = ms(0), "req_011CTest", limits }),
			wantAnswer: unreadable,
		},
		{
			name:    "a limit reached, and the request moved to side with its session",
			session: "one",
			work:    []answer{limitHit},
			side:    []answer{served},
			want: edited(asked, func(l *ledger.Line) {
				l.Account, l.Reason, l.From, l.Attempts, l.FirstMS = "side", "moved: work hit its limit", "work", 2, ms(0)
				l.Tried = []ledger.Tried{{Account: "work", Why: whyLimit}}
			}),
		},
		{
			name:    "throttled, then answered on the same account",
			session: "one",
			work:    []answer{throttled("1"), served},
			want:    edited(asked, func(l *ledger.Line) { l.Attempts, l.FirstMS, l.TotalMS = 2, ms(1000), 1000 }),
		},
		{
			name:    "refused on every account, its session back where it was, and answered by the router",
			session: "one",
			work:    []answer{forbidden},
			side:    []answer{unauthorized},
			want: edited(asked, func(l *ledger.Line) {
				l.Account, l.Reason, l.Status, l.Attempts = "side", "moved: work was refused", http.StatusBadGateway, 2
				l.Tried = []ledger.Tried{{Account: "work", Why: whyRefused}, {Account: "side", Why: whyRefused}}
			}),
		},
		{
			name:    "the quota check, of a new session, which goes where side's quota needs using",
			body:    quotaCheck,
			session: "check",
			side:    []answer{served},
			want: edited(asked, func(l *ledger.Line) {
				l.Kind, l.Session, l.Model, l.Account, l.Reason, l.FirstMS = ledger.KindCheck, "check", haiku, "side", reasonNew, ms(0)
				l.Shape = ledger.Shape{Bytes: len(quotaCheck), Messages: 1, MaxTokens: new(int64(1))}
			}),
		},
		{
			name:    "a count of tokens",
			path:    "/v1/messages/count_tokens",
			session: "one",
			work:    []answer{countAnswered},
			want:    edited(asked, func(l *ledger.Line) { l.Kind, l.FirstMS = ledger.KindCount, ms(0) }),
		},
		{
			name:      "a client gone mid-answer, its answer read as far as it came",
			session:   "one",
			work:      []answer{streamed(time.Second, claudetest.AnswerPieces("Hello")...)},
			goneAfter: 1500 * time.Millisecond,
			want: edited(asked, func(l *ledger.Line) {
				l.Canceled, l.FirstMS, l.TotalMS, l.Answer = true, ms(1000), 1500, ledger.Answer{Model: opus}
			}),
		},
		{
			name:      "a client gone before an answer came",
			session:   "one",
			work:      []answer{throttled("5"), served},
			goneAfter: time.Second,
			want:      edited(asked, func(l *ledger.Line) { l.Status, l.Canceled, l.TotalMS = 0, true, 1000 }),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				// The session is on work, but for the quota check's, which is
				// new. Side has room too, and its quota needs using sooner.
				r := newTestRouter(t, at(start), &stubProber{})
				r.state.record("work", []quota.Window{session, laterWeek}, r.state.mark())
				r.state.record("side", []quota.Window{session, soonWeek}, r.state.mark())
				assign(r.sessions, key{session: "one", model: opus}, "", decision{account: "work", reason: reasonNew}, start)
				r.proxy.transport = &scriptedUpstream{answers: map[string][]answer{workToken: tt.work, sideToken: tt.side}}
				lines := keepingLedger(t, r)

				rec := routeAs(clientGoing(t, tt.goneAfter), r, cmp.Or(tt.path, "/v1/messages"), tt.session, cmp.Or(tt.body, opusAsked))
				if tt.wantAnswer != "" && rec.Body.String() != tt.wantAnswer {
					t.Errorf("the client got %q, want the answer as it came, %q", rec.Body.String(), tt.wantAnswer)
				}
				checkLines(t, lines(), tt.want)
			})
		})
	}
}

func TestTheLedgerHoldsALineOfARequestTheRouterAnswersItself(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Work and side keep a tenth of every window back, and are at their
		// reserves: work in its session, side in its week.
		accounts := []config.Account{
			{ID: "work", Label: "Work", Primary: true, Reserve: 0.1},
			{ID: "personal", Label: "Personal"},
			{ID: "side", Label: "Side", Reserve: 0.1},
		}
		r, err := New(Config{Accounts: accounts, Token: testTokens.Read, Upstream: "http://127.0.0.1:1", Provider: claude.Provider{},
			Prober: &stubProber{}, Policy: testPolicy, Now: at(start)})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		atReserve := session
		atReserve.Utilization = 0.95
		r.state.record("work", []quota.Window{atReserve, soonWeek}, r.state.mark())
		laterAtReserve := laterWeek
		laterAtReserve.Utilization = 0.95
		r.state.record("side", []quota.Window{session, laterAtReserve}, r.state.mark())
		upstream := scripted(served, served)
		r.proxy.transport = upstream
		lines := keepingLedger(t, r)

		if rec := routeAs(t.Context(), r, "/v1/messages", "one", opusAsked); rec.Code != http.StatusTooManyRequests {
			t.Fatalf("answered %d, want a 429 of the router's own", rec.Code)
		}
		if sent := upstream.sent(); len(sent) > 0 {
			t.Fatalf("the request went out on %q, want none", sent)
		}
		want := ledger.Line{At: start, Kind: ledger.KindMessage, Session: "one", Model: opus, Reason: reasonNoRoom,
			Status: http.StatusTooManyRequests, Agent: testAgent, Betas: []string{"oauth-2025-04-20", "context-1m-2025-08-07"},
			Shape: ledger.Shape{Bytes: len(opusAsked), MaxTokens: new(int64(1))}}
		checkLines(t, lines(), want)
	})
}

// messageWhole answers with a message whole, not streamed.
func messageWhole(r *http.Request) *http.Response {
	return respond(r, http.StatusOK, http.Header{"Content-Type": {"application/json"}}, claudetest.Message)
}

// answeredAs answers with the header and body given.
func answeredAs(h http.Header, body string) answer {
	return func(r *http.Request) *http.Response {
		return respond(r, http.StatusOK, h.Clone(), body)
	}
}

// with returns h with the header name set to value.
func with(h http.Header, name, value string) http.Header {
	h = h.Clone()
	h.Set(name, value)
	return h
}

// countAnswered answers a count of a request's tokens.
func countAnswered(r *http.Request) *http.Response {
	return respond(r, http.StatusOK, http.Header{"Content-Type": {"application/json"}}, `{"input_tokens":1234}`)
}

// edited returns line as change edits it.
func edited(line ledger.Line, change func(*ledger.Line)) ledger.Line {
	change(&line)
	return line
}

// ms is n milliseconds, as a line gives them where they may not be given.
func ms(n int64) *int64 {
	return new(n)
}

// routeAs has the router's proxy route a request to path, whose body is
// body, of session, on work's token, as Claude Code sends one, from a client
// whose context is ctx, and returns what it answered.
func routeAs(ctx context.Context, r *Router, path, session, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+workToken)
	req.Header.Set(claude.SessionHeader, session)
	req.Header.Set("User-Agent", testAgent)
	req.Header.Set("Anthropic-Beta", testBetas)
	rec := httptest.NewRecorder()
	r.Proxy().ServeHTTP(rec, req)
	return rec
}

// keepingLedger has the router keep its request ledger in a directory of the
// test's, and returns what reads back the lines it holds, once it has written
// every line noted to it, which stops it.
func keepingLedger(t *testing.T, r *Router) (lines func() []ledger.Line) {
	t.Helper()
	dir := t.TempDir()
	r.ledger.open(dir)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		r.ledger.run(ctx)
		close(done)
	}()
	return func() []ledger.Line {
		cancel()
		<-done
		return ledgerLines(t, dir)
	}
}

// ledgerLines returns the lines the request ledger's plain files in dir hold,
// each day's in the order written.
func ledgerLines(t testing.TB, dir string) []ledger.Line {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "requests-*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var lines []ledger.Line
	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			t.Fatal(err)
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			var line ledger.Line
			if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
				t.Fatalf("%s holds a line that isn't one: %v\n%s", filepath.Base(file), err, scanner.Bytes())
			}
			lines = append(lines, line)
		}
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
	}
	return lines
}

// checkLines checks the ledger holds the one line want, but for the request's
// id, which counts up from a random start.
func checkLines(t *testing.T, got []ledger.Line, want ledger.Line) {
	t.Helper()
	if len(got) != 1 {
		t.Fatalf("the ledger holds %d lines, want 1:\n%s", len(got), showLines(got))
	}
	want.Request = got[0].Request
	if want.Request == "" || !reflect.DeepEqual(got[0], want) {
		t.Errorf("the ledger holds\n%s\nwant\n%s", showLines(got), showLines([]ledger.Line{want}))
	}
}

// showLines shows lines as the ledger writes them, a line of JSON each, for a
// readable diff.
func showLines(lines []ledger.Line) string {
	var b strings.Builder
	encoder := json.NewEncoder(&b)
	for _, line := range lines {
		_ = encoder.Encode(line)
	}
	return b.String()
}
