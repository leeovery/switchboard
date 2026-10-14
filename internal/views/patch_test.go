package views_test

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/status/statustest"
	"github.com/leeovery/switchboard/internal/views"
)

// The sessions the patch's tests plug in, by their shown ids.
const (
	s41e0 = "41e0"
	s9c2e = "9c2e"
	sc61b = "c61b"
	sd28c = "d28c"
	sf41a = "f41a"
	s5b19 = "5b19"
)

// group are the calls of the sessions given running on the account given,
// in RunningOf's order.
func group(account string, sessions ...string) []views.Call {
	calls := make([]views.Call, len(sessions))
	for i, s := range sessions {
		calls[i] = views.Call{Account: account, Session: s}
	}
	return calls
}

// patchLook is a look the patch takes: the calls, and each line's stubs.
type patchLook struct {
	calls []views.Call
	stubs map[string]int
}

// looked is the board after the looks given, each kept from the last, from
// the zero Patched.
func looked(looks ...patchLook) views.Patched {
	var p views.Patched
	for _, l := range looks {
		p = views.Patch(p, l.calls, l.stubs)
	}
	return p
}

// boardOf says the lines of the accounts given, as "personal: 9c2e@1 ╌0":
// each call's session on its jack, top to bottom, then each stub's jack,
// the first hung first.
func boardOf(p views.Patched, accounts ...string) string {
	var lines []string
	for _, a := range accounts {
		l := p.Line(a)
		said := []string{a + ":"}
		for _, c := range l.Calls {
			said = append(said, fmt.Sprintf("%s@%d", c.Session, c.Jack))
		}
		for _, jack := range l.Stubs {
			said = append(said, fmt.Sprintf("╌%d", jack))
		}
		lines = append(lines, strings.Join(said, " "))
	}
	return strings.Join(lines, " | ")
}

func TestThePatchKeepsWhoSitsWhereFromLookToLook(t *testing.T) {
	scene := slices.Concat(group("personal", s41e0, s9c2e), group("side", sc61b, sd28c))
	tests := []struct {
		name  string
		looks []patchLook
		want  string
	}{
		{
			name:  "a first look plugs each group's calls in calls' order, each into its line's first free jack",
			looks: []patchLook{{calls: scene}},
			want:  "personal: 41e0@0 9c2e@1 | side: c61b@0 d28c@1",
		},
		{
			name: "rows keep their places, however the calls order them since",
			looks: []patchLook{
				{calls: scene},
				{calls: slices.Concat(group("personal", s9c2e, s41e0), group("side", sd28c, sc61b))},
			},
			want: "personal: 41e0@0 9c2e@1 | side: c61b@0 d28c@1",
		},
		{
			name: "a new session joins its group's foot, on its line's first free jack",
			looks: []patchLook{
				{calls: scene},
				{calls: slices.Concat(group("personal", sf41a, s9c2e, s41e0, s5b19), group("side", sc61b, sd28c))},
			},
			want: "personal: 41e0@0 9c2e@1 f41a@2 5b19@3 | side: c61b@0 d28c@1",
		},
		{
			name: "a group closes up over a session gone, its jack free for the next to join",
			looks: []patchLook{
				{calls: scene},
				{calls: slices.Concat(group("personal", s9c2e), group("side", sc61b, sd28c))},
				{calls: slices.Concat(group("personal", s9c2e, sf41a), group("side", sc61b, sd28c))},
			},
			want: "personal: 9c2e@1 f41a@0 | side: c61b@0 d28c@1",
		},
		{
			name: "a move no limit counts brings a session to its new group's foot, and hangs no stub",
			looks: []patchLook{
				{calls: scene},
				{calls: slices.Concat(group("personal", s9c2e), group("side", sc61b, sd28c, s41e0))},
			},
			want: "personal: 9c2e@1 | side: c61b@0 d28c@1 41e0@2",
		},
		{
			name: "a limit's move hangs a stub from the jack its session left, not its line's first free one, the rest kept where they sit",
			looks: []patchLook{
				{calls: slices.Concat(group("personal", s5b19, s41e0, s9c2e), group("side", sc61b))},
				{calls: slices.Concat(group("personal", s41e0, s9c2e), group("side", sc61b))},
				{calls: slices.Concat(group("personal", s9c2e), group("side", sc61b, s41e0)), stubs: map[string]int{"personal": 1}},
			},
			want: "personal: 9c2e@2 ╌1 | side: c61b@0 41e0@1",
		},
		{
			name: "a second session the limit moves at its next request keeps its row and jack till then, then hangs the second stub",
			looks: []patchLook{
				{calls: slices.Concat(group("personal", s5b19, s41e0, s9c2e), group("side", sc61b))},
				{calls: slices.Concat(group("personal", s41e0, s9c2e), group("side", sc61b))},
				{calls: slices.Concat(group("personal", s9c2e), group("side", sc61b, s41e0)), stubs: map[string]int{"personal": 1}},
				{calls: group("side", sc61b, s41e0, s9c2e), stubs: map[string]int{"personal": 2}},
			},
			want: "personal: ╌1 ╌2 | side: c61b@0 41e0@1 9c2e@2",
		},
		{
			name: "the stream telling of the limit's move before the router's document counts it, the stub hangs from the jack left as it counts it",
			looks: []patchLook{
				{calls: slices.Concat(group("personal", s5b19, s41e0, s9c2e), group("side", sc61b))},
				{calls: slices.Concat(group("personal", s41e0, s9c2e), group("side", sc61b))},
				{calls: slices.Concat(group("personal", s9c2e), group("side", sc61b, s41e0))},
				{calls: slices.Concat(group("personal", s9c2e), group("side", sc61b, s41e0)), stubs: map[string]int{"personal": 1}},
			},
			want: "personal: 9c2e@2 ╌1 | side: c61b@0 41e0@1",
		},
		{
			name: "of the jacks moves left, a limit's stub hangs from the one left last",
			looks: []patchLook{
				{calls: slices.Concat(group("personal", s5b19, s41e0, s9c2e, sf41a), group("side", sc61b))},
				{calls: slices.Concat(group("personal", s41e0, s9c2e), group("side", sc61b, sf41a))},
				{calls: slices.Concat(group("personal", s9c2e), group("side", sc61b, sf41a, s41e0)), stubs: map[string]int{"personal": 1}},
			},
			want: "personal: 9c2e@2 ╌1 | side: c61b@0 f41a@1 41e0@2",
		},
		{
			name:  "a first look hangs the limit's stubs from its line's first free jacks, once its calls have taken theirs",
			looks: []patchLook{{calls: slices.Concat(group("personal", s9c2e), group("side", sc61b)), stubs: map[string]int{"personal": 2}}},
			want:  "personal: 9c2e@0 ╌1 ╌2 | side: c61b@0",
		},
		{
			name: "the stubs go as the limit lifts, their jacks free",
			looks: []patchLook{
				{calls: group("side", sc61b), stubs: map[string]int{"personal": 2}},
				{calls: group("side", sc61b)},
				{calls: slices.Concat(group("personal", sf41a), group("side", sc61b))},
			},
			want: "personal: f41a@0 | side: c61b@0",
		},
		{
			name: "a session on two accounts has a call on each; its model there moving leaves its other call where it sits",
			looks: []patchLook{
				{calls: slices.Concat(group("personal", s41e0, sd28c), group("side", sd28c, sc61b))},
				{calls: slices.Concat(group("personal", s41e0, sd28c), group("side", sc61b))},
			},
			want: "personal: 41e0@0 d28c@1 | side: c61b@1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := boardOf(looked(tt.looks...), "personal", "side"); got != tt.want {
				t.Errorf("the board is\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

// leftOf says the jacks the sessions that moved off the account's line left,
// the last to leave last, as "41e0@0 5b19@2".
func leftOf(p views.Patched, account string) string {
	var left []string
	for _, c := range p.Line(account).Left {
		left = append(left, fmt.Sprintf("%s@%d", c.Session, c.Jack))
	}
	return strings.Join(left, " ")
}

// A cord a move let go hangs loose from the jack its session left, so a line
// keeps each, by its session, till a call or a stub takes it, or its session
// comes back.
func TestALineKeepsTheJackEachSessionThatMovedElsewhereLeftTillItsTaken(t *testing.T) {
	three := slices.Concat(group("personal", s41e0, s9c2e, s5b19), group("side", sc61b))
	moved := slices.Concat(group("personal", s9c2e, s5b19), group("side", sc61b, s41e0))
	twoMoved := slices.Concat(group("personal", s9c2e), group("side", sc61b, s41e0, s5b19))
	tests := []struct {
		name  string
		looks []patchLook
		want  string
	}{
		{name: "a move no limit counts: the jack its session left", looks: []patchLook{{calls: three}, {calls: moved}}, want: "41e0@0"},
		{name: "two moving off: the last to leave last", looks: []patchLook{{calls: three}, {calls: moved}, {calls: twoMoved}}, want: "41e0@0 5b19@2"},
		{
			name:  "a session ending: none, as it went nowhere",
			looks: []patchLook{{calls: three}, {calls: slices.Concat(group("personal", s9c2e, s5b19), group("side", sc61b))}},
		},
		{
			name:  "a session joining the line: none, its jack taken",
			looks: []patchLook{{calls: three}, {calls: moved}, {calls: slices.Concat(group("personal", s9c2e, s5b19, sf41a), group("side", sc61b, s41e0))}},
		},
		{
			name:  "a limit's stub: none, the stub hanging from it",
			looks: []patchLook{{calls: three}, {calls: moved, stubs: map[string]int{"personal": 1}}},
		},
		{
			name: "a session coming back to another jack: none of its own, nor of the jack it takes",
			looks: []patchLook{
				{calls: three}, {calls: moved}, {calls: twoMoved},
				{calls: slices.Concat(group("personal", s9c2e, s5b19), group("side", sc61b, s41e0))},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := leftOf(looked(tt.looks...), "personal"); got != tt.want {
				t.Errorf("personal's jacks left are %q, want %q", got, tt.want)
			}
		})
	}
}

// A board kept is never changed by the patch of a later look, nor by what
// its line's caller does with what it's given.
func TestABoardKeptNeverChangesUnderItsHolder(t *testing.T) {
	kept := looked(patchLook{calls: slices.Concat(group("personal", s41e0, s9c2e), group("side", sc61b))})
	before := boardOf(kept, "personal", "side")
	views.Patch(kept, slices.Concat(group("personal", s9c2e), group("side", sc61b, s41e0)), map[string]int{"personal": 1})
	kept.Line("personal").Calls[0].Session = sd28c
	if got := boardOf(kept, "personal", "side"); got != before {
		t.Errorf("the board kept is %s, want it as it was, %s", got, before)
	}
}

// Each line hangs as many stubs as its limit has moved sessions, as the
// moment counts them while the limit holds: none for another account, or
// once it lifts.
func TestEachLineHangsAStubForEachSessionItsLimitMoved(t *testing.T) {
	tests := []struct {
		name string
		hm   string
		want map[string]int
	}{
		{name: "one moved", hm: "a limit moving the session it refused, at once", want: map[string]int{"work": 1}},
		{name: "two moved", hm: "a limit moving a second session, at its next request", want: map[string]int{"work": 2}},
		{name: "a limit naming no window, moving none", hm: "a limit naming no window holding", want: map[string]int{}},
	}
	moments := statustest.HardMoments(claude.Policy)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := slices.IndexFunc(moments, func(hm statustest.HardMoment) bool { return hm.Name == tt.hm })
			if i < 0 {
				t.Fatalf("no hard moment %q", tt.hm)
			}
			hm := moments[i]
			if got := views.StubsOf(hm.Doc.At(hm.Now, claude.Policy)); !maps.Equal(got, tt.want) {
				t.Errorf("StubsOf(%s) = %v, want %v", tt.hm, got, tt.want)
			}
		})
	}
}
