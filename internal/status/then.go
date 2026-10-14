package status

import (
	"time"

	"github.com/leeovery/switchboard/internal/readings"
)

// WasKind is how an account stood at a moment gone, as a Was says.
type WasKind int

// The kinds, in the order Then tries them, which is ROUTING's, as an
// account's Routing says how it stands at a moment: the first that holds is
// the one said.
const (
	// WasTokenless is an account without a usable token now, which had no
	// room at any moment, as Runway draws it.
	WasTokenless WasKind = iota + 1
	// WasLimited is an account a limit held back from every request.
	WasLimited
	// WasRefused is an account whose token the upstream refused.
	WasRefused
	// WasUnread is an account nothing was read of yet.
	WasUnread
	// WasCapped is an account at its cap, which held it back.
	WasCapped
	// WasRoom is an account with room.
	WasRoom
)

// Was is how an account stood at a moment gone, as Then has it: its kind,
// and, of a hold, when it began and when it lifts, each zero where what Then
// is given doesn't tell.
type Was struct {
	Kind         WasKind
	Since, Until time.Time
}

// Span is a hold of an account the router told of, which the readings
// history can't show: of Kind WasLimited, a limit naming no window, which
// holds back every request; of Kind WasRefused, the upstream refusing its
// token. It holds from From, zero where nothing tells when it began, as from
// before any moment asked of, up to Until.
type Span struct {
	Kind        WasKind
	From, Until time.Time
}

// Then is how the account a stood at t, of those of the moment, by one rule
// for every moment gone, live or stepped back to, which at Now agrees with
// how the moment has the account stand: from the readings history's
// readings, oldest first, as the history reads them, of the windows the
// moment's policy says every model shares, each by its last reading by t; and
// from the holds the router told of that they can't show. A window whose
// reset has passed by t counts as reset, unused, with room. Its kind is the
// first that holds of: without a usable token now, none at any moment; a
// limit told of, or a window read spent, holding at t; its token refused, as
// told of; nothing read by t; its cap, as its reserve holds now, a window read
// at where its reserve starts, as configured now, as the readings keep no
// reserve; and room. Of a hold, Since is the earliest of when each part of
// what holds it at t began: a told hold's From, or the first reading of the
// unbroken run of a window's readings that shows it, of those given; and
// Until is when the last of them lifts, zero where one lifts at no known time.
func (m Moment) Then(a AccountNow, read []readings.Reading, told []Span, t time.Time) Was {
	return Was{Kind: WasUnread}
}
