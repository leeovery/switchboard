package router

import (
	"cmp"
	"encoding/json"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
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
	// limit: a 429.
	StreamLimited = "limited"
	// StreamThrottled is the upstream throttling the account: a 429 that's
	// sent again.
	StreamThrottled = "throttled"
	// StreamRefused is the upstream refusing the request on the account: a 401
	// or a 403.
	StreamRefused = "refused"
	// StreamMoved is the request's session moving to another account, for its
	// requests of the model.
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
	// it asks for, either "" when it doesn't say.
	Session string `json:"session,omitempty"`
	Model   string `json:"model,omitempty"`
	// Account is the account the request goes out on, or went out on last.
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
// progressEvery at most. Telling of an event never waits for a reader: one
// that falls readerLag events behind is dropped, to reconnect. It's safe for
// concurrent use.
type stream struct {
	now func() time.Time

	mu      sync.Mutex
	flying  map[string]*flight
	readers map[*streamReader]struct{}
	// pacing is closed as the last reader leaves, to stop telling of
	// progress: nil while there's none.
	pacing chan struct{}
	// closed is set once the stream is closed, as the router stops: no
	// reader joins it after.
	closed bool
}

// flight is a request in flight, as the stream keeps it: the inflight event
// that tells of it as it stands, but for the characters its answer has
// streamed so far, chars, and as last told of, told.
type flight struct {
	StreamEvent
	chars, told int
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
// in flight as e leaves it.
func (s *stream) publish(e StreamEvent) {
	s.mu.Lock()
	e.At = s.now().UTC()
	s.keep(e)
	behind := s.tell(e)
	s.mu.Unlock()
	noteBehind(behind)
}

// keep keeps the request e tells of as e leaves it: in flight from each time
// it goes upstream until it's done. s.mu must be held.
func (s *stream) keep(e StreamEvent) {
	switch e.Kind {
	case StreamSent:
		f := &flight{StreamEvent: e}
		f.Kind, f.SentAt = StreamInFlight, e.At
		s.flying[e.Request] = f
	case StreamFirst:
		if f, ok := s.flying[e.Request]; ok {
			f.FirstAt = e.At
		}
	case StreamDone:
		delete(s.flying, e.Request)
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
// streamed chars characters of text, thinking and tools' input so far.
func (s *stream) progressed(request string, chars int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f, ok := s.flying[request]; ok {
		f.chars = chars
	}
}

// join has a reader join the stream: it returns the requests in flight, as
// they stand, the first sent first, and the reader, to be told of every event
// after them. It reports false once the stream is closed.
func (s *stream) join() ([]StreamEvent, *streamReader, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, nil, false
	}
	at := s.now().UTC()
	inFlight := make([]StreamEvent, 0, len(s.flying))
	for _, f := range s.flying {
		e := f.StreamEvent
		e.At, e.Chars = at, f.chars
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
	return inFlight, r, true
}

// startPacing starts telling of the progress of the requests' answers, as
// the first reader joins, whose inflight events tell it of their progress so
// far. s.mu must be held.
func (s *stream) startPacing() {
	for _, f := range s.flying {
		f.told = f.chars
	}
	s.pacing = make(chan struct{})
	go s.pace(s.pacing)
}

// leave stops telling r of events, as it goes.
func (s *stream) leave(r *streamReader) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.readers[r]; ok {
		s.drop(r)
	}
}

// close drops every reader, as the router stops, and has none join after.
func (s *stream) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	for r := range s.readers {
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
		s.pacing = nil
	}
}

// pace tells of the progress of the requests' answers, as tellProgress does,
// every progressEvery, until stop is closed.
func (s *stream) pace(stop <-chan struct{}) {
	timer := time.NewTimer(progressEvery)
	defer timer.Stop()
	for {
		select {
		case <-stop:
			return
		case <-timer.C:
			s.tellProgress()
			timer.Reset(progressEvery)
		}
	}
}

// tellProgress tells every reader of each request whose answer has streamed
// more characters since it was last told of.
func (s *stream) tellProgress() {
	s.mu.Lock()
	at, behind := s.now().UTC(), 0
	for _, f := range s.flying {
		if f.chars == f.told {
			continue
		}
		f.told = f.chars
		e := f.StreamEvent
		e.At, e.Kind, e.Chars = at, StreamProgress, f.chars
		e.SentAt, e.FirstAt = time.Time{}, time.Time{}
		behind += s.tell(e)
	}
	s.mu.Unlock()
	noteBehind(behind)
}

// serveStream answers GET /stream: the requests in flight as the reader
// joins, then every event of the stream as it comes, a line of JSON each,
// until the reader goes or is dropped, or the stream closes.
func (r *Router) serveStream(w http.ResponseWriter, req *http.Request) {
	inFlight, reader, ok := r.stream.join()
	if !ok {
		writeProblem(w, http.StatusServiceUnavailable, "the router is stopping: ask again once it's back")
		return
	}
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
