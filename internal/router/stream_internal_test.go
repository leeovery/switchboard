package router

import (
	"bytes"
	"cmp"
	"compress/gzip"
	"compress/zlib"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"testing/iotest"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/tokens"
)

func TestTheRequestStreamTellsOfEachRequestAsItGoes(t *testing.T) {
	// quotaCheck is Claude Code's quota check, on Claude Haiku.
	const quotaCheck = `{"model":"` + haiku + `","max_tokens":1,"messages":[{"role":"user","content":"quota"}]}`
	tests := []struct {
		name string
		// body is the request's, and session the session it's of.
		body, session string
		// work and side are the answers of each, in turn.
		work, side []answer
		// want are the events the stream tells of the request, but for their
		// request's id.
		want []StreamEvent
	}{
		{
			name:    "a streamed answer",
			session: "one",
			work:    []answer{streamed(0, AnswerPieces("Hello, ", "world ☃")...)},
			want: []StreamEvent{
				{Kind: StreamSent, Attempt: 1, Session: "one", Model: opus, Account: "work"},
				{Kind: StreamFirst, Attempt: 1, Session: "one", Model: opus, Account: "work"},
				{Kind: StreamDone, Attempt: 1, Session: "one", Model: opus, Account: "work", Status: http.StatusOK,
					Chars: AnswerChars("Hello, ", "world ☃"), Tokens: &AnswerTokens},
			},
		},
		{
			name:    "a limit reached, and the request replayed on another account, which the session moves to",
			session: "one",
			work:    []answer{limitHit},
			side:    []answer{served},
			want: []StreamEvent{
				{Kind: StreamSent, Attempt: 1, Session: "one", Model: opus, Account: "work"},
				{Kind: StreamLimited, Attempt: 1, Session: "one", Model: opus, Account: "work", Status: http.StatusTooManyRequests},
				{Kind: StreamMoved, Attempt: 1, Session: "one", Model: opus, Account: "side", From: "work", To: "side", Reason: "moved: work hit its limit"},
				{Kind: StreamSent, Attempt: 2, Session: "one", Model: opus, Account: "side"},
				{Kind: StreamFirst, Attempt: 2, Session: "one", Model: opus, Account: "side"},
				{Kind: StreamDone, Attempt: 2, Session: "one", Model: opus, Account: "side", Status: http.StatusOK},
			},
		},
		{
			name:    "throttling, the request sent again twice, then the 429 the client's",
			session: "one",
			work:    []answer{throttled("1"), throttled("1"), throttled("1")},
			want: []StreamEvent{
				{Kind: StreamSent, Attempt: 1, Session: "one", Model: opus, Account: "work"},
				{Kind: StreamThrottled, Attempt: 1, Session: "one", Model: opus, Account: "work", Status: http.StatusTooManyRequests},
				{Kind: StreamSent, Attempt: 2, Session: "one", Model: opus, Account: "work"},
				{Kind: StreamThrottled, Attempt: 2, Session: "one", Model: opus, Account: "work", Status: http.StatusTooManyRequests},
				{Kind: StreamSent, Attempt: 3, Session: "one", Model: opus, Account: "work"},
				{Kind: StreamThrottled, Attempt: 3, Session: "one", Model: opus, Account: "work", Status: http.StatusTooManyRequests},
				{Kind: StreamFirst, Attempt: 3, Session: "one", Model: opus, Account: "work"},
				{Kind: StreamDone, Attempt: 3, Session: "one", Model: opus, Account: "work", Status: http.StatusTooManyRequests},
			},
		},
		{
			name:    "a refusal, and the request replayed on another account, which refuses it too",
			session: "one",
			work:    []answer{forbidden},
			side:    []answer{unauthorized},
			want: []StreamEvent{
				{Kind: StreamSent, Attempt: 1, Session: "one", Model: opus, Account: "work"},
				{Kind: StreamRefused, Attempt: 1, Session: "one", Model: opus, Account: "work", Status: http.StatusForbidden},
				{Kind: StreamMoved, Attempt: 1, Session: "one", Model: opus, Account: "side", From: "work", To: "side", Reason: "moved: work was refused"},
				{Kind: StreamSent, Attempt: 2, Session: "one", Model: opus, Account: "side"},
				{Kind: StreamRefused, Attempt: 2, Session: "one", Model: opus, Account: "side", Status: http.StatusUnauthorized},
				{Kind: StreamDone, Attempt: 2, Session: "one", Model: opus, Account: "side", Status: http.StatusBadGateway},
			},
		},
		{
			name:    "the quota check, of a new session, which goes where side's quota needs using",
			body:    quotaCheck,
			session: "check",
			side:    []answer{served},
			want: []StreamEvent{
				{Kind: StreamSent, Attempt: 1, Session: "check", Model: haiku, Account: "side", Check: true},
				{Kind: StreamFirst, Attempt: 1, Session: "check", Model: haiku, Account: "side", Check: true},
				{Kind: StreamDone, Attempt: 1, Session: "check", Model: haiku, Account: "side", Check: true, Status: http.StatusOK},
			},
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
				reader := joined(t, r)

				routeAsking(t.Context(), r, tt.session, cmp.Or(tt.body, opusAsked))
				got := heard(reader)
				if len(got) == 0 {
					t.Fatal("the stream told of nothing, want the request")
				}
				for i := range tt.want {
					tt.want[i].At, tt.want[i].Request = start, got[0].Request
				}
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("the stream told of\n%s\nwant\n%s", lines(got), lines(tt.want))
				}
			})
		})
	}
}

func TestTheRequestStreamTellsOfNoLimitFromBeforeAResetMadeByHand(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newTestRouter(t, at(start), &stubProber{})
		r.state.record("work", []quota.Window{session, laterWeek}, r.state.mark())
		r.state.record("side", []quota.Window{session, soonWeek}, r.state.mark())
		assign(r.sessions, key{session: "one", model: opus}, "", decision{account: "work", reason: reasonNew}, start)
		// Work answers the request late, rejected in its session, which the
		// answer to a request sent after it read reset by hand meanwhile.
		reset := session
		reset.Utilization = 0.02
		late := func(req *http.Request) *http.Response {
			r.state.record("work", []quota.Window{reset, laterWeek}, r.state.mark())
			return sessionRejected(req)
		}
		r.proxy.transport = &scriptedUpstream{answers: map[string][]answer{workToken: {late, served}}}
		reader := joined(t, r)

		routeAsking(t.Context(), r, "one", opusAsked)
		got := heard(reader)
		if len(got) == 0 {
			t.Fatal("the stream told of nothing, want the request")
		}
		request := StreamEvent{At: start, Request: got[0].Request, Session: "one", Model: opus, Account: "work"}
		want := []StreamEvent{
			told(request, StreamSent, 1, 0), told(request, StreamSent, 2, 0),
			told(request, StreamFirst, 2, 0), told(request, StreamDone, 2, http.StatusOK),
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("the stream told of\n%s\nwant\n%s: the 429 from before the reset is no limit", lines(got), lines(want))
		}
	})
}

func TestTheRequestStreamTellsOfNoRefusalOfATokenReplacedSince(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const replacement = "test-token-work-replaced"
		var replaced atomic.Bool
		read := func(id string) (tokens.Token, error) {
			if id == "work" && replaced.Load() {
				return tokens.Parse(replacement)
			}
			return testTokens.Read(id)
		}
		r := newTestRouterReading(t, at(start), &stubProber{}, read)
		assign(r.sessions, key{session: "one", model: opus}, "", decision{account: "work", reason: reasonNew}, start)
		// Work's token is replaced in its file, then refused.
		refusedOnceReplaced := func(req *http.Request) *http.Response {
			replaced.Store(true)
			return unauthorized(req)
		}
		r.proxy.transport = &scriptedUpstream{answers: map[string][]answer{workToken: {refusedOnceReplaced}, replacement: {served}}}
		reader := joined(t, r)

		routeAsking(t.Context(), r, "one", opusAsked)
		got := heard(reader)
		if len(got) == 0 {
			t.Fatal("the stream told of nothing, want the request")
		}
		request := StreamEvent{At: start, Request: got[0].Request, Session: "one", Model: opus, Account: "work"}
		want := []StreamEvent{
			told(request, StreamSent, 1, 0), told(request, StreamSent, 2, 0),
			told(request, StreamFirst, 2, 0), told(request, StreamDone, 2, http.StatusOK),
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("the stream told of\n%s\nwant\n%s: the 401 of the token replaced is no refusal of the account", lines(got), lines(want))
		}
	})
}

func TestTheQuotaCheckStartsNoSession(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newTestRouter(t, at(start), &stubProber{})
		r.proxy.transport = scripted(served, served)
		var heard []Event
		r.proxy.emit = func(e Event) { heard = append(heard, e) }

		routeAsking(t.Context(), r, "check", `{"model":"`+haiku+`","max_tokens":1,"messages":[{"role":"user","content":"quota"}]}`)
		if found := r.sessions.lookup(key{session: "check", model: haiku}); found.assigned {
			t.Errorf("the quota check's session is assigned %+v, want it forgotten: the check starts no session", found.current)
		}
		if len(heard) > 0 {
			t.Errorf("events = %+v, want none: the check answered with success starts no session", heard)
		}
	})
}

func TestTheRequestStreamCountsAnAnswerAsItCame(t *testing.T) {
	texts := []string{"Hello, ", "world ☃"}
	pieces := strings.Join(AnswerPieces(texts...), "")
	tests := []struct {
		name     string
		encoding string
		body     []byte
		// wantChars and wantTokens are what done gives.
		wantChars  int
		wantTokens *quota.Tokens
	}{
		{name: "not encoded", body: []byte(pieces), wantChars: AnswerChars(texts...), wantTokens: &AnswerTokens},
		{name: "gzip", encoding: "gzip", body: encoded(t, "gzip", pieces), wantChars: AnswerChars(texts...), wantTokens: &AnswerTokens},
		{name: "deflate", encoding: "deflate", body: encoded(t, "deflate", pieces), wantChars: AnswerChars(texts...), wantTokens: &AnswerTokens},
		// The standard library has no decoder for br, so its counts go
		// untold.
		{name: "br", encoding: "br", body: []byte("\x1b\x2c\x00\xf8 brotli, notionally")},
		{name: "gzip that isn't", encoding: "gzip", body: []byte("not gzip after all")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				r := newTestRouter(t, at(start), &stubProber{})
				assign(r.sessions, key{session: "one", model: opus}, "", decision{account: "work", reason: reasonNew}, start)
				answered := func(req *http.Request) *http.Response {
					resp := respond(req, http.StatusOK, http.Header{"Content-Type": {"text/event-stream"}}, string(tt.body))
					if tt.encoding != "" {
						resp.Header.Set("Content-Encoding", tt.encoding)
					}
					return resp
				}
				r.proxy.transport = scripted(answered, served)
				reader := joined(t, r)

				rec := routeAsking(t.Context(), r, "one", opusAsked)
				if !bytes.Equal(rec.Body.Bytes(), tt.body) || rec.Header().Get("Content-Encoding") != tt.encoding {
					t.Errorf("the client got %q encoded %q, want the answer as it came: %q encoded %q",
						rec.Body.Bytes(), rec.Header().Get("Content-Encoding"), tt.body, tt.encoding)
				}
				got := heard(reader)
				kinds := make([]string, len(got))
				for i, e := range got {
					kinds[i] = e.Kind
				}
				if want := []string{StreamSent, StreamFirst, StreamDone}; !slices.Equal(kinds, want) {
					t.Fatalf("the stream told of %q, want %q", kinds, want)
				}
				done := got[2]
				if done.Chars != tt.wantChars || !reflect.DeepEqual(done.Tokens, tt.wantTokens) || done.Status != http.StatusOK {
					t.Errorf("done = %d characters, tokens %+v, status %d, want %d, %+v, 200", done.Chars, done.Tokens, done.Status, tt.wantChars, tt.wantTokens)
				}
			})
		})
	}
}

func TestTheRequestStreamTellsOfAnAnswersProgressEveryQuarterSecondAtMost(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		began := time.Now()
		r := newTestRouter(t, time.Now, &stubProber{})
		assign(r.sessions, key{session: "one", model: opus}, "", decision{account: "work", reason: reasonNew}, began)
		// The answer's start, twenty characters of text, one a piece, and its
		// end, a piece each 101ms from 111ms after the reader joins: none
		// comes as the counting takes what has passed, every 100ms from the
		// first, nor as the stream tells of progress.
		pieces := slices.Concat([]string{MessageStart}, slices.Repeat([]string{TextDelta("x")}, 20), []string{MessageEnd})
		r.proxy.transport = scripted(streamed(101*time.Millisecond, pieces...), served)
		reader := joined(t, r)

		time.Sleep(10 * time.Millisecond)
		routeAsking(t.Context(), r, "one", opusAsked)
		got := heard(reader)
		var progress []StreamEvent
		for _, e := range got {
			if e.Kind == StreamProgress {
				progress = append(progress, StreamEvent{At: e.At, Kind: e.Kind, Chars: e.Chars})
			}
		}
		// Each quarter second, the characters counted by then, once there are
		// any; the answer done at 2.232s, with all twenty.
		var want []StreamEvent
		for i, chars := range []int{2, 5, 7, 10, 12, 15, 17} {
			want = append(want, StreamEvent{At: began.Add(time.Duration(i+2) * progressEvery).UTC(), Kind: StreamProgress, Chars: chars})
		}
		if !reflect.DeepEqual(progress, want) {
			t.Errorf("the stream told of the progress\n%s\nwant\n%s", lines(progress), lines(want))
		}
		if done := got[len(got)-1]; done.Kind != StreamDone || done.Chars != 20 || !done.At.Equal(began.Add(2232*time.Millisecond)) {
			t.Errorf("the stream's last event = %+v, want the request done at 2.232s, with 20 characters", done)
		}
	})
}

func TestAReaderJoiningMidAnswerIsToldOfTheRequestAsItStands(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		began := time.Now()
		r := newTestRouter(t, time.Now, &stubProber{})
		assign(r.sessions, key{session: "one", model: opus}, "", decision{account: "work", reason: reasonNew}, began)
		// A piece each 1.01s: the start, then text, then more, then the end;
		// none comes as the counting takes what has passed, every 100ms from
		// the first, nor as the stream tells of progress.
		r.proxy.transport = scripted(streamed(1010*time.Millisecond, MessageStart, TextDelta("Hello"), TextDelta(" world"), MessageEnd), served)

		answered := make(chan struct{})
		go func() {
			routeAsking(context.Background(), r, "one", opusAsked)
			close(answered)
		}()
		time.Sleep(2600 * time.Millisecond)
		inFlight, reader, ok := r.stream.join()
		if !ok {
			t.Fatal("join() reported the stream closed")
		}
		t.Cleanup(func() { r.stream.leave(reader) })
		<-answered
		got := heard(reader)
		if len(inFlight) != 1 {
			t.Fatalf("the reader joined to %d requests in flight, want 1", len(inFlight))
		}
		id := inFlight[0].Request
		wantInFlight := StreamEvent{At: began.Add(2600 * time.Millisecond).UTC(), Kind: StreamInFlight, Request: id, Attempt: 1, Session: "one", Model: opus, Account: "work",
			SentAt: began.UTC(), FirstAt: began.Add(1010 * time.Millisecond).UTC(), Chars: 5}
		if !reflect.DeepEqual(inFlight[0], wantInFlight) {
			t.Errorf("the reader joined to\n%+v\nwant\n%+v", inFlight[0], wantInFlight)
		}
		// The rest of the text came at 3.03s, and was counted at 3.11s, which
		// the stream tells of at its next quarter second from the reader's
		// joining.
		want := []StreamEvent{
			{At: began.Add(3350 * time.Millisecond).UTC(), Kind: StreamProgress, Request: id, Attempt: 1, Session: "one", Model: opus, Account: "work", Chars: 11},
			{At: began.Add(4040 * time.Millisecond).UTC(), Kind: StreamDone, Request: id, Attempt: 1, Session: "one", Model: opus, Account: "work",
				Status: http.StatusOK, Chars: 11, Tokens: &AnswerTokens},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("then the stream told of\n%s\nwant\n%s", lines(got), lines(want))
		}
	})
}

func TestAReaderThatFallsBehindIsDropped(t *testing.T) {
	log := logstest.Capture(t)
	s := newStream(at(start))
	_, slow, _ := s.join()
	_, keeping, _ := s.join()
	t.Cleanup(func() { s.leave(keeping) })

	for i := range readerLag + 1 {
		s.publish(StreamEvent{Kind: StreamSent, Request: strings.Repeat("r", i+1), Attempt: 1})
		<-keeping.events
	}
	var told int
	for range slow.events {
		told++
	}
	if told != readerLag {
		t.Errorf("the reader that fell behind was told of %d events before it was dropped, want %d", told, readerLag)
	}
	if !log.Has("level=INFO", `msg="dropped a reader of the request stream, as it fell behind"`, "events=256") {
		t.Errorf("log reads\n%s\nwant the reader dropped", log)
	}
	s.publish(StreamEvent{Kind: StreamDone, Request: "r", Attempt: 1})
	if e := <-keeping.events; e.Kind != StreamDone {
		t.Errorf("the reader that kept up was told of %+v, want the request done: it reads on", e)
	}
}

func TestAClosedStreamDropsItsReadersAndTakesNoMore(t *testing.T) {
	r := newTestRouter(t, at(start), &stubProber{})
	_, reader, _ := r.stream.join()

	r.stream.close()
	if _, open := <-reader.events; open {
		t.Error("the reader is told of an event, want it dropped as the stream closes")
	}
	if _, _, ok := r.stream.join(); ok {
		t.Error("join() let a reader join the closed stream")
	}
	rec := httptest.NewRecorder()
	r.Control().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/stream", nil))
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"error":"the router is stopping`) {
		t.Errorf("GET /stream answered %d %s once closed, want 503, saying why", rec.Code, rec.Body)
	}
}

func TestATapPassesTheAnswerOnUntouchedThoughItsCountingFallsBehind(t *testing.T) {
	sent := strings.Repeat("a piece of an answer\n", tapMost/20)
	tapped := &tap{body: io.NopCloser(iotest.HalfReader(strings.NewReader(sent))), nudge: make(chan struct{}, 1), ended: make(chan StreamEvent, 1)}

	got, err := io.ReadAll(tapped)
	if err != nil || string(got) != sent {
		t.Errorf("read %d bytes through the tap (%v), want the %d sent, untouched", len(got), err, len(sent))
	}
	if err := tapped.Close(); err != nil {
		t.Errorf("Close() = %v", err)
	}
	if !tapped.gaveUp() {
		t.Error("the counting, never taking what's kept, didn't give the answer up, want it given up once it fell behind")
	}
	if kept, last := tapped.take(nil); len(kept) > tapMost || !last {
		t.Errorf("%d bytes wait to be counted, the last: %v, want %d at most, the last", len(kept), last, tapMost)
	}
}

// opusAsked is a request's body asking Claude Opus 5.5 for little.
const opusAsked = `{"model":"` + opus + `","max_tokens":1}`

// routeAsking has the router's proxy route a messages request of session,
// whose body is body, on work's token, and returns what it answered.
func routeAsking(ctx context.Context, r *Router, session, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+workToken)
	req.Header.Set(claude.SessionHeader, session)
	rec := httptest.NewRecorder()
	r.Proxy().ServeHTTP(rec, req)
	return rec
}

// joined has a reader join the router's request stream until the test ends.
func joined(t *testing.T, r *Router) *streamReader {
	t.Helper()
	inFlight, reader, ok := r.stream.join()
	if !ok || len(inFlight) > 0 {
		t.Fatalf("join() = %+v, %v, want the stream open, with nothing in flight", inFlight, ok)
	}
	t.Cleanup(func() { r.stream.leave(reader) })
	return reader
}

// heard returns the events the stream has told reader of, once everything
// the test's requests set going has settled.
func heard(reader *streamReader) []StreamEvent {
	synctest.Wait()
	var events []StreamEvent
	for {
		select {
		case e, open := <-reader.events:
			if !open {
				return events
			}
			events = append(events, e)
		default:
			return events
		}
	}
}

// lines shows events a line of JSON each, as the stream gives them, for a
// readable diff.
func lines(events []StreamEvent) string {
	var b strings.Builder
	lines := json.NewEncoder(&b)
	for _, e := range events {
		_ = lines.Encode(e)
	}
	return b.String()
}

// streamed answers with a stream of events, its pieces each coming after the
// pause given.
func streamed(pause time.Duration, pieces ...string) answer {
	return func(r *http.Request) *http.Response {
		resp := respond(r, http.StatusOK, http.Header{"Content-Type": {"text/event-stream"}}, "")
		resp.Body, resp.ContentLength = io.NopCloser(&paced{pause: pause, pieces: slices.Clone(pieces)}), -1
		return resp
	}
}

// paced reads pieces, each after the pause given.
type paced struct {
	pause  time.Duration
	pieces []string
}

func (p *paced) Read(b []byte) (int, error) {
	if len(p.pieces) == 0 {
		return 0, io.EOF
	}
	time.Sleep(p.pause)
	n := copy(b, p.pieces[0])
	if p.pieces[0] = p.pieces[0][n:]; p.pieces[0] == "" {
		p.pieces = p.pieces[1:]
	}
	return n, nil
}

// unauthorized answers with a 401, refusing the account's token.
func unauthorized(r *http.Request) *http.Response {
	return respond(r, http.StatusUnauthorized, http.Header{}, `{"type":"error","error":{"type":"authentication_error","message":"invalid bearer token"}}`)
}

// sessionRejected answers with a 429 rejecting the request in the session
// window, at its limit, which resets as session does.
func sessionRejected(r *http.Request) *http.Response {
	h := http.Header{
		"Anthropic-Ratelimit-Unified-Status":         {"rejected"},
		"Anthropic-Ratelimit-Unified-5h-Utilization": {"1"},
		"Anthropic-Ratelimit-Unified-5h-Reset":       {strconv.FormatInt(session.ResetsAt.Unix(), 10)},
		"Anthropic-Ratelimit-Unified-5h-Status":      {"rejected"},
	}
	return respond(r, http.StatusTooManyRequests, h, `{"type":"error","error":{"type":"rate_limit_error","message":"You've hit your limit"}}`)
}

// told is the event of the kind given, at the attempt given, telling of
// request, with status, zero for none.
func told(request StreamEvent, kind string, attempt, status int) StreamEvent {
	request.Kind, request.Attempt, request.Status = kind, attempt, status
	return request
}

// encoded is s encoded as the Content-Encoding given, gzip or deflate, names.
func encoded(t *testing.T, encoding, s string) []byte {
	t.Helper()
	var b bytes.Buffer
	var w io.WriteCloser = gzip.NewWriter(&b)
	if encoding == "deflate" {
		w = zlib.NewWriter(&b)
	}
	if _, err := io.WriteString(w, s); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
