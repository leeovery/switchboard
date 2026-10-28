package config

import (
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/prose"
)

// ShownSequences are labels whose unseen characters each do something that
// shows, so taking one keeps it whole.
var ShownSequences = map[string]string{
	"a family":                              "\U0001f468\u200d\U0001f469\u200d\U0001f467",
	"an eye in a speech bubble":             "\U0001f441\ufe0f\u200d\U0001f5e8\ufe0f",
	"a rainbow flag":                        "\U0001f3f3\ufe0f\u200d\U0001f308",
	"a heart on fire":                       "\u2764\ufe0f\u200d\U0001f525",
	"a woman running":                       "\U0001f3c3\u200d\u2640\ufe0f",
	"a technologist with a skin tone":       "\U0001f468\U0001f3fd\u200d\U0001f4bb",
	"people holding hands":                  "\U0001f9d1\u200d\U0001f91d\u200d\U0001f9d1",
	"a keycap one":                          "1\ufe0f\u20e3",
	"a keycap number sign":                  "#\ufe0f\u20e3",
	"England's flag":                        "\U0001f3f4\U000e0067\U000e0062\U000e0065\U000e006e\U000e0067\U000e007f",
	"Scotland's flag":                       "\U0001f3f4\U000e0067\U000e0062\U000e0073\U000e0063\U000e0074\U000e007f",
	"a Persian word with a non-joiner":      "\u0645\u06cc\u200c\u062e\u0648\u0627\u0647\u0645",
	"a Devanagari conjunct with a joiner":   "\u0915\u094d\u200d\u0937",
	"a Han character's ideographic variant": "\u845b\U000e0100",
}

// unseenProbes are labels whose unseen characters do nothing that shows, or
// only seem to.
var unseenProbes = []string{
	"x \ufe0f\ufe0f",
	"\ufe0f\ufe0fHome",
	"\ufe0f\u200dwork",
	"\u200e\ufe0f\u200dwork",
	"work \U000e0067\ufe0f",
	"work\U000e0067\u200db",
	"work\ufe0f\ufe0f\ufe0f",
	"wo\u200drk",
	"work\ufe0f",
	"\U0001f3f4\U000e0067\U000e0062",
	"\u2764\u200e\ufe0f\u200d\U0001f525",
	"a \u200d b\u200b\u2063",
}

// FuzzTakeLabel holds a label taken to what it shows: taking it again
// changes nothing, and quoted, it's as it is but for its direction marks,
// quote marks and backslashes escaped, as every other unseen character
// it holds is owned by a sequence.
func FuzzTakeLabel(f *testing.F) {
	for _, given := range unseenProbes {
		f.Add(given)
	}
	for _, given := range ShownSequences {
		f.Add(given)
	}
	f.Fuzz(func(t *testing.T, given string) {
		label, err := takeLabel("work", given)
		if err != nil {
			return
		}
		if again, err := takeLabel("work", label); err != nil || again != label {
			t.Fatalf("takeLabel(%q) = %q, then takeLabel of that = %q, %v; want it as it was", given, label, again, err)
		}
		if got, want := prose.Quoted(label), `"`+markEscaper.Replace(label)+`"`; got != want {
			t.Errorf("Quoted(%q) = %q, want %q", label, got, want)
		}
	})
}

// markEscaper escapes a label's quote marks and backslashes, and its
// direction marks, the only unseen characters a label keeps that no
// sequence owns.
var markEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`,
	"\u200e", `\u200E`, "\u200f", `\u200F`, "\u061c", `\u061C`)
