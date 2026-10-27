package cli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

func TestWhyTheRoutersDocumentWasntReadIsTheRoutersStateAsAsked(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want status.Fallback
	}{
		{name: "read"},
		{
			name: "nothing listening on its socket",
			err:  fmt.Errorf("%w: dial unix control.sock: connect: no such file or directory", router.ErrNotRunning),
			want: status.Fallback{Router: status.RouterNotRunning},
		},
		{
			name: "the connection closed before any answer, at the last ask, as a router stopping closes it",
			err:  fmt.Errorf(`%w: Get "http://switchboard/health": EOF`, router.ErrUnanswered),
			want: status.Fallback{Router: status.RouterNotRunning},
		},
		{
			name: "no answer in time",
			err:  errors.New("no answer within 5s"),
			want: status.Fallback{Router: status.RouterUnhealthy, Reason: "no answer within 5s"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fallbackFrom(tt.err); got != tt.want {
				t.Errorf("fallbackFrom(%v) = %+v, want %+v", tt.err, got, tt.want)
			}
		})
	}
}
