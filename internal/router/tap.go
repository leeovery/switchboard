package router

import (
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
)

const (
	// tapMost is how much of an answer's body can wait to be counted: should
	// the counting fall so far behind, it gives the answer up, rather than
	// ever hold its body back.
	tapMost = 4 << 20
	// countEvery is how often, at most, the counting takes what has passed of
	// an answer's body since it last did, while the body passes.
	countEvery = 100 * time.Millisecond
)

// tap is the body of the answer a routed request's client has, tapped: it
// passes on as it's read, untouched, and a copy of what passes is kept for
// counting for the request stream, on a goroutine of its own. The body never
// waits for the counting, and wakes it only once it waits for more, having
// taken all that had passed, and at the body's end, as waking a goroutine can
// cost the one passing the body a signal to another thread: while the body
// passes, the counting takes what's kept every countEvery at most, and while
// it's silent, the counting sleeps.
type tap struct {
	body io.ReadCloser
	// nudge wakes the counting.
	nudge chan struct{}
	// ended takes the stream's done event of the request, once it ends, and
	// told closes once the counting has told of it.
	ended chan StreamEvent
	told  chan struct{}

	mu sync.Mutex
	// kept is what has passed since the counting last took it.
	kept []byte
	// waiting is set while the counting waits for more of the body, having
	// taken all there was; stopped once it has all of the body it's to have;
	// and lost once it fell behind, which gives no count then.
	waiting, stopped, lost bool
}

// count has the answer to the request ex counted for the request stream as
// its body passes on to the client: the body is tapped, and its counting
// starts. An answer switching the connection to another protocol isn't
// counted, as the reverse proxy takes its body for the connection itself.
func (p *proxy) count(ex *exchange, resp *http.Response) {
	if resp.StatusCode == http.StatusSwitchingProtocols {
		return
	}
	t := &tap{body: resp.Body, nudge: make(chan struct{}, 1), ended: make(chan StreamEvent, 1), told: make(chan struct{})}
	resp.Body, ex.tap = t, t
	go t.count(p.stream, p.provider, ex.event(StreamFirst), resp.Header.Get("Content-Encoding"), resp.Header.Get("Content-Type"))
}

func (t *tap) Read(p []byte) (int, error) {
	n, err := t.body.Read(p)
	if n > 0 {
		t.pass(p[:n])
	}
	return n, err
}

func (t *tap) Close() error {
	t.stop()
	return t.body.Close()
}

// pass keeps a copy of a piece of the body for the counting, waking it while
// it waits for more: should it have fallen behind, the counting gives the
// answer up.
func (t *tap) pass(piece []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch {
	case t.stopped:
	case len(t.kept)+len(piece) > tapMost:
		t.lost = true
		t.halt()
	default:
		t.kept = append(t.kept, piece...)
		if t.waiting {
			t.waiting = false
			t.wake()
		}
	}
}

// stop has the counting go on with what it has of the body, which is all
// it's to have.
func (t *tap) stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.halt()
}

// halt stops the counting's taking more than it has, once, waking it to
// take the rest. t.mu must be held.
func (t *tap) halt() {
	if !t.stopped {
		t.stopped = true
		t.wake()
	}
}

// wake wakes the counting, unless it's to wake already.
func (t *tap) wake() {
	select {
	case t.nudge <- struct{}{}:
	default:
	}
}

// take returns what's kept of the body for the counting, keeping what passes
// from now on in done, which the counting has done with, and reports whether
// it's the last. The two go back and forth, so keeping what passes seldom
// allocates. Taking nothing before the last, the counting waits for more,
// which wakes it as it passes.
func (t *tap) take(done []byte) ([]byte, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	kept := t.kept
	t.kept = done[:0]
	t.waiting = len(kept) == 0 && !t.stopped
	return kept, t.stopped
}

// gaveUp reports whether the counting gave the answer up, as it fell behind.
func (t *tap) gaveUp() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lost
}

// end hands over the stream's done event of the request, which has ended,
// and waits for the counting to tell of it, once it has counted what it has
// of the body, which has all passed: so a request's end is told of before
// its handler returns, as the router's last requests are while it stops.
func (t *tap) end(done StreamEvent) {
	t.stop()
	t.ended <- done
	<-t.told
}

// count counts the answer for the request stream s as its body passes: it
// tells of its first byte, as first, and of the characters of text, thinking
// and tools' input as they come, as the provider counts them, the body
// decoded from the encoding given, of the content type given. Once the
// request has ended, it tells of its done, with the tokens the answer's
// closing usage gives, and its characters in all, unless the counting gave
// the answer up.
func (t *tap) count(s *stream, provider Provider, first StreamEvent, encoding, contentType string) {
	defer close(t.told)
	passed := &passed{tap: t, first: func() { s.publish(first) }}
	chars := 0
	tokens, counted, err := tally(provider, passed, encoding, contentType, func(n int) {
		chars = n
		s.progressed(first.Request, n)
	})
	if err != nil {
		logger.Debug("can't count an answer", "id", first.Request, "error", err)
	}
	_, _ = io.Copy(io.Discard, passed)
	done := <-t.ended
	if !t.gaveUp() {
		done.Chars = chars
		if counted {
			done.Tokens = &tokens
		}
	}
	s.publish(done)
}

// tally has provider count an answer's body, as it passes, decoded from the
// encoding given, of the content type given, telling chars of its text,
// thinking and tools' input so far. It returns the tokens the answer's
// closing usage gives, reporting false when none came. It fails when the body
// can't be decoded.
func tally(provider Provider, body io.Reader, encoding, contentType string, chars func(int)) (tokens quota.Tokens, counted bool, err error) {
	// Counting runs on every answer, so a fault in it costs that answer's
	// counts alone, never the router.
	defer func() {
		if v := recover(); v != nil {
			logger.Error("counting an answer failed", "panic", v, "stack", string(debug.Stack()))
			tokens, counted = quota.Tokens{}, false
		}
	}()
	decoded, err := decode(body, encoding)
	if err != nil {
		return quota.Tokens{}, false, err
	}
	tokens, counted = provider.Count(contentType, decoded, chars)
	return tokens, counted, nil
}

// decode returns body decoded from the encoding an answer's Content-Encoding
// names: none, gzip or deflate, as the standard library decodes them. It
// fails for any other, such as br.
func decode(body io.Reader, encoding string) (io.Reader, error) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "", "identity":
		return body, nil
	case "gzip", "x-gzip":
		r, err := gzip.NewReader(body)
		if err != nil {
			return nil, fmt.Errorf("read its gzip header: %w", err)
		}
		return r, nil
	case "deflate":
		r, err := zlib.NewReader(body)
		if err != nil {
			return nil, fmt.Errorf("read its deflate header: %w", err)
		}
		return r, nil
	}
	return nil, fmt.Errorf("it's encoded as %q, which can't be decoded", encoding)
}

// passed reads what has passed of a tapped answer's body, for its counting,
// as the tap keeps it, waiting until more has, and calls first as the first
// byte is read.
type passed struct {
	tap   *tap
	first func()
	began bool
	// taken is what was last taken from the tap, and piece what of it is yet
	// to be read. flowing is set when the last take found some of the body,
	// and last when it was the last.
	taken, piece  []byte
	flowing, last bool
	// gathering times the wait for more of the body before the next take.
	gathering *time.Timer
}

func (p *passed) Read(b []byte) (int, error) {
	for len(p.piece) == 0 {
		if p.last {
			return 0, io.EOF
		}
		if p.flowing {
			p.gather()
		}
		p.taken, p.last = p.tap.take(p.taken)
		p.piece, p.flowing = p.taken, len(p.taken) > 0
		if !p.flowing && !p.last {
			<-p.tap.nudge
		}
	}
	if !p.began {
		p.began = true
		p.first()
	}
	n := copy(b, p.piece)
	p.piece = p.piece[n:]
	return n, nil
}

// gather lets more of the body pass before the next take, while it flows:
// for countEvery, or until the tap wakes the reading at the body's end.
func (p *passed) gather() {
	if p.gathering == nil {
		p.gathering = time.NewTimer(countEvery)
	} else {
		p.gathering.Reset(countEvery)
	}
	select {
	case <-p.tap.nudge:
	case <-p.gathering.C:
	}
}
