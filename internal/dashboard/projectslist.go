package dashboard

import (
	"slices"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/views"
)

// projectsPart is one of Projects' list's columns.
type projectsPart int

// The list's columns, in the row's order.
const (
	listedName projectsPart = iota
	listedLive
	listedSessions
	listedWorth
	listedBar
	listedRequests
	listedLast
)

// The list's figures, in cells: a name's least, as it's cut; and the bar's
// least.
const (
	projectNameLeast = 10
	projectBarLeast  = 8
)

// The turns the list's columns give way in as it narrows, once the bar is
// at its least: requests leave, then last; then the name is cut toward its
// least.
const (
	requestsLeave = iota + 1
	lastLeaves
	nameNarrows
)

// The list's words: its heads, but the bar's, which has none; the words of
// the sessions with no directory, and of every project's row; and what it
// says with no project to list.
var (
	projectHeads = map[projectsPart]string{
		listedName: "project", listedLive: "live", listedSessions: "sessions", listedWorth: "worth", listedRequests: "requests", listedLast: "last",
	}
	noDirectory = "no directory"
	allProjects = "all projects"
	noProjects  = line{{"no sessions", faintInk}}
	projectsOut = []outForm{{noun: "project", gap: " · ", inks: keyedOut}, {gap: " · ", inks: keyedOut}}
)

// projectColumn is a column of the list as it's laid out: which it is, from
// column x, width cells wide.
type projectColumn struct {
	part     projectsPart
	x, width int
}

// projectsList is Projects' list as a frame lays it out of its page at now,
// in its look, width cells wide: its rows, the one chosen among them, and
// every project's; its columns; whether ↑ and ↓ act in it, lighting its
// chosen row; and how many of its projects it shows from the first shown,
// as far as it's scrolled to show the one chosen.
type projectsList struct {
	rows    []views.ProjectRow
	chosen  int
	all     views.ProjectTotals
	columns []projectColumn
	lit     bool
	shown   int
	first   int
	width   int
	look    Look
	now     time.Time
}

// listedProject is a row of the list: what names it, its totals, and its
// project's row, nil of every project's.
type listedProject struct {
	named  line
	totals views.ProjectTotals
	row    *views.ProjectRow
}

// projectsListOf is the list of the frame f's page at now, as columnsOf lays
// its columns out, its projects shown in as many rows as given, the line
// saying how many more there are among them, scrolled to show the one
// chosen, as revealing has it, from as far as ListScroll has it; its chosen
// row lit where lit says.
func projectsListOf(f Frame, rows int, lit bool, now time.Time) projectsList {
	s := f.Projects
	l := projectsList{rows: s.rows(), chosen: s.chosenAt(), lit: lit, width: f.Width, look: f.Look, now: now}
	if s.Page != nil {
		l.all = s.Page.All
	}
	l.columns = l.columnsOf(f.Width)
	scrolling := projectsScrolling(len(l.rows), rows)
	l.shown, l.first = scrolling.Page, scrolling.revealing(s.ListScroll, l.chosen)
	return l
}

// listed are the list's rows, in its order, each as a listedProject.
func (l projectsList) listed() []listedProject {
	listed := make([]listedProject, len(l.rows))
	for i := range l.rows {
		listed[i] = listedProject{named: projectNamed(l.rows[i]), totals: l.rows[i].ProjectTotals, row: &l.rows[i]}
	}
	return listed
}

// every is every project's row, named all projects, in bold text.secondary.
func (l projectsList) every() listedProject {
	return listedProject{named: line{{allProjects, strongInk}}, totals: l.all}
}

// projectsScrolling is how far a list of n projects scrolls in the rows
// given, by rows: as many show as fit, a row left for how many more there
// are where they don't all fit.
func projectsScrolling(n, rows int) Scrolling {
	page := min(rows, n)
	if page < n {
		page = max(page-1, 1)
	}
	return Scrolling{Most: n - page, Page: page}
}

// columnsOf are the list's columns across a frame width cells wide, from
// listFrom, listGap apart, as flex lays them: the name at its widest whole,
// then live, sessions and worth at their widest, each of its rows' and its
// head's; the bar, from projectBarLeast, taking what's spare; then requests
// and last at their widest. As it narrows, the bar
// narrows to its least, then requests and last leave, and the name is cut
// toward projectNameLeast, in the turns above. No natural comes from the
// rows shown, so the columns hold still as the list scrolls.
func (l projectsList) columnsOf(width int) []projectColumn {
	listed := append(l.listed(), l.every())
	widest := func(p projectsPart) int {
		cells := ansi.StringWidth(projectHeads[p])
		for _, lp := range listed {
			cells = max(cells, l.says(p, lp).width())
		}
		return cells
	}
	kinds := []projectsPart{listedName, listedLive, listedSessions, listedWorth, listedBar, listedRequests, listedLast}
	parts := make([]flex.Part, len(kinds))
	for i, p := range kinds {
		w := widest(p)
		parts[i] = flex.Part{Least: w, Natural: w, Gap: listGap}
	}
	parts[listedName] = flex.Part{Least: projectNameLeast, Natural: widest(listedName), Narrows: nameNarrows}
	parts[listedBar] = flex.Part{Least: projectBarLeast, Grows: true, Gap: listGap}
	parts[listedRequests].Leaves, parts[listedLast].Leaves = requestsLeave, lastLeaves
	spans, _ := flex.Lay(width-listFrom, parts...)
	return spannedColumns(spans, kinds, func(p projectsPart, x, width int) projectColumn { return projectColumn{part: p, x: x, width: width} })
}

// end is the column after the list's last.
func (l projectsList) end() int {
	last := l.columns[len(l.columns)-1]
	return last.x + last.width
}

// rowsTaken are the rows the list takes: its heads, its projects shown, the
// line saying how many more there are, where any are out of view, its rule
// and every project's row.
func (l projectsList) rowsTaken() int {
	return 1 + l.linesShown() + 2
}

// linesShown are the rows its projects and the line of how many more take.
func (l projectsList) linesShown() int {
	if l.shown < len(l.rows) {
		return l.shown + 1
	}
	return l.shown
}

// draw draws the list from row y: its heads, in text.subtle, but over the
// bar; its projects shown, as row draws each; the line saying how many more
// there are above and below, as Detail's list says them; the rule across the
// list, in border.subtle; and every project's row. It returns the row after
// it. Of no project, it says there's none.
func (l projectsList) draw(c *canvas, y int) int {
	if len(l.rows) == 0 {
		c.line(listFrom, y, noProjects)
		return y + 1
	}
	for _, col := range l.columns {
		if head := projectHeads[col.part]; head != "" {
			l.cell(c, y, col, line{{head, dimInk}})
		}
	}
	listed := l.listed()
	for i := range l.shown {
		at := l.first + i
		l.row(c, y+1+i, listed[at], at == l.chosen)
	}
	y += 1 + l.shown
	if more := outOfView(l.first, len(l.rows)-l.first-l.shown, l.end()-listFrom, projectsOut...); more != nil {
		c.line(listFrom, y, more)
		y++
	}
	c.line(listFrom, y, line{{rule(l.end() - listFrom), borderInk}})
	every := l.every()
	for _, col := range l.columns {
		l.cell(c, y+1, col, l.says(col.part, every).fit(col.width))
	}
	return y + 2
}

// row draws a project's row along row y: each column's words, as says has
// them, cut to its cells; its bar, as bar draws it; and where it's chosen, ▸
// in bold accent.key at column 0, the row picked out as chosenRow picks it
// out, lit where ↑ and ↓ act in the list, else bold.
func (l projectsList) row(c *canvas, y int, lp listedProject, chosen bool) {
	for _, col := range l.columns {
		if col.part == listedBar {
			l.bar(c, y, col, *lp.row)
			continue
		}
		l.cell(c, y, col, l.says(col.part, lp).fit(col.width))
	}
	if chosen {
		c.line(0, y, line{{"▸", keyInk}})
		chosenRow(c, 0, y, l.width, l.lit)
	}
}

// bar draws the row's bar in its column along row y: as many eighths of its
// cells as its worth is of the top project's, the first row's, as the list
// ranks by worth, as barUnits has them, split by model family in the
// families' colours, in the version table's order of families, as BY MONTH
// splits a month's.
func (l projectsList) bar(c *canvas, y int, col projectColumn, r views.ProjectRow) {
	amounts, hues := familyParts(r.ByFamily)
	eighthsBar(c, l.look, col.x, y, barUnits(amountOf(r.Worth), amountOf(l.rows[0].Worth), col.width*8), amounts, hues)
}

// cell draws what a column says along row y, in its cells: its figures
// ending at its end, the rest from its start.
func (l projectsList) cell(c *canvas, y int, col projectColumn, said line) {
	switch col.part {
	case listedSessions, listedWorth, listedRequests:
		c.right(col.x+col.width, y, said)
	default:
		c.line(col.x, y, said)
	}
}

// says is what the column says of a row of the list: its name; ● in
// state.positive and the sessions running, in text.secondary, where any
// run; its sessions and requests, in text.secondary; its worth, as Rounded
// says it, bold text.primary; and a project's last request, as lastSays has
// it. Every project's row has no bar and no last.
func (l projectsList) says(p projectsPart, lp listedProject) line {
	t := lp.totals
	switch p {
	case listedName:
		return lp.named
	case listedLive:
		if t.Live == 0 {
			return nil
		}
		return line{{"●", positiveInk}, {" " + prose.Grouped(t.Live), secondaryInk}}
	case listedSessions:
		return line{{prose.Grouped(t.Sessions), secondaryInk}}
	case listedWorth:
		return line{{t.Rounded(), titleInk}}
	case listedRequests:
		return line{{prose.Grouped(t.Requests), secondaryInk}}
	case listedLast:
		if lp.row != nil {
			return l.lastSays(*lp.row)
		}
	}
	return nil
}

// projectNamed is the project's name, as the list and the head name it, in
// bold text.primary; the sessions with no directory's words in text.subtle.
func projectNamed(r views.ProjectRow) line {
	if r.NoDirectory() {
		return line{{noDirectory, dimInk}}
	}
	return line{{r.Name, titleInk}}
}

// lastSays is when the project's last request came, in text.muted: now
// while a session of it is busy, as its Activity's Busy has it; else as
// prose.Lately says it; nothing where none came.
func (l projectsList) lastSays(r views.ProjectRow) line {
	switch {
	case r.Activity.Busy(l.now):
		return line{{"now", mutedInk}}
	case r.Last.IsZero():
		return nil
	}
	return line{{prose.Lately(l.now, r.Last), mutedInk}}
}

// targets are the list's projects shown, from row y, a click on which
// chooses the project, each across the width given.
func (l projectsList) targets(y, width int) []Target {
	targets := make([]Target, 0, l.shown)
	for i := range l.shown {
		targets = append(targets, Target{Y: y + 1 + i, Width: width, On: OnProject, Project: l.rows[l.first+i].Path})
	}
	return slices.Clip(targets)
}
