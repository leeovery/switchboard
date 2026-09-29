package router

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
)

func TestAThrottledRequestIsSentAgainOnItsAccountAfterAPause(t *testing.T) {
	tests := []struct {
		name string
		// answers are work's, in order.
		answers []answer
		// goneAfter is when the client goes, if it does.
		goneAfter time.Duration
		wantWait  time.Duration
		// wantStatus is what the client is answered, or zero when it's gone.
		wantStatus int
		wantSent   int
		wantLog    [][]string
	}{
		{
			name:       "after the wait the upstream asks for",
			answers:    []answer{throttled("3"), served},
			wantWait:   3 * time.Second,
			wantStatus: http.StatusOK,
			wantSent:   2,
			wantLog: [][]string{
				{"level=WARN", "msg=throttled", "account=work", "wait=3s"},
				{"level=INFO", "msg=replaying", "attempt=2", "from=work", "to=work", `why="was throttled"`},
				{"level=INFO", "msg=routed", "account=work", "status=200", "attempts=2"},
			},
		},
		{
			name:       "after two seconds when it asks for none",
			answers:    []answer{throttled(""), served},
			wantWait:   2 * time.Second,
			wantStatus: http.StatusOK,
			wantSent:   2,
			wantLog:    [][]string{{"level=WARN", "msg=throttled", "account=work", "wait=2s"}},
		},
		{
			name:       "after ten seconds at most",
			answers:    []answer{throttled("60"), served},
			wantWait:   10 * time.Second,
			wantStatus: http.StatusOK,
			wantSent:   2,
			wantLog:    [][]string{{"level=WARN", "msg=throttled", "account=work", "wait=10s"}},
		},
		{
			name:       "twice at most, after which the 429 is the client's",
			answers:    []answer{throttled("1"), throttled("1"), throttled("1"), served},
			wantWait:   2 * time.Second,
			wantStatus: http.StatusTooManyRequests,
			wantSent:   3,
			wantLog: [][]string{
				{"level=INFO", "msg=replaying", "attempt=3", "from=work", "to=work", `why="was throttled"`},
				{"level=WARN", `msg="still throttled; passing the answer on"`, "account=work", "attempts=3"},
				{"level=INFO", "msg=routed", "account=work", "status=429", "attempts=3"},
			},
		},
		{
			name:       "unless the client goes first",
			answers:    []answer{throttled("5"), served},
			goneAfter:  time.Second,
			wantWait:   time.Second,
			wantStatus: 0,
			wantSent:   1,
			wantLog:    [][]string{{"level=INFO", "msg=routed", "account=work", "status=0", "canceled=true"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				log := logstest.Capture(t)
				// The session is on work. Side has room too, and its quota
				// needs using sooner, but throttling never moves a session.
				r := newTestRouter(t, at(start), &stubProber{})
				r.state.record("work", []quota.Window{session, laterWeek}, fromResponse)
				r.state.record("side", []quota.Window{session, soonWeek}, fromResponse)
				r.sessions.remember(key{session: "one", model: opus}, "", decision{account: "work", reason: reasonNew}, start)
				upstream := &scriptedUpstream{answers: map[string][]answer{workToken: tt.answers, sideToken: {served}}}
				r.proxy.transport = upstream
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				if tt.goneAfter > 0 {
					time.AfterFunc(tt.goneAfter, cancel)
				}

				began := time.Now()
				status := routeAlone(ctx, r, "one")
				if waited := time.Since(began); waited != tt.wantWait {
					t.Errorf("the request took %v, want %v", waited, tt.wantWait)
				}
				if status != tt.wantStatus {
					t.Errorf("answered %d, want %d", status, tt.wantStatus)
				}
				counted := 1
				if tt.wantStatus == 0 {
					counted = 0
				}
				if got := r.health.report().Requests; got != counted {
					t.Errorf("the router's health counts %d requests, want %d: a request counts once it's answered", got, counted)
				}
				if got, want := upstream.sent(), slices.Repeat([]string{"work"}, tt.wantSent); !slices.Equal(got, want) {
					t.Errorf("the request went out on %q, want %q: throttling never moves it", got, want)
				}
				if got, _, _ := r.sessions.lookup(key{session: "one", model: opus}); got.Account != "work" {
					t.Errorf("the session is on %s, want work, where it was", got.Account)
				}
				for _, want := range tt.wantLog {
					if !log.Has(want...) {
						t.Errorf("log reads\n%s\nwant a line with %q", log, want)
					}
				}
				if log.Has("msg=moved") {
					t.Errorf("log reads\n%s\nwant no move", log)
				}
			})
		})
	}
}

func TestEachAccountARequestMovesToThrottlesItAfresh(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newTestRouter(t, at(start), &stubProber{})
		r.state.record("work", []quota.Window{session, laterWeek}, fromResponse)
		r.state.record("side", []quota.Window{session, soonWeek}, fromResponse)
		r.sessions.remember(key{session: "one", model: opus}, "", decision{account: "work", reason: reasonNew}, start)
		upstream := &scriptedUpstream{answers: map[string][]answer{
			workToken: {throttled("1"), throttled("1"), limitHit},
			sideToken: {throttled("1"), throttled("1"), served},
		}}
		r.proxy.transport = upstream

		began := time.Now()
		status := routeAlone(t.Context(), r, "one")
		if waited := time.Since(began); waited != 4*time.Second {
			t.Errorf("the request took %v, want 4s: two pauses on each account", waited)
		}
		if status != http.StatusOK {
			t.Errorf("answered %d, want side's 200", status)
		}
		if got, want := upstream.sent(), []string{"work", "work", "work", "side", "side", "side"}; !slices.Equal(got, want) {
			t.Errorf("the request went out on %q, want %q", got, want)
		}
	})
}

func TestHoldKeepsAnAnswersBodyWholeOrNotAtAll(t *testing.T) {
	tests := []struct {
		name string
		size int
		want bool
	}{
		{name: "a body well within the cap", size: 90, want: true},
		{name: "a body at the cap", size: maxDiscard, want: true},
		{name: "a body over the cap", size: maxDiscard + 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sent := strings.Repeat("x", tt.size)
			body := &closing{Reader: strings.NewReader(sent)}
			resp := respond(httptest.NewRequest(http.MethodPost, "/v1/messages", nil), http.StatusTooManyRequests, http.Header{"Retry-After": {"30"}}, "")
			resp.Body = body

			held, ok := hold(resp)
			if !body.closed {
				t.Error("the answer's body is still open, want it closed")
			}
			if ok != tt.want {
				t.Fatalf("hold() kept the answer: %v, want %v", ok, tt.want)
			}
			if !ok {
				return
			}
			if got := read(t, held.Body); got != sent || held.ContentLength != int64(tt.size) {
				t.Errorf("the answer kept has a body of %d bytes, length %d, want the %d read", len(got), held.ContentLength, tt.size)
			}
			if held.StatusCode != http.StatusTooManyRequests || held.Header.Get("Retry-After") != "30" {
				t.Errorf("the answer kept is %d with header %v, want it as it came", held.StatusCode, held.Header)
			}
		})
	}
}

// closing is a body that notes whether it was closed.
type closing struct {
	io.Reader
	closed bool
}

func (c *closing) Close() error {
	c.closed = true
	return nil
}

// answer is an upstream's answer to a request.
type answer func(r *http.Request) *http.Response

// served answers with a 200.
func served(r *http.Request) *http.Response {
	return respond(r, http.StatusOK, http.Header{}, `{"type":"message"}`)
}

// limitHit answers with a 429 saying the account's limit is reached.
func limitHit(r *http.Request) *http.Response {
	h := http.Header{"Anthropic-Ratelimit-Unified-Status": {"rejected"}}
	return respond(r, http.StatusTooManyRequests, h, `{"type":"error","error":{"type":"rate_limit_error","message":"You've hit your limit"}}`)
}

// throttled answers with a 429 that throttles the account, asking for a wait
// of retryAfter seconds, or none when it's empty.
func throttled(retryAfter string) answer {
	return func(r *http.Request) *http.Response {
		h := http.Header{}
		if retryAfter != "" {
			h.Set("Retry-After", retryAfter)
		}
		return respond(r, http.StatusTooManyRequests, h, `{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`)
	}
}

func respond(r *http.Request, status int, h http.Header, body string) *http.Response {
	return &http.Response{
		Status:        http.StatusText(status),
		StatusCode:    status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        h,
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       r,
	}
}

// scriptedUpstream is the upstream as a transport, for tests that can't reach
// it over a network, as those whose clock is synctest's can't: it answers
// each request, by the token it carries, with the account's next answer, the
// last answering every request after, and notes the account each went out
// on.
type scriptedUpstream struct {
	mu       sync.Mutex
	answers  map[string][]answer
	accounts []string
}

func (u *scriptedUpstream) RoundTrip(r *http.Request) (*http.Response, error) {
	_ = r.Body.Close()
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	u.mu.Lock()
	defer u.mu.Unlock()
	u.accounts = append(u.accounts, map[string]string{workToken: "work", sideToken: "side"}[token])
	answers := u.answers[token]
	if len(answers) > 1 {
		u.answers[token] = answers[1:]
	}
	return answers[0](r), nil
}

// sent returns the account each request went out on, in order.
func (u *scriptedUpstream) sent() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return slices.Clone(u.accounts)
}

// routeAlone has the router's proxy route a messages request of session on
// work's token, and returns the status it answered with, or zero when it
// answered nothing, every answer here having a body.
func routeAlone(ctx context.Context, r *Router, session string) int {
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"`+opus+`","max_tokens":1}`))
	req.Header.Set("Authorization", "Bearer "+workToken)
	req.Header.Set(claude.SessionHeader, session)
	rec := httptest.NewRecorder()
	r.Proxy().ServeHTTP(rec, req)
	if rec.Body.Len() == 0 {
		return 0
	}
	return rec.Code
}
