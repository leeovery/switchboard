package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// readingLayout lays out when a reading was read, or a window resets, as
// "Tue 6 Oct 22:14".
const readingLayout = "Mon 2 Jan 15:04"

// writeWindows writes each account's windows' readings as history --windows's
// text: a heading a window, its account's id and its name, then a line a
// reading, when it was read, its use, when the window resets, its status and
// where it came from, times in now's time zone; or says there were none since
// from.
func writeWindows(out io.Writer, windows views.Windows, from, now time.Time) error {
	if len(windows.Windows) == 0 {
		_, err := fmt.Fprintf(out, "no readings since %s\n", from.In(now.Location()).Format(dayLayout))
		return err
	}
	for i, w := range windows.Windows {
		heading := status.Clean(w.Account) + "  " + status.Clean(claude.WindowLabel(w.Window))
		if i > 0 {
			heading = "\n" + heading
		}
		if _, err := fmt.Fprintln(out, heading); err != nil {
			return err
		}
		rows := make([][]string, len(w.Readings))
		for j, r := range w.Readings {
			rows[j] = []string{"  " + r.At.In(now.Location()).Format(readingLayout), status.Percent(r.Utilization),
				resetsText(r.ResetsAt, now), orNone(status.Clean(string(r.Status))), status.Clean(string(r.Source))}
		}
		if err := writeTable(out, rows); err != nil {
			return err
		}
	}
	return nil
}

// resetsText says when a window resets, as "resets Tue 6 Oct 23:10", in
// now's time zone: a dash where the reading didn't say.
func resetsText(at, now time.Time) string {
	if at.IsZero() {
		return dash
	}
	return "resets " + at.In(now.Location()).Format(readingLayout)
}
