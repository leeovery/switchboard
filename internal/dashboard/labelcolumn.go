package dashboard

import (
	"github.com/charmbracelet/x/ansi"
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

// widestLabel is a column of names' natural: the widest of the names of the
// accounts the moment m knows, as the dashboard names them, as
// labelColumnCells counts them; none where it knows none.
func widestLabel(m status.Moment) int {
	widest := 0
	for _, a := range m.Accounts {
		widest = max(widest, labelColumnCells(name(a)))
	}
	return widest
}
