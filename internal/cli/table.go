package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// writeTable writes rows as aligned columns.
func writeTable(w io.Writer, rows [][]string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, row := range rows {
		if _, err := fmt.Fprintln(tw, strings.Join(row, "\t")); err != nil {
			return err
		}
	}
	return tw.Flush()
}
