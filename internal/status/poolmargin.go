package status

import (
	"time"

	"github.com/leeovery/switchboard/internal/forecast"
	"github.com/leeovery/switchboard/internal/prose"
)

// PoolMargin is how near the pool is to running out within the shared week
// ahead, as the document's forecast plays it from when it was built, said as
// it stands at a moment: how much busier its use could get and it still
// last, or when it runs out, and how much less use would last. Its zero
// value says nothing, as of a document without a forecast, a pool of fewer
// than two accounts, or one with no room as the document was built.
type PoolMargin struct {
	margin forecast.Margin
	now    time.Time
}

// PoolMargin is how near the pool is to running out within the week ahead,
// as the document's forecast plays it, said at Now. A run-out Now has
// reached, as of a document left on screen while the router doesn't answer,
// is past, not to come, as runOutOf has it: the margin says nothing.
func (m Moment) PoolMargin() PoolMargin {
	margin := m.doc.poolMargin()
	if margin.Kind == forecast.MarginRunsOut && !margin.RunsOut.After(m.Now) {
		return PoolMargin{}
	}
	return PoolMargin{margin: margin, now: m.Now}
}

// RunsOut is when the pool runs out within the week ahead, reporting false
// where it lasts, or the margin says nothing.
func (p PoolMargin) RunsOut() (time.Time, bool) {
	return p.margin.RunsOut, p.margin.Kind == forecast.MarginRunsOut
}

// Words say the margin in full: "lasts the week, 15% to spare", or "runs out
// ~Sat · lasts at 20% less", its run-out as prose.RunOut says it, its
// figures figures, and without its "lasts at" where even 50% less use
// wouldn't last. None where it says nothing.
func (p PoolMargin) Words() Words {
	switch p.margin.Kind {
	case forecast.MarginLasts:
		return append(Words{Plain("lasts the week, ")}, p.spare()...)
	case forecast.MarginRunsOut:
		said := Words{Plain("runs out ~"), Time(prose.RunOut(p.now, p.margin.RunsOut))}
		if p.margin.Less == 0 {
			return said
		}
		return append(said, Plain(" · lasts at "), Figure(percentOf(p.margin.Less)), Plain(" less"))
	case forecast.MarginNone:
	}
	return nil
}

// ShortWords say the margin for a narrow screen: "15% to spare", or "out
// ~Sat". None where it says nothing.
func (p PoolMargin) ShortWords() Words {
	switch p.margin.Kind {
	case forecast.MarginLasts:
		return p.spare()
	case forecast.MarginRunsOut:
		return Words{Plain("out ~"), Time(prose.RunOut(p.now, p.margin.RunsOut))}
	case forecast.MarginNone:
	}
	return nil
}

// spare says how much busier the pool's use could get and it still last:
// "15% to spare", "under 5% to spare" where it couldn't get a step busier,
// and "over 50% to spare" past as far as the forecast plays it.
func (p PoolMargin) spare() Words {
	switch spare := p.margin.Spare; {
	case spare < forecast.MarginStep:
		return Words{Plain("under "), Figure(percentOf(forecast.MarginStep)), Plain(" to spare")}
	case spare > forecast.MarginReach:
		return Words{Plain("over "), Figure(percentOf(forecast.MarginReach)), Plain(" to spare")}
	default:
		return Words{Figure(percentOf(spare)), Plain(" to spare")}
	}
}

// percentOf says a whole percent, n of a hundred, as prose.Percent does:
// "15%".
func percentOf(n int) string {
	return prose.Percent(float64(n) / 100)
}
