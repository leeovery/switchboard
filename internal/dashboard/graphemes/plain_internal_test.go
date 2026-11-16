package graphemes

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// A box drawing character or block element is plain only where the cluster
// rules give it a cell, as ansi is set, East Asian or not.
func TestABoxCharacterIsPlainOnlyWhereItTakesACell(t *testing.T) {
	for r := boxFirst; r <= boxLast; r++ {
		_, width := ansi.FirstGraphemeCluster(string(r), ansi.GraphemeWidth)
		if got, want := plain(r), width == 1; got != want {
			t.Errorf("plain(%q) is %v, but the cluster rules give it %d cells", r, got, width)
		}
	}
}
