package cli

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"unicode/utf8"
)

// columnGap is the blanks after each column but the last.
const columnGap = 2

// writeTable writes rows as aligned columns, each line ending at its last
// cell that says anything: a row's cells left empty at its end pad nothing.
func writeTable(w io.Writer, rows [][]string) error {
	var table bytes.Buffer
	tw := tabwriter.NewWriter(&table, 0, 0, columnGap, ' ', 0)
	for _, row := range rows {
		if _, err := fmt.Fprintln(tw, strings.Join(row, "\t")); err != nil {
			return err
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	var trimmed strings.Builder
	for line := range strings.Lines(table.String()) {
		trimmed.WriteString(strings.TrimRight(line, " \n") + "\n")
	}
	_, err := io.WriteString(w, trimmed.String())
	return err
}

// widths returns how wide each column of rows is, as writeTable aligns them,
// each at least as wide as at gives it: the widest of its cells, but the
// last's, which nothing follows.
func widths(at []int, rows ...[]string) []int {
	w := append([]int(nil), at...)
	for _, row := range rows {
		for i, cell := range row[:max(len(row)-1, 0)] {
			if i == len(w) {
				w = append(w, 0)
			}
			w[i] = max(w[i], utf8.RuneCountInString(cell))
		}
	}
	return w
}

// writeRow writes row, its columns as wide as widths says, as writeTable
// writes a row of a table whose columns are that wide.
func writeRow(w io.Writer, row []string, widths []int) error {
	var line strings.Builder
	for i, cell := range row {
		line.WriteString(cell)
		if i < len(row)-1 {
			line.WriteString(strings.Repeat(" ", widths[i]-utf8.RuneCountInString(cell)+columnGap))
		}
	}
	_, err := fmt.Fprintln(w, strings.TrimRight(line.String(), " "))
	return err
}
