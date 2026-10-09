package claude

// Command is the name the Claude Code CLI runs by.
const Command = "claude"

// The variables Claude Code reads from its environment that decide where its
// requests go, on what, and what they tell of themselves.
const (
	// BaseURLEnv is the API's base URL, which Claude Code sends its requests
	// to in place of the API's own.
	BaseURLEnv = "ANTHROPIC_BASE_URL"
	// TokenEnv is an OAuth token, such as a setup token, which Claude Code
	// authenticates with in place of its own login.
	TokenEnv = "CLAUDE_CODE_OAUTH_TOKEN"
	// CustomHeadersEnv holds headers Claude Code adds to its requests to the
	// API, a "Name: Value" line each.
	CustomHeadersEnv = "ANTHROPIC_CUSTOM_HEADERS"
	// HintHeadersEnv, set to 1, has Claude Code send a base URL the headers
	// telling of each request it sends the API directly, such as the prompt
	// the request serves; set to 0, it sends them nowhere.
	HintHeadersEnv = "CLAUDE_CODE_GATEWAY_HINT_HEADERS"
)

// keyEnvs are the variables holding a key Claude Code may use in place of a
// subscription's token: an API key, and a token it sends as a bearer token.
var keyEnvs = []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"}

// KeyEnv returns the first of the variables holding a key Claude Code may use
// in place of a subscription's token that getenv finds set: "" when none is.
func KeyEnv(getenv func(key string) string) string {
	for _, name := range keyEnvs {
		if getenv(name) != "" {
			return name
		}
	}
	return ""
}
