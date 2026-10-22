package dashboard

import (
	"cmp"
	"slices"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/status"
)

// A column of accounts' names, as a label's column is laid out at every
// width: at least labelLeast cells, its natural the widest name of the
// accounts configured, each name counting labelMost cells at the most.
const (
	labelLeast = 8
	labelMost  = 24
)

// labelsPart is a column of names laid out by flex, its gap after it: its
// natural the widest name given and nameGap, held within labelLeast and
// labelMost, narrowing toward its least in the turn given, so its names
// read whole till then.
func labelsPart(widest, narrows int) flex.Part {
	return flex.Part{Least: labelLeast, Natural: widest + nameGap, Most: labelMost, Narrows: narrows}
}

// labelColumnCells are the cells an account's name takes in a column of
// names: its own, labelMost at the most.
func labelColumnCells(name string) int {
	return min(ansi.StringWidth(name), labelMost)
}

// labelsCells are the cells of the names of the accounts the moment m
// knows, as the dashboard names them, as labelColumnCells counts them, the
// widest first.
func labelsCells(m status.Moment) []int {
	cells := make([]int, len(m.Accounts))
	for i, a := range m.Accounts {
		cells[i] = labelColumnCells(name(a))
	}
	return slices.SortedFunc(slices.Values(cells), func(a, b int) int { return cmp.Compare(b, a) })
}

// widestLabel is a column of names' natural: the widest of the names of the
// accounts the moment m knows, as labelsCells has them; none where it knows
// none.
func widestLabel(m status.Moment) int {
	if cells := labelsCells(m); len(cells) > 0 {
		return cells[0]
	}
	return 0
}
