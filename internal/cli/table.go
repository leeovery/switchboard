package cli

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// writeTable writes rows as aligned columns, each line ending at its last
// cell that says anything: a row's cells left empty at its end pad nothing.
func writeTable(w io.Writer, rows [][]string) error {
	var table bytes.Buffer
	tw := tabwriter.NewWriter(&table, 0, 0, 2, ' ', 0)
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
