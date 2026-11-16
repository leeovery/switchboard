package swipe

import (
	"context"
	"testing"
	"time"
)

// The read of the setting, with the wait for its output once defaults ends,
// ends within 2 seconds: the runner's deadline leaves room for that wait.
func TestTheReadEndsWithinTwoSecondsOutputAndAll(t *testing.T) {
	const bound = 2 * time.Second
	if waitDelay <= 0 || waitDelay >= bound {
		t.Fatalf("waitDelay is %v, want some time, less than %v", waitDelay, bound)
	}
	var given time.Duration
	Read(t.Context(), func(ctx context.Context) ([]byte, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("the setting was read without a deadline, want one")
		}
		given = time.Until(deadline)
		return nil, nil
	})
	if want := bound - waitDelay; given > want || given < want-time.Second {
		t.Errorf("the runner was given %v to read the setting, want %v: %v less the %v its output is waited for", given, want, bound, waitDelay)
	}
}
