package prose_test

import (
	"testing"

	"github.com/leeovery/switchboard/internal/prose"
)

func TestList(t *testing.T) {
	tests := []struct {
		items []string
		want  string
	}{
		{items: nil, want: ""},
		{items: []string{"work"}, want: "work"},
		{items: []string{"work", "side"}, want: "work and side"},
		{items: []string{"work", "personal", "side"}, want: "work, personal and side"},
	}
	for _, tt := range tests {
		if got := prose.List(tt.items); got != tt.want {
			t.Errorf("List(%q) = %q, want %q", tt.items, got, tt.want)
		}
	}
}

func TestListOrRunsWhatsChosenBetweenTogether(t *testing.T) {
	tests := []struct {
		items []string
		want  string
	}{
		{items: nil, want: ""},
		{items: []string{"work"}, want: "work"},
		{items: []string{"work", "side"}, want: "work or side"},
		{items: []string{"work", "side", "personal"}, want: "work, side or personal"},
	}
	for _, tt := range tests {
		if got := prose.ListOr(tt.items); got != tt.want {
			t.Errorf("ListOr(%q) = %q, want %q", tt.items, got, tt.want)
		}
	}
}

// What goes before each item of a list is what List and ListOr put there, so
// a list drawn a part at a time reads as one run together.
func TestAJoinerIsWhatGoesBeforeAnItemOfAList(t *testing.T) {
	tests := []struct {
		i, n int
		last string
		want string
	}{
		{i: 0, n: 1, last: prose.And, want: ""},
		{i: 0, n: 3, last: prose.Or, want: ""},
		{i: 1, n: 3, last: prose.Or, want: ", "},
		{i: 2, n: 3, last: prose.Or, want: " or "},
		{i: 1, n: 2, last: prose.And, want: " and "},
		{i: 2, n: 3, last: ", ", want: ", "},
	}
	for _, tt := range tests {
		if got := prose.Joiner(tt.i, tt.n, tt.last); got != tt.want {
			t.Errorf("Joiner(%d, %d, %q) = %q, want %q", tt.i, tt.n, tt.last, got, tt.want)
		}
	}
}

func TestListOfNamesTheThingsANounNamesByTheirKinds(t *testing.T) {
	tests := []struct {
		names []string
		want  string
	}{
		{names: nil, want: "limit"},
		{names: []string{"5-hour"}, want: "5-hour limit"},
		{names: []string{"5-hour", "week"}, want: "5-hour and week limits"},
		{names: []string{"5-hour", "week", "Fable week"}, want: "5-hour, week and Fable week limits"},
	}
	for _, tt := range tests {
		if got := prose.ListOf(tt.names, "limit"); got != tt.want {
			t.Errorf("ListOf(%q, limit) = %q, want %q", tt.names, got, tt.want)
		}
	}
}

// CutShort cuts text whether or not it fits, as where what follows it won't,
// so text that fills its cells exactly is cut too.
func TestCutShortCutsTextWhetherOrNotItFits(t *testing.T) {
	tests := []struct {
		text  string
		cells int
		want  string
	}{
		{text: "switchboard", cells: 11, want: "switchboar…"},
		{text: "switchboard", cells: 20, want: "switchboard…"},
		{text: "two words", cells: 5, want: "two…"},
		{text: "anything", cells: 1, want: "…"},
		{text: "anything", cells: 0, want: ""},
	}
	for _, tt := range tests {
		if got := prose.CutShort(tt.text, tt.cells); got != tt.want {
			t.Errorf("CutShort(%q, %d) = %q, want %q", tt.text, tt.cells, got, tt.want)
		}
	}
}

func TestCutEndsTextCutToItsCellsWithAnEllipsis(t *testing.T) {
	tests := []struct {
		text  string
		cells int
		want  string
	}{
		{text: "switchboard", cells: 12, want: "switchboard"},
		{text: "switchboard-dashboard", cells: 12, want: "switchboard…"},
		{text: "two words", cells: 5, want: "two…"},
		{text: "コードの部屋", cells: 7, want: "コード…"},
		{text: "anything", cells: 1, want: "…"},
		{text: "anything", cells: 0, want: ""},
		{text: "", cells: 0, want: ""},
	}
	for _, tt := range tests {
		if got := prose.Cut(tt.text, tt.cells); got != tt.want {
			t.Errorf("Cut(%q, %d) = %q, want %q", tt.text, tt.cells, got, tt.want)
		}
	}
}

func TestCutFrontKeepsTheEndOfTextCutToItsCells(t *testing.T) {
	tests := []struct {
		text  string
		cells int
		want  string
	}{
		{text: "~/Code/switchboard", cells: 18, want: "~/Code/switchboard"},
		{text: "~/Code/clients/switchboard", cells: 13, want: "…/switchboard"},
		{text: "~/Code/clients/switchboard", cells: 16, want: "…nts/switchboard"},
		{text: "~/コード/部屋", cells: 6, want: "…/部屋"},
		{text: "anything", cells: 1, want: "…"},
		{text: "anything", cells: 0, want: ""},
	}
	for _, tt := range tests {
		if got := prose.CutFront(tt.text, tt.cells); got != tt.want {
			t.Errorf("CutFront(%q, %d) = %q, want %q", tt.text, tt.cells, got, tt.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		s    string
		n    int
		want string
	}{
		{s: "0b5c6f2e-7d41-4a3b", n: 8, want: "0b5c6f2e"},
		{s: "0b5c6f2e", n: 8, want: "0b5c6f2e"},
		{s: "short", n: 8, want: "short"},
		{s: "", n: 8, want: ""},
		{s: "·······", n: 3, want: "···"},
		{s: "anything", n: 0, want: ""},
	}
	for _, tt := range tests {
		if got := prose.Truncate(tt.s, tt.n); got != tt.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
		}
	}
}

func TestTruncateBytes(t *testing.T) {
	tests := []struct {
		s    string
		n    int
		want string
	}{
		{s: "0b5c6f2e-7d41-4a3b", n: 8, want: "0b5c6f2e"},
		{s: "0b5c6f2e", n: 8, want: "0b5c6f2e"},
		{s: "short", n: 8, want: "short"},
		{s: "", n: 8, want: ""},
		{s: "☃☃☃", n: 6, want: "☃☃"},
		{s: "☃☃☃", n: 5, want: "☃"},
		{s: "☃☃☃", n: 2, want: ""},
		{s: "anything", n: 0, want: ""},
	}
	for _, tt := range tests {
		if got := prose.TruncateBytes(tt.s, tt.n); got != tt.want {
			t.Errorf("TruncateBytes(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
		}
	}
}

func TestTruncateBytesFront(t *testing.T) {
	tests := []struct {
		s    string
		n    int
		want string
	}{
		{s: "~/Code/clients/project", n: 11, want: "…/project"},
		{s: "~/Code/clients/project", n: 12, want: "…s/project"},
		{s: "~/Code/project", n: 14, want: "~/Code/project"},
		{s: "short", n: 8, want: "short"},
		{s: "", n: 8, want: ""},
		{s: "☃☃☃", n: 6, want: "…☃"},
		{s: "☃☃☃", n: 8, want: "…☃"},
		{s: "☃☃☃", n: 5, want: "…"},
		{s: "☃☃☃", n: 3, want: "…"},
		{s: "☃☃☃", n: 2, want: ""},
		{s: "anything", n: 0, want: ""},
	}
	for _, tt := range tests {
		if got := prose.TruncateBytesFront(tt.s, tt.n); got != tt.want {
			t.Errorf("TruncateBytesFront(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
		}
	}
}

func TestClean(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{text: "Work", want: "Work"},
		{text: "  Work \t team\n", want: "Work team"},
		{text: "Work\x1b[31m team\a", want: "Work [31m team"},
		{text: "\x1b]0;a title\a", want: "]0;a title"},
	}
	for _, tt := range tests {
		if got := prose.Clean(tt.text); got != tt.want {
			t.Errorf("Clean(%q) = %q, want %q", tt.text, got, tt.want)
		}
	}
}

// england is England's flag: a black flag, then the tags that name the
// subdivision, then the tag that ends them.
const england = "\U0001f3f4\U000e0067\U000e0062\U000e0065\U000e006e\U000e0067\U000e007f"

func TestQuoted(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "a label", text: "Day job", want: `"Day job"`},
		{name: "words that read as more of the line", text: "a, plan b (was c)", want: `"a, plan b (was c)"`},
		{name: "quotes and backslashes escaped", text: `the "side" one\`, want: `"the \"side\" one\\"`},
		{name: "cleaned", text: "Work\x1b[31m team", want: `"Work [31m team"`},
		{name: "text beyond ASCII as it is", text: "Café", want: `"Café"`},
		{name: "an emoji sequence's joiner as it is", text: "Team \U0001f469\u200d\U0001f4bb", want: "\"Team \U0001f469\u200d\U0001f4bb\""},
		{name: "a private-use glyph as it is", text: "\ue0a0 main", want: "\"\ue0a0 main\""},
		{name: "a subdivision's flag as it is", text: england, want: `"` + england + `"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prose.Quoted(tt.text); got != tt.want {
				t.Errorf("Quoted(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}
