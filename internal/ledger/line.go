package ledger

import (
	"time"

	"github.com/leeovery/switchboard/internal/prose"
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
	// listMost is how many of a list's entries a line holds.
	listMost = 32
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
	// Session is the id of the session the request belongs to, and Model the
	// model it asked for.
	Session string `json:"session,omitempty"`
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
	Bytes       int        `json:"bytes"`
	Messages    int        `json:"messages"`
	System      int        `json:"system"`
	Tools       int        `json:"tools"`
	MaxTokens   *int64     `json:"max_tokens,omitempty"`
	Thinking    Thinking   `json:"thinking,omitzero"`
	Stream      *bool      `json:"stream,omitempty"`
	ToolChoice  ToolChoice `json:"tool_choice,omitzero"`
	Temperature *float64   `json:"temperature,omitempty"`
	ServiceTier string     `json:"service_tier,omitempty"`
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

// written returns the line as it's written: at in UTC, to the millisecond,
// each of its texts cut to textMost bytes, and each of its lists to
// listMost, so no line runs past lineMax.
func (l *Line) written() *Line {
	w := *l
	w.At = l.At.UTC().Truncate(time.Millisecond)
	w.Request, w.Kind, w.Session, w.Model = cut(l.Request), cut(l.Kind), cut(l.Session), cut(l.Model)
	w.Account, w.Reason, w.From, w.Agent = cut(l.Account), cut(l.Reason), cut(l.From), cut(l.Agent)
	w.Tried = nil
	for _, t := range l.Tried[:min(len(l.Tried), listMost)] {
		w.Tried = append(w.Tried, Tried{Account: cut(t.Account), Why: cut(t.Why)})
	}
	w.Betas = cutAll(l.Betas)
	w.Shape.Thinking.Type, w.Shape.Thinking.Display = cut(l.Shape.Thinking.Type), cut(l.Shape.Thinking.Display)
	w.Shape.ToolChoice.Type, w.Shape.ServiceTier = cut(l.Shape.ToolChoice.Type), cut(l.Shape.ServiceTier)
	return &w
}

// cut is s cut to textMost bytes, at the end of a character.
func cut(s string) string {
	return prose.TruncateBytes(s, textMost)
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
