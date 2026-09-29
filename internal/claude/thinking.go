package claude

import (
	"slices"
	"strings"
)

// thinkingBound are the models whose thinking works only in the account that
// produced it, or one linked to it, each named by how its ids start, which
// its snapshots share. A request on another account has the thinking dropped
// before the model sees it, silently, and succeeds without that reasoning.
// Claude Sonnet 5.5's is bound, since 28 September 2026; Claude Opus 5.5's
// and Claude Fable 5.1's aren't.
var thinkingBound = []string{"claude-sonnet-5-5"}

// ThinkingBound reports whether the thinking model produces is bound to the
// account that produced it, so a conversation on it moved to another account
// carries on without its earlier reasoning.
func (Provider) ThinkingBound(model string) bool {
	return slices.ContainsFunc(thinkingBound, func(prefix string) bool { return strings.HasPrefix(model, prefix) })
}
