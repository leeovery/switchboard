package cli

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/leeovery/switchboard/internal/dashboard/watch"
)

// What the dashboard's help says of a page's keys is said of a page it
// names, so a page renamed never leaves its sentence unsaid.
func TestEveryPageKeysSaidOfIsOneTheHelpNames(t *testing.T) {
	var pages []string
	for _, p := range watch.EveryPagesKeys() {
		pages = append(pages, p.Page)
	}
	for page := range keysSaid {
		if !slices.Contains(pages, page) {
			t.Errorf("keys are said of %q, which isn't among the pages the help names, %q", page, pages)
		}
	}
}

// The help's keys, and what's said of them, are wrapped to keysColumns.
func TestTheHelpsKeysFitTheirColumns(t *testing.T) {
	for l := range strings.SplitSeq(dashboardKeys(), "\n") {
		if n := utf8.RuneCountInString(l); n > keysColumns {
			t.Errorf("%q is %d columns, want %d at most", l, n, keysColumns)
		}
	}
}
