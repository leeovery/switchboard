package router

import (
	"cmp"
	"encoding/json"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

// The kinds of event the request stream tells of, as a StreamEvent's Kind
// says.
const (
	// StreamInFlight is a request in flight as a reader joins the stream, as
	// it stands then: the stream opens with one for each.
	StreamInFlight = "inflight"
	// StreamSent is a request going upstream.
	StreamSent = "sent"
	// StreamFirst is the first byte of the answer the client has coming.
	StreamFirst = "first"
	// StreamProgress is how many characters of text, thinking and tools'
	// input the answer has streamed so far.
	StreamProgress = "progress"
	// StreamDone is a request ending, with what the client was answered, and
	// the tokens the answer's closing usage gives.
	StreamDone = "done"
	// StreamLimited is the upstream answering that the account has reached its
	// limit: a 429, but for one to a request sent before the limit was reset
	// by hand, which goes out again on the account.
	StreamLimited = "limited"
	// StreamThrottled is the upstream throttling the account: a 429 that's
	// sent again.
	StreamThrottled = "throttled"
	// StreamRefused is the upstream refusing the request on the account: a 401
	// or a 403, but for a 401 of a token the account's file has replaced since,
	// which goes out again on the new one.
	StreamRefused = "refused"
	// StreamMoved is the request's session moving to another account, for its
	// requests of the model, its account the one it moved to: never the
	// client's quota check's, which no session is remembered for.
	StreamMoved = "moved"
)

const (
	// readerLag is how many events a reader of the request stream can fall
	// behind before it's dropped, to reconnect.
	readerLag = 256
	// progressEvery is how often the request stream tells of the progress of
	// a request's answer, at most.
	progressEvery = 250 * time.Millisecond
	// streamWriteWait is how long a reader of the request stream has to take
	// what's written to it: one that doesn't is cut off, rather than held
	// open.
	streamWriteWait = 10 * time.Second
)

// StreamEvent is something befalling a routed request, as GET /stream tells
// of it: what kind of thing, the request as it stands, and the rest as each
// kind needs, left out where it doesn't.
type StreamEvent struct {
	At time.Time `json:"at"`
	// Kind says what befell the request, such as StreamSent.
	Kind string `json:"kind"`
	// Request is the router's id for the request, unique while it runs, and
	// Attempt which time it went upstream: zero before it first has.
	Request string `json:"request"`
	Attempt int    `json:"attempt,omitzero"`
	// Session and Model are the session the request belongs to and the model
	// it asks for, either "" when it doesn't say, each cut to 200 bytes, and
	// Dir the directory the session was started in, as the request ledger
	// gives it, "" when the request doesn't say.
	Session string `json:"session,omitempty"`
	Dir     string `json:"dir,omitempty"`
	Model   string `json:"model,omitempty"`
	// Account is the account the request goes out on, or went out on last,
	// or, once the client has an answer, the one it came from; and of a
	// move, the account the session moved to.
	Account string `json:"account,omitempty"`
	// Check is set when the request is the client's quota check.
	Check bool `json:"check,omitzero"`
	// SentAt is when a request in flight last went upstream, and FirstAt when
	// the first byte of its answer came, zero until it has.
	SentAt  time.Time `json:"sent_at,omitzero"`
	FirstAt time.Time `json:"first_at,omitzero"`
	// Chars is how many characters of text, thinking and tools' input the
	// answer has streamed so far, or in all once the request is done: zero
	// when it streamed none, or they couldn't be counted.
	Chars int `json:"chars,omitzero"`
	// Verdict is, of a request in flight as a reader joins, the last the
	// stream told of its answer on the account it went out on last,
	// StreamLimited, StreamThrottled or StreamRefused, with that answer's
	// Status: "" while there's none.
	Verdict string `json:"verdict,omitempty"`
	// Status is the upstream's answer that limited, throttled or refused the
	// request, or what the client was answered once it's done: zero when the
	// client went before it was answered.
	Status int `json:"status,omitzero"`
	// Tokens are the tokens the answer's closing usage gives, once the
	// request is done: nil when none came, or they couldn't be counted.
	Tokens *quota.Tokens `json:"tokens,omitempty"`
	// From and To are the accounts the request's session moved from and to,
	// and Reason why, as the routed line's reason gives it.
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// stream is the router's request stream: what befalls each routed request,
// as it happens, for the readers of GET /stream, and the requests in flight,
// as each stands, which a reader is told of first as it joins. While it has
// readers, it tells of the progress of each request's answer every
// progressEvery at most while answers progress, and sleeps while none does.
// Telling of an event never waits for a reader: one that falls readerLag
// events behind is dropped, to reconnect. A reader's stream ends as the
// control API closes, as the router stops. It's safe for concurrent use.
type stream struct {
	now func() time.Time

	mu      sync.Mutex
	flying  map[string]*flight
	readers map[*streamReader]struct{}
	// pacing is closed as the last reader leaves, to stop telling of
	// progress: nil while there's none.
	pacing chan struct{}
	// idle is set while the pacer sleeps, no answer having streamed more
	// since it last told of progress, and stir wakes it.
	idle bool
	stir chan struct{}
}

// flight is a request in flight, as the stream keeps it: the inflight event
// that tells of it as it stands, its Chars those its answer has streamed so
// far, told those last told of, and of its session and model in full, which
// the event may cut.
type flight struct {
	StreamEvent
	told int
	of   key
}

// streamReader is a reader of the stream, and the events it's yet to take:
// closed once it's dropped.
type streamReader struct {
	events chan StreamEvent
}

func newStream(now func() time.Time) *stream {
	return &stream{now: now, flying: make(map[string]*flight), readers: make(map[*streamReader]struct{})}
}

// publish tells every reader of e, as happening now, and keeps the request
// in flight as e leaves it. A request going upstream, sent tells of.
func (s *stream) publish(e StreamEvent) {
	s.publishOf(e, key{})
}

// sent tells every reader of e, a request going upstream, as publish does:
// of names its session and model in full.
func (s *stream) sent(e StreamEvent, of key) {
	s.publishOf(e, of)
}

// publishOf tells every reader of e, as happening now, and keeps the request
// in flight as e leaves it: of names, in full, the session and model of a
// request going upstream.
func (s *stream) publishOf(e StreamEvent, of key) {
	s.mu.Lock()
	e.At = s.now().UTC()
	s.keep(e, of)
	behind := s.tell(e)
	s.mu.Unlock()
	noteBehind(behind)
}

// keep keeps the request e tells of as e leaves it: in flight from each time
// it goes upstream until it's done, of naming its session and model in full,
// with the last verdict told of its answer there, on the account whose answer
// the client has once it has one. s.mu must be held.
func (s *stream) keep(e StreamEvent, of key) {
	f, ok := s.flying[e.Request]
	switch {
	case e.Kind == StreamSent:
		sent := &flight{StreamEvent: e, of: of}
		sent.Kind, sent.SentAt = StreamInFlight, e.At
		s.flying[e.Request] = sent
	case e.Kind == StreamDone:
		delete(s.flying, e.Request)
	case !ok:
	case e.Kind == StreamFirst:
		f.FirstAt, f.Account = e.At, e.Account
	case e.Kind == StreamLimited, e.Kind == StreamThrottled, e.Kind == StreamRefused:
		f.Verdict, f.Status = e.Kind, e.Status
	}
}

// tell tells every reader of e, dropping any that has fallen readerLag
// events behind, and returns how many it dropped. s.mu must be held.
func (s *stream) tell(e StreamEvent) (behind int) {
	for r := range s.readers {
		select {
		case r.events <- e:
		default:
			s.drop(r)
			behind++
		}
	}
	return behind
}

// noteBehind notes in the log the readers dropped for falling behind, when
// there are any.
func noteBehind(readers int) {
	if readers > 0 {
		logger.Info("dropped a reader of the request stream, as it fell behind", "readers", readers, "events", readerLag)
	}
}

// progressed notes that the answer to the request with the given id has
// streamed chars characters of text, thinking and tools' input so far,
// waking the pacer should it sleep.
func (s *stream) progressed(request string, chars int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.flying[request]
	if !ok || f.Chars == chars {
		return
	}
	f.Chars = chars
	if s.idle {
		s.idle = false
		select {
		case s.stir <- struct{}{}:
		default:
		}
	}
}

// join has a reader join the stream: it returns the requests in flight, as
// they stand, the first sent first, and the reader, to be told of every event
// after them.
func (s *stream) join() ([]StreamEvent, *streamReader) {
	s.mu.Lock()
	defer s.mu.Unlock()
	at := s.now().UTC()
	inFlight := make([]StreamEvent, 0, len(s.flying))
	for _, f := range s.flying {
		e := f.StreamEvent
		e.At = at
		inFlight = append(inFlight, e)
	}
	slices.SortFunc(inFlight, func(a, b StreamEvent) int {
		return cmp.Or(a.SentAt.Compare(b.SentAt), cmp.Compare(a.Request, b.Request))
	})
	r := &streamReader{events: make(chan StreamEvent, readerLag)}
	s.readers[r] = struct{}{}
	if s.pacing == nil {
		s.startPacing()
	}
	return inFlight, r
}

// doing returns what the requests in flight are doing, by the session and
// model they're of, as an assignment's InFlight gives it: status.Answering
// where an answer streams to any of them, else status.Asking. The client's
// quota checks, which are no session's work, are left out.
func (s *stream) doing() map[key]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	doing := make(map[key]string, len(s.flying))
	for _, f := range s.flying {
		switch {
		case f.Check:
		case !f.FirstAt.IsZero():
			doing[f.of] = status.Answering
		case doing[f.of] == "":
			doing[f.of] = status.Asking
		}
	}
	return doing
}

// startPacing starts telling of the progress of the requests' answers, as
// the first reader joins, whose inflight events tell it of their progress so
// far: the pacer sleeps until an answer streams more. s.mu must be held.
func (s *stream) startPacing() {
	for _, f := range s.flying {
		f.told = f.Chars
	}
	s.pacing, s.stir, s.idle = make(chan struct{}), make(chan struct{}, 1), true
	go s.pace(s.pacing, s.stir)
}

// leave stops telling r of events, as it goes.
func (s *stream) leave(r *streamReader) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.readers[r]; ok {
		s.drop(r)
	}
}

// drop stops telling r of events, and closes them, stopping telling of
// progress once no reader is left. s.mu must be held.
func (s *stream) drop(r *streamReader) {
	delete(s.readers, r)
	close(r.events)
	if len(s.readers) == 0 {
		close(s.pacing)
		s.pacing, s.idle = nil, false
	}
}

// pace tells of the progress of the requests' answers, as tellProgress does,
// every progressEvery while they stream more, from progressEvery after stir
// wakes it, as an answer streams more, sleeping again once a look finds none
// has, until stop is closed.
func (s *stream) pace(stop, stir <-chan struct{}) {
	timer := time.NewTimer(progressEvery)
	timer.Stop()
	defer timer.Stop()
	for {
		select {
		case <-stop:
			return
		case <-stir:
		}
		for told := true; told; told = s.tellProgress(stop) {
			timer.Reset(progressEvery)
			select {
			case <-stop:
				return
			case <-timer.C:
			}
		}
	}
}

// tellProgress tells every reader of each request whose answer has streamed
// more characters since it was last told of, for the pacer stop stops, and
// reports whether there were any: when there were none, the pacer is idle
// from then on. A pacer stopped, as the last reader left, tells of nothing.
func (s *stream) tellProgress(stop <-chan struct{}) bool {
	s.mu.Lock()
	if s.pacing != stop {
		s.mu.Unlock()
		return false
	}
	at, told, behind := s.now().UTC(), false, 0
	for _, f := range s.flying {
		if f.Chars == f.told {
			continue
		}
		f.told, told = f.Chars, true
		e := f.StreamEvent
		e.At, e.Kind = at, StreamProgress
		e.SentAt, e.FirstAt, e.Verdict, e.Status = time.Time{}, time.Time{}, "", 0
		behind += s.tell(e)
	}
	s.idle = !told
	s.mu.Unlock()
	noteBehind(behind)
	return told
}

// serveStream answers GET /stream: the requests in flight as the reader
// joins, then every event of the stream as it comes, a line of JSON each,
// until the reader goes or is dropped, or the control API closes.
func (r *Router) serveStream(w http.ResponseWriter, req *http.Request) {
	inFlight, reader := r.stream.join()
	defer r.stream.leave(reader)
	w.Header().Set("Content-Type", "application/x-ndjson")
	if !writeEvents(w, inFlight) {
		return
	}
	for {
		select {
		case <-req.Context().Done():
			return
		case e, open := <-reader.events:
			if !open || !writeEvents(w, gather(e, reader.events)) {
				return
			}
		}
	}
}

// gather returns e and every event after it that has already come.
func gather(e StreamEvent, events <-chan StreamEvent) []StreamEvent {
	gathered := []StreamEvent{e}
	for {
		select {
		case e, open := <-events:
			if !open {
				return gathered
			}
			gathered = append(gathered, e)
		default:
			return gathered
		}
	}
}

// writeEvents writes events to a reader of the stream, a line of JSON each,
// and has them sent, giving the reader streamWriteWait to take them. It
// reports whether it did.
func writeEvents(w http.ResponseWriter, events []StreamEvent) bool {
	response := http.NewResponseController(w)
	_ = response.SetWriteDeadline(time.Now().Add(streamWriteWait))
	lines := json.NewEncoder(w)
	for _, e := range events {
		if lines.Encode(e) != nil {
			return false
		}
	}
	return response.Flush() == nil
}
