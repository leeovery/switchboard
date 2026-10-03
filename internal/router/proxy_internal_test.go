package router

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestARoutedRequestOffersTheEncodingsTheRouterCanCountIn(t *testing.T) {
	tests := []struct {
		name string
		// offered are the request's Accept-Encoding lines, and want what it
		// goes upstream with, "" for none.
		offered []string
		want    string
	}{
		{name: "Claude Code's", offered: []string{"gzip, deflate, br, zstd"}, want: "gzip, deflate"},
		{name: "in the client's order, weighed as it weighs them", offered: []string{"br;q=1.0, Deflate, GZIP;q=0.8, *;q=0.1"}, want: "Deflate, GZIP;q=0.8"},
		{name: "over several lines", offered: []string{"br, gzip", "identity"}, want: "gzip, identity"},
		{name: "none it can count", offered: []string{"br, zstd"}, want: "identity"},
		{name: "none at all, which Go's transport asks gzip for, and decodes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.Header{}
			for _, line := range tt.offered {
				h.Add("Accept-Encoding", line)
			}
			countable(h)
			if got := strings.Join(h.Values("Accept-Encoding"), " | "); got != tt.want {
				t.Errorf("Accept-Encoding goes upstream as %q, want %q", got, tt.want)
			}
		})
	}
}

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
