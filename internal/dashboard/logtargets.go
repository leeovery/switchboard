package dashboard

import "time"

// logTargets are the zones of the Log's page at now a click acts on, beside
// its sub-tabs, which the heading row gives, once its first read lands:
// each row of the table shown in view, by its place among the table's
// lines, across the width; and, while the state line counts the lines come
// since a pause, "▼ 214 new", which follows again, as end does.
func (f Frame) logTargets(now time.Time) []Target {
	p := f.Log
	if !p.Read {
		return nil
	}
	lines, s, rows := p.lines(), p.stand(), f.LogRows()
	first, last := p.window(lines, s, rows)
	var targets []Target
	for i := first; i <= last; i++ {
		if lines[i].choosable() {
			targets = append(targets, Target{X: 0, Y: f.top() + 1 + i - first, Width: f.Width, On: OnRow, Row: i})
		}
	}
	if x, width, ok := p.freshAt(rows, f.Width, now); ok {
		targets = append(targets, Target{X: x, Y: f.Height - underView, Width: width, On: OnKey, Key: "end"})
	}
	return targets
}

// freshAt is where the state line, width cells wide in a view rows rows
// tall, at now, draws the lines come since a pause, "▼ 214 new": from
// column x, width cells wide; false where it draws none, or not whole, or
// the find's line takes its place.
func (p LogPage) freshAt(rows, width int, now time.Time) (x, cells int, ok bool) {
	s := p.state(rows)
	if p.Finding.Open || s.standing != logPaused || s.fresh == 0 {
		return 0, 0, false
	}
	if _, kept := s.said(now); kept.width() > width {
		return 0, 0, false
	}
	return s.pausedSaid(now).width() + headingGap, s.freshSaid().width(), true
}

// Showing is the Log showing the sub-tab given, as a click on it shows it:
// each table stands as it stood.
func (p LogPage) Showing(v LogView) LogPage {
	if v == LogRequests || v == LogEvents {
		p.Shown = v
	}
	return p
}

// Lit reports whether the line at the place given among the table shown's
// is the one chosen, a click on which opens its details.
func (p LogPage) Lit(at int) bool {
	lit, ok := p.lit(p.lines(), p.stand())
	return ok && lit == at
}

// ChoseLine is the table shown in a view rows rows tall with the row at the
// place given among its lines chosen, as a click on it chooses it:
// following, it pauses where it is, as Chose pauses it. A line that's no
// row, or no line, chooses nothing. A view of no rows is placed as
// placedRows has it.
func (p LogPage) ChoseLine(at, rows int) LogPage {
	lines, s, rows := p.lines(), p.stand(), placedRows(rows)
	if at < 0 || at >= len(lines) || !lines[at].choosable() {
		return p
	}
	if !s.paused {
		s = p.pausedAt(lines, rows)
	}
	return p.standing(p.choosing(lines, s, at, rows))
}
