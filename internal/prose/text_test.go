package prose_test

import (
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/prose"
)

func TestVersion(t *testing.T) {
	tests := []struct{ version, want string }{
		{"0.2.10", "v0.2.10"},
		{"1.0.0-rc.1", "v1.0.0-rc.1"},
		{"dev", "dev"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := prose.Version(tt.version); got != tt.want {
			t.Errorf("Version(%q) = %q, want %q", tt.version, got, tt.want)
		}
	}
}

func TestAccountNamed(t *testing.T) {
	tests := []struct{ name, label, id, want string }{
		{name: "by its label, its id beside", label: "Acme 1", id: "k3f9", want: "Acme 1 (k3f9)"},
		{name: "labelled by its id", label: "work", id: "work", want: "work"},
		{name: "labelled by its id in another case", label: "Work", id: "work", want: "work"},
		{name: "not labelled", id: "k3f9", want: "k3f9"},
		{name: "each cleaned", label: "Acme\t1\n", id: "k3f9\x1b", want: "Acme 1 (k3f9)"},
		{name: "a label of a direction mark alone by its id", label: "\u200e", id: "k3f9", want: "k3f9"},
		{name: "a label of characters drawn as nothing by its id", label: "\u200b\u2066", id: "k3f9", want: "k3f9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prose.AccountNamed(tt.label, tt.id); got != tt.want {
				t.Errorf("AccountNamed(%q, %q) = %q, want %q", tt.label, tt.id, got, tt.want)
			}
		})
	}
}

func TestAForeignTokenIsSaidWithWhatPutsItRight(t *testing.T) {
	tests := []struct{ id, want string }{
		{"work", "its token is another subscription's: give it its own with switchboard accounts token work, or add that subscription as an account of its own"},
		{"wo\x1brk", "its token is another subscription's: give it its own with switchboard accounts token wo rk, or add that subscription as an account of its own"},
	}
	for _, tt := range tests {
		if got := prose.ForeignTokenFix(tt.id); got != tt.want {
			t.Errorf("ForeignTokenFix(%q) = %q, want %q", tt.id, got, tt.want)
		}
	}
}

func TestAnAccountWithoutATokenOfItsOwnIsSaidShortByWhy(t *testing.T) {
	for foreign, want := range map[bool]string{false: "no token", true: "wrong token"} {
		if got := prose.Tokenless(foreign); got != want {
			t.Errorf("Tokenless(%v) = %q, want %q", foreign, got, want)
		}
	}
	if got, want := "work"+prose.ForeignTokenAfterName, "work's token is another subscription's"; got != want {
		t.Errorf("a foreign token said after its account's name reads %q, want %q", got, want)
	}
}

// A reading the API refuses says why in full and short: its subscription
// lapsed, or its token refused.
func TestARefusedReadingIsSaidByWhyInFullAndShort(t *testing.T) {
	tests := []struct{ full, short, wantFull, wantShort string }{
		{prose.SubscriptionLapsed, prose.SubscriptionLapsedShort, "subscription lapsed", "lapsed"},
		{prose.TokenRefused, prose.TokenRefusedShort, "token refused", "refused"},
	}
	for _, tt := range tests {
		if tt.full != tt.wantFull || tt.short != tt.wantShort {
			t.Errorf("said %q, short %q; want %q, short %q", tt.full, tt.short, tt.wantFull, tt.wantShort)
		}
	}
}

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
		name string
		text string
		want string
	}{
		{name: "plain text as it is", text: "Work", want: "Work"},
		{name: "spaces run together and trimmed", text: "  Work \t team\n", want: "Work team"},
		{name: "two spaces together run into one", text: "Work  team", want: "Work team"},
		{name: "a space at the end trimmed", text: "Work ", want: "Work"},
		{name: "a delete as a space", text: "Work\x7fteam", want: "Work team"},
		{name: "nothing as nothing", text: "", want: ""},
		{name: "a space alone as nothing", text: " ", want: ""},
		{name: "control characters as spaces", text: "Work\x1b[31m team\a", want: "Work [31m team"},
		{name: "a title's escape as spaces", text: "\x1b]0;a title\a", want: "]0;a title"},
		{name: "text beyond ASCII as it is", text: "Café résumé", want: "Café résumé"},
		{name: "a right-to-left override dropped", text: "\u202ekrow and the rest", want: "krow and the rest"},
		{name: "an embedding and its pop dropped", text: "\u202aWork\u202c team", want: "Work team"},
		{name: "isolates dropped", text: "a\u2067b\u2069 c\u2066d\u2068e", want: "ab cde"},
		{name: "a zero-width space dropped", text: "Day\u200b job", want: "Day job"},
		{name: "a character drawn as nothing alone between words dropped", text: "Day \u200b job", want: "Day job"},
		{name: "a joiner standing alone dropped", text: "Day \u200d job", want: "Day job"},
		{name: "a soft hyphen dropped", text: "co\u00adop", want: "coop"},
		{name: "Hangul fillers dropped", text: "W\u3164o\u115fr\u1160k", want: "Work"},
		{name: "a lone tag dropped", text: "work\U000e0067", want: "work"},
		{name: "a fake flag's tags dropped", text: "Side \U0001f3f4\U000e0068\U000e0069\U000e0064\U000e0065\U000e007f", want: "Side \U0001f3f4"},
		{name: "a variation selector after a Latin letter dropped", text: "Wor\ufe00k", want: "Work"},
		{name: "an emoji sequence's joiners kept", text: "Team \U0001f468\u200d\U0001f469\u200d\U0001f467", want: "Team \U0001f468\u200d\U0001f469\u200d\U0001f467"},
		{name: "a variation selector after a symbol kept", text: "Home \u2764\ufe0f", want: "Home \u2764\ufe0f"},
		{name: "a keycap kept", text: "1\ufe0f\u20e3 first", want: "1\ufe0f\u20e3 first"},
		{name: "a subdivision's flag kept", text: "Work " + england, want: "Work " + england},
		{name: "a joining script's joiner kept", text: "\u0915\u094d\u200d\u0937", want: "\u0915\u094d\u200d\u0937"},
		{name: "a direction mark dropped from text written left to right", text: "a\u200fb 12", want: "ab 12"},
		{name: "an Arabic letter mark dropped from text written left to right", text: "a\u061cb", want: "ab"},
		{name: "a direction mark kept within text holding Hebrew", text: "\u05d0\u200f 12 b", want: "\u05d0\u200f 12 b"},
		{name: "a direction mark kept within text holding Arabic", text: "\u0645\u061c 12", want: "\u0645\u061c 12"},
		{name: "direction marks at the edges trimmed, though Hebrew's within", text: "\u200f \u05d0\u05d1 \u200e", want: "\u05d0\u05d1"},
		{name: "direction marks at the edges trimmed", text: "\u200eWork\u200f", want: "Work"},
		{name: "a direction mark alone as nothing", text: "\u200e", want: ""},
		{name: "an Arabic number sign dropped, as it hides what's after it", text: "\u0600123", want: "123"},
		{name: "the end of an ayah dropped", text: "\u06dd1 Work", want: "1 Work"},
		{name: "a Kaithi number sign dropped", text: "Work\U000110bd1", want: "Work1"},
		{name: "a private-use glyph kept", text: "\ue0a0 main", want: "\ue0a0 main"},
		{name: "a joiner judged by what's kept, the space dropped before it passed over", text: "\U0001f600\u200b\u200d\U0001f600", want: "\U0001f600\u200d\U0001f600"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prose.Clean(tt.text); got != tt.want {
				t.Errorf("Clean(%+q) = %+q, want %+q", tt.text, got, tt.want)
			}
		})
	}
}

// Clean leaves nothing it cleans away, and nothing it would clean again, so
// text shown twice over, as a label cleaned then named, shows the same.
func TestCleanLeavesNothingToClean(t *testing.T) {
	for _, text := range []string{
		"  Work \t team\n",
		"\u202ekrow \u2066a\u2069",
		"Day\u200b job\u200f",
		"\U0001f600\u200b\u200d\U0001f600",
		"Side \U0001f3f4\U000e0068\U000e0069\U000e007f",
		"Day \u200b\u200d job",
		"\u200f\u05d0 12\u061c",
		"\u0600123 \u06dd",
	} {
		once := prose.Clean(text)
		switch {
		case strings.ContainsFunc(once, prose.Reorders):
			t.Errorf("Clean(%+q) = %+q, holding a bidirectional control", text, once)
		case strings.ContainsFunc(once, prose.Prepends):
			t.Errorf("Clean(%+q) = %+q, holding a prepended concatenation mark", text, once)
		case prose.MapUnseen(once, dropMarksOnly) != once:
			t.Errorf("Clean(%+q) = %+q, holding a character drawn as nothing that does nothing that shows", text, once)
		case strings.TrimFunc(once, prose.DirectionMark) != once:
			t.Errorf("Clean(%+q) = %+q, a direction mark at its edge", text, once)
		}
		if twice := prose.Clean(once); twice != once {
			t.Errorf("Clean(Clean(%+q)) = %+q, want %+q, as Clean left it", text, twice, once)
		}
	}
}

// dropMarksOnly is what a test has MapUnseen make of an unseen character that
// does nothing that shows: a direction mark kept, for Clean keeps those
// within text written right to left, and anything else dropped.
func dropMarksOnly(r rune) string {
	if prose.DirectionMark(r) {
		return string(r)
	}
	return ""
}

func TestVisible(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{text: "fi  # switchboard", want: "fi  # switchboard"},
		{text: "a\tb\x1b[31m c\n", want: "a b [31m c "},
	}
	for _, tt := range tests {
		if got := prose.Visible(tt.text); got != tt.want {
			t.Errorf("Visible(%q) = %q, want %q", tt.text, got, tt.want)
		}
	}
}

// england is England's flag: a black flag, then the tags that name the
// subdivision, then the tag that ends them.
const england = "\U0001f3f4\U000e0067\U000e0062\U000e0065\U000e006e\U000e0067\U000e007f"

// Scotland's and Wales's flags, as england is England's.
const (
	scotland = "\U0001f3f4\U000e0067\U000e0062\U000e0073\U000e0063\U000e0074\U000e007f"
	wales    = "\U0001f3f4\U000e0067\U000e0062\U000e0077\U000e006c\U000e0073\U000e007f"
)

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
		{name: "Wales's flag as it is", text: wales, want: `"` + wales + `"`},
		{name: "a fake flag's tags escaped", text: "Side \U0001f3f4\U000e0068\U000e0069\U000e0064\U000e0065\U000e007f", want: "\"Side \U0001f3f4\\U000E0068\\U000E0069\\U000E0064\\U000E0065\\U000E007F\""},
		{name: "a variation selector as it is", text: "Home \u2764\uFE0F", want: "\"Home \u2764\uFE0F\""},
		{name: "a zero-width space escaped", text: "Day job\u200B", want: `"Day job\u200B"`},
		{name: "a joiner standing alone escaped", text: "Day \u200D job", want: `"Day \u200D job"`},
		{name: "a lone tag escaped", text: "work\U000e0067", want: `"work\U000E0067"`},
		{name: "an invisible separator escaped", text: "side\u2063", want: `"side\u2063"`},
		{name: "a right-to-left override escaped", text: "\u202Ekrow", want: `"\u202Ekrow"`},
		{name: "an isolate escaped", text: "a\u2067b\u2069", want: `"a\u2067b\u2069"`},
		{name: "a direction mark escaped", text: "a\u200Fb", want: `"a\u200Fb"`},
		{name: "an Arabic number sign as it is", text: "\u0600123", want: "\"\u0600123\""},
		{name: "a selector after an escape escaped", text: "\u263a\u200b\ufe0f", want: "\"\u263a\\u200B\\uFE0F\""},
		{name: "a joiner after an escape escaped", text: "\u0915\u200b\u200d\u0937", want: "\"\u0915\\u200B\\u200D\u0937\""},
		{name: "a backslash before an escape kept apart", text: "a\\\u200B", want: `"a\\\u200B"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prose.Quoted(tt.text); got != tt.want {
				t.Errorf("Quoted(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}
