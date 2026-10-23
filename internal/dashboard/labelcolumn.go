package dashboard

import (
	"cmp"
	"slices"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
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

// labelsPart is a column of names laid out by flex, its gap after it: its
// natural the widest name given and nameGap, held within labelLeast and
// labelMost, narrowing toward its least in the turn given, so its names
// read whole till then.
func labelsPart(widest, narrows int) flex.Part {
	return flex.Part{Least: labelLeast, Natural: widest + nameGap, Most: labelMost, Narrows: narrows}
}

// labelColumn is a column of names laid out by flex, with no gap of its
// own: its natural the widest name given, held within labelLeast and
// labelMost, narrowing freely toward its least.
func labelColumn(widest int) flex.Part {
	return flex.Part{Least: labelLeast, Natural: widest, Most: labelMost}
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
