package config_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
)

// The flags of Scotland and Wales, as england is England's.
const (
	scotlandFlag = "\U0001f3f4\U000e0067\U000e0062\U000e0073\U000e0063\U000e0074\U000e007f"
	walesFlag    = "\U0001f3f4\U000e0067\U000e0062\U000e0077\U000e006c\U000e0073\U000e007f"
)

// joinsTheNext is why a label holding a prepended concatenation mark is
// refused.
const joinsTheNext = "holds a sign that joins the character after it, such as an Arabic number sign, which can hide that character"

// A label's own rule keeps hidden content out of it, so what's saved is what
// shows: each hidden kind dropped where it's drawn as nothing, as
// prose.MapUnseen has it, or refused, saying why, where it hides what's
// beside it; a real flag and characters joined within themselves pass.
func TestALabelCarriesNothingHidden(t *testing.T) {
	tests := []struct {
		name, label string
		// want is the label saved; wantWhy, where it's refused, why.
		want, wantWhy string
	}{
		{name: "a tag run, as a fake flag, its tags dropped", label: "Work\U0001f3f4\U000e0068\U000e0069\U000e0064\U000e0065\U000e007f", want: "Work\U0001f3f4"},
		{name: "a tag run of a flag Unicode doesn't recommend, its tags dropped", label: "Work\U0001f3f4\U000e0075\U000e0073\U000e0074\U000e0078\U000e007f", want: "Work\U0001f3f4"},
		{name: "a tag run alone, dropped", label: "Wo\U000e0068\U000e0069rk", want: "Work"},
		{name: "a run of variation selectors after a symbol, all but the first dropped", label: "Work\u2b50\ufe0f\ufe0e\ufe01\ufe02", want: "Work\u2b50\ufe0f"},
		{name: "a run of supplementary variation selectors, all but the first dropped", label: "\u845b\U000e0100\U000e0101\U000e0102", want: "\u845b\U000e0100"},
		{name: "variation selectors after a Latin letter, dropped", label: "Wor\ufe00\ufe01k", want: "Work"},
		{name: "a combining grapheme joiner, dropped", label: "Wo\u034frk", want: "Work"},
		{name: "Hangul fillers, dropped", label: "W\u3164o\u115fr\u1160k\uffa0", want: "Work"},
		{name: "zero-width spaces and joiners between Latin letters, dropped", label: "W\u200bo\u200dr\u2060k", want: "Work"},
		{name: "an Arabic number sign, refused", label: "Work \u0600123", wantWhy: joinsTheNext},
		{name: "the end of an ayah, refused", label: "\u06ddWork", wantWhy: joinsTheNext},
		{name: "the Syriac abbreviation mark, refused", label: "Wo\u070frk", wantWhy: joinsTheNext},
		{name: "the Arabic pound mark above, refused", label: "Wo\u0890rk", wantWhy: joinsTheNext},
		{name: "the Arabic disputed end of ayah, refused", label: "Wo\u08e2rk", wantWhy: joinsTheNext},
		{name: "a Kaithi number sign, refused", label: "Work\U000110bd1", wantWhy: joinsTheNext},
		{name: "a Kaithi number sign above, refused", label: "Work\U000110cd1", wantWhy: joinsTheNext},
		{name: "England's flag, kept", label: "Work " + england, want: "Work " + england},
		{name: "Scotland's flag, kept", label: "Work " + scotlandFlag, want: "Work " + scotlandFlag},
		{name: "Wales's flag, kept", label: "Work " + walesFlag, want: "Work " + walesFlag},
		{name: "a family joined into one emoji, kept", label: "Home \U0001f468\u200d\U0001f469\u200d\U0001f467", want: "Home \U0001f468\u200d\U0001f469\u200d\U0001f467"},
		{name: "a Devanagari conjunct with its joiner, kept", label: "\u0915\u094d\u200d\u0937", want: "\u0915\u094d\u200d\u0937"},
		{name: "a Persian half-space, kept", label: "\u0645\u06cc\u200c\u062e\u0648\u0627\u0647\u0645", want: "\u0645\u06cc\u200c\u062e\u0648\u0627\u0647\u0645"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			draft := edit(t, writeConfig(t, commented))
			err := draft.SetLabel("work", tt.label)
			if tt.wantWhy != "" {
				wrong, ok := errors.AsType[*config.WrongLabel](err)
				if !ok || wrong.Why != tt.wantWhy {
					t.Fatalf("SetLabel(%+q) error = %v, want a WrongLabel saying %q", tt.label, err, tt.wantWhy)
				}
				if got, want := err.Error(), `account "work": label `+tt.wantWhy; got != want {
					t.Errorf("the error reads %q, want %q", got, want)
				}
				return
			}
			if err != nil {
				t.Fatalf("SetLabel(%+q) error = %v", tt.label, err)
			}
			if got := draft.Config().Accounts[0].Label; got != tt.want {
				t.Errorf("the label is %+q, want %+q", got, tt.want)
			}
		})
	}
}

// Every refusal of a label is a WrongLabel, why it's refused said alone,
// and a blank one matching ErrBlankLabel.
func TestALabelsRefusalsSayWhyAlone(t *testing.T) {
	tests := []struct {
		name, label, why string
		is               error
	}{
		{name: "blank", label: " \u200b ", why: "is blank, showing nothing", is: config.ErrBlankLabel},
		{name: "a control", label: "a\tb", why: "holds a control character, such as a newline or a tab, which can't show"},
		{name: "a bidirectional control", label: "a\u202eb", why: "holds a bidirectional embedding, override or isolate, which could reorder the rest of what it's shown in"},
		{name: "not UTF-8", label: "a\xffb", why: "isn't UTF-8 text, as a config's text must be"},
		{name: "too long", label: strings.Repeat("s", 101), why: "is 101 characters: must be at most 100"},
		{name: "shaped like a token", label: tokenShaped, why: "looks like a token, which a label mustn't, as it shows wherever the account does"},
		{name: "a prepended mark", label: "\u0600work", why: joinsTheNext},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := edit(t, writeConfig(t, commented)).SetLabel("work", tt.label)
			wrong, ok := errors.AsType[*config.WrongLabel](err)
			if !ok || wrong.Why != tt.why {
				t.Fatalf("SetLabel error = %v, want a WrongLabel saying %q", err, tt.why)
			}
			if tt.is != nil && !errors.Is(err, tt.is) {
				t.Errorf("the error %v doesn't match %v", err, tt.is)
			}
		})
	}
}

// Removing the only account is refused, matching ErrOnlyAccount.
func TestRemovingTheOnlyAccountMatchesErrOnlyAccount(t *testing.T) {
	err := edit(t, writeConfig(t, "[[account]]\nid = \"work\"\n")).RemoveAccount("work")
	if !errors.Is(err, config.ErrOnlyAccount) {
		t.Errorf("RemoveAccount error = %v, want it matching ErrOnlyAccount", err)
	}
}
