package dashboard

import (
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/status/statustest"
	"github.com/leeovery/switchboard/internal/views"
)

// At each hard moment, the Switchboard's lines say of each account what the
// Overview's views say of it: each window as USAGE draws it, bar and words;
// how it stands, in ROUTING's glyph and first words, as its name row draws
// them; and as many stubs hanging from its line as the sessions its limit
// moved, as the moment counts them.
func TestTheSwitchboardsLinesAgreeWithTheViewsAtEachHardMoment(t *testing.T) {
	for _, hm := range statustest.HardMoments(claudeLike) {
		t.Run(hm.Name, func(t *testing.T) {
			d := drawnAt(t, hm.Doc, hm.Now)
			f := boardFrame(160, 40, Look{}, boardOf(boardLook{stubs: views.StubsOf(d.moment)}), Traffic{}).At(hm.Doc, hm.Now)
			laid := f.switchboardLaid(hm.Doc, hm.Now)
			lines := f.linesDrawn(d.moment, laid)
			rows := textOf(f.Draw(hm.Doc, hm.Now))
			for _, a := range d.moment.Accounts {
				for _, w := range a.Windows {
					if g, ok := d.gauge(a, w); ok {
						agree(t, a.ID+"'s "+w.Key, []viewSays{{"USAGE", gaugeSays(g)}, {"the Switchboard's line", gaugeSays(lines.gauge(a, w.Key))}})
					}
				}
				ln, ok := laid.line(a.ID)
				if !ok {
					t.Fatalf("the Switchboard lays out no line of %s", a.ID)
				}
				agree(t, a.ID+" stands", []viewSays{{"ROUTING", d.routingSays(a)}, {"the Switchboard's line", standsSays(rows[f.top()+ln.top], lines, a)}})
				agree(t, a.ID+"'s stubs", []viewSays{
					{"its limit's event", strconv.Itoa(a.LimitMoved)}, {"the Switchboard's line", strconv.Itoa(len(ln.stubs))},
				})
			}
		})
	}
}

// standsSays is how account a's name row, row, says it stands, as ROUTING
// would: its glyph and first words, from the name column, before the pin's
// mark.
func standsSays(row string, lines linesDrawn, a status.AccountNow) string {
	words := strings.TrimSuffix(strings.TrimSpace(string([]rune(row)[lines.x()+lines.column:])), pinMark(a).plain())
	said, _, _ := strings.Cut(strings.TrimSpace(words), clauseApart)
	return said
}

// The moment's stubs are what a limit's event counts, while its limit holds:
// none once it lifts, though the event stays.
func TestTheSwitchboardHangsNoStubOnceTheLimitLifts(t *testing.T) {
	doc := usageWorld(personalAtItsLimit(atClock(14, 12, 0), atClock(14, 40, 0), 2))
	m := doc.At(routedAt, claudeLike)
	if a, _ := m.Account("personal"); a.LimitMoved != 0 {
		t.Fatalf("personal's limit lifted, LimitMoved = %d, want 0", a.LimitMoved)
	}
	f := boardFrame(160, 37, Look{}, boardOf(boardLook{stubs: views.StubsOf(m)}), Traffic{})
	if got := strings.Count(strings.Join(textOf(f.Draw(doc, routedAt)), "\n"), hangingGlyph); got != 0 {
		t.Errorf("drew %d of %s, want no stub hanging", got, hangingGlyph)
	}
}
