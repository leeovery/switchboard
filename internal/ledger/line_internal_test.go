package ledger

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestALineIsCutToWhatTheLedgerHolds(t *testing.T) {
	long := strings.Repeat("☃", textMost)
	got := (&Line{Session: long, Betas: slices.Repeat([]string{long}, 2*listMost), Tried: slices.Repeat([]Tried{{Account: long, Why: long}}, 2*listMost),
		Shape: Shape{ServiceTier: long}}).written()
	cut := strings.Repeat("☃", textMost/len("☃"))
	if got.Session != cut || got.Shape.ServiceTier != cut {
		t.Errorf("a session's id and a service tier of %d bytes are written as %d and %d, want %d: cut to %d bytes at the end of a character",
			len(long), len(got.Session), len(got.Shape.ServiceTier), len(cut), textMost)
	}
	if len(got.Betas) != listMost || len(got.Tried) != listMost {
		t.Fatalf("%d betas and %d accounts tried are written as %d and %d, want %d of each", 2*listMost, 2*listMost, len(got.Betas), len(got.Tried), listMost)
	}
	if got.Betas[0] != cut || got.Tried[0] != (Tried{Account: cut, Why: cut}) {
		t.Errorf("a beta is written as %d bytes, an account tried as %d and %d, want %d: each cut", len(got.Betas[0]), len(got.Tried[0].Account), len(got.Tried[0].Why), len(cut))
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

// longest is a line with every text and list longer than the ledger holds,
// each character of every text one JSON escapes in six bytes.
func longest() *Line {
	text := strings.Repeat("\x00", 4*textMost)
	return &Line{
		At: time.Now(), Request: text, Kind: text, Session: text, Model: text, Account: text, Reason: text, From: text,
		Tried: slices.Repeat([]Tried{{Account: text, Why: text}}, 2*listMost), Agent: text, Betas: slices.Repeat([]string{text}, 2*listMost),
		Shape: Shape{Thinking: Thinking{Type: text, Display: text}, ToolChoice: ToolChoice{Type: text}, ServiceTier: text},
	}
}
