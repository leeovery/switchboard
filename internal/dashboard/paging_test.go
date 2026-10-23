package dashboard

import (
	"fmt"
	"slices"
	"testing"
)

// windowsPaged is a view laid out as USAGE stacks its windows, rows tall a
// page: each window's heading, a blank row under it where its accounts are
// two rows apart, its accounts that far apart, then under the last, the
// pool's rule and the pool; a blank row between windows; then a blank row
// and a key line of two rows; each window's heading kept over its rows, and
// the heading and the rule each shown with the row under it.
func windowsPaged(accounts, windows, apart, rows int) rowsPaged {
	p := rowsPaged{rows: rows, says: true}
	y := 0
	for w := range windows {
		if w > 0 {
			y++
		}
		heading, first := y, y+apart
		rule := first + (accounts-1)*apart + 1
		p.heads = append(p.heads, headed{at: heading, first: first}, headed{at: rule, first: rule + 1})
		p.kept = append(p.kept, keptHeading{at: heading, end: rule + 2})
		y = rule + 2
	}
	p.content = y + 3
	return p
}

// sectionsPaged is a view laid out as History's are, rows tall a page:
// sections, each a heading, a blank row under it, and lines of its own, none
// of its headings kept, each shown with its first line.
func sectionsPaged(sections, lines, rows int) rowsPaged {
	p := rowsPaged{rows: rows, says: true}
	y := 0
	for range sections {
		p.heads = append(p.heads, headed{at: y, first: y + 2})
		y += 2 + lines + 1
	}
	p.content = y
	return p
}

// pageSeen is what a page shows: the heading kept on its first row, -1
// where it keeps none, and the content's rows under it.
type pageSeen struct {
	kept int
	rows []int
}

// pageShows is what the page scrolled offset down shows.
func pageShows(p rowsPaged, offset int) pageSeen {
	from, to := p.shown(offset)
	seen := pageSeen{kept: -1}
	if at, ok := p.keeps(from, to); ok {
		seen.kept = at
	}
	for r := p.top(from, to); r < to; r++ {
		seen.rows = append(seen.rows, r)
	}
	return seen
}

// pagedThrough pages p from the offset given a page at a time, down or up,
// as PgDn and PgUp step it, till it goes no further, and returns what each
// page shows, the first's first.
func pagedThrough(p rowsPaged, offset, pages int) []pageSeen {
	seen := []pageSeen{pageShows(p, offset)}
	for range p.content + 1 {
		next := p.scrolling(offset).Paged(offset, pages)
		if next == offset {
			break
		}
		offset = next
		seen = append(seen, pageShows(p, offset))
	}
	return seen
}

// Paged a page at a time, from the top down and from the foot up, every row
// a view laid out whole holds shows, at every page from two rows: a page
// down starts at the first row the page before didn't show, and a page up
// ends at the row above the first it showed, a row a heading kept covers
// counting as not shown. No page but the last, which goes no further than
// the view does, shows again more of the page before than the row a page
// steps back so as not to cover, and the rows between a heading and its
// first a page cut short over it leaves.
func TestPagingDownAndUpShowsEveryRowAndRepeatsNoneItNeedNot(t *testing.T) {
	var views []rowsPaged
	for rows := 3; rows <= 21; rows++ {
		for accounts := 1; accounts <= 10; accounts++ {
			for windows := 1; windows <= 3; windows++ {
				for _, apart := range []int{1, 2} {
					views = append(views, windowsPaged(accounts, windows, apart, rows))
				}
			}
		}
		for sections := 1; sections <= 4; sections++ {
			for _, lines := range []int{1, 3, 7, 12} {
				views = append(views, sectionsPaged(sections, lines, rows))
			}
		}
	}
	for _, p := range views {
		if p.content <= p.page() {
			continue
		}
		name := fmt.Sprintf("%d rows of %d heads, %d kept, in %d", p.content, len(p.heads), len(p.kept), p.rows)
		most := p.scrolling(0).Most
		for _, way := range []struct {
			name          string
			offset, pages int
		}{{"down", 0, 1}, {"up", most, -1}} {
			pages := pagedThrough(p, way.offset, way.pages)
			reached := make([]bool, p.content)
			for i, page := range pages {
				for _, r := range page.rows {
					reached[r] = true
				}
				if page.kept >= 0 {
					reached[page.kept] = true
				}
				if i == 0 || i == len(pages)-1 {
					continue
				}
				if again := repeated(pages[i-1].rows, page.rows); again > repeatsAtMost(p) {
					t.Errorf("%s, a page %s shows again %d rows of the page before: %v then %v", name, way.name, again, pages[i-1], page)
				}
			}
			for r, ok := range reached {
				if !ok {
					t.Errorf("%s, paged %s, row %d never shows: %v", name, way.name, r, pages)
					break
				}
			}
		}
	}
}

// repeated is how many of the rows after shows before showed too.
func repeated(before, after []int) int {
	n := 0
	for _, r := range after {
		if slices.Contains(before, r) {
			n++
		}
	}
	return n
}

// repeatsAtMost is how many rows of the page before a page of p may show
// again: the row a page down steps back so as not to cover, and the rows
// between a heading and its first that a page cut short over it leaves.
func repeatsAtMost(p rowsPaged) int {
	most := 1
	for _, h := range p.heads {
		most = max(most, 1+h.first-h.at-1)
	}
	return most
}

// fittingFrom is how many of things, each as many rows tall as it says,
// show whole from the at-th in rows rows, a row less where any are above,
// at least one.
func fittingFrom(things []int, rows, at int) int {
	if at > 0 {
		rows--
	}
	n, used := 1, things[at]
	for _, h := range things[at+1:] {
		if used += h; used > rows {
			break
		}
		n++
	}
	return n
}

// Things shown whole, a thing at a time, as the Switchboard's accounts and
// Tokens' periods are, paged from the first down and from the last up,
// every thing shows, however tall each is: a page down starts at the first
// the page didn't show, and a page up ends at the one above the first it
// showed.
func TestPagingWholeThingsDownAndUpShowsEveryThing(t *testing.T) {
	for _, things := range [][]int{{2, 5, 3, 2, 6, 4, 2}, {1, 1, 1, 1, 1, 1}, {6, 1, 1, 1, 6, 2, 2, 5}, {3, 7, 2, 2, 2, 7, 3}} {
		for rows := 7; rows <= 16; rows++ {
			shown := func(at int) int { return fittingFrom(things, rows, at) }
			most := 0
			for most < len(things)-1 && shown(most) < len(things)-most {
				most++
			}
			for _, way := range []struct {
				name          string
				offset, pages int
			}{{"down", 0, 1}, {"up", most, -1}} {
				reached := make([]bool, len(things))
				for at, n := way.offset, 0; n <= len(things); n++ {
					for i := range shown(at) {
						reached[at+i] = true
					}
					next := thingsPaged(most, at, shown).Paged(at, way.pages)
					if next == at {
						break
					}
					at = next
				}
				if i := slices.Index(reached, false); i >= 0 {
					t.Errorf("things %v in %d rows, paged %s, thing %d never shows", things, rows, way.name, i)
				}
			}
		}
	}
}

// Things shown whole that page a thing at a time where each fits the view,
// and a row at a time where one doesn't, show as many rows from the first,
// at every height, as a row shorter, however tall each is: a row at a time
// goes on up past where each fits while a thing at a time would show fewer.
func TestPagingWholeThingsNeverShowsFewerRowsAsTheViewGrows(t *testing.T) {
	for _, things := range [][]int{{1, 2, 4}, {2, 5, 3, 2, 6, 4, 2}, {1, 1, 1, 1, 1, 1}, {6, 1, 1, 1, 6, 2, 2, 5}, {3, 7, 2, 2, 2, 7, 3}} {
		content := 0
		for _, h := range things {
			content += h
		}
		fits := func(rows int) bool {
			for i, h := range things {
				if h+min(i, 1) > rows {
					return false
				}
			}
			return true
		}
		whole := func(rows int) int {
			n := fittingFrom(things, rows, 0)
			taken := 0
			for _, h := range things[:n] {
				taken += h
			}
			return taken
		}
		paged := func(rows int) rowsPaged { return rowsPaged{rows: rows, content: content} }
		was := 0
		for rows := 1; rows <= content; rows++ {
			shown := whole(rows)
			byRows := pagesByRows(rows, fits, whole, paged)
			if byRows {
				_, shown = paged(rows).shown(0)
			}
			if !fits(rows) && !byRows {
				t.Errorf("things %v in %d rows, one not fitting, page a thing at a time", things, rows)
			}
			if shown < was {
				t.Errorf("things %v in %d rows show %d rows, fewer than the %d a row shorter", things, rows, shown, was)
			}
			was = shown
		}
	}
}
