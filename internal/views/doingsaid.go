package views

import (
	"strconv"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// Said says what it's doing at now, as a session's page says it on row 6,
// and the Switchboard and sessions <id>'s text say it too: "↑ asking" while
// it asks; "↓ answering" while its answer streams; else as State says it.
func (a Activity) Said(now time.Time) string {
	switch a.Doing {
	case Asking:
		return MarkAsking.String() + " " + status.Asking
	case Answering:
		return MarkAnswering.String() + " " + status.Answering
	}
	return a.State(now)
}

// State says what it's doing at now, as the List's state says it, beside
// the row's mark, which shows ↑ and ↓: "asking"; "answering"; "✕ 429" or "✕
// 403", while a refusal shows, before its request goes out again elsewhere;
// "… 429" while it's throttled, to go out again on its account; "idle 9m",
// for as long as it's been since it was last at work, as prose.Span says it,
// "idle" alone where that isn't known; and once it's over, "ended yesterday
// 18:47", when its last request came, as prose.Past says it.
func (a Activity) State(now time.Time) string {
	switch a.Doing {
	case Asking:
		return status.Asking
	case Answering:
		return status.Answering
	case Throttled:
		return "… " + strconv.Itoa(a.Status)
	case Refused:
		return "✕ " + strconv.Itoa(a.Status)
	case Ended:
		return "ended " + prose.Past(now, a.Since)
	}
	if a.Since.IsZero() {
		return "idle"
	}
	return "idle " + prose.Span(now.Sub(a.Since))
}

// Short says what it's doing as SESSIONS says it, in a row's few cells, the
// long form Said gives shortened: "↓ ~1.2k" while its answer streams, its
// tokens so far, "↓" alone till the stream counts any; "↑ ask" while it
// waits for its answer, as Waiting says, throttled or refused as well as
// asking; else "idle", as a session SESSIONS lists never has ended.
func (a Activity) Short() string {
	switch {
	case a.Doing == Answering && a.Tokens() == 0:
		return MarkAnswering.String()
	case a.Doing == Answering:
		return MarkAnswering.String() + " ~" + prose.Tokens(a.Tokens())
	case a.Waiting():
		return MarkAsking.String() + " ask"
	}
	return "idle"
}
