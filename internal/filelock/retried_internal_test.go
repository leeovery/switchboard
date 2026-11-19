//go:build unix

package filelock

import (
	"errors"
	"syscall"
	"testing"
)

func TestALockCutShortByASignalIsTriedAgain(t *testing.T) {
	tests := []struct {
		name string
		// answers are what each call of the lock returns, in turn.
		answers   []error
		want      error
		wantCalls int
	}{
		{name: "taken at once", answers: []error{nil}, want: nil, wantCalls: 1},
		{name: "cut short, then taken", answers: []error{syscall.EINTR, syscall.EINTR, nil}, want: nil, wantCalls: 3},
		{name: "cut short, then held by another", answers: []error{syscall.EINTR, syscall.EWOULDBLOCK}, want: syscall.EWOULDBLOCK, wantCalls: 2},
		{name: "failed", answers: []error{syscall.EBADF}, want: syscall.EBADF, wantCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			lock := func(fd, how int) error {
				if fd != 7 || how != syscall.LOCK_EX {
					t.Errorf("lock(%d, %d), want lock(7, %d)", fd, how, syscall.LOCK_EX)
				}
				calls++
				return tt.answers[calls-1]
			}
			if err := retried(lock, 7, syscall.LOCK_EX); !errors.Is(err, tt.want) {
				t.Errorf("retried() = %v, want %v", err, tt.want)
			}
			if calls != tt.wantCalls {
				t.Errorf("the lock was called %d times, want %d", calls, tt.wantCalls)
			}
		})
	}
}
