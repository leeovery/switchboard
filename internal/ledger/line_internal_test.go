package ledger

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestALineIsCutToWhatTheLedgerHolds(t *testing.T) {
	long := strings.Repeat("☃", textMost)
	longs := slices.Repeat([]string{long}, 2*listMost)
	got := (&Line{Session: long, Betas: longs, Tried: slices.Repeat([]Tried{{Account: long, Why: long}}, 2*listMost),
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
		if !slices.Equal(list, slices.Repeat([]string{cut}, listMost)) {
			t.Errorf("%d %s are written as %d of %d bytes, want %d, each cut", 2*listMost, name, len(list), len(list[0]), listMost)
		}
	}
	if len(got.Tried) != listMost || got.Tried[0] != (Tried{Account: cut, Why: cut}) {
		t.Errorf("%d accounts tried are written as %d, the first's %d and %d bytes, want %d, each cut", 2*listMost, len(got.Tried), len(got.Tried[0].Account), len(got.Tried[0].Why), listMost)
	}
}

func TestAnAnswerIsCutToWhatTheLedgerHolds(t *testing.T) {
	blocks, limits := make(map[string]int), make(map[string]string)
	for i := range 2 * listMost {
		blocks[fmt.Sprintf("kind-%02d", i)] = i
	}
	for i := range 2 * limitsMost {
		limits[fmt.Sprintf("window-%03d", i)] = "0.5"
	}
	tools := make([]string, 2*listMost)
	for i := range tools {
		tools[i] = fmt.Sprintf("Tool%02d", i)
	}
	usage := json.RawMessage(`{"input_tokens":"` + strings.Repeat("9", usageMost) + `"}`)

	got := (&Line{Answer: Answer{Blocks: blocks, Tools: tools}, Usage: usage, Limits: limits}).written()
	if len(got.Answer.Blocks) != listMost || got.Answer.Blocks["kind-00"] != 0 || got.Answer.Blocks[fmt.Sprintf("kind-%02d", listMost-1)] != listMost-1 {
		t.Errorf("the blocks of %d kinds are written as %v, want the first %d kinds, by their names", len(blocks), got.Answer.Blocks, listMost)
	}
	if !slices.Equal(got.Answer.Tools, tools[:listMost]) {
		t.Errorf("%d tools called are written as %q, want the first %d", len(tools), got.Answer.Tools, listMost)
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
	texts := slices.Repeat([]string{text}, 2*listMost)
	blocks, limits := make(map[string]int), make(map[string]string)
	for i := range 2 * listMost {
		blocks[fmt.Sprintf("%02d", i)+text] = i
	}
	for i := range 2 * limitsMost {
		limits[fmt.Sprintf("%03d", i)+text] = text
	}
	usage := `{"x":"` + strings.Repeat("<", usageMost-len(`{"x":""}`)) + `"}`
	return &Line{
		At: time.Now(), Request: text, Kind: text, Session: text, Model: text, Account: text, Reason: text, From: text,
		Tried: slices.Repeat([]Tried{{Account: text, Why: text}}, 2*listMost), Agent: text, Betas: texts,
		Shape: Shape{Thinking: Thinking{Type: text, Display: text}, ToolChoice: ToolChoice{Type: text}, ServiceTier: text,
			OutputConfig: OutputConfig{Effort: text}, Speed: text, InferenceGeo: text, ContextManagement: ContextManagement{Edits: texts}},
		Answer: Answer{ID: text, Model: text, Stop: text, Blocks: blocks, Tools: texts, Error: Error{Type: text, Message: text}},
		Usage:  json.RawMessage(usage),
		Limits: limits,
	}
}
