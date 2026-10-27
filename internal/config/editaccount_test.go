package config_test

import (
	"cmp"
	"reflect"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
)

// workThenSide is a config of two accounts, work's table followed by a
// comment of its own, then side's, labelled, and a table after them.
const workThenSide = "[[account]]\nid = \"work\"\n# reserve = 0.2\n\n[[account]]\nid    = \"side\"\nlabel = \"Side\"\n\n[prime]\nday = \"08:00-23:00\"\n"

func TestSetLabel(t *testing.T) {
	tests := []struct {
		name   string
		config string
		id     string
		label  string
		want   string
	}{
		{
			name:   "in place of its label, keeping the comment beside it and the layout",
			config: commented,
			id:     "work",
			label:  "Day job",
			want:   replaced(t, commented, `label   = "Work"  # optional`, `label   = "Day job"  # optional`),
		},
		{
			name:   "below the account's last key, above the comments directly below it",
			config: workThenSide,
			id:     "work",
			label:  "Work",
			want:   replaced(t, workThenSide, "id = \"work\"\n", "id = \"work\"\nlabel = \"Work\"\n"),
		},
		{
			name:   "none, taking its label out",
			config: commented,
			id:     "personal",
			label:  "",
			want:   replaced(t, commented, "label = \"Personal\"\n", ""),
		},
		{
			name:   "quoted as TOML asks",
			config: workThenSide,
			id:     "side",
			label:  `The "side" one`,
			want:   replaced(t, workThenSide, `label = "Side"`, `label = "The \"side\" one"`),
		},
		{
			name:   "the label it has already, left as it is",
			config: commented,
			id:     "work",
			label:  "Work",
			want:   commented,
		},
		{
			name:   "as it shows: trimmed, each run of whitespace within it one space",
			config: commented,
			id:     "work",
			label:  "  Day   job ",
			want:   replaced(t, commented, `label   = "Work"  # optional`, `label   = "Day job"  # optional`),
		},
		{
			name:   "the longest, once its surrounding whitespace is trimmed",
			config: commented,
			id:     "work",
			label:  "  " + strings.Repeat("s", 98) + "   ",
			want:   replaced(t, commented, `label   = "Work"  # optional`, `label   = "`+strings.Repeat("s", 98)+`"  # optional`),
		},
		{
			name:   "the label it has already but for the whitespace within it, left as it is",
			config: "[[account]]\nid    = \"work\"\nlabel = \"Day job\"\n",
			id:     "work",
			label:  "Day  job",
			want:   "[[account]]\nid    = \"work\"\nlabel = \"Day job\"\n",
		},
		{
			name:   "below the last key in a file of mixed endings, ending as the line it follows",
			config: "[[account]]\r\nid = \"work\"\r\n\n[[account]]\nid = \"side\"\n",
			id:     "work",
			label:  "W",
			want:   "[[account]]\r\nid = \"work\"\r\nlabel = \"W\"\r\n\n[[account]]\nid = \"side\"\n",
		},
		{
			name:   "below a last line without an ending in a file of mixed endings, as the nearest line with one ends",
			config: "\n[[account]]\nid = \"side\"\n\n[[account]]\r\nid = \"work\"",
			id:     "work",
			label:  "W",
			want:   "\n[[account]]\nid = \"side\"\n\n[[account]]\r\nid = \"work\"\r\nlabel = \"W\"\r\n",
		},
		{
			name:   "the label it has already but for surrounding whitespace, left as it is",
			config: commented,
			id:     "work",
			label:  "Work ",
			want:   commented,
		},
		{
			name:   "its id, for an account shown by its id already, left as it is",
			config: workThenSide,
			id:     "work",
			label:  "work",
			want:   workThenSide,
		},
		{
			name:   "none, for an account labelled with its id, left as it is",
			config: "[[account]]\nid    = \"work\"\nlabel = \"work\"\n",
			id:     "work",
			label:  "",
			want:   "[[account]]\nid    = \"work\"\nlabel = \"work\"\n",
		},
		{
			name:   "in place of a label in triple quotes on the one line, keeping the comment beside it",
			config: "[[account]]\nid    = \"work\"\nlabel = \"\"\"Work\"\"\"  # keep me\n",
			id:     "work",
			label:  "Day job",
			want:   "[[account]]\nid    = \"work\"\nlabel = \"Day job\"  # keep me\n",
		},
		{
			name:   "in place of a label in triple quotes holding quotes of its own",
			config: "[[account]]\nid    = \"work\"\nlabel = \"\"\"the \"work\" one\"\"\"\"\"  # keep me\n",
			id:     "work",
			label:  "Day job",
			want:   "[[account]]\nid    = \"work\"\nlabel = \"Day job\"  # keep me\n",
		},
		{
			name:   "below the last key, indented as the table's keys are",
			config: "[[account]]\n    id = \"work\"\n    # reserve = 0.1\n",
			id:     "work",
			label:  "Work",
			want:   "[[account]]\n    id = \"work\"\n    label = \"Work\"\n    # reserve = 0.1\n",
		},
		{
			name:   "below the last key in a file whose lines end in CRLF, ending so too",
			config: "[[account]]\r\nid = \"work\"\r\n\r\n[[account]]\r\nid    = \"side\"\r\nlabel = \"Side\"\r\n",
			id:     "work",
			label:  "Work",
			want:   "[[account]]\r\nid = \"work\"\r\nlabel = \"Work\"\r\n\r\n[[account]]\r\nid    = \"side\"\r\nlabel = \"Side\"\r\n",
		},
		{
			name:   "in a file that starts with a byte order mark, which it keeps",
			config: "\uFEFF[[account]]\nid    = \"work\"\nlabel = \"Work\"\n",
			id:     "work",
			label:  "Day job",
			want:   "\uFEFF[[account]]\nid    = \"work\"\nlabel = \"Day job\"\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.config)
			draft := edit(t, path)

			if err := draft.SetLabel(tt.id, tt.label); err != nil {
				t.Fatalf("SetLabel(%q, %q) error = %v", tt.id, tt.label, err)
			}
			got := checkSaved(t, path, draft, tt.id, tt.want)
			if want := cmp.Or(strings.Join(strings.Fields(tt.label), " "), tt.id); got.Label != want {
				t.Errorf("the label loads as %q, want %q", got.Label, want)
			}
		})
	}
}

// england is England's flag: a black flag, then the tags that name the
// subdivision, then the tag that ends them, which trimming would leave a
// plain black flag.
const england = "\U0001f3f4\U000e0067\U000e0062\U000e0065\U000e006e\U000e0067\U000e007f"

func TestSetLabelTakesTheLabelAsItShows(t *testing.T) {
	const side = "[[account]]\nid    = \"side\"\nlabel = \"Side\"\n"
	tests := []struct {
		name  string
		given string
		// want is the label saved, "" for none.
		want string
	}{
		{name: "format characters at its edges trimmed", given: "\u200b\ufeff\u200eDay job\u200f\u2060 ", want: "Day job"},
		{name: "joiners within it kept, as an emoji sequence needs them", given: "Team \U0001f469\u200d\U0001f4bb", want: "Team \U0001f469\u200d\U0001f4bb"},
		{name: "a private-use glyph, such as a Nerd Font icon, which shows", given: "\ue0a0", want: "\ue0a0"},
		{name: "its id, which takes the label out, as it shows the same", given: "side", want: ""},
		{name: "its id once trimmed", given: " side\u200b", want: ""},
		{name: "a subdivision's flag, its tags kept at the edge", given: "Team " + england, want: "Team " + england},
		{name: "an Arabic number sign kept at the edge", given: "\u0600123", want: "\u0600123"},
		{name: "characters that play no part dropped, the whitespace either side then one space", given: "Work \u200b Team", want: "Work Team"},
		{name: "a soft hyphen and a word joiner dropped within it", given: "Day\u00adjob\u2060s", want: "Dayjobs"},
		{name: "direction marks within it kept, trimmed at its edges", given: "\u200f\u061cabc\u200fdef\u200e", want: "abc\u200fdef"},
		{name: "the invisible operators and separator dropped", given: "a\u2061b\u2062c\u2063d\u2064e", want: "abcde"},
		{name: "the Mongolian vowel separator dropped", given: "a\u180eb", want: "ab"},
		{name: "the deprecated format characters dropped", given: "a\u206a\u206b\u206c\u206d\u206e\u206fb", want: "ab"},
		{name: "the interlinear annotation characters dropped", given: "a\ufff9b\ufffac\ufffbd", want: "abcd"},
		{name: "a Hangul filler dropped", given: "a\u3164b", want: "ab"},
		{name: "a lone tag dropped", given: "work\U000e0067", want: "work"},
		{name: "a variation selector kept at the end, as it shapes what it follows", given: "Home \u2764\ufe0f", want: "Home \u2764\ufe0f"},
		{name: "a variation selector at the start dropped, with nothing to shape", given: "\ufe0fHome", want: "Home"},
		{name: "joiners at its edges dropped", given: "\u200dDay job\u200c", want: "Day job"},
		{name: "a joiner standing alone between spaces dropped, one space left", given: "a \u200d b", want: "a b"},
		{name: "its id once an invisible separator is dropped", given: "side\u2063", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, side)
			draft := edit(t, path)

			if err := draft.SetLabel("side", tt.given); err != nil {
				t.Fatalf("SetLabel(%q) error = %v", tt.given, err)
			}
			want := "[[account]]\nid    = \"side\"\n"
			if tt.want != "" {
				want += "label = \"" + tt.want + "\"\n"
			}
			got := checkSaved(t, path, draft, "side", want)
			if shown := cmp.Or(tt.want, "side"); got.Label != shown || draft.Labelled("side") != (tt.want != "") {
				t.Errorf("the label loads as %q, labelled: %v; want %q, labelled: %v", got.Label, draft.Labelled("side"), shown, tt.want != "")
			}
		})
	}
}

// overLines is a config whose account's label goes on over lines.
const overLines = "[[account]]\nid    = \"work\"\nlabel = \"\"\"\nWork\"\"\"\n"

func TestAnAccountWithALabelOverLinesTakesAPlanBelowIt(t *testing.T) {
	path := writeConfig(t, overLines)
	draft := edit(t, path)

	if err := draft.SetPlan("work", "pro"); err != nil {
		t.Fatalf("SetPlan() error = %v", err)
	}
	checkSaved(t, path, draft, "work", overLines+"plan = \"pro\"\n")
}

func TestAnAccountWithALabelOverLinesTakesAReserveBelowIt(t *testing.T) {
	path := writeConfig(t, overLines)
	draft := edit(t, path)

	if err := draft.SetReserve("work", 0.05); err != nil {
		t.Fatalf("SetReserve() error = %v", err)
	}
	checkSaved(t, path, draft, "work", overLines+"reserve = 0.05\n")
}

func TestAnAccountWithALabelOverLinesIsMadeThePrimaryBelowIt(t *testing.T) {
	const side = "\n[[account]]\nid = \"side\"\n"
	path := writeConfig(t, overLines+side+"primary = true\n")
	draft := edit(t, path)

	if err := draft.SetPrimary("work"); err != nil {
		t.Fatalf("SetPrimary() error = %v", err)
	}
	checkSaved(t, path, draft, "work", overLines+"primary = true\n"+side)
}

// keyInLabel is a config whose account's label, over lines, holds a line
// that reads as a key.
const keyInLabel = "[[account]]\nid    = \"work\"\nlabel = \"\"\"\nplan = \"max5x\"\nreserve = 0.1\n\"\"\"\n"

func TestALabelOverLinesHoldingAKeyLineTakesThatKeyBelowIt(t *testing.T) {
	tests := []struct {
		name string
		set  func(d *config.Draft) error
		want string
	}{
		{name: "a plan", set: func(d *config.Draft) error { return d.SetPlan("work", "pro") }, want: "plan = \"pro\"\n"},
		{name: "a reserve", set: func(d *config.Draft) error { return d.SetReserve("work", 0.05) }, want: "reserve = 0.05\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, keyInLabel)
			draft := edit(t, path)

			if err := tt.set(draft); err != nil {
				t.Fatalf("setting %s: error = %v", tt.name, err)
			}
			checkSaved(t, path, draft, "work", keyInLabel+tt.want)
		})
	}
}

func TestAnAccountWithALabelOverLinesRefusesALabel(t *testing.T) {
	path := writeConfig(t, overLines)
	draft := edit(t, path)

	err := draft.SetLabel("work", "Day job")
	checkRefused(t, path, overLines, draft, err, cantMakeIt+`set label to "Day job" in account "work" by hand`, nil)
}

func TestAddAccountWritesNoLabelThatsItsID(t *testing.T) {
	const work = "[[account]]\nid = \"work\"\n"
	path := writeConfig(t, work)
	draft := edit(t, path)

	if err := draft.AddAccount(config.NewAccount{ID: "side", Label: "side"}); err != nil {
		t.Fatalf("AddAccount() error = %v", err)
	}
	checkSaved(t, path, draft, "side", work+"\n[[account]]\nid = \"side\"\n")
}

func TestSetPlan(t *testing.T) {
	tests := []struct {
		name   string
		config string
		id     string
		plan   string
		want   string
	}{
		{
			name:   "below the account's last key",
			config: workThenSide,
			id:     "side",
			plan:   "max5x",
			want:   replaced(t, workThenSide, "label = \"Side\"\n", "label = \"Side\"\nplan = \"max5x\"\n"),
		},
		{
			name:   "in place of its plan, keeping the comment beside it",
			config: "[[account]]\nid   = \"work\"\nplan = \"pro\"   # for now\n",
			id:     "work",
			plan:   "max20x",
			want:   "[[account]]\nid   = \"work\"\nplan = \"max20x\"   # for now\n",
		},
		{
			name:   "in place of a plan in triple literal quotes on the one line, keeping the comment beside it",
			config: "[[account]]\nid   = \"work\"\nplan = '''pro''' # c\n",
			id:     "work",
			plan:   "max5x",
			want:   "[[account]]\nid   = \"work\"\nplan = \"max5x\" # c\n",
		},
		{
			name:   "none, taking its plan out",
			config: "[[account]]\nid   = \"work\"\nplan = \"pro\"\n\n[[account]]\nid = \"side\"\n",
			id:     "work",
			plan:   "",
			want:   "[[account]]\nid   = \"work\"\n\n[[account]]\nid = \"side\"\n",
		},
		{
			name:   "none for an account without one, left as it is",
			config: commented,
			id:     "work",
			plan:   "",
			want:   commented,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.config)
			draft := edit(t, path)

			if err := draft.SetPlan(tt.id, tt.plan); err != nil {
				t.Fatalf("SetPlan(%q, %q) error = %v", tt.id, tt.plan, err)
			}
			if got := checkSaved(t, path, draft, tt.id, tt.want); got.Plan != tt.plan {
				t.Errorf("the plan loads as %q, want %q", got.Plan, tt.plan)
			}
		})
	}
}

func TestSetReserve(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		id      string
		reserve float64
		want    string
	}{
		{
			name:    "in place of its reserve, keeping the comment beside it",
			config:  commented,
			id:      "work",
			reserve: 0.05,
			want:    replaced(t, commented, "reserve = 0.1     # optional", "reserve = 0.05     # optional"),
		},
		{
			name:    "below the account's last key",
			config:  commented,
			id:      "personal",
			reserve: 0.023,
			want:    replaced(t, commented, "label = \"Personal\"\n", "label = \"Personal\"\nreserve = 0.023\n"),
		},
		{
			name:    "in place of a bare value in a file whose lines end in CRLF, keeping the ending",
			config:  "[[account]]\r\nid      = \"work\"\r\nreserve = 0.1\r\n",
			id:      "work",
			reserve: 0.05,
			want:    "[[account]]\r\nid      = \"work\"\r\nreserve = 0.05\r\n",
		},
		{
			name:    "in place of a bare value ending in CR in a file of mixed endings, keeping it",
			config:  "[[account]]\nid      = \"work\"\r\nreserve = 0.1\r\n",
			id:      "work",
			reserve: 0.05,
			want:    "[[account]]\nid      = \"work\"\r\nreserve = 0.05\r\n",
		},
		{
			name:    "none, taking its reserve out",
			config:  commented,
			id:      "work",
			reserve: 0,
			want:    replaced(t, commented, "reserve = 0.1     # optional\n", ""),
		},
		{
			name:    "the reserve it has already, left as it is",
			config:  commented,
			id:      "work",
			reserve: 0.1,
			want:    commented,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.config)
			draft := edit(t, path)

			if err := draft.SetReserve(tt.id, tt.reserve); err != nil {
				t.Fatalf("SetReserve(%q, %v) error = %v", tt.id, tt.reserve, err)
			}
			if got := checkSaved(t, path, draft, tt.id, tt.want); got.Reserve != tt.reserve {
				t.Errorf("the reserve loads as %v, want %v", got.Reserve, tt.reserve)
			}
		})
	}
}

// accountsInline lists the accounts inline, which a key can't be put in.
const accountsInline = "account = [{ id = \"work\" }, { id = \"side\" }]\n"

// cantMakeIt is the error of a change editing the text can't make, %s
// standing for the config file's path.
const cantMakeIt = "editing %s as text wouldn't make exactly the change meant, so it's left as it was: "

func TestSetLabelRefuses(t *testing.T) {
	tests := []struct {
		name   string
		config string
		id     string
		label  string
		// wantErr is the error, %s standing for the config file's path.
		wantErr string
		wantIs  error
	}{
		{
			name:    "an id no account has",
			config:  commented,
			id:      "side",
			label:   "Side",
			wantErr: `account "side" is not configured`,
			wantIs:  config.ErrNotConfigured,
		},
		{
			name:    "a label shaped like a token, never quoted",
			config:  commented,
			id:      "work",
			label:   tokenShaped,
			wantErr: `account "work": label looks like a token, which a label mustn't, as it shows wherever the account does`,
		},
		{
			name:    "a label of spaces alone",
			config:  commented,
			id:      "work",
			label:   "   ",
			wantErr: `account "work": label is blank, showing nothing`,
			wantIs:  config.ErrBlankLabel,
		},
		{
			name:    "a label holding a newline, which can't show",
			config:  commented,
			id:      "work",
			label:   "Side\nTeam",
			wantErr: `account "work": label holds a control character, such as a newline or a tab, which can't show`,
		},
		{
			name:    "a label holding a tab",
			config:  commented,
			id:      "work",
			label:   "Side\tTeam",
			wantErr: `account "work": label holds a control character, such as a newline or a tab, which can't show`,
		},
		{
			name:    "a label holding an escape, which would restyle what follows",
			config:  commented,
			id:      "work",
			label:   "Side\x1b[31m",
			wantErr: `account "work": label holds a control character, such as a newline or a tab, which can't show`,
		},
		{
			name:    "a label holding a bidirectional override, which can reorder the row",
			config:  commented,
			id:      "work",
			label:   "Side \u202e team",
			wantErr: `account "work": label holds a bidirectional embedding, override or isolate, which could reorder the rest of what it's shown in`,
		},
		{
			name:    "a label holding a bidirectional isolate",
			config:  commented,
			id:      "work",
			label:   "\u2067Side",
			wantErr: `account "work": label holds a bidirectional embedding, override or isolate, which could reorder the rest of what it's shown in`,
		},
		{
			name:    "a label that draws as nothing: Hangul fillers, blank braille and a mark on its own",
			config:  commented,
			id:      "work",
			label:   "\u3164\u115f\u1160\uffa0\u2800\u0301",
			wantErr: `account "work": label is blank, showing nothing`,
			wantIs:  config.ErrBlankLabel,
		},
		{
			name:    "a label past the longest, counted as it shows",
			config:  commented,
			id:      "work",
			label:   strings.Repeat("s", 50) + "   " + strings.Repeat("s", 51),
			wantErr: `account "work": label is 102 characters: must be at most 100`,
		},
		{
			name:    "a blank label for an id no account has, which is what's said",
			config:  commented,
			id:      "side",
			label:   "  ",
			wantErr: `account "side" is not configured`,
			wantIs:  config.ErrNotConfigured,
		},
		{
			name:    "a label of format characters alone, such as zero-width spaces and joiners, which show as nothing",
			config:  commented,
			id:      "work",
			label:   "\u200b \u2060\ufeff\u00ad",
			wantErr: `account "work": label is blank, showing nothing`,
			wantIs:  config.ErrBlankLabel,
		},
		{
			name:    "a label that isn't UTF-8",
			config:  commented,
			id:      "work",
			label:   "a\xffb",
			wantErr: `account "work": label isn't UTF-8 text, as a config's text must be`,
		},
		{
			name:    "accounts listed inline",
			config:  accountsInline,
			id:      "side",
			label:   "Side",
			wantErr: cantMakeIt + `set label to "Side" in account "side" by hand`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.config)
			draft := edit(t, path)

			err := draft.SetLabel(tt.id, tt.label)
			checkRefused(t, path, tt.config, draft, err, tt.wantErr, tt.wantIs)
		})
	}
}

func TestSetPlanRefuses(t *testing.T) {
	tests := []struct {
		name   string
		config string
		id     string
		plan   string
		// wantErr is the error, %s standing for the config file's path.
		wantErr string
		wantIs  error
	}{
		{
			name:    "a token given as the id, never quoted",
			config:  commented,
			id:      tokenShaped,
			plan:    "pro",
			wantErr: `account "[redacted]" is not configured`,
			wantIs:  config.ErrNotConfigured,
		},
		{
			name:    "a plan that isn't one",
			config:  commented,
			id:      "work",
			plan:    "free",
			wantErr: `account "work": plan "free": must be pro, max5x or max20x, the plan the account's subscription is on`,
		},
		{
			name:    "accounts listed inline",
			config:  accountsInline,
			id:      "side",
			plan:    "pro",
			wantErr: cantMakeIt + `set plan to "pro" in account "side" by hand`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.config)
			draft := edit(t, path)

			err := draft.SetPlan(tt.id, tt.plan)
			checkRefused(t, path, tt.config, draft, err, tt.wantErr, tt.wantIs)
		})
	}
}

func TestSetReserveRefuses(t *testing.T) {
	const outOfBounds = "must be at least 0 and less than 1, the share of every window the router leaves unused, such as 0.1"
	tests := []struct {
		name    string
		config  string
		id      string
		reserve float64
		// wantErr is the error, %s standing for the config file's path.
		wantErr string
		wantIs  error
	}{
		{
			name:    "an id no account has",
			config:  commented,
			id:      "side",
			reserve: 0.1,
			wantErr: `account "side" is not configured`,
			wantIs:  config.ErrNotConfigured,
		},
		{
			name:    "the whole window",
			config:  commented,
			id:      "work",
			reserve: 1,
			wantErr: `account "work": reserve 1: ` + outOfBounds,
		},
		{
			name:    "below none",
			config:  commented,
			id:      "work",
			reserve: -0.1,
			wantErr: `account "work": reserve -0.1: ` + outOfBounds,
		},
		{
			name:    "accounts listed inline",
			config:  accountsInline,
			id:      "side",
			reserve: 0.1,
			wantErr: cantMakeIt + `set reserve to 0.1 in account "side" by hand`,
		},
		{
			name:    "taking out a reserve written so the edit doesn't find it",
			config:  "[[account]]\nid = \"work\"\n\"res\\u0065rve\" = 0.1\n",
			id:      "work",
			reserve: 0,
			wantErr: cantMakeIt + `take reserve out of account "work" by hand`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.config)
			draft := edit(t, path)

			err := draft.SetReserve(tt.id, tt.reserve)
			checkRefused(t, path, tt.config, draft, err, tt.wantErr, tt.wantIs)
		})
	}
}

func TestInAnotherCase(t *testing.T) {
	accounts := config.Accounts{{ID: "work"}, {ID: "side"}}
	tests := []struct {
		id     string
		want   string
		wantOK bool
	}{
		{id: "Side", want: "side", wantOK: true},
		{id: "WORK", want: "work", wantOK: true},
		{id: "side"},
		{id: "spare"},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got, ok := accounts.InAnotherCase(tt.id)
			if got.ID != tt.want || ok != tt.wantOK {
				t.Errorf("InAnotherCase(%q) = %q, %v; want %q, %v", tt.id, got.ID, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestAddAccountTrimsTheLabelsSurroundingWhitespace(t *testing.T) {
	const work = "[[account]]\nid = \"work\"\n"
	path := writeConfig(t, work)
	draft := edit(t, path)

	if err := draft.AddAccount(config.NewAccount{ID: "side", Label: " Side  "}); err != nil {
		t.Fatalf("AddAccount() error = %v", err)
	}
	if got := checkSaved(t, path, draft, "side", work+"\n[[account]]\nid    = \"side\"\nlabel = \"Side\"\n"); got.Label != "Side" {
		t.Errorf("the label loads as %q, want %q", got.Label, "Side")
	}
}

func TestAddingAndRemovingKeepAByteOrderMark(t *testing.T) {
	const (
		work     = "\uFEFF[[account]]\nid = \"work\"\n"
		withSide = work + "\n[[account]]\nid    = \"side\"\nlabel = \"Side\"\n"
	)
	path := writeConfig(t, work)
	draft := edit(t, path)

	if err := draft.AddAccount(config.NewAccount{ID: "side", Label: "Side"}); err != nil {
		t.Fatalf("AddAccount() error = %v", err)
	}
	checkSaved(t, path, draft, "side", withSide)
	draft = edit(t, path)
	if err := draft.RemoveAccount("side"); err != nil {
		t.Fatalf("RemoveAccount() error = %v", err)
	}
	checkSaved(t, path, draft, "work", work)
}

// replaced returns text with old replaced by with, once, failing the test
// unless text holds old.
func replaced(t *testing.T, text, old, with string) string {
	t.Helper()
	if !strings.Contains(text, old) {
		t.Fatalf("%q doesn't hold %q", text, old)
	}
	return strings.Replace(text, old, with, 1)
}

func TestParsePlan(t *testing.T) {
	tests := []struct {
		given string
		want  string
	}{
		{given: "pro", want: "pro"},
		{given: "max5x", want: "max5x"},
		{given: "max20x", want: "max20x"},
		{given: "none", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.given, func(t *testing.T) {
			if got, err := config.ParsePlan(tt.given); err != nil || got != tt.want {
				t.Errorf("ParsePlan(%q) = %q, %v; want %q", tt.given, got, err, tt.want)
			}
		})
	}
}

func TestParsePlanRefuses(t *testing.T) {
	tests := []struct {
		name    string
		given   string
		wantErr string
	}{
		{name: "a plan it doesn't know", given: "free", wantErr: `plan "free": must be pro, max5x, max20x or none`},
		{name: "a plan in another case", given: "Max5x", wantErr: `plan "Max5x": must be pro, max5x, max20x or none`},
		{name: "nothing", given: "", wantErr: `plan "": must be pro, max5x, max20x or none`},
		{name: "a token, never quoted", given: tokenShaped, wantErr: `plan "[redacted]": must be pro, max5x, max20x or none`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := config.ParsePlan(tt.given); err == nil || err.Error() != tt.wantErr {
				t.Errorf("ParsePlan(%q) error = %v, want %q", tt.given, err, tt.wantErr)
			}
		})
	}
}

func TestParseReserve(t *testing.T) {
	tests := []struct {
		given string
		want  float64
	}{
		{given: "5%", want: 0.05},
		{given: "2.5%", want: 0.025},
		{given: "2.3%", want: 0.023},
		{given: "99.9%", want: 0.999},
		{given: "99.99%", want: 0.9999},
		{given: " 10 % ", want: 0.1},
		{given: "0%", want: 0},
		{given: "0", want: 0},
		{given: "0.0", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.given, func(t *testing.T) {
			if got, err := config.ParseReserve(tt.given); err != nil || got != tt.want {
				t.Errorf("ParseReserve(%q) = %v, %v; want %v", tt.given, got, err, tt.want)
			}
		})
	}
}

func TestParseReserveRefuses(t *testing.T) {
	const (
		notAPercentage = "must be a percentage, to two decimal places at most, such as 5% or 2.5%, or 0 for none"
		bare           = "must be written as a percentage, such as 5%, as a number alone could be a share or a percentage"
		outOfBounds    = "must be at least 0% and less than 100%, the share of every window the router leaves unused"
	)
	tests := []struct {
		name  string
		given string
		why   string
	}{
		{name: "a whole number alone", given: "5", why: bare},
		{name: "a share alone", given: "0.05", why: bare},
		{name: "the whole window", given: "100%", why: outOfBounds},
		{name: "below none", given: "-5%", why: notAPercentage},
		{name: "a sign", given: "+5%", why: notAPercentage},
		{name: "none with a sign", given: "-0%", why: notAPercentage},
		{name: "words", given: "five%", why: notAPercentage},
		{name: "nothing", given: "", why: notAPercentage},
		{name: "a percentage given twice", given: "5%%", why: notAPercentage},
		{name: "an exponent", given: "1e1%", why: notAPercentage},
		{name: "infinity", given: "inf%", why: notAPercentage},
		{name: "not a number", given: "NaN%", why: notAPercentage},
		{name: "past two decimal places", given: "99.999%", why: notAPercentage},
		{name: "a sliver past two decimal places", given: "0.001%", why: notAPercentage},
		{name: "digits set apart by underscores", given: "1_0%", why: notAPercentage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := `reserve "` + tt.given + `": ` + tt.why
			if _, err := config.ParseReserve(tt.given); err == nil || err.Error() != want {
				t.Errorf("ParseReserve(%q) error = %v, want %q", tt.given, err, want)
			}
		})
	}
}

// checkSaved saves the draft of the config at path, checks the file reads
// want and loads as the draft has it, and returns the account with the given
// id as it loads.
func checkSaved(t *testing.T, path string, draft *config.Draft, id, want string) config.Account {
	t.Helper()
	save(t, draft)
	if got := readFile(t, path); got != want {
		t.Errorf("the config reads\n%s\nwant\n%s", got, want)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(draft.Config(), cfg) {
		t.Errorf("Config() = %+v, want what the file saved loads as, %+v", draft.Config(), cfg)
	}
	for _, a := range cfg.Accounts {
		if a.ID == id {
			return a
		}
	}
	t.Fatalf("no account %q loads", id)
	return config.Account{}
}
