package router

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWithBodyReadsAsOftenAsAsked(t *testing.T) {
	original := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader("sent"))
	r := withBody(original, []byte(`{"model":"claude-opus-5-5"}`))

	if got := read(t, r.Body); got != `{"model":"claude-opus-5-5"}` {
		t.Errorf("body reads %q, want the bytes given", got)
	}
	for range 2 {
		again, err := r.GetBody()
		if err != nil {
			t.Fatalf("GetBody() error = %v", err)
		}
		if got := read(t, again); got != `{"model":"claude-opus-5-5"}` {
			t.Errorf("GetBody() reads %q, want the bytes given", got)
		}
	}
	if got := read(t, original.Body); got != "sent" {
		t.Errorf("the original request's body reads %q, want it untouched", got)
	}
}

func read(t *testing.T, r io.Reader) string {
	t.Helper()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestBearer(t *testing.T) {
	tests := []struct {
		authorization string
		want          string
	}{
		{authorization: "Bearer test-token-work", want: "test-token-work"},
		{authorization: "bearer test-token-work", want: "test-token-work"},
		{authorization: "BEARER  test-token-work ", want: "test-token-work"},
		{authorization: "Basic dXNlcjpwYXNz", want: ""},
		{authorization: "Bearer", want: ""},
		{authorization: "test-token-work", want: ""},
		{authorization: "", want: ""},
	}
	for _, tt := range tests {
		h := http.Header{}
		if tt.authorization != "" {
			h.Set("Authorization", tt.authorization)
		}
		if got := bearer(h); got != tt.want {
			t.Errorf("bearer(%q) = %q, want %q", tt.authorization, got, tt.want)
		}
	}
}
