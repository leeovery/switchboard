package router

import (
	"context"
	"errors"
	"io"
	"net"
	"net/url"
	"syscall"
	"testing"
)

func TestARequestClosedUnansweredIsToldApart(t *testing.T) {
	asked := func(err error) error { return &url.Error{Op: "Get", URL: "http://switchboard/health", Err: err} }
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "closed at its end", err: asked(io.EOF), want: true},
		{name: "reset as it was read", err: asked(&net.OpError{Op: "read", Net: "unix", Err: syscall.ECONNRESET}), want: true},
		{name: "closed as the request was written", err: asked(&net.OpError{Op: "write", Net: "unix", Err: syscall.EPIPE}), want: true},
		{name: "cut off by its deadline", err: asked(context.DeadlineExceeded)},
		{name: "answered with what isn't HTTP", err: asked(errors.New(`net/http: HTTP/1.x transport connection broken: malformed HTTP status code "2"`))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := closedUnanswered(tt.err); got != tt.want {
				t.Errorf("closedUnanswered(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
