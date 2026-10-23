package dashboard

import (
	"cmp"
	"slices"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// A column of accounts' names, as every view lays one out: its natural the
// widest name of the accounts configured, each counting labelMost cells at
// the most, cut with an ellipsis past it; its least labelLeast cells, or its
// natural where that's fewer. No view counts either itself: each takes its
// column from labelColumn, namesColumn or accountsColumn, its least alone
// from labelsLeast, or a sentence's cut from labelCut.
const (
	labelLeast = 8
	labelMost  = 24
)

// labelsLeast is the least of a column of names of the natural given:
// labelLeast, or its natural where that's fewer, so a column of short names
// narrows only as far as them.
func labelsLeast(natural int) int {
	return min(labelLeast, natural)
}

// labelColumn is a column laid out by flex whose bounds count its natural,
// given, whole, any marks and gap among it: labelsLeast of it at the least,
// labelMost at the most.
func labelColumn(natural int) flex.Part {
	return flex.Part{Least: labelsLeast(natural), Natural: natural, Most: labelMost}
}

// columnName is a name as a column of names holds it: its label's cells,
// and the cells kept beside it, its number and marks.
type columnName struct {
	label, beside int
}

// namesColumn is a column of the names given laid out by flex, gap cells
// after it, each name's cells beside it kept at every width and its label
// counted alone: at the least, labelsLeast of it; at the natural, as
// labelColumnCells counts it; and labelMost at the most.
func namesColumn(gap int, names ...columnName) flex.Part {
	p := flex.Part{Least: gap, Natural: gap, Most: gap}
	for _, n := range names {
		p.Least = max(p.Least, gap+n.beside+labelsLeast(n.label))
		p.Natural = max(p.Natural, gap+n.beside+min(n.label, labelMost))
		p.Most = max(p.Most, gap+n.beside+labelMost)
	}
	return p
}

// accountsColumn is the name column of the accounts' names given, gap cells
// after it, as namesColumn lays it out, each name's number and marks beside
// its label, as accountName's cells counts them.
func accountsColumn(gap int, names []accountName) flex.Part {
	column := make([]columnName, len(names))
	for i, n := range names {
		column[i] = columnName{label: ansi.StringWidth(n.label), beside: n.cells(0)}
	}
	return namesColumn(gap, column...)
}

// headedNames is a column of names under a head, its natural given, as the
// widest name or a move's two: labelsLeast of it at the least; each no
// fewer than the head's cells, so a column of short names never cuts its
// head.
func headedNames(natural int, head string) flex.Part {
	cells := ansi.StringWidth(head)
	return flex.Part{Least: max(labelsLeast(natural), cells), Natural: max(natural, cells)}
}

// labelCut is the most cells each name a sentence says is cut to, as fits
// reports a cut fits: labelMost where it fits so, a cell fewer at a time,
// and labelLeast at the least, which leaves a shorter name whole.
func labelCut(fits func(cut int) bool) int {
	for cut := labelMost; cut > labelLeast; cut-- {
		if fits(cut) {
			return cut
		}
	}
	return labelLeast
}

// widestNamed is a column of names' natural: the widest of the names given,
// as labelColumnCells counts each; none of none.
func widestNamed(names []string) int {
	widest := 0
	for _, n := range names {
		widest = max(widest, labelColumnCells(n))
	}
	return widest
}

// accountNames are the names of the accounts given, as the dashboard names
// them.
func accountNames(accounts []status.AccountNow) []string {
	names := make([]string, len(accounts))
	for i, a := range accounts {
		names[i] = name(a)
	}
	return names
}

// labelColumnCells are the cells an account's name takes in a column of
// names: its own, labelMost at the most.
func labelColumnCells(name string) int {
	return min(ansi.StringWidth(name), labelMost)
}

// asLabelled is an account's name as a column of names counts it, where
// it's said outside one: cut to labelColumnCells.
func asLabelled(name string) string {
	return prose.Cut(name, labelColumnCells(name))
}

// labelsCells are the cells of the names of the accounts the moment m
// knows, as the dashboard names them, as labelColumnCells counts them, the
// widest first.
func labelsCells(m status.Moment) []int {
	names := accountNames(m.Accounts)
	cells := make([]int, len(names))
	for i, n := range names {
		cells[i] = labelColumnCells(n)
	}
	return slices.SortedFunc(slices.Values(cells), func(a, b int) int { return cmp.Compare(b, a) })
}

// widestLabel is a column of names' natural: the widest of the names of the
// accounts the moment m knows, as widestNamed has it; none where it knows
// none.
func widestLabel(m status.Moment) int {
	return widestNamed(accountNames(m.Accounts))
}
