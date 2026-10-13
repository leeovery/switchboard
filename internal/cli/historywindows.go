package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// writeWindows writes each account's windows' readings as history --windows's
// text: a heading a window, its account's id and its name, as
// status.NameOfKey names a window, as "Week", then a line a reading, when it
// was read, its use, when the window resets, its status and where it came
// from, times in now's time zone; or says there were none since from.
func writeWindows(out io.Writer, windows views.Windows, from, now time.Time) error {
	if len(windows.Windows) == 0 {
		_, err := fmt.Fprintf(out, "no readings since %s\n", dayIn(now, from))
		return err
	}
	for i, w := range windows.Windows {
		heading := prose.Clean(w.Account) + "  " + status.NameOfKey(w.Window)
		if i > 0 {
			heading = "\n" + heading
		}
		if _, err := fmt.Fprintln(out, heading); err != nil {
			return err
		}
		rows := make([][]string, len(w.Readings))
		for j, r := range w.Readings {
			rows[j] = []string{"  " + prose.Reading(now, r.At), prose.Percent(r.Utilization),
				resetsText(r.ResetsAt, now), orNone(prose.Clean(string(r.Status))), prose.Clean(string(r.Source))}
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
	return "resets " + prose.Reading(now, at)
}
