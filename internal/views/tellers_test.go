package views_test

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// saying reads a fact the Log's Events and LATELY both say of an event of
// the kinds given off each one's words, as one or more parts: the Log's what
// happened, as loggedAlike puts it, and LATELY's sentence in full.
type saying struct {
	fact           string
	kinds          []string
	logged, lately *regexp.Regexp
}

// sayings are the facts both tellers say: when the account a move's choice
// passed over runs out, by the clock, whatever day each names; the sessions a
// limit, a cap or a refusal moved, counted, and where to; and the windows an
// event names. Those one teller leaves unsaid by design, as LATELY a cap's
// windows, are left out.
var sayings = []saying{
	{
		fact: "when the account passed over runs out", kinds: []string{status.EventMoved},
		logged: regexp.MustCompile(`, its \S+ to run out at (?:\w{3} )?(\d\d:\d\d)`),
		lately: regexp.MustCompile(`, its \S+ to run out at (?:\w{3} )?(\d\d:\d\d)`),
	},
	{
		fact: "the sessions it moved", kinds: []string{status.EventLimit, status.EventCap, status.EventRefused},
		logged: regexp.MustCompile(`: its (\d+) sessions move to (.+?)(?: as .*)?$`),
		lately: regexp.MustCompile(`: (\d+) sessions? moved to (.+)$`),
	},
	{
		fact: "the windows it reached", kinds: []string{status.EventLimit},
		logged: regexp.MustCompile(`^its (.+?) windows?(?:,|:|$)`),
		lately: regexp.MustCompile(`reached its (.+?) limits?(?::|$)`),
	},
	{
		fact: "the windows it started or reset", kinds: []string{status.EventPrimed, status.EventRoom},
		logged: regexp.MustCompile(`its (.+?) windows? (?:started|reset)`),
		lately: regexp.MustCompile(`its (.+?) windows? (?:started|reset)`),
	},
	{
		fact: "the window under pressure", kinds: []string{status.EventPressure},
		logged: regexp.MustCompile(`^its (.+?) to run out`),
		lately: regexp.MustCompile(`its (.+?) runs out`),
	},
}

// loggedAlike puts the Log's words for one session moved, and for several
// accounts, as LATELY's are, so the two read alike.
var loggedAlike = strings.NewReplacer("its session moves to", "its 1 sessions move to", "another account", "other accounts")

// readOff is what re reads off words: its parts, " " between them, or "" where
// it reads none.
func readOff(re *regexp.Regexp, words string) string {
	parts := re.FindStringSubmatch(words)
	if parts == nil {
		return ""
	}
	return strings.Join(parts[1:], " ")
}

// For the same events, the document's at each hard moment, the Log's Events
// and LATELY say the same of each fact both say, each read off its words, as
// both take the facts from one owner: none where one says none. Every fact
// is said by both somewhere among the moments.
func TestTheLogAndLATELYSayTheSameFactsOfTheSameEvents(t *testing.T) {
	saidByBoth := make(map[string]bool)
	for _, hm := range viewstest.EventMoments() {
		t.Run(hm.Name, func(t *testing.T) {
			m := status.Document{Source: status.SourceRouter, Events: hm.Live}.At(hm.Now, claude.Policy())
			telling := views.NewTelling(m.NameOf)
			logged := make(map[int]string)
			for _, e := range slices.Backward(hm.Live) {
				logged[e.ID] = loggedAlike.Replace(telling.Line(e, hm.Now).What.String())
			}
			for _, s := range views.Lately(m, len(hm.Live)) {
				for _, say := range sayings {
					if !slices.Contains(say.kinds, s.Kind) {
						continue
					}
					byLog, byLately := readOff(say.logged, logged[s.ID]), readOff(say.lately, s.Full.String())
					if byLog != byLately {
						t.Errorf("of event %d, %s: the Log says %q, in %q; LATELY says %q, in %q", s.ID, say.fact, byLog, logged[s.ID], byLately, s.Full)
					}
					saidByBoth[say.fact] = saidByBoth[say.fact] || byLog != ""
				}
			}
		})
	}
	for _, say := range sayings {
		if !saidByBoth[say.fact] {
			t.Errorf("no hard moment has both say %s", say.fact)
		}
	}
}
