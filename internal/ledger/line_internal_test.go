package ledger

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestALineIsCutToWhatTheLedgerHolds(t *testing.T) {
	long := strings.Repeat("☃", textMost)
	longs := slices.Repeat([]string{long}, 2*ListMost)
	dir := "~/" + long + "/project"
	got := (&Line{Session: long, Dir: dir, Betas: longs, Tried: slices.Repeat([]Tried{{Account: long, Why: long}}, 2*ListMost),
		Shape:  Shape{ServiceTier: long, OutputConfig: OutputConfig{Effort: long}, ContextManagement: ContextManagement{Edits: longs}},
		Answer: Answer{Error: Error{Type: "overloaded_error", Message: long}}}).written()
	cut := strings.Repeat("☃", textMost/len("☃"))
	for name, text := range map[string]string{"a session's id": got.Session, "a service tier": got.Shape.ServiceTier,
		"an effort": got.Shape.OutputConfig.Effort, "an error's message": got.Answer.Error.Message} {
		if text != cut {
			t.Errorf("%s of %d bytes is written as %d, want %d: cut to %d bytes at the end of a character", name, len(long), len(text), len(cut), textMost)
		}
	}
	for name, list := range map[string][]string{"betas": got.Betas, "context edits": got.Shape.ContextManagement.Edits} {
		if !slices.Equal(list, slices.Repeat([]string{cut}, ListMost)) {
			t.Errorf("%d %s are written as %d of %d bytes, want %d, each cut", 2*ListMost, name, len(list), len(list[0]), ListMost)
		}
	}
	if len(got.Tried) != ListMost || got.Tried[0] != (Tried{Account: cut, Why: cut}) {
		t.Errorf("%d accounts tried are written as %d, the first's %d and %d bytes, want %d, each cut", 2*ListMost, len(got.Tried), len(got.Tried[0].Account), len(got.Tried[0].Why), ListMost)
	}
	if want := "…" + strings.Repeat("☃", (textMost-len("…/project"))/len("☃")) + "/project"; got.Dir != want {
		t.Errorf("a directory of %d bytes is written as %q, want %q: its end, its own name, in %d bytes, an ellipsis for the rest", len(dir), got.Dir, want, textMost)
	}
}

func TestHintsAreCutAsASessionIsAndTheirToolsTimesAsTheAccountsTried(t *testing.T) {
	// long holds something shaped like a token at its start, which no cut
	// may part from its prefix.
	long := "sk-ant-oat01-fake_hint-token-shaped " + strings.Repeat("☃", textMost)
	cut := "[redacted] " + strings.Repeat("☃", (textMost-len("[redacted] "))/len("☃"))
	times := make([]ToolTime, 2*ListMost)
	for i := range times {
		times[i] = ToolTime{Tool: long, MS: int64(i)}
	}

	got := (&Line{Prompt: long, Class: long, AgentID: long, ParentAgentID: long, AgentType: long,
		Compaction: long, Compacted: long, ToolMS: times}).written().Hints
	for name, text := range map[string]string{"a prompt": got.Prompt, "a class": got.Class, "an agent": got.AgentID,
		"a parent agent": got.ParentAgentID, "an agent's type": got.AgentType, "a compaction": got.Compaction, "a compaction after": got.Compacted} {
		if text != cut {
			t.Errorf("%s of %d bytes is written as %q, want %q: the token hidden, then cut to %d bytes at the end of a character", name, len(long), text, cut, textMost)
		}
	}
	if len(got.ToolMS) != ListMost {
		t.Fatalf("%d tools' times are written as %d, want the first %d", len(times), len(got.ToolMS), ListMost)
	}
	for i, tt := range got.ToolMS {
		if want := (ToolTime{Tool: cut, MS: int64(i)}); tt != want {
			t.Errorf("the tool's time %d is written as %+v, want %+v: its name cut, its time as it was", i, tt, want)
		}
	}
}

func TestAnAnswerIsCutToWhatTheLedgerHolds(t *testing.T) {
	blocks, limits := make(map[string]int), make(map[string]string)
	for i := range 2 * ListMost {
		blocks[fmt.Sprintf("kind-%02d", i)] = i
	}
	for i := range 2 * limitsMost {
		limits[fmt.Sprintf("window-%03d", i)] = "0.5"
	}
	tools := make([]string, 2*ListMost)
	for i := range tools {
		tools[i] = fmt.Sprintf("Tool%02d", i)
	}
	usage := json.RawMessage(`{"input_tokens":"` + strings.Repeat("9", usageMost) + `"}`)

	got := (&Line{Answer: Answer{Blocks: blocks, Tools: tools}, Usage: usage, Limits: limits}).written()
	if len(got.Answer.Blocks) != ListMost || got.Answer.Blocks["kind-00"] != 0 || got.Answer.Blocks[fmt.Sprintf("kind-%02d", ListMost-1)] != ListMost-1 {
		t.Errorf("the blocks of %d kinds are written as %v, want the first %d kinds, by their names", len(blocks), got.Answer.Blocks, ListMost)
	}
	if !slices.Equal(got.Answer.Tools, tools[:ListMost]) {
		t.Errorf("%d tools called are written as %q, want the first %d", len(tools), got.Answer.Tools, ListMost)
	}
	if len(got.Limits) != limitsMost || got.Limits["window-000"] != "0.5" {
		t.Errorf("%d usage headers are written as %d, want the first %d, by their names", len(limits), len(got.Limits), limitsMost)
	}
	if got.Usage != nil {
		t.Errorf("a usage of %d bytes is written as %d, want it left out: past %d bytes, no usage the API gives comes near", len(usage), len(got.Usage), usageMost)
	}
}

func TestTheLongestLineIsReadBack(t *testing.T) {
	data, err := json.Marshal(longest().written())
	if err != nil {
		t.Fatal(err)
	}
	if 2*len(data) > lineMax {
		t.Errorf("the longest line runs to %d bytes, want half the %d the ledger reads back at most, for room to spare", len(data), lineMax)
	}
}

// longest is a line with every text, list and count longer than the ledger
// holds, and its usage as long as it holds, each character of every text one
// JSON escapes in six bytes.
func longest() *Line {
	text := strings.Repeat("\x00", 4*textMost)
	texts := slices.Repeat([]string{text}, 2*ListMost)
	blocks, limits := make(map[string]int), make(map[string]string)
	for i := range 2 * ListMost {
		blocks[fmt.Sprintf("%02d", i)+text] = i
	}
	for i := range 2 * limitsMost {
		limits[fmt.Sprintf("%03d", i)+text] = text
	}
	usage := `{"x":"` + strings.Repeat("<", usageMost-len(`{"x":""}`)) + `"}`
	return &Line{
		At: time.Now(), Request: text, Kind: text, Session: text, Dir: text, Model: text, Account: text, Reason: text, From: text,
		Tried: slices.Repeat([]Tried{{Account: text, Why: text}}, 2*ListMost), Agent: text, Betas: texts,
		Prompt: text, Class: text, AgentID: text, ParentAgentID: text, AgentType: text, Compaction: text, Compacted: text,
		ToolMS: slices.Repeat([]ToolTime{{Tool: text, MS: math.MaxInt64}}, 2*ListMost),
		Shape: Shape{Thinking: Thinking{Type: text, Display: text}, ToolChoice: ToolChoice{Type: text}, ServiceTier: text,
			OutputConfig: OutputConfig{Effort: text}, Speed: text, InferenceGeo: text, ContextManagement: ContextManagement{Edits: texts}},
		Answer: Answer{ID: text, Model: text, Stop: text, Blocks: blocks, Tools: texts, Error: Error{Type: text, Message: text}},
		Usage:  json.RawMessage(usage),
		Limits: limits,
	}
}
