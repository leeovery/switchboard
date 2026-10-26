package launch_test

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/launch"
	"github.com/leeovery/switchboard/internal/router"
)

// never is how long a router that's stuck takes to answer: longer than it's
// ever given.
const never = time.Hour

func TestAskWaitsForASlowRouterAndAsksOneComingBackAgain(t *testing.T) {
	answer := router.Health{OK: true, Listen: proxyAddr, PID: 4242}
	tests := []struct {
		name   string
		router *timedRouter
		want   router.Health
		// wantErr is what asking fails with, "" for nothing; wantNotRunning
		// whether that's router.ErrNotRunning.
		wantErr        string
		wantNotRunning bool
		// wantTook is how long asking took, and wantAsks when each ask came,
		// from the first.
		wantTook time.Duration
		wantAsks []time.Duration
	}{
		{name: "answering at once", router: &timedRouter{health: answer}, want: answer, wantAsks: asksTo(0)},
		{
			name:     "answering at 3s",
			router:   &timedRouter{health: answer, answersIn: 3 * time.Second},
			want:     answer,
			wantTook: 3 * time.Second,
			wantAsks: asksTo(0),
		},
		{
			name:     "never answering",
			router:   &timedRouter{answersIn: never},
			wantErr:  "no answer within 5s",
			wantTook: 5 * time.Second,
			wantAsks: asksTo(0),
		},
		{
			name:     "not running, then answering at 1s",
			router:   &timedRouter{health: answer, upAt: time.Second},
			want:     answer,
			wantTook: time.Second,
			wantAsks: asksTo(time.Second),
		},
		{
			name:           "not running throughout",
			router:         &timedRouter{upAt: never},
			wantErr:        "the router isn't running: dial unix control.sock: connect: no such file or directory",
			wantNotRunning: true,
			wantTook:       2 * time.Second,
			wantAsks:       asksTo(2 * time.Second),
		},
		{
			name:           "not running, saying so in 100ms, asked again 200ms after each answer",
			router:         &timedRouter{upAt: never, failsIn: 100 * time.Millisecond},
			wantErr:        "the router isn't running: dial unix control.sock: connect: no such file or directory",
			wantNotRunning: true,
			wantTook:       1900 * time.Millisecond,
			wantAsks:       []time.Duration{0, 300 * time.Millisecond, 600 * time.Millisecond, 900 * time.Millisecond, 1200 * time.Millisecond, 1500 * time.Millisecond, 1800 * time.Millisecond},
		},
		{
			name:           "not running, saying so in 500ms, never asked past 2s",
			router:         &timedRouter{upAt: never, failsIn: 500 * time.Millisecond},
			wantErr:        "the router isn't running: dial unix control.sock: connect: no such file or directory",
			wantNotRunning: true,
			wantTook:       1900 * time.Millisecond,
			wantAsks:       []time.Duration{0, 700 * time.Millisecond, 1400 * time.Millisecond},
		},
		{
			name:     "not running, then connecting at 1s and stuck",
			router:   &timedRouter{upAt: time.Second, answersIn: never},
			wantErr:  "no answer within 5s",
			wantTook: 5 * time.Second,
			wantAsks: asksTo(time.Second),
		},
		{
			name:     "not running, then connecting at 1s, answering in what's left",
			router:   &timedRouter{health: answer, upAt: time.Second, answersIn: 3900 * time.Millisecond},
			want:     answer,
			wantTook: 4900 * time.Millisecond,
			wantAsks: asksTo(time.Second),
		},
		{
			name:     "closing the connection before answering, as one stopping does, then answering at 1s",
			router:   &timedRouter{health: answer, upAt: time.Second, down: errClosedUnanswered},
			want:     answer,
			wantTook: time.Second,
			wantAsks: asksTo(time.Second),
		},
		{
			name:     "closing the connection once it has begun to answer",
			router:   &timedRouter{err: fmt.Errorf(`read the router's answer to GET /health: %w`, io.ErrUnexpectedEOF)},
			wantErr:  "read the router's answer to GET /health: unexpected EOF",
			wantAsks: asksTo(0),
		},
		{
			name:     "answering unhealthy",
			router:   &timedRouter{health: router.Health{Reason: unhealthy, PID: 4242}},
			want:     router.Health{Reason: unhealthy, PID: 4242},
			wantAsks: asksTo(0),
		},
		{
			name:     "failing otherwise",
			router:   &timedRouter{err: errors.New("the router answered GET /health with 404 Not Found")},
			wantErr:  "the router answered GET /health with 404 Not Found",
			wantAsks: asksTo(0),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				tt.router.began = time.Now()

				got, err := launch.Patience{}.Ask(t.Context(), tt.router)

				if took := time.Since(tt.router.began); took != tt.wantTook {
					t.Errorf("asking took %v, want %v", took, tt.wantTook)
				}
				if got != tt.want || errText(err) != tt.wantErr || errors.Is(err, router.ErrNotRunning) != tt.wantNotRunning {
					t.Errorf("Ask() = %+v, %v; want %+v, %q (not running: %v)", got, err, tt.want, tt.wantErr, tt.wantNotRunning)
				}
				if !slices.Equal(tt.router.asks, tt.wantAsks) {
					t.Errorf("asked at %v, want at %v", tt.router.asks, tt.wantAsks)
				}
			})
		})
	}
}

func TestAskStopsWhenItsCallerGivesUp(t *testing.T) {
	tests := []struct {
		name   string
		router *timedRouter
	}{
		{name: "waiting for an answer", router: &timedRouter{answersIn: never}},
		{name: "waiting to ask one not running again", router: &timedRouter{upAt: never}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				tt.router.began = time.Now()
				time.AfterFunc(300*time.Millisecond, cancel)

				_, err := launch.Patience{}.Ask(ctx, tt.router)

				if took := time.Since(tt.router.began); took != 300*time.Millisecond || !errors.Is(err, context.Canceled) {
					t.Errorf("Ask() error = %v after %v, want context.Canceled at once, after 300ms", err, took)
				}
			})
		})
	}
}

func TestAskSaysWhichDeadlineRanOut(t *testing.T) {
	answer := router.Health{OK: true, Listen: proxyAddr, PID: 4242}
	inside := fmt.Errorf("ask the router: %w", context.DeadlineExceeded)
	refused := errors.New("the router answered GET /health with 404 Not Found")
	tests := []struct {
		name   string
		router *timedRouter
		// deadline is the caller's, none where it's zero, and late how long
		// after it's set the first ask comes.
		deadline, late time.Duration
		want           router.Health
		wantErr        string
		wantTook       time.Duration
	}{
		{name: "its own", router: &timedRouter{answersIn: never}, deadline: 8 * time.Second, wantErr: "no answer within 5s", wantTook: 5 * time.Second},
		{name: "the caller's, first", router: &timedRouter{answersIn: never}, deadline: 1500 * time.Millisecond, wantErr: "no answer within 1.5s", wantTook: 1500 * time.Millisecond},
		{
			name:     "the caller's, first, to the millisecond",
			router:   &timedRouter{answersIn: never},
			deadline: 1500 * time.Millisecond,
			late:     400 * time.Microsecond,
			wantErr:  "no answer within 1.5s",
			wantTook: 1500*time.Millisecond - 400*time.Microsecond,
		},
		{name: "the caller's, already past", router: &timedRouter{answersIn: never}, deadline: -time.Second, wantErr: "no answer within 0s"},
		{
			name:     "the caller's, first, before the next ask of one not running: not running, as it said",
			router:   &timedRouter{upAt: never},
			deadline: 1500 * time.Millisecond,
			wantErr:  "the router isn't running: dial unix control.sock: connect: no such file or directory",
			wantTook: 1400 * time.Millisecond,
		},
		{
			name:     "the caller's, first, on the next ask of one not running: not running, as it said",
			router:   &timedRouter{upAt: never},
			deadline: 1600 * time.Millisecond,
			wantErr:  "the router isn't running: dial unix control.sock: connect: no such file or directory",
			wantTook: 1400 * time.Millisecond,
		},
		{
			name:     "the caller's, first, cutting off a dial",
			router:   &timedRouter{upAt: never, failsIn: 500 * time.Millisecond},
			deadline: 1500 * time.Millisecond,
			wantErr:  "no answer within 1.5s",
			wantTook: 1500 * time.Millisecond,
		},
		{name: "its own, cutting off a dial", router: &timedRouter{upAt: never, failsIn: never}, wantErr: "no answer within 5s", wantTook: 5 * time.Second},
		{name: "the client's own, as it says", router: &timedRouter{answersIn: time.Second, err: inside}, wantErr: inside.Error(), wantTook: time.Second},
		{
			name:     "none, an error landing as its own passes, as it is",
			router:   &timedRouter{answersIn: 5 * time.Second, deaf: true, err: refused},
			wantErr:  refused.Error(),
			wantTook: 5 * time.Second,
		},
		{
			name:     "none, an answer landing as the caller's passes, as it is",
			router:   &timedRouter{answersIn: 1500 * time.Millisecond, deaf: true, health: answer},
			deadline: 1500 * time.Millisecond,
			want:     answer,
			wantTook: 1500 * time.Millisecond,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx := t.Context()
				if tt.deadline != 0 {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, tt.deadline)
					defer cancel()
				}
				time.Sleep(tt.late)
				tt.router.began = time.Now()

				got, err := launch.Patience{}.Ask(ctx, tt.router)

				if took := time.Since(tt.router.began); got != tt.want || errText(err) != tt.wantErr || took != tt.wantTook {
					t.Errorf("Ask() = %+v, %v after %v; want %+v, %q after %v", got, err, took, tt.want, tt.wantErr, tt.wantTook)
				}
			})
		})
	}
}

func TestAskTakesItsPatience(t *testing.T) {
	tests := []struct {
		name     string
		patience launch.Patience
		router   *timedRouter
		wantErr  string
		wantTook time.Duration
		wantAsks []time.Duration
	}{
		{
			name:     "asking one not running once",
			patience: launch.Patience{Answer: time.Second},
			router:   &timedRouter{upAt: never},
			wantErr:  "the router isn't running: dial unix control.sock: connect: no such file or directory",
			wantAsks: asksTo(0),
		},
		{
			name:     "asking one not running again, every 200ms where it doesn't say",
			patience: launch.Patience{Answer: time.Second, Back: 600 * time.Millisecond},
			router:   &timedRouter{upAt: never},
			wantErr:  "the router isn't running: dial unix control.sock: connect: no such file or directory",
			wantTook: 600 * time.Millisecond,
			wantAsks: asksTo(600 * time.Millisecond),
		},
		{
			name:     "giving one stuck its time",
			patience: launch.Patience{Answer: time.Second},
			router:   &timedRouter{answersIn: never},
			wantErr:  "no answer within 1s",
			wantTook: time.Second,
			wantAsks: asksTo(0),
		},
		{
			name:     "giving one stuck 5s where it doesn't say",
			patience: launch.Patience{Back: 600 * time.Millisecond},
			router:   &timedRouter{answersIn: never},
			wantErr:  "no answer within 5s",
			wantTook: 5 * time.Second,
			wantAsks: asksTo(0),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				tt.router.began = time.Now()

				_, err := tt.patience.Ask(t.Context(), tt.router)

				if took := time.Since(tt.router.began); errText(err) != tt.wantErr || took != tt.wantTook || !slices.Equal(tt.router.asks, tt.wantAsks) {
					t.Errorf("Ask() error = %v after %v, asking at %v; want %q after %v, asked at %v", err, took, tt.router.asks, tt.wantErr, tt.wantTook, tt.wantAsks)
				}
			})
		})
	}
}

// asksTo is when a router not running until up is asked, from the first ask:
// every 200ms until it's up.
func asksTo(up time.Duration) []time.Duration {
	var asks []time.Duration
	for at := time.Duration(0); at <= up; at += 200 * time.Millisecond {
		asks = append(asks, at)
	}
	return asks
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// errClosedUnanswered is how router.Client fails an ask whose connection the
// router closed before it began to answer.
var errClosedUnanswered = fmt.Errorf(`%w: Get "http://switchboard/health": EOF`, router.ErrUnanswered)

// timedRouter answers as a router does over time, from began: not running
// until upAt, saying so, or as down says, failsIn after each ask, then
// answering health, or err, answersIn after each ask. An ask its caller gives
// up on fails as router.Client fails one, a dial or a request alike, unless
// it's deaf, answering all the same.
type timedRouter struct {
	began     time.Time
	upAt      time.Duration
	down      error
	failsIn   time.Duration
	answersIn time.Duration
	deaf      bool
	health    router.Health
	err       error
	// asks are when it was asked, from began.
	asks []time.Duration
}

func (r *timedRouter) Health(ctx context.Context) (router.Health, error) {
	at := time.Since(r.began)
	r.asks = append(r.asks, at)
	if at < r.upAt {
		down := cmp.Or(r.down, fmt.Errorf("%w: dial unix control.sock: connect: no such file or directory", router.ErrNotRunning))
		return r.within(ctx, r.failsIn, router.Health{}, down)
	}
	return r.within(ctx, r.answersIn, r.health, r.err)
}

// within answers h and err in d, unless ctx ends first and the router hears
// it.
func (r *timedRouter) within(ctx context.Context, d time.Duration, h router.Health, err error) (router.Health, error) {
	answering := time.NewTimer(d)
	defer answering.Stop()
	gaveUp := ctx.Done()
	if r.deaf {
		gaveUp = nil
	}
	select {
	case <-answering.C:
		return h, err
	case <-gaveUp:
		return router.Health{}, fmt.Errorf(`ask the router: Get "http://switchboard/health": %w`, ctx.Err())
	}
}
