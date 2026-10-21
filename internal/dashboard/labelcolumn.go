package dashboard

import "github.com/charmbracelet/x/ansi"

// A column of accounts' names, as a label's column is laid out at every
// width: at least labelLeast cells, its natural the widest name it shows,
// each name counting labelMost cells at the most.
const (
	labelLeast = 8
	labelMost  = 24
)

// labelColumnCells are the cells an account's name takes in a column of names:
// its own, labelMost at the most.
func labelColumnCells(name string) int {
	return min(ansi.StringWidth(name), labelMost)
}
