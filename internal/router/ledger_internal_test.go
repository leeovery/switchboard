package router

import (
	"bufio"
	"cmp"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/claude/claudetest"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/tokens/tokenstest"
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
	asked := stickyLine()
	// answered is what the answer AnswerPieces streams tells of itself, and
	// its usage, which Message's is too.
	answered := ledger.Reply{Answer: ledger.Answer{Model: opus, Stop: "end_turn"}, Usage: claudetest.AnswerUsage}
	// limited is an answer's header as the API gives it, its id and its usage
	// headers, of a stream of events; and the limits a line has of them.
	limited := http.Header{"Content-Type": {"text/event-stream"}, "Request-Id": {"req_011CTest"},
		"Anthropic-Ratelimit-Unified-Status": {"allowed"}, "Anthropic-Ratelimit-Unified-5h-Utilization": {"0.23"}}
	limits := map[string]string{"status": "allowed", "5h-utilization": "0.23"}
	// unreadable is an answer encoded as the router can't decode.
	const unreadable = "\x1b\x2c\x00\xf8 brotli, notionally"
	// overflowing is a streamed answer longer than can wait to be counted.
	overflowing := claudetest.MessageStart + strings.Repeat(claudetest.TextDelta(strings.Repeat("x", 1000)), tapMost/1000) + claudetest.MessageEnd
	tests := []struct {
		name string
		// path is the request's, /v1/messages unless it says, and body its
		// body, opusAsked unless it says, of session.
		path, body, session string
		// work and side are the answers of each, in turn.
		work, side []answer
		// late is how long the counting of an answer takes to start, if it's
		// late.
		late time.Duration
		// goneAfter is when the client goes, if it does, and cutAfter when the
		// router cuts the request off as it stops, if it does.
		goneAfter, cutAfter time.Duration
		want                ledger.Line
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
			name:    "a message answered whole, its counting a second late, its first byte when it passed",
			session: "one",
			work:    []answer{messageWhole},
			late:    time.Second,
			want: edited(asked, func(l *ledger.Line) {
				l.FirstMS, l.Reply = ms(0), answered
				l.Answer.Blocks = map[string]int{"text": 1}
			}),
		},
		{
			name:    "a streamed answer, its counting late past its first byte, which a second on passed",
			session: "one",
			work:    []answer{streamed(time.Second, claudetest.AnswerPieces("Hello")...)},
			late:    2500 * time.Millisecond,
			want:    edited(asked, func(l *ledger.Line) { l.FirstMS, l.TotalMS, l.Reply = ms(1000), 6000, answered }),
		},
		{
			name:    "an answer its counting fell too far behind to count, its header read alone",
			session: "one",
			work:    []answer{answeredAs(limited, overflowing)},
			late:    time.Second,
			want:    edited(asked, func(l *ledger.Line) { l.FirstMS, l.Answer.ID, l.Limits = ms(0), "req_011CTest", limits }),
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
			name:    "a new session's request, moved off side at its limit, then failing on work, the session forgotten with its moves",
			session: "new",
			side:    []answer{limitHit},
			work:    []answer{serverError},
			want: edited(asked, func(l *ledger.Line) {
				l.Session, l.Reason, l.Status, l.Attempts, l.FirstMS = "new", "moved: side hit its limit", http.StatusInternalServerError, 2, ms(0)
				l.Tried = []ledger.Tried{{Account: "side", Why: whyLimit}}
			}),
		},
		{
			name:    "a limit reached, then a refusal on the account it went to, the limit's 429 the client's, as work was picked",
			session: "one",
			work:    []answer{limitHit},
			side:    []answer{forbidden},
			want: edited(asked, func(l *ledger.Line) {
				l.Status, l.Attempts, l.FirstMS, l.Limits = http.StatusTooManyRequests, 2, ms(0), map[string]string{"status": "rejected"}
				l.Tried = []ledger.Tried{{Account: "work", Why: whyLimit}, {Account: "side", Why: whyRefused}}
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
		{
			name:     "a message answered whole, cut off as the router stopped while its counting finished",
			session:  "one",
			work:     []answer{messageWhole},
			late:     time.Second,
			cutAfter: 500 * time.Millisecond,
			want: edited(asked, func(l *ledger.Line) {
				l.CutOff, l.FirstMS, l.Reply = true, ms(0), answered
				l.Answer.Blocks = map[string]int{"text": 1}
			}),
		},
		{
			name:      "a message answered whole, its client gone while its counting finished, then cut off as the router stopped",
			session:   "one",
			work:      []answer{messageWhole},
			late:      time.Second,
			goneAfter: 300 * time.Millisecond,
			cutAfter:  600 * time.Millisecond,
			want: edited(asked, func(l *ledger.Line) {
				l.Canceled, l.FirstMS, l.Reply = true, ms(0), answered
				l.Answer.Blocks = map[string]int{"text": 1}
			}),
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
				if tt.late > 0 {
					r.proxy.provider = slowCounting{late: tt.late}
				}
				lines := keepingLedger(t, r)
				client := cutOffAfter(t, clientGoing(t, tt.goneAfter), tt.cutAfter)

				rec := routeAs(client, r, cmp.Or(tt.path, "/v1/messages"), tt.session, cmp.Or(tt.body, opusAsked))
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

func TestALineOfAnAnswerHeldBackIsOfItsAccountAsItWasPicked(t *testing.T) {
	// Work refuses the request, which moves to side, at its limit, then to
	// personal, which refuses it too: side's 429 is the client's.
	held := edited(stickyLine(), func(l *ledger.Line) {
		l.Account, l.Reason = "side", "moved: work was refused"
		l.Tried = []ledger.Tried{{Account: "work", Why: whyRefused}, {Account: "side", Why: whyLimit}, {Account: "personal", Why: whyRefused}}
		l.Status, l.Attempts, l.FirstMS, l.Limits = http.StatusTooManyRequests, 3, ms(0), map[string]string{"status": "rejected"}
	})
	tests := []struct {
		name    string
		session string
		want    ledger.Line
	}{
		{name: "of a session on work, whose move to side stands", session: "one", want: edited(held, func(l *ledger.Line) { l.From = "work" })},
		{name: "of a new session, forgotten with its moves", session: "new", want: edited(held, func(l *ledger.Line) { l.Session = "new" })},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				// Personal has a token too. Session one is on work. Work's quota
				// needs using soonest, so a new session goes there, then side's.
				r := newTestRouterReading(t, at(start), &stubProber{}, tokenstest.Files{"work": workToken, "personal": personalToken, "side": sideToken}.Read)
				soonestWeek := soonWeek
				soonestWeek.ResetsAt = start.Add(12 * time.Hour)
				r.state.record("work", []quota.Window{session, soonestWeek}, r.state.mark())
				r.state.record("personal", []quota.Window{session, laterWeek}, r.state.mark())
				r.state.record("side", []quota.Window{session, soonWeek}, r.state.mark())
				assign(r.sessions, key{session: "one", model: opus}, "", decision{account: "work", reason: reasonNew}, start)
				r.proxy.transport = &scriptedUpstream{answers: map[string][]answer{workToken: {forbidden}, sideToken: {limitHit}, personalToken: {forbidden}}}
				lines := keepingLedger(t, r)

				if rec := routeAs(t.Context(), r, "/v1/messages", tt.session, opusAsked); rec.Code != http.StatusTooManyRequests {
					t.Fatalf("answered %d, want side's 429", rec.Code)
				}
				checkLines(t, lines(), tt.want)
			})
		})
	}
}

func TestALineKeepsAMoveAnotherRequestOfItsSessionStayedOn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newTestRouter(t, at(start), &stubProber{})
		r.state.record("work", []quota.Window{session, laterWeek}, r.state.mark())
		r.state.record("side", []quota.Window{session, soonWeek}, r.state.mark())
		assign(r.sessions, key{session: "one", model: opus}, "", decision{account: "work", reason: reasonNew}, start)
		// Work refuses the first request, which moves the session to side,
		// where the request is held until the second, which stays there, has
		// been served; then side refuses it too.
		release := make(chan struct{})
		r.proxy.transport = &scriptedUpstream{answers: map[string][]answer{workToken: {forbidden}, sideToken: {heldUntil(release, forbidden), served}}}
		lines := keepingLedger(t, r)

		answered := make(chan int, 1)
		go func() { answered <- routeAs(t.Context(), r, "/v1/messages", "one", opusAsked).Code }()
		synctest.Wait()
		second := routeAs(t.Context(), r, "/v1/messages", "one", opusAsked).Code
		close(release)
		if first := <-answered; first != http.StatusBadGateway || second != http.StatusOK {
			t.Fatalf("the requests were answered %d, then %d, want side's 200 for the second, then a 502 for the first, refused on every account", second, first)
		}
		stayed := edited(stickyLine(), func(l *ledger.Line) { l.Account, l.FirstMS = "side", ms(0) })
		refused := edited(stickyLine(), func(l *ledger.Line) {
			l.Account, l.Reason, l.From = "side", "moved: work was refused", "work"
			l.Tried = []ledger.Tried{{Account: "work", Why: whyRefused}, {Account: "side", Why: whyRefused}}
			l.Status, l.Attempts = http.StatusBadGateway, 2
		})
		checkLines(t, lines(), stayed, refused)
	})
}

func TestTheLedgerHoldsALineOfARequestWhoseBodyCouldntBeRead(t *testing.T) {
	betas := []string{"oauth-2025-04-20", "context-1m-2025-08-07"}
	tests := []struct {
		name string
		path string
		// body is the request's body, as its client, whose context is ctx,
		// sends it.
		body func(ctx context.Context) io.Reader
		// goneAfter is when the client goes, if it does, and cutAfter when the
		// router cuts the request off as it stops, if it does.
		goneAfter, cutAfter time.Duration
		want                ledger.Line
	}{
		{
			name: "a message whose body is over its cap",
			path: "/v1/messages",
			body: func(context.Context) io.Reader { return io.LimitReader(zeros{}, maxBody+1) },
			want: ledger.Line{At: start, Kind: ledger.KindMessage, Session: "one", Dir: "~/Code/project",
				Status: http.StatusRequestEntityTooLarge, Agent: testAgent, Betas: betas},
		},
		{
			name: "a count of tokens whose client went a second into sending its body",
			path: "/v1/messages/count_tokens",
			body: func(ctx context.Context) io.Reader {
				return &paced{ctx: ctx, pause: 2 * time.Second, pieces: []string{opusAsked}}
			},
			goneAfter: time.Second,
			want: ledger.Line{At: start, Kind: ledger.KindCount, Session: "one", Dir: "~/Code/project",
				Status: http.StatusBadRequest, Canceled: true, TotalMS: 1000, Agent: testAgent, Betas: betas},
		},
		{
			name: "a message whose body was still coming as the router cut it off",
			path: "/v1/messages",
			body: func(ctx context.Context) io.Reader {
				return &paced{ctx: ctx, pause: 2 * time.Second, pieces: []string{opusAsked}}
			},
			cutAfter: time.Second,
			want: ledger.Line{At: start, Kind: ledger.KindMessage, Session: "one", Dir: "~/Code/project",
				Status: http.StatusBadRequest, CutOff: true, TotalMS: 1000, Agent: testAgent, Betas: betas},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				log := logstest.Capture(t)
				r := newTestRouter(t, at(start), &stubProber{})
				upstream := scripted(served, served)
				r.proxy.transport = upstream
				reader := joined(t, r)
				written := keepingLedger(t, r)
				client := cutOffAfter(t, clientGoing(t, tt.goneAfter), tt.cutAfter)
				req := claudeCodeAsks(client, tt.path, "one", tt.body(client))
				req.Header.Set(DirHeader, "~/Code/project")

				rec := httptest.NewRecorder()
				r.Proxy().ServeHTTP(rec, req)
				if rec.Code != tt.want.Status {
					t.Errorf("answered %d, want %d, whoever is left to read it", rec.Code, tt.want.Status)
				}
				got := written()
				checkLines(t, got, tt.want)
				if sent := upstream.sent(); len(sent) > 0 {
					t.Errorf("the request went out on %q, want none", sent)
				}
				if told := heard(reader); len(told) > 0 {
					t.Errorf("the stream told of %+v, want nothing: the request never went upstream", told)
				}
				if counted := r.health.report().Requests; counted != 0 {
					t.Errorf("the router's health counts %d requests, want none: a body refused says nothing of the router", counted)
				}
				refused := []string{"level=WARN", `msg="request refused: body unread"`, "id=" + got[0].Request, "path=" + tt.path}
				if !log.Has(refused...) || log.Has("msg=routed") {
					t.Errorf("log reads\n%s\nwant the request refused, by its id, and no routed line", log)
				}
				for finish, want := range map[string]bool{"canceled=true": tt.want.Canceled, "cut_off=true": tt.want.CutOff} {
					if log.Has(append(refused, finish)...) != want {
						t.Errorf("log reads\n%s\nwant %s on the refusal: %v", log, finish, want)
					}
				}
			})
		})
	}
}

func TestABodyTheRouterCantReadIsAnsweredWhoeverIsLeftToReadIt(t *testing.T) {
	tests := []struct {
		name string
		// framing is the header that frames the request's body, of which the
		// client sends body, then does as then does, where it's given.
		framing string
		body    io.Reader
		then    func(*net.TCPConn) error
		// stop is set where the router stops as the client waits, cutting the
		// request off.
		stop bool
		// wantAnswer is the status of the answer that comes back on the wire,
		// 0 for none, and wantError the type of the error it gives.
		wantAnswer int
		wantError  string
		// want is the line's status, and how the request finished.
		want ledger.Line
	}{
		{
			name: "a body over the cap", framing: "Content-Length: " + strconv.Itoa(maxBody+1), body: io.LimitReader(zeros{}, maxBody+1),
			wantAnswer: http.StatusRequestEntityTooLarge, wantError: "request_too_large",
			want: ledger.Line{Status: http.StatusRequestEntityTooLarge},
		},
		{
			name: "a body framed wrong, its client still there", framing: "Transfer-Encoding: chunked", body: strings.NewReader("not a chunk's size\r\n"),
			wantAnswer: http.StatusBadRequest, wantError: "invalid_request_error",
			want: ledger.Line{Status: http.StatusBadRequest},
		},
		{
			name: "a client gone mid-body", framing: "Content-Length: 100", body: strings.NewReader("ten bytes."), then: (*net.TCPConn).Close,
			want: ledger.Line{Status: http.StatusBadRequest, Canceled: true},
		},
		{
			name: "a client that closed its side mid-body, still reading", framing: "Content-Length: 100", body: strings.NewReader("ten bytes."),
			then:       (*net.TCPConn).CloseWrite,
			wantAnswer: http.StatusBadRequest, wantError: "invalid_request_error",
			want: ledger.Line{Status: http.StatusBadRequest, Canceled: true},
		},
		{
			name: "a body still coming as the router stops", framing: "Content-Length: 100", body: strings.NewReader("ten bytes."), stop: true,
			want: ledger.Line{Status: http.StatusBadRequest, CutOff: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			r := newTestRouter(t, at(start), &stubProber{})
			r.cfg.DrainFor = 50 * time.Millisecond
			lines := keepingLedger(t, r)
			srv := newProxyServer(r.Proxy())
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			served := make(chan struct{})
			go func() {
				_ = srv.Serve(ln)
				close(served)
			}()
			conn, err := net.DialTCP("tcp", nil, ln.Addr().(*net.TCPAddr))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = conn.Close() })
			answers := bufio.NewReader(conn)
			// The client sends the body once the router asks for it, as
			// Expect: 100-continue has it, so the router is reading it as the
			// client goes on.
			header := "POST /v1/messages HTTP/1.1\r\nHost: switchboard\r\nAuthorization: Bearer " + workToken + "\r\nExpect: 100-continue\r\n" + tt.framing + "\r\n\r\n"
			if _, err := io.WriteString(conn, header); err != nil {
				t.Fatal(err)
			}
			if asked, err := http.ReadResponse(answers, nil); err != nil || asked.StatusCode != http.StatusContinue {
				t.Fatalf("the router answered the header with %v (%v), want it asking for the body", asked, err)
			}
			if _, err := io.Copy(conn, tt.body); err != nil {
				t.Fatal(err)
			}
			if tt.then != nil {
				if err := tt.then(conn); err != nil {
					t.Fatal(err)
				}
			}
			if tt.stop {
				r.drain(srv)
			}

			if status, kind := answerOn(answers); status != tt.wantAnswer || kind != tt.wantError {
				t.Errorf("the client read %d, its error %q, want %d, %q", status, kind, tt.wantAnswer, tt.wantError)
			}
			_ = srv.Shutdown(t.Context())
			<-served
			got := lines()
			if len(got) != 1 || got[0].Status != tt.want.Status || got[0].Canceled != tt.want.Canceled || got[0].CutOff != tt.want.CutOff {
				t.Errorf("the ledger holds\n%s\nwant the request's line, its status %d, canceled %v, cut off %v",
					showLines(got), tt.want.Status, tt.want.Canceled, tt.want.CutOff)
			}
		})
	}
}

// answerOn reads the answer that comes back on a connection: its status, and
// the type of the error its body gives; 0 where none comes, as when the
// connection has closed.
func answerOn(answers *bufio.Reader) (int, string) {
	resp, err := http.ReadResponse(answers, nil)
	if err != nil {
		return 0, ""
	}
	defer func() { _ = resp.Body.Close() }()
	var body apiError
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return resp.StatusCode, body.Error.Type
}

func TestALinesAtIsWhenItsRequestArrivedThoughItsBodyCameLater(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		began := time.Now().UTC()
		r := newTestRouter(t, time.Now, &stubProber{})
		assign(r.sessions, key{session: "one", model: opus}, "", decision{account: "work", reason: reasonNew}, began)
		r.proxy.transport = scripted(served, served)
		lines := keepingLedger(t, r)
		// The request's body comes a second after its header.
		req := claudeCodeAsks(t.Context(), "/v1/messages", "one", &paced{ctx: t.Context(), pause: time.Second, pieces: []string{opusAsked}})

		r.Proxy().ServeHTTP(httptest.NewRecorder(), req)
		want := edited(stickyLine(), func(l *ledger.Line) { l.At, l.FirstMS, l.TotalMS = began, ms(1000), 1000 })
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

// heldUntil answers as then does, once release closes.
func heldUntil(release <-chan struct{}, then answer) answer {
	return func(r *http.Request) *http.Response {
		<-release
		return then(r)
	}
}

// after answers as then does, once d has passed, or not at all, should the
// request's context end first.
func after(d time.Duration, then answer) answer {
	return func(r *http.Request) *http.Response {
		if sleep(r.Context(), d) != nil {
			return nil
		}
		return then(r)
	}
}

// zeros reads as endless zero bytes.
type zeros struct{}

func (zeros) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

// stickyLine is the line of a request routeAs sends, of session one, asking
// opusAsked, answered on work, where its session sticks: what else befalls a
// request, a test edits in.
func stickyLine() ledger.Line {
	return ledger.Line{At: start, Kind: ledger.KindMessage, Session: "one", Model: opus, Account: "work", Reason: reasonSticky,
		Status: http.StatusOK, Attempts: 1, Agent: testAgent, Betas: []string{"oauth-2025-04-20", "context-1m-2025-08-07"},
		Shape: ledger.Shape{Bytes: len(opusAsked), MaxTokens: new(int64(1))}}
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
	rec := httptest.NewRecorder()
	r.Proxy().ServeHTTP(rec, claudeCodeAsks(ctx, path, session, strings.NewReader(body)))
	return rec
}

// claudeCodeAsks is a request to path, whose body body reads, of session, on
// work's token, as Claude Code sends one, from a client whose context is
// ctx.
func claudeCodeAsks(ctx context.Context, path, session string, body io.Reader) *http.Request {
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, path, body)
	req.Header.Set("Authorization", "Bearer "+workToken)
	req.Header.Set(claude.SessionHeader, session)
	req.Header.Set("User-Agent", testAgent)
	req.Header.Set("Anthropic-Beta", testBetas)
	return req
}

// keepingLedger has the router keep its request ledger in a directory of the
// test's, and returns what reads back the lines it holds, once it has written
// every line noted to it, which stops it.
func keepingLedger(t *testing.T, r *Router) (lines func() []ledger.Line) {
	t.Helper()
	dir := t.TempDir()
	r.ledger.open(dir, r.history.readings)
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

// checkLines checks the ledger holds the lines want, in order, but for their
// requests' ids, which count up from a random start.
func checkLines(t *testing.T, got []ledger.Line, want ...ledger.Line) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("the ledger holds %d lines, want %d:\n%s", len(got), len(want), showLines(got))
	}
	for i := range want {
		want[i].Request = got[i].Request
	}
	unnamed := slices.ContainsFunc(got, func(l ledger.Line) bool { return l.Request == "" })
	if unnamed || !reflect.DeepEqual(got, want) {
		t.Errorf("the ledger holds\n%s\nwant\n%s", showLines(got), showLines(want))
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
