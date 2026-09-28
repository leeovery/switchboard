package claude

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/leeovery/switchboard/internal/quota"
)

// SessionHeader names the Claude Code session a request belongs to. Claude
// Code sends it on every request to the messages API, and the id survives
// --resume.
const SessionHeader = "X-Claude-Code-Session-Id"

// messagesPath is the messages API, the one whose requests the router may
// send on another account.
const messagesPath = "/v1/messages"

// Provider is Claude's side of the router: which requests may go out on
// another account's token, and what the requests and their responses say.
type Provider struct{}

// Routable reports whether a request to path may go out on another account's
// token: only the messages API's, such as /v1/messages and its
// /v1/messages/count_tokens. Every other path keeps the client's own token,
// as the identity-bound ones, such as /v1/code/… and file uploads, must.
func (Provider) Routable(path string) bool {
	return path == messagesPath || strings.HasPrefix(path, messagesPath+"/")
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

// Usage reads the usage windows off a response's headers, as ParseWindows
// does.
func (Provider) Usage(h http.Header) []quota.Window {
	return ParseWindows(h)
}
