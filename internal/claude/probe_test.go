package claude_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/quota"
)

const (
	haiku      = "claude-haiku-4-5-20251001"
	fable      = "claude-fable-5-1"
	fableOlder = "claude-fable-5"
	token      = "test-token-work"
	version    = "2.1.300"
)

var (
	session   = quota.Window{Key: "5h", Label: "Session", Utilization: 0.23, ResetsAt: time.Date(2026, 9, 28, 18, 10, 0, 0, time.UTC), Status: quota.StatusAllowed}
	week      = quota.Window{Key: "7d", Label: "Week", Utilization: 0.93, ResetsAt: time.Date(2026, 10, 2, 21, 0, 0, 0, time.UTC), Status: quota.StatusAllowedWarning}
	fableWeek = quota.Window{Key: "7d_oi", Label: "Fable week", Utilization: 0.05, ResetsAt: time.Date(2026, 10, 4, 1, 10, 0, 0, time.UTC), Status: quota.StatusAllowed}
)

func TestProbeModelRequest(t *testing.T) {
	tests := []struct {
		name          string
		upstreamPath  string
		version       string
		wantPath      string
		wantUserAgent string
	}{
		{name: "claims the version given", version: version, wantPath: "/v1/messages", wantUserAgent: "claude-code/2.1.300"},
		{name: "claims a floor version without one", version: "", wantPath: "/v1/messages", wantUserAgent: "claude-code/2.1.283"},
		{name: "keeps the upstream's path", upstreamPath: "/anthropic/", version: version, wantPath: "/anthropic/v1/messages", wantUserAgent: "claude-code/2.1.300"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requests := make(chan sentRequest, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read request body: %v", err)
				}
				requests <- sentRequest{method: r.Method, url: r.URL, header: r.Header.Clone(), body: body}
				maps.Copy(w.Header(), withUsage(http.StatusOK, session).header)
			}))
			t.Cleanup(srv.Close)
			prober := &claude.Prober{Upstream: srv.URL + tt.upstreamPath, Version: tt.version}

			if _, err := prober.ProbeModel(t.Context(), token, fable); err != nil {
				t.Fatalf("ProbeModel() error = %v", err)
			}
			got := <-requests
			if got.method != http.MethodPost || got.url.Path != tt.wantPath || got.url.RawQuery != "" {
				t.Errorf("request = %s %s, want POST %s", got.method, got.url, tt.wantPath)
			}
			wantHeader := map[string]string{
				"Authorization":     "Bearer " + token,
				"Anthropic-Version": "2023-06-01",
				"Anthropic-Beta":    "oauth-2025-04-20",
				"User-Agent":        tt.wantUserAgent,
				"Content-Type":      "application/json",
			}
			for name, want := range wantHeader {
				if values := got.header.Values(name); !slices.Equal(values, []string{want}) {
					t.Errorf("header %s = %q, want %q", name, values, want)
				}
			}
			var body any
			if err := json.Unmarshal(got.body, &body); err != nil {
				t.Fatalf("request body %s: %v", got.body, err)
			}
			wantBody := map[string]any{
				"model":      fable,
				"max_tokens": 1.0,
				"system":     "You are Claude Code, Anthropic's official CLI for Claude.",
				"messages":   []any{map[string]any{"role": "user", "content": "."}},
			}
			if !reflect.DeepEqual(body, wantBody) {
				t.Errorf("request body = %s, want %v", got.body, wantBody)
			}
		})
	}
}

func TestProbeModel(t *testing.T) {
	rejected := session
	rejected.Utilization, rejected.Status = 1, quota.StatusRejected
	tests := []struct {
		name    string
		reply   reply
		want    []quota.Window
		wantErr string
	}{
		{name: "200 carrying usage", reply: withUsage(http.StatusOK, week, session), want: []quota.Window{session, week}},
		{name: "429 carrying usage", reply: withUsage(http.StatusTooManyRequests, rejected, week), want: []quota.Window{rejected, week}},
		{name: "any other status carrying usage", reply: withUsage(529, session), want: []quota.Window{session}},
		{name: "401 with the API's error", reply: apiError(http.StatusUnauthorized, "Invalid bearer token"), wantErr: "HTTP 401 · Invalid bearer token"},
		{
			name:    "error message cut to 60 characters",
			reply:   apiError(http.StatusBadRequest, strings.Repeat("0123456789", 7)),
			wantErr: "HTTP 400 · " + strings.Repeat("0123456789", 6),
		},
		{
			name:    "error message cut by character, not by byte",
			reply:   apiError(http.StatusBadRequest, strings.Repeat("é", 61)),
			wantErr: "HTTP 400 · " + strings.Repeat("é", 60),
		},
		{name: "error body that isn't JSON", reply: reply{status: http.StatusBadGateway, body: "<html>Bad gateway</html>"}, wantErr: "HTTP 502"},
		{name: "JSON error without a message", reply: reply{status: http.StatusInternalServerError, body: `{"type":"error","error":{"type":"api_error"}}`}, wantErr: "HTTP 500"},
		{name: "empty error body", reply: reply{status: http.StatusServiceUnavailable}, wantErr: "HTTP 503"},
		{
			name:    "usage headers without a utilization",
			reply:   reply{status: http.StatusOK, header: header("anthropic-ratelimit-unified-5h-reset", "1790619000")},
			wantErr: "HTTP 200",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(t, map[string]reply{haiku: tt.reply})
			prober := &claude.Prober{Upstream: api.URL, Version: version}

			got, err := prober.ProbeModel(t.Context(), token, haiku)
			if gotErr := errorText(err); gotErr != tt.wantErr {
				t.Errorf("ProbeModel() error = %q, want %q", gotErr, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ProbeModel() =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

func TestProbeModelFollowsNoRedirect(t *testing.T) {
	tests := []struct {
		name   string
		status int
		// elsewhere redirects to another server, rather than to another path
		// on the upstream.
		elsewhere bool
	}{
		{name: "302 on the upstream", status: http.StatusFound},
		{name: "302 to another server", status: http.StatusFound, elsewhere: true},
		{name: "307 on the upstream", status: http.StatusTemporaryRedirect},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reached requestLog
			other := httptest.NewServer(http.HandlerFunc(reached.note))
			t.Cleanup(other.Close)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reached.note(w, r)
				if r.URL.Path != "/v1/messages" {
					return
				}
				location := "/moved"
				if tt.elsewhere {
					location = other.URL + location
				}
				http.Redirect(w, r, location, tt.status)
			}))
			t.Cleanup(upstream.Close)
			prober := &claude.Prober{Upstream: upstream.URL, Version: version}

			_, err := prober.ProbeModel(t.Context(), token, haiku)
			if want := fmt.Sprintf("HTTP %d", tt.status); errorText(err) != want {
				t.Errorf("ProbeModel() error = %q, want %q", errorText(err), want)
			}
			if want := []string{"POST " + upstream.Listener.Addr().String() + "/v1/messages, with the token"}; !slices.Equal(reached.all(), want) {
				t.Errorf("requests sent: %q, want the probe alone", reached.all())
			}
		})
	}
}

// requestLog notes each request a server is sent: its method, host and path,
// and whether it carried the token, never the token itself.
type requestLog struct {
	mu   sync.Mutex
	sent []string
}

func (l *requestLog) note(_ http.ResponseWriter, r *http.Request) {
	carried := "without the token"
	if r.Header.Get("Authorization") == "Bearer "+token {
		carried = "with the token"
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sent = append(l.sent, r.Method+" "+r.Host+r.URL.Path+", "+carried)
}

func (l *requestLog) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.sent)
}

func TestProbeModelTimesOut(t *testing.T) {
	api := newFakeAPI(t, map[string]reply{haiku: {hang: true}})
	prober := &claude.Prober{Upstream: api.URL, Version: version, Timeout: 20 * time.Millisecond}

	_, err := prober.ProbeModel(t.Context(), token, haiku)
	if want := "timed out after 20ms"; errorText(err) != want {
		t.Errorf("ProbeModel() error = %q, want %q", errorText(err), want)
	}
}

func TestProbeModelReportsTransportErrors(t *testing.T) {
	upstream := closedServerURL(t)
	prober := &claude.Prober{Upstream: upstream, Version: version}

	_, err := prober.ProbeModel(t.Context(), token, haiku)
	if want := `Post "` + upstream + `/v1/messages": `; !strings.HasPrefix(errorText(err), want) {
		t.Errorf("ProbeModel() error = %q, want the transport error, starting %q", errorText(err), want)
	}
}

func TestProbe(t *testing.T) {
	nudged := session
	nudged.Utilization = 0.24
	rejected := session
	rejected.Utilization, rejected.Status = 1, quota.StatusRejected
	spentFableWeek := fableWeek
	spentFableWeek.Utilization, spentFableWeek.Status = 1, quota.StatusRejected
	overloaded := apiError(529, "Overloaded")
	extraReset := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	extraOff := quota.ExtraUsage{Status: quota.StatusRejected, Utilization: new(0.0), ResetsAt: extraReset}
	extraOn := quota.ExtraUsage{Status: quota.StatusAllowed, Utilization: new(0.4), ResetsAt: extraReset}
	tests := []struct {
		name      string
		replies   map[string]reply
		want      quota.Probe
		wantErr   string
		wantAsked []string
	}{
		{
			name: "both families answer, keeping the higher reading of a shared window",
			replies: map[string]reply{
				haiku: withUsage(http.StatusOK, session, week),
				fable: withUsage(http.StatusOK, nudged, week, fableWeek),
			},
			want: probed(
				quota.Usage{Windows: []quota.Window{nudged, week, fableWeek}},
				map[string][]string{"5h": {haiku, fable}, "7d": {haiku, fable}, "7d_oi": {fable}},
			),
			wantAsked: []string{haiku, fable},
		},
		{
			name: "an exhausted account, read off 429s",
			replies: map[string]reply{
				haiku: withUsage(http.StatusTooManyRequests, rejected, week),
				fable: withUsage(http.StatusTooManyRequests, rejected, week, fableWeek),
			},
			want: quota.Probe{
				Usage:  quota.Usage{Windows: []quota.Window{rejected, week, fableWeek}},
				Models: map[string][]string{"5h": {haiku, fable}, "7d": {haiku, fable}, "7d_oi": {fable}},
			},
			wantAsked: []string{haiku, fable},
		},
		{
			name: "an account taking one family's requests, its Fable week spent",
			replies: map[string]reply{
				haiku: withUsage(http.StatusOK, session, week),
				fable: withUsage(http.StatusTooManyRequests, session, week, spentFableWeek),
			},
			want: probed(
				quota.Usage{Windows: []quota.Window{session, week, spentFableWeek}},
				map[string][]string{"5h": {haiku, fable}, "7d": {haiku, fable}, "7d_oi": {fable}},
			),
			wantAsked: []string{haiku, fable},
		},
		{
			name: "a family falls back to its older model",
			replies: map[string]reply{
				haiku:      withUsage(http.StatusOK, session, week),
				fable:      overloaded,
				fableOlder: withUsage(http.StatusOK, session, week, fableWeek),
			},
			want: probed(
				quota.Usage{Windows: []quota.Window{session, week, fableWeek}},
				map[string][]string{"5h": {haiku, fableOlder}, "7d": {haiku, fableOlder}, "7d_oi": {fableOlder}},
			),
			wantAsked: []string{haiku, fable, fableOlder},
		},
		{
			name: "a family that reads nothing is a failure, with its last model's error",
			replies: map[string]reply{
				haiku:      withUsage(http.StatusOK, session, week),
				fable:      overloaded,
				fableOlder: apiError(http.StatusNotFound, "model: claude-fable-5"),
			},
			want: probed(
				quota.Usage{
					Windows:  []quota.Window{session, week},
					Failures: []quota.Failure{{Label: "Fable", Window: "7d_oi", Error: "HTTP 404 · model: claude-fable-5"}},
				},
				map[string][]string{"5h": {haiku}, "7d": {haiku}},
			),
			wantAsked: []string{haiku, fable, fableOlder},
		},
		{
			name: "no failure for a window another family read",
			replies: map[string]reply{
				haiku:      withUsage(http.StatusOK, session, week, fableWeek),
				fable:      overloaded,
				fableOlder: overloaded,
			},
			want: probed(
				quota.Usage{Windows: []quota.Window{session, week, fableWeek}},
				map[string][]string{"5h": {haiku}, "7d": {haiku}, "7d_oi": {haiku}},
			),
			wantAsked: []string{haiku, fable, fableOlder},
		},
		{
			name: "the base failing while Fable answers",
			replies: map[string]reply{
				haiku: overloaded,
				fable: withUsage(http.StatusOK, session, week, fableWeek),
			},
			want: probed(
				quota.Usage{Windows: []quota.Window{session, week, fableWeek}},
				map[string][]string{"5h": {fable}, "7d": {fable}, "7d_oi": {fable}},
			),
			wantAsked: []string{haiku, fable},
		},
		{
			name: "extra usage read with the windows, the base family's first",
			replies: map[string]reply{
				haiku: withUsage(http.StatusOK, session, week).withExtra(extraOff),
				fable: withUsage(http.StatusOK, session, week, fableWeek).withExtra(extraOn),
			},
			want: probed(
				quota.Usage{Windows: []quota.Window{session, week, fableWeek}, Extra: extraOff},
				map[string][]string{"5h": {haiku, fable}, "7d": {haiku, fable}, "7d_oi": {fable}},
			),
			wantAsked: []string{haiku, fable},
		},
		{
			name: "extra usage from the family that gives it",
			replies: map[string]reply{
				haiku: withUsage(http.StatusOK, session, week),
				fable: withUsage(http.StatusOK, session, week, fableWeek).withExtra(extraOn),
			},
			want: probed(
				quota.Usage{Windows: []quota.Window{session, week, fableWeek}, Extra: extraOn},
				map[string][]string{"5h": {haiku, fable}, "7d": {haiku, fable}, "7d_oi": {fable}},
			),
			wantAsked: []string{haiku, fable},
		},
		{
			name: "extra usage of a family that read no window left out",
			replies: map[string]reply{
				haiku:      withUsage(http.StatusOK, session, week),
				fable:      apiError(http.StatusNotFound, "model: claude-fable-5-1").withExtra(extraOn),
				fableOlder: apiError(http.StatusNotFound, "model: claude-fable-5"),
			},
			want: probed(
				quota.Usage{
					Windows:  []quota.Window{session, week},
					Failures: []quota.Failure{{Label: "Fable", Window: "7d_oi", Error: "HTTP 404 · model: claude-fable-5"}},
				},
				map[string][]string{"5h": {haiku}, "7d": {haiku}},
			),
			wantAsked: []string{haiku, fable, fableOlder},
		},
		{
			name: "every family failing, with the base family's error",
			replies: map[string]reply{
				haiku:      apiError(http.StatusUnauthorized, "Invalid bearer token"),
				fable:      overloaded,
				fableOlder: overloaded,
			},
			wantErr:   "HTTP 401 · Invalid bearer token",
			wantAsked: []string{haiku, fable, fableOlder},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(t, tt.replies)
			prober := &claude.Prober{Upstream: api.URL, Version: version}

			got, err := prober.Probe(t.Context(), token)
			if gotErr := errorText(err); gotErr != tt.wantErr {
				t.Errorf("Probe() error = %q, want %q", gotErr, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Probe() =\n%+v\nwant\n%+v", got, tt.want)
			}
			if want := slices.Sorted(slices.Values(tt.wantAsked)); !slices.Equal(api.asked(), want) {
				t.Errorf("models probed = %v, want %v", api.asked(), want)
			}
		})
	}
}

// probed is a probe that read usage, a request of it answered with success,
// with models naming, by window key, the models that reported each window.
func probed(usage quota.Usage, models map[string][]string) quota.Probe {
	return quota.Probe{Usage: usage, Models: models, Admitted: true}
}

func TestProbeAsksTheFamiliesAtOnce(t *testing.T) {
	nudged := session
	nudged.Utilization = 0.24
	// Each family's reply awaits the other family's request, which can only
	// arrive in time if the families are probed at the same time.
	base := withUsage(http.StatusOK, nudged, week)
	base.awaits = fable
	fableFamily := withUsage(http.StatusOK, session, week, fableWeek)
	fableFamily.awaits = haiku
	api := newFakeAPI(t, map[string]reply{haiku: base, fable: fableFamily})
	prober := &claude.Prober{Upstream: api.URL, Version: version, Timeout: time.Second}

	got, err := prober.Probe(t.Context(), token)
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if want := []quota.Window{nudged, week, fableWeek}; !reflect.DeepEqual(got.Windows, want) {
		t.Errorf("Probe() read\n%+v\nwant\n%+v", got.Windows, want)
	}
}

func TestProbeFallsBackFromAModelThatTimesOut(t *testing.T) {
	api := newFakeAPI(t, map[string]reply{
		haiku:      withUsage(http.StatusOK, session, week),
		fable:      {hang: true},
		fableOlder: withUsage(http.StatusOK, session, week, fableWeek),
	})
	// Short enough to wait out, long enough for the answering models under the
	// race detector.
	prober := &claude.Prober{Upstream: api.URL, Version: version, Timeout: 250 * time.Millisecond}

	got, err := prober.Probe(t.Context(), token)
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	want := probed(
		quota.Usage{Windows: []quota.Window{session, week, fableWeek}},
		map[string][]string{"5h": {haiku, fableOlder}, "7d": {haiku, fableOlder}, "7d_oi": {fableOlder}},
	)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Probe() =\n%+v\nwant\n%+v", got, want)
	}
}

func TestProbeErrorsNeverContainTheToken(t *testing.T) {
	tests := []struct {
		name    string
		message string
		wantErr string
	}{
		{
			name:    "in the API's error message",
			message: "invalid bearer token " + token,
			wantErr: "HTTP 401 · invalid bearer token [redacted]",
		},
		{
			name:    "straddling where the message is cut",
			message: strings.Repeat("x", 55) + token,
			wantErr: "HTTP 401 · " + strings.Repeat("x", 55) + "[reda",
		},
		{
			name:    "shaped like a token but not this one",
			message: "invalid x-api-key sk-ant-oat01-fake_token-shaped",
			wantErr: "HTTP 401 · invalid x-api-key [redacted]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rejected := apiError(http.StatusUnauthorized, tt.message)
			api := newFakeAPI(t, map[string]reply{haiku: rejected, fable: rejected, fableOlder: rejected})
			prober := &claude.Prober{Upstream: api.URL, Version: version}

			_, err := prober.Probe(t.Context(), token)
			if errorText(err) != tt.wantErr {
				t.Errorf("Probe() error = %q, want %q", errorText(err), tt.wantErr)
			}
		})
	}

	t.Run("in the upstream's path", func(t *testing.T) {
		upstream := closedServerURL(t)
		prober := &claude.Prober{Upstream: upstream + "/" + token, Version: version}

		_, err := prober.Probe(t.Context(), token)
		if want := `Post "` + upstream + `/[redacted]/v1/messages": `; !strings.HasPrefix(errorText(err), want) {
			t.Errorf("Probe() error = %q, want it to start %q", errorText(err), want)
		}
		if strings.Contains(errorText(err), token) {
			t.Error("Probe() error contains the token")
		}
	})
}

// sentRequest is what a probe sent the API.
type sentRequest struct {
	method string
	url    *url.URL
	header http.Header
	body   []byte
}

// reply is how the fake API answers a request for one model.
type reply struct {
	status int
	header http.Header
	body   string
	// hang leaves the request unanswered until the client gives up.
	hang bool
	// awaits holds the reply until the API has been asked about that model too.
	awaits string
}

// withUsage returns a reply whose headers carry windows.
func withUsage(status int, windows ...quota.Window) reply {
	h := http.Header{}
	for _, w := range windows {
		prefix := "anthropic-ratelimit-unified-" + w.Key + "-"
		h.Set(prefix+"utilization", strconv.FormatFloat(w.Utilization, 'f', -1, 64))
		h.Set(prefix+"reset", strconv.FormatInt(w.ResetsAt.Unix(), 10))
		h.Set(prefix+"status", string(w.Status))
	}
	return reply{status: status, header: h, body: `{"type":"message"}`}
}

// withExtra returns the reply with its headers carrying extra usage too.
func (r reply) withExtra(e quota.ExtraUsage) reply {
	h := http.Header{}
	maps.Copy(h, r.header)
	h.Set("anthropic-ratelimit-unified-overage-status", string(e.Status))
	h.Set("anthropic-ratelimit-unified-overage-utilization", strconv.FormatFloat(*e.Utilization, 'f', -1, 64))
	h.Set("anthropic-ratelimit-unified-overage-reset", strconv.FormatInt(e.ResetsAt.Unix(), 10))
	r.header = h
	return r
}

// apiError returns a reply without usage headers, whose body is the API's
// error carrying message.
func apiError(status int, message string) reply {
	return reply{status: status, body: fmt.Sprintf(`{"type":"error","error":{"type":"api_error","message":%q}}`, message)}
}

// fakeAPI is a messages API that answers each model with its reply.
type fakeAPI struct {
	URL string
	// arrived holds a channel per model, closed when the API is first asked about it.
	arrived map[string]chan struct{}

	mu     sync.Mutex
	models []string
}

func newFakeAPI(t *testing.T, replies map[string]reply) *fakeAPI {
	t.Helper()
	api := &fakeAPI{arrived: make(map[string]chan struct{}, len(replies))}
	for model := range replies {
		api.arrived[model] = make(chan struct{})
	}
	released := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		model := api.record(t, r)
		rep, ok := replies[model]
		if !ok {
			t.Errorf("probed model %q, which the test gave no reply", model)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if !api.hold(r.Context(), rep, released) {
			return
		}
		maps.Copy(w.Header(), rep.header)
		w.WriteHeader(rep.status)
		_, _ = io.WriteString(w, rep.body)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(released) }) // runs first, so Close isn't left waiting on a held reply
	api.URL = srv.URL
	return api
}

// record notes the model a request is for, and returns it. It reads the whole
// body, which is what lets the server notice a client that gives up.
func (a *fakeAPI) record(t *testing.T, r *http.Request) string {
	var req struct {
		Model string `json:"model"`
	}
	body, err := io.ReadAll(r.Body)
	if err == nil {
		err = json.Unmarshal(body, &req)
	}
	if err != nil {
		t.Errorf("read request body: %v", err)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if arrived, ok := a.arrived[req.Model]; ok && !slices.Contains(a.models, req.Model) {
		close(arrived)
	}
	a.models = append(a.models, req.Model)
	return req.Model
}

// hold keeps back a reply that hangs or awaits another model, and reports
// whether to send it after all.
func (a *fakeAPI) hold(ctx context.Context, rep reply, released <-chan struct{}) bool {
	var until chan struct{} // nil for a reply that hangs: it's held until abandoned
	switch {
	case rep.awaits != "":
		until = a.arrived[rep.awaits]
	case !rep.hang:
		return true
	}
	select {
	case <-until:
		return true
	case <-ctx.Done():
	case <-released:
	}
	return false
}

// asked returns the models the API was asked about, sorted.
func (a *fakeAPI) asked() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Sorted(slices.Values(a.models))
}

func TestProbeSaysWhetherTheAPIRefusedIt(t *testing.T) {
	tests := []struct {
		name        string
		reply       reply
		wantRefused bool
	}{
		{name: "401, the token refused", reply: apiError(http.StatusUnauthorized, "Invalid bearer token"), wantRefused: true},
		{name: "403, the request refused", reply: apiError(http.StatusForbidden, "Not allowed"), wantRefused: true},
		{name: "400", reply: apiError(http.StatusBadRequest, "Invalid request")},
		{name: "429 without usage", reply: apiError(http.StatusTooManyRequests, "Rate limited")},
		{name: "529", reply: apiError(529, "Overloaded")},
		{name: "no answer in time", reply: reply{hang: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(t, map[string]reply{haiku: tt.reply, fable: tt.reply, fableOlder: tt.reply})
			prober := &claude.Prober{Upstream: api.URL, Version: version, Timeout: 250 * time.Millisecond}

			_, err := prober.Probe(t.Context(), token)
			if err == nil {
				t.Fatal("Probe() succeeded, want an error")
			}
			if got := refused(err); got != tt.wantRefused {
				t.Errorf("Probe() error %q says the API refused it: %v, want %v", err, got, tt.wantRefused)
			}
		})
	}

	t.Run("no API to answer", func(t *testing.T) {
		prober := &claude.Prober{Upstream: closedServerURL(t), Version: version}

		_, err := prober.Probe(t.Context(), token)
		if err == nil || refused(err) {
			t.Errorf("Probe() error = %v, want one that doesn't say the API refused it", err)
		}
	})
}

// refused reports whether a probe's error says the API refused the probe.
func refused(err error) bool {
	r, ok := errors.AsType[interface {
		error
		Refused() bool
	}](err)
	return ok && r.Refused()
}

// closedServerURL returns the URL of a server that has shut down, so
// connecting to it fails.
func closedServerURL(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	return srv.URL
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
