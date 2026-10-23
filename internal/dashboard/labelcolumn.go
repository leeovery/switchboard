package dashboard

import (
	"cmp"
	"slices"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// A column of accounts' names, as a label's column is laid out at every
// width: at least labelLeast cells, its natural the widest name of the
// accounts configured, each name counting labelMost cells at the most.
const (
	labelLeast = 8
	labelMost  = 24
)

// labelColumnCells are the cells an account's name takes in a column of
// names: its own, labelMost at the most.
func labelColumnCells(name string) int {
	return min(ansi.StringWidth(name), labelMost)
}

// labelAtMost is an account's label as a label's column holds it at its
// most: cut with an ellipsis past labelColumnCells, so one account reads one
// way wherever it's named.
func labelAtMost(label string) string {
	return prose.Cut(label, labelColumnCells(label))
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
