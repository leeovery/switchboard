package claude

// Command is the name the Claude Code CLI runs by.
const Command = "claude"

// The variables Claude Code reads from its environment that decide where its
// requests go, and on what.
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
)
