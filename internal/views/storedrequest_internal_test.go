package views

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
)

// storedWorlds are the generated worlds whose lines the stored form's tests
// decode: a session's, a run of feeds', and a page's, among other sessions'.
var storedWorlds = []struct {
	name  string
	lines func(seed uint64) []ledger.Line
}{
	{name: "generatedSession", lines: generatedSession},
	{name: "generatedRun", lines: generatedRun},
	{name: "pagedRun", lines: func(seed uint64) []ledger.Line {
		lines, _ := pagedRun(seed)
		return lines
	}},
}

// A request restored from its stored form, as a file holds it, is the one
// decoded of its line, its sum included, of every line of the generated
// worlds.
func TestARequestRestoredFromItsStoredFormIsTheOneDecoded(t *testing.T) {
	for _, w := range storedWorlds {
		for seed := range uint64(150) {
			for i, l := range w.lines(seed) {
				if problem := restoredOtherwise(&l); problem != "" {
					t.Errorf("%s %d, line %d: %s", w.name, seed, i, problem)
				}
			}
		}
	}
}

// Of lines a router never writes, but a file edited by hand could hold, a
// request restored from its stored form is the one decoded of its line all
// the same, exactly.
func TestARequestRestoredFromItsStoredFormIsExactWhateverItsLine(t *testing.T) {
	tests := []struct {
		name string
		edit func(l *ledger.Line)
	}{
		{name: "every name given", edit: func(*ledger.Line) {}},
		{name: "none given", edit: func(l *ledger.Line) { *l = ledger.Line{} }},
		{name: "arrived at no time at all", edit: func(l *ledger.Line) { l.At = time.Time{} }},
		{name: "arrived before 1970", edit: func(l *ledger.Line) { l.At = time.Date(1900, 1, 1, 0, 0, 0, 1, time.UTC) }},
		{name: "arrived to the nanosecond, in another zone", edit: func(l *ledger.Line) {
			l.At = time.Date(2026, 10, 7, 20, 0, 0, 123456789, time.FixedZone("elsewhere", 5*3600))
		}},
		{name: "ended thousands of years on", edit: func(l *ledger.Line) { l.TotalMS = 1 << 50 }},
		{name: "without an id", edit: func(l *ledger.Line) { l.Request = "" }},
		{name: "tried on none, given as an empty list", edit: func(l *ledger.Line) { l.Tried = []ledger.Tried{} }},
		{name: "a tool without a name", edit: func(l *ledger.Line) { l.Answer.Tools = []string{""} }},
		{name: "a tool without a name first", edit: func(l *ledger.Line) { l.Answer.Tools = []string{"", "Bash"} }},
		{name: "a tool without a name last", edit: func(l *ledger.Line) { l.Answer.Tools = []string{"Bash", ""} }},
		{name: "a count's name in capitals", edit: func(l *ledger.Line) { l.Usage = json.RawMessage(`{"Input_Tokens":12,"output_tokens":10}`) }},
		{name: "a quota check", edit: func(l *ledger.Line) { l.Kind = ledger.KindCheck }},
		{name: "upstream without usage", edit: func(l *ledger.Line) { l.Usage, l.CutOff, l.Status = nil, true, 0 }},
		{name: "never upstream", edit: func(l *ledger.Line) { l.Usage, l.Attempts, l.Status = nil, 0, 429 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := sumLine()
			tt.edit(&l)
			if problem := restoredOtherwise(&l); problem != "" {
				t.Error(problem)
			}
		})
	}
}

// restoredOtherwise is how the request of the line l, decoded, stored as a
// file holds it and restored, differs from the one decoded: none where it's
// the same.
func restoredOtherwise(l *ledger.Line) string {
	r := heldRequestOf(l)
	stored, err := json.Marshal(r.stored())
	if err != nil {
		return fmt.Sprintf("request %q: storing it: %v", l.Request, err)
	}
	var back storedRequest
	if err := json.Unmarshal(stored, &back); err != nil {
		return fmt.Sprintf("request %q: reading back %s: %v", l.Request, stored, err)
	}
	if restored := back.restored(); !reflect.DeepEqual(restored, r) {
		return fmt.Sprintf("request %q, stored as %s, restored as %+v, want %+v", l.Request, stored, restored, r)
	}
	return ""
}

// sumLine is a request's line, as the sums' tests edit it: of its session's
// own conversation, moved after an account couldn't serve it, its answer
// calling tools, every name a held request decodes given.
func sumLine() ledger.Line {
	return ledger.Line{
		At: time.Date(2026, 10, 7, 20, 0, 0, 123e6, time.UTC), Request: "r1", Kind: ledger.KindMessage, Session: "s", Dir: "/code/app",
		Model: foldOpus, Account: "side", Reason: "moved: work hit its limit", From: "work", Tried: []ledger.Tried{{Account: "work", Why: "hit its limit"}},
		Status: 200, Attempts: 1, TotalMS: 3000, Class: ledger.ClassMain, AgentID: "a1", Prompt: "p1", Shape: ledger.Shape{InferenceGeo: "us"},
		Usage:  json.RawMessage(foldUsages[2]),
		Answer: ledger.Answer{Stop: "tool_use", Tools: []string{"Read", "Edit"}},
	}
}

// A held request sums otherwise than another where any of what it decodes
// of its line differs, and alike where the lines say otherwise only of what
// none of it decodes, as their usages' bytes; and restored from its stored
// form, it sums as it did.
func TestAHeldRequestSumsAllItDecodes(t *testing.T) {
	tests := []struct {
		name  string
		edit  func(l *ledger.Line)
		alike bool
	}{
		{name: "its id", edit: func(l *ledger.Line) { l.Request = "r2" }},
		{name: "when it arrived", edit: func(l *ledger.Line) { l.At = l.At.Add(time.Nanosecond) }},
		{name: "when it ended", edit: func(l *ledger.Line) { l.TotalMS++ }},
		{name: "its directory", edit: func(l *ledger.Line) { l.Dir = "/code/other" }},
		{name: "its model", edit: func(l *ledger.Line) { l.Model = foldSonnet }},
		{name: "its account", edit: func(l *ledger.Line) { l.Account = "personal" }},
		{name: "why it went there", edit: func(l *ledger.Line) { l.Reason = "sticky" }},
		{name: "the account it moved from", edit: func(l *ledger.Line) { l.From = "personal" }},
		{name: "its class", edit: func(l *ledger.Line) { l.Class = "subagent" }},
		{name: "its subagent", edit: func(l *ledger.Line) { l.AgentID = "a2" }},
		{name: "its geo", edit: func(l *ledger.Line) { l.Shape.InferenceGeo = "" }},
		{name: "its prompt", edit: func(l *ledger.Line) { l.Prompt = "p2" }},
		{name: "why its answer stopped", edit: func(l *ledger.Line) { l.Answer.Stop = "end_turn" }},
		{name: "the tools its answer called", edit: func(l *ledger.Line) { l.Answer.Tools = []string{"Read"} }},
		{name: "the tools' names parted otherwise", edit: func(l *ledger.Line) { l.Answer.Tools = []string{"ReadEdit"} }},
		{name: "whether it went upstream", edit: func(l *ledger.Line) { l.Attempts = 0 }},
		{name: "the accounts it went out on", edit: func(l *ledger.Line) {
			l.Tried = append(l.Tried, ledger.Tried{Account: "personal", Why: "hit its limit"})
		}},
		{name: "why an account couldn't serve it", edit: func(l *ledger.Line) { l.Tried = []ledger.Tried{{Account: "work", Why: "refused"}} }},
		{name: "what it spent", edit: func(l *ledger.Line) { l.Usage = json.RawMessage(foldUsages[3]) }},
		{name: "its usage missing", edit: func(l *ledger.Line) { l.Usage = nil }},
		{name: "its usage written otherwise, the same counts", alike: true, edit: func(l *ledger.Line) {
			l.Usage = json.RawMessage(`{"output_tokens":500, "cache_creation":{"ephemeral_1h_input_tokens":1000,"ephemeral_5m_input_tokens":0},` +
				`"cache_read_input_tokens":100000,"cache_creation_input_tokens":1000,"input_tokens":10,"service_tier":"standard"}`)
		}},
		{name: "its session", alike: true, edit: func(l *ledger.Line) { l.Session = "other" }},
		{name: "its kind, of another the views count", alike: true, edit: func(l *ledger.Line) { l.Kind = "batch" }},
		{name: "its status", alike: true, edit: func(l *ledger.Line) { l.Status = 529 }},
		{name: "its windows", alike: true, edit: func(l *ledger.Line) {
			l.Limits = map[string]string{"anthropic-ratelimit-unified-5h-utilization": "0.5"}
		}},
	}
	base := sumLine()
	was := heldRequestOf(&base)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := sumLine()
			tt.edit(&l)
			r := heldRequestOf(&l)
			if alike := r.sum == was.sum; alike != tt.alike {
				t.Errorf("a line differing in %s sums alike: %t, want %t", tt.name, alike, tt.alike)
			}
			stored := r.stored()
			if restored := stored.restored(); restored.sum != r.sum {
				t.Errorf("a line differing in %s, restored from its stored form, sums otherwise than it did", tt.name)
			}
		})
	}
}

// A held request's stored form is all it decodes of its line, each by a
// short key, every zero left out, what it spent in its own stored form:
// never its worth, though it's priced, nor its sum.
func TestAHeldRequestsStoredFormIsWhatItDecodes(t *testing.T) {
	l := sumLine()
	r := heldRequestOf(&l).pricedOn(ledger.Pricing, "2026-10-07")
	got, err := json.Marshal(r.stored())
	want := `{"r":"r1","a":{"s":1791403200,"n":123000000},"e":{"s":1791403203,"n":123000000},"d":"/code/app","m":"claude-opus-5-5",` +
		`"ac":"side","rs":"moved: work hit its limit","f":"work","c":"main","ag":"a1","g":"us","p":"p1","st":"tool_use","tl":["Read","Edit"],` +
		`"u":true,"t":[{"a":"work","w":"hit its limit"}],"sp":{"i":10,"1h":1000,"r":100000,"o":500,"w":1000,"b":1000,"d":true,"c":true}}`
	if err != nil || string(got) != want {
		t.Errorf("stored as\n%s, %v, want\n%s", got, err, want)
	}
}
