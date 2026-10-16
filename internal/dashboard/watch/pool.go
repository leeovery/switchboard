package watch

import (
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/views"
)

// poolTab is what the watch holds of the pool's charts GRAPHS draws beside
// Σ pool by account: the charts, as the last look laid them out, none while
// they don't show; and the pool's room in each window charted, by its key,
// which holds what it judged of the past till what that's judged from
// changes, kept while the charts don't show for when they show again, and
// let go once they show without its window; and look, where it's given, how
// a look builds the room, as a test counts the builds through it.
type poolTab struct {
	charts []dashboard.PoolChart
	rooms  map[string]views.PoolRoom
	look   func(views.PoolRoom, views.PoolRoomFrom) views.PoolRoom
}

// looking is how the tab builds the pool's room in a window at a look: its
// look, where it's given, else as views.PoolRoom.Look builds it.
func (p poolTab) looking() func(views.PoolRoom, views.PoolRoomFrom) views.PoolRoom {
	if p.look == nil {
		return views.PoolRoom.Look
	}
	return p.look
}

// poolBehind reports whether the past of any of the pool's charts is judged
// at now through a moment after through, when the read of what the live
// tail holds was asked, as views.PoolRoomThrough has it: so its last column
// waits for a read now.
func (m Model) poolBehind(through, now time.Time) bool {
	return slices.ContainsFunc(m.pool.charts, func(ch dashboard.PoolChart) bool {
		return views.PoolRoomThrough(ch.Key, ch.Columns, now).After(through)
	})
}

// poolLooked is m with the pool's charts built at the look msg takes, as
// poolBuilt builds them, of f, the frame the look built, and f with the
// pool's room in each window as that leaves it. A frame takes nothing in, so
// builds nothing.
func (m Model) poolLooked(msg tea.Msg, f dashboard.Frame) (Model, dashboard.Frame) {
	if _, frame := msg.(frameMsg); frame {
		return m, f
	}
	m = m.poolBuilt(f)
	f.PoolRooms = m.pool.rooms
	return m, f
}

// poolBuilt is m with the pool's charts laid out afresh at the moment of the
// message taken in, as f, the frame the look built, lays them out, while the
// Overview shows with no session's page over it, and the pool's room built
// in each of their windows from the last look's, as the tab's looking builds
// it, a window no longer charted let go: of the readings Runway's live lanes
// are judged by, the readings history's live tail and those the watch read
// itself, as views.RunwayReadings joins them, read through when the live
// tail's last read was asked.
func (m Model) poolBuilt(f dashboard.Frame) Model {
	m.pool.charts = nil
	if m.view != dashboard.Overview || m.session != nil {
		return m
	}
	now := m.now()
	charts := f.PoolCharts(m.doc, now)
	if len(charts) == 0 {
		return m
	}
	rooms := make(map[string]views.PoolRoom, len(charts))
	mo, read, look := m.moment(now), views.RunwayReadings(m.runwayTab.live.held, m.runwayTab.seen.readings()), m.pool.looking()
	for _, ch := range charts {
		rooms[ch.Key] = look(m.pool.rooms[ch.Key], views.PoolRoomFrom{Moment: mo, Readings: read, ReadThrough: m.runwayTab.live.through, Key: ch.Key, Columns: ch.Columns})
	}
	m.pool.charts, m.pool.rooms = charts, rooms
	return m
}
