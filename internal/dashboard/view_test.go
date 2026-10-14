package dashboard

import (
	"slices"
	"testing"
)

func TestTheSixTabsInTheirOrder(t *testing.T) {
	var titles []string
	for _, v := range Views() {
		titles = append(titles, v.Title())
	}
	if want := []string{"Overview", "Accounts", "History", "Sessions", "Runway", "Log"}; !slices.Equal(titles, want) {
		t.Errorf("the tabs are %q, want %q", titles, want)
	}
}
