package ledger

import (
	"testing"
	"time"
)

// A day summarised again keeps the reserve a reset recorded: the one
// summarised afresh, else the one the summary it replaces kept, as where the
// readings that told it have been pruned since.
func TestAResetSummarisedAgainKeepsItsReserve(t *testing.T) {
	at := time.Date(2026, 10, 5, 11, 0, 0, 0, time.UTC)
	tests := []struct {
		name         string
		afresh, held *float64
		want         *float64
	}{
		{name: "summarised afresh", afresh: new(0.1), held: new(0.2), want: new(0.1)},
		{name: "kept before alone", held: new(0.2), want: new(0.2)},
		{name: "recorded by neither"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resetsKnowing([]Reset{{Window: "7d", At: at, Before: 0.8, Reserve: tt.afresh}}, []Reset{{Window: "7d", At: at, Before: 0.85, Reserve: tt.held}})
			if len(got) != 1 || got[0].Before != 0.85 || (got[0].Reserve == nil) != (tt.want == nil) || tt.want != nil && *got[0].Reserve != *tt.want {
				t.Errorf("the resets are %+v, want one from 0.85 under %v", got, tt.want)
			}
		})
	}
}
