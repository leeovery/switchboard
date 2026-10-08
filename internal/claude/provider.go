package claude

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
)

// SessionHeader names the Claude Code session a request belongs to. Claude
// Code sends it on every request to the messages API, and the id survives
// --resume.
const SessionHeader = "X-Claude-Code-Session-Id"

// messagesPath is the request that sends a message.
const messagesPath = "/v1/messages"

// routablePaths are the requests that may go out on another account's token:
// sending a message and counting its tokens, which leave nothing behind on
// the account. The rest of the messages API, such as batches, names things
// that belong to the account that made them.
var routablePaths = []string{messagesPath, messagesPath + "/count_tokens"}

// Provider is Claude's side of the router: which requests may go out on
// another account's token, and what the requests and their responses say.
type Provider struct{}

// Routable reports whether a request to path may go out on another account's
// token: only one to send a message or count its tokens, at exactly those
// paths. Every other path keeps the client's own token, as the
// identity-bound ones, such as /v1/code/…, file uploads and batches, must.
func (Provider) Routable(path string) bool {
	return slices.Contains(routablePaths, path)
}

// Spends reports whether a request to path spends its account's quota:
// sending a message does, and counting its tokens doesn't, so its success
// says nothing of the account's limits.
func (Provider) Spends(path string) bool {
	return path == messagesPath
}

// Session returns the id of the Claude Code session a request belongs to, or
// "" when it doesn't say.
func (Provider) Session(h http.Header) string {
	return h.Get(SessionHeader)
}

// betaHeader lists the features a request asks the API for beyond its
// version, separated by commas.
const betaHeader = "Anthropic-Beta"

// Betas returns the features a request's header asks the API for beyond its
// version, in the order its anthropic-beta header lists them.
func (Provider) Betas(h http.Header) []string {
	var betas []string
	for _, listed := range h.Values(betaHeader) {
		for beta := range strings.SplitSeq(listed, ",") {
			if beta = strings.TrimSpace(beta); beta != "" {
				betas = append(betas, beta)
			}
		}
	}
	return betas
}

// quotaCheck is what Claude Code's quota check asks, in its one message.
const quotaCheck = "quota"

// Asks reads a messages request's body for the model it asks for, or "" when
// the body doesn't say; whether it's Claude Code's quota check: one message,
// quota, answered with a token at most, which Claude Code sends as it starts;
// and its shape, as the request ledger keeps it, the zero Shape where the body
// is neither a JSON object nor null. The body is read whole once, as it can
// run to megabytes, its lists counted without their entries being read; only
// one asking for a token at most is read again, for its message.
func (Provider) Asks(body []byte) (model string, check bool, shape ledger.Shape) {
	var req asked
	if !readAsked(body, &req) {
		return "", false, ledger.Shape{}
	}
	check = req.MaxTokens.given && req.MaxTokens.read == 1 && asksOnly(body, quotaCheck)
	return req.Model, check, req.shape(len(body))
}

// asksOnly reports whether a messages request's body has one message, asking
// text: as a string, or a block of text.
func asksOnly(body []byte, text string) bool {
	var req struct {
		Messages []struct {
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if json.Unmarshal(body, &req) != nil || len(req.Messages) != 1 {
		return false
	}
	content := req.Messages[0].Content
	var asked string
	if json.Unmarshal(content, &asked) == nil {
		return asked == text
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	return json.Unmarshal(content, &blocks) == nil && len(blocks) == 1 && blocks[0].Type == "text" && blocks[0].Text == text
}

// modelFamilies are the families Claude's models come in. A model's id names
// its family as one of its hyphenated parts, wherever it falls: both
// claude-opus-5-5 and claude-3-5-haiku-20241022 do.
var modelFamilies = []string{"haiku", "sonnet", "opus", "fable"}

// Family returns the family a model belongs to, such as "opus" for
// claude-opus-5-5, or the model's own id when it names no family. Models of a
// family are counted against the same windows.
func (Provider) Family(model string) string {
	for part := range strings.SplitSeq(model, "-") {
		if slices.Contains(modelFamilies, part) {
			return part
		}
	}
	return model
}

// Usage reads the account's usage off a response's headers, its windows and
// its extra usage, as ParseUsage does.
func (Provider) Usage(h http.Header) quota.Usage {
	return ParseUsage(h)
}

// ErrorMessage returns the message of the API error a response's body holds,
// reading no more than 64 KiB of it, with token and anything else shaped like
// a Claude token hidden; or "" when it holds none.
func (Provider) ErrorMessage(body io.Reader, token string) string {
	return errorMessage(body, token)
}
