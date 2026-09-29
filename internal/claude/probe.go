// Package claude is the Claude provider. It reads an account's usage windows
// off the rate-limit headers the API sends with every response, and probes
// for them when there's no traffic to read them from.
package claude

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
)

// defaultTimeout bounds each request. Healthy probes answer in under a second,
// but an account/model pair that 529s has taken 12 seconds to fail.
const defaultTimeout = 5 * time.Second

// systemPrompt must open the system prompt of a request made with a Claude Code
// token, or the API answers a generic 400. Haiku is exempt today; the probe
// doesn't rely on that.
const systemPrompt = "You are Claude Code, Anthropic's official CLI for Claude."

const (
	// maxErrorBody bounds how much of a response is read for its error message.
	maxErrorBody = 64 << 10
	// maxErrorMessage is how many characters of that message an error keeps.
	maxErrorMessage = 60
)

const redacted = "[redacted]"

// tokenShaped matches Claude API keys and OAuth tokens, which all begin sk-ant-.
var tokenShaped = regexp.MustCompile(`sk-ant-[A-Za-z0-9_-]+`)

// probeClient sends probes, over http.DefaultTransport, following no
// redirect: a client that did would carry the token along to the upstream's
// host, or any subdomain of it, over plain http too.
var probeClient = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// Prober reads Claude accounts' usage by sending a model of each family a
// request capped at one output token, and reading the usage headers off the
// response. The headers report only the weekly caps that apply to the model
// asked, so each family's cap needs a probe of its own.
//
// The API's usage endpoint would spend nothing, but it 429s aggressively
// (verified 2026-06: rate-limited on the very first request).
type Prober struct {
	// Upstream is the API's base URL, such as https://api.anthropic.com.
	Upstream string
	// Version is the Claude Code version the requests claim, such as
	// "2.1.283": see InstalledVersion. Empty means a floor version.
	Version string
	// Timeout bounds each request. Zero means five seconds.
	Timeout time.Duration
}

// reading is what probing one family found: the windows its model reported,
// or why none did.
type reading struct {
	model   string
	windows []quota.Window
	err     error
}

// Probe reads an account's usage: it probes the model families concurrently
// and merges what they report, noting which models reported each window. A
// family that owns a window and reads nothing is reported as a failure, unless
// another family read that window anyway. Probe fails only when no family
// reads anything, with the base family's error.
func (p *Prober) Probe(ctx context.Context, token string) (quota.Probe, error) {
	readings := make([]reading, len(families))
	var wg sync.WaitGroup
	for i, f := range families {
		wg.Go(func() { readings[i] = p.probeFamily(ctx, token, f) })
	}
	wg.Wait()
	return combine(readings)
}

// probeFamily tries the family's models in order until one reports usage.
func (p *Prober) probeFamily(ctx context.Context, token string, f family) reading {
	var r reading
	for _, model := range f.models {
		r.model = model
		if r.windows, r.err = p.ProbeModel(ctx, token, model); r.err == nil {
			break
		}
	}
	return r
}

// combine merges readings, which are in families' order, into an account's usage.
func combine(readings []reading) (quota.Probe, error) {
	probe := quota.Probe{Models: make(map[string][]string)}
	for _, r := range readings {
		probe.Windows = quota.Merge(probe.Windows, r.windows)
		for _, w := range r.windows {
			probe.Models[w.Key] = append(probe.Models[w.Key], r.model)
		}
	}
	if len(probe.Windows) == 0 {
		return quota.Probe{}, readings[0].err
	}
	for i, f := range families {
		if r := readings[i]; r.err != nil && f.window != "" && !hasWindow(probe.Windows, f.window) {
			probe.Failures = append(probe.Failures, quota.Failure{Label: f.label, Window: f.window, Error: r.err.Error()})
		}
	}
	return probe, nil
}

func hasWindow(windows []quota.Window, key string) bool {
	return slices.ContainsFunc(windows, func(w quota.Window) bool { return w.Key == key })
}

// ProbeModel sends model a request capped at one output token and reads the
// usage windows off the response. Any response that carries them counts: the
// API sends them on a 429 as well as a 200. Its errors never contain the token.
func (p *Prober) ProbeModel(ctx context.Context, token, model string) ([]quota.Window, error) {
	windows, err := p.probeModel(ctx, token, model)
	if err != nil {
		// Not wrapped: the cause's own text can carry the token, as a transport
		// error quotes the URL.
		return nil, errors.New(redact(err.Error(), token))
	}
	return windows, nil
}

func (p *Prober) probeModel(ctx context.Context, token, model string) ([]quota.Window, error) {
	timeout := cmp.Or(p.Timeout, defaultTimeout)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := p.newRequest(ctx, token, model)
	if err != nil {
		return nil, err
	}
	resp, err := probeClient.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("timed out after %s", timeout)
		}
		return nil, err
	}
	defer drainAndClose(resp.Body)
	if windows := ParseWindows(resp.Header); len(windows) > 0 {
		return windows, nil
	}
	return nil, errors.New(noUsageReason(resp, token))
}

type messagesRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system"`
	Messages  []message `json:"messages"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (p *Prober) newRequest(ctx context.Context, token, model string) (*http.Request, error) {
	endpoint, err := url.JoinPath(p.Upstream, "v1", "messages")
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(messagesRequest{
		Model:     model,
		MaxTokens: 1,
		System:    systemPrompt,
		Messages:  []message{{Role: "user", Content: "."}},
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	// A request that doesn't look like Claude Code lands in an aggressively
	// rate-limited bucket that 429s persistently.
	req.Header.Set("User-Agent", "claude-code/"+cmp.Or(p.Version, fallbackVersion))
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

// noUsageReason says why a response carried no usage: its status, and the API's
// error message when the body has one.
func noUsageReason(resp *http.Response, token string) string {
	reason := fmt.Sprintf("HTTP %d", resp.StatusCode)
	message := errorMessage(resp.Body, token)
	if message == "" {
		return reason
	}
	// Redacted before it's cut short, so the cut can't leave part of the token.
	return reason + " · " + truncate(message, maxErrorMessage)
}

// errorMessage returns the message of the API error body holds, reading no
// more than maxErrorBody of it, with the token and anything else shaped like a
// Claude token hidden; or "" when it holds none.
func errorMessage(body io.Reader, token string) string {
	var apiError struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(body, maxErrorBody)).Decode(&apiError); err != nil {
		return ""
	}
	return redact(apiError.Error.Message, token)
}

// redact hides the token, and anything else shaped like a Claude token, in text.
func redact(text, token string) string {
	if token != "" {
		text = strings.ReplaceAll(text, token, redacted)
	}
	return tokenShaped.ReplaceAllString(text, redacted)
}

// truncate keeps the first n characters of s.
func truncate(s string, n int) string {
	if runes := []rune(s); len(runes) > n {
		return string(runes[:n])
	}
	return s
}

// drainAndClose reads the body to its end before closing it, so its
// connection can carry the next request.
func drainAndClose(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, body)
	_ = body.Close()
}
