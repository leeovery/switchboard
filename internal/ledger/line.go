package ledger

import (
	"encoding/json"
	"maps"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/redact"
)

// What a request asked, as a line's Kind gives it.
const (
	// KindMessage is a request that spends its account's quota.
	KindMessage = "message"
	// KindCheck is Claude Code's quota check, which spends a token to see
	// the account has quota.
	KindCheck = "check"
	// KindCount is a count of a request's tokens, which spends nothing.
	KindCount = "count"
)

const (
	// textMost is how many bytes of a text a line holds, cut at the end of a
	// character, as the request stream cuts a session's id and a model's.
	textMost = 200
	// listMost is how many of a list's entries a line holds, or kinds of a
	// count's.
	listMost = 32
	// limitsMost is how many of an answer's usage headers a line holds: the
	// API gives some twenty.
	limitsMost = 64
	// usageMost is the most bytes of an answer's usage a line holds: the API
	// gives a few hundred, and a line leaves out one longer.
	usageMost = 8 << 10
)

// Line is a request the router routed, as the ledger holds it, a line of
// JSON: how the router handled it, and what of it switchboard knows. It
// holds nothing of the request's content, and an account by its id alone,
// never its token or label. Fields may be added to it, never renamed, and
// each that's optional is left out where it isn't given.
type Line struct {
	// At is when the request arrived, written in UTC, to the millisecond.
	At time.Time `json:"at"`
	// Request is the router's id for the request, as its routed line in the
	// log gives it, and Kind what it asked: KindMessage, KindCheck or
	// KindCount.
	Request string `json:"request"`
	Kind    string `json:"kind"`
	// Session is the id of the session the request belongs to, Dir the
	// directory run started claude in, as it tells the router, and Model the
	// model it asked for.
	Session string `json:"session,omitempty"`
	Dir     string `json:"dir,omitempty"`
	Model   string `json:"model,omitempty"`
	// Account is the account whose answer the client got, none where the
	// router answered it itself, and Reason why it was chosen, as the
	// routed line gives it.
	Account string `json:"account,omitempty"`
	Reason  string `json:"reason"`
	// From is the account the request's session was on before, where the
	// request moved it.
	From string `json:"from,omitempty"`
	// Tried are the accounts the request went out on that couldn't serve
	// it, in the order tried.
	Tried []Tried `json:"tried,omitempty"`
	// Status is what the client was answered, 0 where it went away before an
	// answer, and Canceled is set where it went away before the end.
	Status   int  `json:"status"`
	Canceled bool `json:"canceled,omitzero"`
	// Attempts is how many times the request went upstream, 0 where the
	// router answered it itself.
	Attempts int `json:"attempts"`
	// FirstMS is how long after the request arrived its answer's first byte
	// came, nil where none came, and TotalMS its end, in milliseconds.
	FirstMS *int64 `json:"first_ms,omitempty"`
	TotalMS int64  `json:"total_ms"`
	// Agent is the client's user agent, and Betas the features its request
	// asked the API for beyond its version, some of which change what a
	// request costs.
	Agent string   `json:"agent,omitempty"`
	Betas []string `json:"betas,omitempty"`
	// Shape is the request's shape, zero where its body isn't a request.
	Shape Shape `json:"shape,omitzero"`
	// Reply is what came back, of the answer the client got.
	Reply
}

// Tried is an account a request went out on that couldn't serve it, and why
// it was left, as "hit its limit".
type Tried struct {
	Account string `json:"account"`
	Why     string `json:"why"`
}

// Shape is a request's shape: its size in bytes, how many messages, system
// blocks and tools it carried, and those of its settings switchboard knows,
// each left out where the request doesn't give it. A setting switchboard
// doesn't know isn't kept, as one may carry a secret.
type Shape struct {
	Bytes             int               `json:"bytes"`
	Messages          int               `json:"messages"`
	System            int               `json:"system"`
	Tools             int               `json:"tools"`
	MaxTokens         *int64            `json:"max_tokens,omitempty"`
	Thinking          Thinking          `json:"thinking,omitzero"`
	Stream            *bool             `json:"stream,omitempty"`
	ToolChoice        ToolChoice        `json:"tool_choice,omitzero"`
	Temperature       *float64          `json:"temperature,omitempty"`
	TopK              *int64            `json:"top_k,omitempty"`
	TopP              *float64          `json:"top_p,omitempty"`
	ServiceTier       string            `json:"service_tier,omitempty"`
	OutputConfig      OutputConfig      `json:"output_config,omitzero"`
	Speed             string            `json:"speed,omitempty"`
	InferenceGeo      string            `json:"inference_geo,omitempty"`
	ContextManagement ContextManagement `json:"context_management,omitzero"`
}

// Thinking is a request's thinking, of the fields switchboard knows.
type Thinking struct {
	Type         string `json:"type,omitempty"`
	BudgetTokens *int64 `json:"budget_tokens,omitempty"`
	Display      string `json:"display,omitempty"`
}

// ToolChoice is a request's choice of the tools it gives, by its type alone.
type ToolChoice struct {
	Type string `json:"type,omitempty"`
}

// OutputConfig is how a request has its output made, of the fields
// switchboard knows.
type OutputConfig struct {
	Effort string `json:"effort,omitempty"`
}

// ContextManagement is how a request has its context managed: the edits it
// asks for, by their types alone.
type ContextManagement struct {
	Edits []string `json:"edits,omitempty"`
}

// Reply is what the API said back, of the answer the client got, each part
// left out where the answer didn't give it.
type Reply struct {
	// Answer is what the answer told of itself.
	Answer Answer `json:"answer,omitzero"`
	// Usage is the answer's closing usage, as the API gave it, field for
	// field, whatever it counts: none where the answer gave none, as an
	// error, a count of tokens or an answer cut short gives none.
	Usage json.RawMessage `json:"usage,omitempty"`
	// Limits are the answer's anthropic-ratelimit-unified-* headers, by
	// their names, the prefix taken off, and their values as given.
	Limits map[string]string `json:"limits,omitempty"`
}

// Answer is what an answer told of itself: Anthropic's id for it, the model
// it named, why it stopped, how many blocks of each kind it held, the tools
// it called, by name alone, and its error, each left out where it didn't
// give it.
type Answer struct {
	ID     string         `json:"id,omitempty"`
	Model  string         `json:"model,omitempty"`
	Stop   string         `json:"stop,omitempty"`
	Blocks map[string]int `json:"blocks,omitempty"`
	Tools  []string       `json:"tools,omitempty"`
	Error  Error          `json:"error,omitzero"`
}

// Error is the error an answer gave: its type and message.
type Error struct {
	Type    string `json:"type,omitempty"`
	Message string `json:"message,omitempty"`
}

// written returns the line as it's written: at in UTC, to the millisecond,
// each of its texts cut to textMost bytes, a directory from its front, so it
// keeps its own name, and each of its lists and counts to its most, so no
// line runs past lineMax.
func (l *Line) written() *Line {
	w := *l
	w.At = l.At.UTC().Truncate(time.Millisecond)
	w.Request, w.Kind, w.Session, w.Model = cut(l.Request), cut(l.Kind), cut(l.Session), cut(l.Model)
	w.Dir = cutFront(l.Dir)
	w.Account, w.Reason, w.From, w.Agent = cut(l.Account), cut(l.Reason), cut(l.From), cut(l.Agent)
	w.Tried = nil
	for _, t := range l.Tried[:min(len(l.Tried), listMost)] {
		w.Tried = append(w.Tried, Tried{Account: cut(t.Account), Why: cut(t.Why)})
	}
	w.Betas = cutAll(l.Betas)
	w.Shape = l.Shape.written()
	w.Reply = l.Reply.written()
	return &w
}

// written returns the shape as a line writes it.
func (s Shape) written() Shape {
	s.Thinking.Type, s.Thinking.Display = cut(s.Thinking.Type), cut(s.Thinking.Display)
	s.ToolChoice.Type, s.ServiceTier = cut(s.ToolChoice.Type), cut(s.ServiceTier)
	s.OutputConfig.Effort, s.Speed, s.InferenceGeo = cut(s.OutputConfig.Effort), cut(s.Speed), cut(s.InferenceGeo)
	s.ContextManagement.Edits = cutAll(s.ContextManagement.Edits)
	return s
}

// written returns the reply as a line writes it: without its usage, should
// that run past usageMost bytes.
func (r Reply) written() Reply {
	r.Answer = r.Answer.written()
	if len(r.Usage) > usageMost {
		r.Usage = nil
	}
	r.Limits = cutMap(r.Limits, limitsMost, cut)
	return r
}

// written returns the answer as a line writes it.
func (a Answer) written() Answer {
	a.ID, a.Model, a.Stop = cut(a.ID), cut(a.Model), cut(a.Stop)
	a.Error = Error{Type: cut(a.Error.Type), Message: cut(a.Error.Message)}
	a.Blocks = cutMap(a.Blocks, listMost, func(n int) int { return n })
	a.Tools = cutAll(a.Tools)
	return a
}

// cut is s cut to textMost bytes, at the end of a character, once anything
// shaped like a token in it is hidden, so no cut works on a token: hiding one
// goes by its prefix, which a cut through it could part from the rest.
func cut(s string) string {
	return prose.TruncateBytes(redact.Text(s), textMost)
}

// cutFront is s cut as cut cuts it, but from its front, an ellipsis standing
// for what's cut.
func cutFront(s string) string {
	return prose.TruncateBytesFront(redact.Text(s), textMost)
}

// cutAll returns the first listMost of texts, each cut, as a list of its own:
// nil for none.
func cutAll(texts []string) []string {
	var all []string
	for _, s := range texts[:min(len(texts), listMost)] {
		all = append(all, cut(s))
	}
	return all
}

// cutMap returns the first most of m's entries, in the order of their names,
// each name cut and each value as value gives it, as a map of their own: nil
// for none.
func cutMap[V any](m map[string]V, most int, value func(V) V) map[string]V {
	var kept map[string]V
	for _, name := range slices.Sorted(maps.Keys(m))[:min(len(m), most)] {
		if kept == nil {
			kept = make(map[string]V)
		}
		kept[cut(name)] = value(m[name])
	}
	return kept
}
