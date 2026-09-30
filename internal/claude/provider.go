package claude

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"

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

// Model returns the model a messages request's body asks for, or "" when the
// body doesn't say.
func (Provider) Model(body []byte) string {
	var req struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return ""
	}
	return req.Model
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

// Usage reads the usage windows off a response's headers, as ParseWindows
// does.
func (Provider) Usage(h http.Header) []quota.Window {
	return ParseWindows(h)
}

// ErrorMessage returns the message of the API error a response's body holds,
// reading no more than 64 KiB of it, with token and anything else shaped like
// a Claude token hidden; or "" when it holds none.
func (Provider) ErrorMessage(body io.Reader, token string) string {
	return errorMessage(body, token)
}
