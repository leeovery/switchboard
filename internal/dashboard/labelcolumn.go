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
// widest name of the accounts configured, each label counting labelMost
// cells at the most, cut with an ellipsis past it; its least labelLeast
// cells, or its natural where that's fewer; a name's number and marks, and
// the column's gap, beside its label, never among it. Each view takes its
// column here, from labelColumn, namesColumn, accountsColumn,
// recordedNames or recordedAccounts, its least alone from labelsLeast, or
// a sentence's cut from labelCut. Compare's 8 and 24, Detail's list's 14
// and 24, the Sessions List's on's 14 and the Switchboard's call's 6 are
// those views' own figures, as the design gives them.
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

// labelColumn is a column of names, bare, the widest given, as namesColumn
// lays it out.
func labelColumn(widest int) flex.Part {
	return namesColumn(0, columnName{label: widest})
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

// recordedNatural is the natural of a column of names a record may hold,
// of the natural given, as the widest configured name or a move's two:
// never fewer than labelLeast, as a recorded line may name an account no
// longer configured, which then reads its first cells and an ellipsis.
func recordedNatural(natural int) int {
	return max(natural, labelLeast)
}

// recordedNames is a column of names a record may hold, its natural given:
// recordedNatural of it, and labelsLeast of that at the least.
func recordedNames(natural int) flex.Part {
	natural = recordedNatural(natural)
	return flex.Part{Least: labelsLeast(natural), Natural: natural}
}

// recordedAccounts is the name column of the accounts' names given, as
// accountsColumn lays it out, where a record may name an account no longer
// configured: each label counted at recordedNatural of its cells, so the
// column keeps labelLeast cells of a label beside the widest marks.
func recordedAccounts(gap int, names []accountName) flex.Part {
	column := make([]columnName, len(names))
	for i, n := range names {
		column[i] = columnName{label: recordedNatural(ansi.StringWidth(n.label)), beside: n.cells(0)}
	}
	return namesColumn(gap, column...)
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
