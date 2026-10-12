package views

import (
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// Sentence is one of the router's events as LATELY tells it: its id, which
// the cue on a new row follows; when it happened; its kind, which its mark
// is drawn for; and its words, in full, and short, as a phone has them, as
// "work reached its cap: 3 sessions moved to side" and "work at its cap: 3
// moved to side".
type Sentence struct {
	ID          int
	At          time.Time
	Kind        string
	Full, Short status.Words
}

// Lately are doc's events as LATELY tells them, the newest first, n at most:
// the sessions a limit, a cap or a refusal moved folded into its line,
// counted, a move showing alone only where the line it's folded into isn't
// among those shown.
func Lately(doc status.Document, n int) []Sentence {
	return nil
}
