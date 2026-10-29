package textfield_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard/textfield"
)

// key is the key a terminal sends for name: a key by its name, as left, or
// one that types name.
func key(name string) tea.KeyPressMsg {
	switch name {
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "delete":
		return tea.KeyPressMsg{Code: tea.KeyDelete}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Code: []rune(name)[0], Text: name}
}

// pressed is the field once each key is pressed in it, failing the test at
// a key it doesn't take.
func pressed(t *testing.T, field textfield.Text, keys ...string) textfield.Text {
	t.Helper()
	for _, k := range keys {
		next, ok := field.Pressed(key(k))
		if !ok {
			t.Fatalf("the field doesn't take %s", k)
		}
		field = next
	}
	return field
}

// graphemes are characters a terminal draws as one, each of more than one
// rune or two cells wide.
var graphemes = []struct{ name, text string }{
	{name: "a CJK character, two cells", text: "漢"},
	{name: "an emoji, two cells", text: "😀"},
	{name: "an emoji and its presentation", text: "❤\uFE0F"},
	{name: "a flag, two regional indicators", text: "🇬🇧"},
	{name: "a family, joined by zero-width joiners", text: "👨\u200d👩\u200d👧"},
	{name: "a letter and a combining accent", text: "e\u0301"},
}

// edit is a field edited from from by keys, holding value, its cursor at
// cursor.
type edit struct {
	name   string
	from   textfield.Text
	keys   []string
	value  string
	cursor int
}

func TestAFieldOfAValueHasItsCursorAfterIt(t *testing.T) {
	field := textfield.Of("08:00-23:00")
	if field.Value() != "08:00-23:00" || field.Cursor() != 11 {
		t.Errorf("the field holds %q, its cursor at %d; want 08:00-23:00, at 11", field.Value(), field.Cursor())
	}
	if empty := (textfield.Text{}); empty.Value() != "" || empty.Cursor() != 0 {
		t.Errorf("the zero field holds %q, its cursor at %d; want nothing, at 0", empty.Value(), empty.Cursor())
	}
}

func TestTheKeysEditTheField(t *testing.T) {
	tests := []edit{
		{name: "typing puts each character in at the cursor", keys: []string{"0", "8", ":", "0", "0"}, value: "08:00", cursor: 5},
		{name: "j, k and q type, as any letter does", keys: []string{"j", "k", "q"}, value: "jkq", cursor: 3},
		{name: "space types a space", from: textfield.Of("a"), keys: []string{"space", "b"}, value: "a b", cursor: 3},
		{name: "← moves the cursor back a character, typing there", from: textfield.Of("0800"), keys: []string{"left", "left", ":"}, value: "08:00", cursor: 3},
		{name: "← stops at the start", from: textfield.Of("ab"), keys: []string{"left", "left", "left"}, value: "ab", cursor: 0},
		{name: "→ moves the cursor on a character, stopping at the end", from: textfield.Of("ab"), keys: []string{"home", "right", "right", "right"}, value: "ab", cursor: 2},
		{name: "home goes to the start, end to the end", from: textfield.Of("08:00"), keys: []string{"home", "1", "end", "2"}, value: "108:002", cursor: 7},
		{name: "backspace takes out the character before the cursor", from: textfield.Of("08:000"), keys: []string{"backspace"}, value: "08:00", cursor: 5},
		{name: "backspace mid-way closes up behind the cursor", from: textfield.Of("08::00"), keys: []string{"left", "left", "backspace"}, value: "08:00", cursor: 3},
		{name: "backspace at the start does nothing", from: textfield.Of("ab"), keys: []string{"home", "backspace"}, value: "ab", cursor: 0},
		{name: "delete takes out the character after the cursor", from: textfield.Of("x08:00"), keys: []string{"home", "delete"}, value: "08:00", cursor: 0},
		{name: "delete at the end does nothing", from: textfield.Of("ab"), keys: []string{"delete"}, value: "ab", cursor: 2},
		{name: "a field cleared is empty", from: textfield.Of("08:00"), keys: []string{"backspace", "backspace", "backspace", "backspace", "backspace"}, value: "", cursor: 0},
		{name: "an accent typed after a letter joins it", from: textfield.Of("ab"), keys: []string{"left", "\u0301"}, value: "a\u0301b", cursor: 1},
	}
	for _, g := range graphemes {
		in := "a" + g.text + "b"
		tests = append(tests,
			edit{name: g.name + ": counted as one", from: textfield.Of(in), value: in, cursor: 3},
			edit{name: g.name + ": ← and → step over it whole", from: textfield.Of(in), keys: []string{"left", "left", "right", "left"}, value: in, cursor: 1},
			edit{name: g.name + ": backspace takes it out whole", from: textfield.Of(in), keys: []string{"left", "backspace"}, value: "ab", cursor: 1},
			edit{name: g.name + ": delete takes it out whole", from: textfield.Of(in), keys: []string{"home", "right", "delete"}, value: "ab", cursor: 1},
			edit{name: g.name + ": typed at the start", from: textfield.Of("ab"), keys: []string{"home", g.text}, value: g.text + "ab", cursor: 1},
			edit{name: g.name + ": typed mid-way", from: textfield.Of("ab"), keys: []string{"left", g.text}, value: in, cursor: 2},
			edit{name: g.name + ": typed at the end", from: textfield.Of("ab"), keys: []string{g.text}, value: "ab" + g.text, cursor: 3},
		)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pressed(t, tt.from, tt.keys...)
			if got.Value() != tt.value || got.Cursor() != tt.cursor {
				t.Errorf("the field holds %q, its cursor at %d; want %q, at %d", got.Value(), got.Cursor(), tt.value, tt.cursor)
			}
		})
	}
}

func TestAnEditLeavesTheFieldItWasMadeFromAsItWas(t *testing.T) {
	from := textfield.Of("08:00")
	pressed(t, from, "left", "backspace", "1")
	if from.Value() != "08:00" || from.Cursor() != 5 {
		t.Errorf("the field edited from holds %q, its cursor at %d; want 08:00, at 5, as it was", from.Value(), from.Cursor())
	}
}

func TestAFieldLeavesTheKeysThatDontEditIt(t *testing.T) {
	keys := map[string]tea.KeyPressMsg{
		"enter":  key("enter"),
		"esc":    key("esc"),
		"tab":    key("tab"),
		"ctrl+c": key("ctrl+c"),
		// As a terminal with Kitty's keyboard protocol sends them, with the
		// text the key would type.
		"ctrl+a, carrying its text": {Code: 'a', Mod: tea.ModCtrl, Text: "a"},
		"alt+b, carrying its text":  {Code: 'b', Mod: tea.ModAlt, Text: "b"},
	}
	for name, k := range keys {
		from := textfield.Of("08:00")
		if got, ok := from.Pressed(k); ok || got != from {
			t.Errorf("%s: the field takes it, holding %q, its cursor at %d; want it left, the field as it was", name, got.Value(), got.Cursor())
		}
	}
}

func TestAPastePutsItsTextInAtTheCursor(t *testing.T) {
	type paste struct {
		name   string
		from   textfield.Text
		paste  string
		value  string
		cursor int
	}
	tests := []paste{
		{name: "into an empty field", paste: "08:00-23:00", value: "08:00-23:00", cursor: 11},
		{name: "at the cursor, mid-way", from: pressed(t, textfield.Of("0823:00"), "home", "right", "right"), paste: ":00-", value: "08:00-23:00", cursor: 6},
		{name: "a line break and a tab left out, as a field is a line", paste: "08:00-\t23:00\r\n", value: "08:00-23:00", cursor: 11},
		{name: "an escape left out, so nothing in it reaches the terminal", paste: "\x1b[2J\x1b]0;title\x0708:00", value: "[2J]0;title08:00", cursor: 16},
		{name: "the controls turning the text's direction left out", paste: "a\u202eb\u2066c\u200fd", value: "abcd", cursor: 4},
		{name: "a family's joiners kept, one character", from: textfield.Of("ab"), paste: "👨\u200d👩\u200d👧", value: "ab👨\u200d👩\u200d👧", cursor: 3},
		{name: "a subdivision's flag, its tags kept, one character", paste: "🏴\U000E0067\U000E0062\U000E0065\U000E006E\U000E0067\U000E007F", value: "🏴\U000E0067\U000E0062\U000E0065\U000E006E\U000E0067\U000E007F", cursor: 1},
	}
	for _, g := range graphemes {
		tests = append(tests, paste{name: g.name + ", mid-way", from: pressed(t, textfield.Of("ab"), "left"), paste: g.text + g.text, value: "a" + g.text + g.text + "b", cursor: 3})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.from.Pasted(tt.paste)
			if got.Value() != tt.value || got.Cursor() != tt.cursor {
				t.Errorf("the field holds %q, its cursor at %d; want %q, at %d", got.Value(), got.Cursor(), tt.value, tt.cursor)
			}
			if strings.ContainsRune(got.Value(), '\x1b') {
				t.Errorf("the field holds %q, an escape among it", got.Value())
			}
		})
	}
}

// What the field leaves out, typed or pasted: what would disturb the
// terminal or the line, every control, C0 and C1, the line and paragraph
// separators, and every control that turns the text's direction; and what
// it keeps wherever it comes: joiners, tags, variation selectors, spaces of
// every kind and private-use characters.
func TestTheFieldLeavesOutOnlyWhatDisturbsTheTerminalOrTheLine(t *testing.T) {
	tests := []struct {
		name, put string
		kept      bool
	}{
		{name: "an escape", put: "\x1b"},
		{name: "a bell", put: "\x07"},
		{name: "delete", put: "\x7f"},
		{name: "a C1 control, next line", put: "\U00000085"},
		{name: "a C1 control, the control sequence introducer", put: "\U0000009B"},
		{name: "a line separator", put: "\U00002028"},
		{name: "a paragraph separator", put: "\U00002029"},
		{name: "a left-to-right mark", put: "\U0000200E"},
		{name: "a right-to-left mark", put: "\U0000200F"},
		{name: "a left-to-right embedding", put: "\U0000202A"},
		{name: "a right-to-left embedding", put: "\U0000202B"},
		{name: "a pop of the direction", put: "\U0000202C"},
		{name: "a left-to-right override", put: "\U0000202D"},
		{name: "a right-to-left override", put: "\U0000202E"},
		{name: "a left-to-right isolate", put: "\U00002066"},
		{name: "a right-to-left isolate", put: "\U00002067"},
		{name: "a first-strong isolate", put: "\U00002068"},
		{name: "a pop of the isolate", put: "\U00002069"},
		{name: "an Arabic letter mark", put: "\U0000061C"},
		{name: "a zero-width joiner, alone", put: "\U0000200D", kept: true},
		{name: "a zero-width non-joiner, alone", put: "\U0000200C", kept: true},
		{name: "a tag, outside any flag", put: "\U000E0067", kept: true},
		{name: "a variation selector", put: "\U0000FE0F", kept: true},
		{name: "a supplementary variation selector", put: "\U000E0100", kept: true},
		{name: "the ideographic space an input method sends", put: "\U00003000", kept: true},
		{name: "a no-break space", put: "\U000000A0", kept: true},
		{name: "a private-use glyph, as a Nerd Font's", put: "\U0000F07B", kept: true},
	}
	for _, tt := range tests {
		want := "ab"
		if tt.kept {
			want = "a" + tt.put + "b"
		}
		t.Run(tt.name+", pasted", func(t *testing.T) {
			if got := textfield.Of("a").Pasted(tt.put + "b"); got.Value() != want {
				t.Errorf("the field holds %q, want %q", got.Value(), want)
			}
		})
		t.Run(tt.name+", typed", func(t *testing.T) {
			got, _ := textfield.Of("a").Pressed(tea.KeyPressMsg{Code: []rune(tt.put)[0], Text: tt.put})
			if got = pressed(t, got, "b"); got.Value() != want {
				t.Errorf("the field holds %q, want %q", got.Value(), want)
			}
		})
	}
}

// Joiners typed a key at a time, in the order they're written, or coming as
// the keys of a paste a terminal doesn't bracket: each kept where it comes,
// the cursor after it.
func TestJoinersTypedInWritingOrderAreKept(t *testing.T) {
	tests := []struct {
		name  string
		keys  []string
		value string
	}{
		{name: "a Persian half-space between two letters", keys: []string{"\U00000645", "\U0000200C", "\U00000647"}, value: "\U00000645\U0000200C\U00000647"},
		{name: "an Indic joiner after a virama, at a key's end, then the next letter", keys: []string{"\U00000915\U0000094D", "\U0000200D", "\U00000937"}, value: "\U00000915\U0000094D\U0000200D\U00000937"},
		{name: "an emoji's joiner typed alone between two emoji", keys: []string{"👨", "\U0000200D", "👩"}, value: "👨\U0000200D👩"},
		{name: "a paste of a, a joiner and b, as three keys", keys: []string{"a", "\U0000200D", "b"}, value: "a\U0000200Db"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pressed(t, textfield.Text{}, tt.keys...)
			if end := textfield.Of(tt.value).Cursor(); got.Value() != tt.value || got.Cursor() != end {
				t.Errorf("the field holds %q, its cursor at %d; want %q, at its end, %d", got.Value(), got.Cursor(), tt.value, end)
			}
		})
	}
}

// A paste a terminal doesn't bracket comes as keys, at times many characters
// to one.
func TestAPasteThatComesAsAKeyTypesItAll(t *testing.T) {
	got, ok := textfield.Text{}.Pressed(tea.KeyPressMsg{Code: '0', Text: "08:00-23:00"})
	if !ok || got.Value() != "08:00-23:00" || got.Cursor() != 11 {
		t.Errorf("the field holds %q, its cursor at %d, taken %v; want 08:00-23:00, at 11, taken", got.Value(), got.Cursor(), ok)
	}
}
