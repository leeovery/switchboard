package prose_test

import (
	"testing"

	"github.com/leeovery/switchboard/internal/prose"
)

// shownText is outside text as a Log row and a session's page show it, by
// what it holds: plain ASCII, as nearly all of it is, beyond ASCII, and
// holding characters drawn as nothing.
var shownText = []struct {
	name string
	text string
}{
	{name: "label", text: "work"},
	{name: "model", text: "claude-opus-5-5"},
	{name: "title", text: "Fix the flaky retry in the router's replay path"},
	{name: "long directory", text: "/Users/me/Code/project/.claude/worktrees/agent-0123456789abcdef0/internal/dashboard/watch"},
	{name: "accented title", text: "Café menu: résumé tidy-up for the naïve façade"},
	{name: "emoji title", text: "Team \U0001f469\u200d\U0001f4bb notes \u2764\ufe0f and a 1\ufe0f\u20e3 keycap"},
	{name: "reordering label", text: "\u202ekrow\u202c and \u2066personal\u2069 \u200bspaced"},
}

func BenchmarkClean(b *testing.B) {
	for _, tt := range shownText {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				prose.Clean(tt.text)
			}
		})
	}
}
