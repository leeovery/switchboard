package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// writeTable writes rows as aligned columns, each line without the spaces an
// empty last cell would leave at its end.
func writeTable(w io.Writer, rows [][]string) error {
	var table strings.Builder
	tw := tabwriter.NewWriter(&table, 0, 0, 2, ' ', 0)
	for _, row := range rows {
		if _, err := fmt.Fprintln(tw, strings.Join(row, "\t")); err != nil {
			return err
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	for line := range strings.Lines(table.String()) {
		if _, err := fmt.Fprintln(w, strings.TrimRight(line, " \n")); err != nil {
			return err
		}
	}
	return nil
}
