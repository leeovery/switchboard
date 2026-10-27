package cli

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/leeovery/switchboard/internal/router"
)

func TestARouterNotRunningIsSaidPlainly(t *testing.T) {
	refused := errors.New("router answered 400 Bad Request: no such account")
	tests := []struct {
		name string
		err  error
		want error
	}{
		{
			name: "nothing listening on its socket",
			err:  fmt.Errorf("%w: dial unix control.sock: connect: no such file or directory", router.ErrNotRunning),
			want: errRouterDown,
		},
		{
			name: "the connection closed before any answer, as a router stopping closes it",
			err:  fmt.Errorf(`%w: Get "http://switchboard/status": EOF`, router.ErrUnanswered),
			want: errRouterDown,
		},
		{name: "no answer in time", err: context.DeadlineExceeded, want: context.DeadlineExceeded},
		{name: "an answer refusing the ask", err: refused, want: refused},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fromRouter(tt.err); got != tt.want {
				t.Errorf("fromRouter(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
