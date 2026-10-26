package launch_test

import (
	"context"
	"errors"
	"fmt"
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
			name:           "not running, slow to say so, asked a beat after each answer",
			router:         &timedRouter{upAt: never, failsIn: 500 * time.Millisecond},
			wantErr:        "the router isn't running: dial unix control.sock: connect: no such file or directory",
			wantNotRunning: true,
			wantTook:       2600 * time.Millisecond,
			wantAsks:       []time.Duration{0, 700 * time.Millisecond, 1400 * time.Millisecond, 2100 * time.Millisecond},
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
	inside := fmt.Errorf("ask the router: %w", context.DeadlineExceeded)
	tests := []struct {
		name   string
		router *timedRouter
		// deadline is the caller's, from the first ask, none where it's zero.
		deadline time.Duration
		wantErr  string
		wantTook time.Duration
	}{
		{name: "its own", router: &timedRouter{answersIn: never}, deadline: 8 * time.Second, wantErr: "no answer within 5s", wantTook: 5 * time.Second},
		{name: "the caller's, first", router: &timedRouter{answersIn: never}, deadline: 1500 * time.Millisecond, wantErr: "no answer within 1.5s", wantTook: 1500 * time.Millisecond},
		{
			name:     "the caller's, first, while asking one not running again",
			router:   &timedRouter{upAt: never},
			deadline: 1500 * time.Millisecond,
			wantErr:  "no answer within 1.5s",
			wantTook: 1500 * time.Millisecond,
		},
		{name: "the client's own, as it says", router: &timedRouter{answersIn: time.Second, err: inside}, wantErr: inside.Error(), wantTook: time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx := t.Context()
				if tt.deadline > 0 {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, tt.deadline)
					defer cancel()
				}
				tt.router.began = time.Now()

				_, err := launch.Patience{}.Ask(ctx, tt.router)

				if took := time.Since(tt.router.began); errText(err) != tt.wantErr || took != tt.wantTook {
					t.Errorf("Ask() error = %v after %v, want %q after %v", err, took, tt.wantErr, tt.wantTook)
				}
			})
		})
	}
}

func TestAskTakesItsPatience(t *testing.T) {
	tests := []struct {
		name     string
		patience launch.Patience
		wantTook time.Duration
		wantAsks []time.Duration
	}{
		{name: "asking once", patience: launch.Patience{Answer: time.Second}, wantAsks: asksTo(0)},
		{name: "asking again, every 200ms where it doesn't say", patience: launch.Patience{Answer: time.Second, Back: 600 * time.Millisecond}, wantTook: 600 * time.Millisecond, wantAsks: asksTo(600 * time.Millisecond)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				r := &timedRouter{upAt: never, began: time.Now()}

				_, err := tt.patience.Ask(t.Context(), r)

				if took := time.Since(r.began); !errors.Is(err, router.ErrNotRunning) || took != tt.wantTook || !slices.Equal(r.asks, tt.wantAsks) {
					t.Errorf("Ask() error = %v after %v, asking at %v; want the router not running after %v, asked at %v", err, took, r.asks, tt.wantTook, tt.wantAsks)
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

// timedRouter answers as a router does over time, from began: not running
// until upAt, saying so failsIn after each ask, then answering health, or
// err, answersIn after each ask.
type timedRouter struct {
	began     time.Time
	upAt      time.Duration
	failsIn   time.Duration
	answersIn time.Duration
	health    router.Health
	err       error
	// asks are when it was asked, from began.
	asks []time.Duration
}

func (r *timedRouter) Health(ctx context.Context) (router.Health, error) {
	at := time.Since(r.began)
	r.asks = append(r.asks, at)
	if at < r.upAt {
		return within(ctx, r.failsIn, router.Health{}, fmt.Errorf("%w: dial unix control.sock: connect: no such file or directory", router.ErrNotRunning))
	}
	return within(ctx, r.answersIn, r.health, r.err)
}

// within answers h and err in d, unless ctx ends first.
func within(ctx context.Context, d time.Duration, h router.Health, err error) (router.Health, error) {
	answering := time.NewTimer(d)
	defer answering.Stop()
	select {
	case <-answering.C:
		return h, err
	case <-ctx.Done():
		return router.Health{}, fmt.Errorf("ask the router: %w", ctx.Err())
	}
}
