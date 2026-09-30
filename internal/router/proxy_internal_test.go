package router

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
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

func TestAPinHonouredHasTheNextIgnoredWarnedOfAfresh(t *testing.T) {
	var p ignoredPins
	for _, want := range []bool{true, false} {
		if got := p.first("personal", "one"); got != want {
			t.Fatalf("first() = %v, want %v", got, want)
		}
	}
	p.honoured("personal")
	if !p.first("personal", "one") {
		t.Error("once a pin to personal is honoured, first() = false for the next ignored, want true")
	}
}

func TestIgnoredPinsHoldSoManyAtMost(t *testing.T) {
	var p ignoredPins
	for i := range toldAtMost {
		p.first("gone", strconv.Itoa(i))
	}
	if p.first("gone", "0") {
		t.Fatal("first() = true for a session told of, want false while it's held")
	}
	p.first("gone", "one more")
	if n := len(p.told["gone"]); n > toldAtMost {
		t.Errorf("holds %d sessions of an account, want %d at most", n, toldAtMost)
	}
	if !p.first("gone", "0") {
		t.Error("first() = false for a session told of before it held too many, want true, forgotten")
	}
	for i := range toldAtMost + 1 {
		p.first("gone-"+strconv.Itoa(i), "one")
	}
	if n := len(p.told); n > toldAtMost {
		t.Errorf("holds %d accounts, want %d at most", n, toldAtMost)
	}
}
