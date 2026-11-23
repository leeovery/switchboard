package ledger

import (
	"testing"
	"time"
)

// A day summarised again keeps a reset from the higher use before it, under
// the reserve of the summary that read that use, the higher of the two where
// both read it, as where the readings that told one have been pruned since.
func TestAResetSummarisedAgainKeepsTheReserveOfItsHigherUse(t *testing.T) {
	at := time.Date(2026, 10, 5, 11, 0, 0, 0, time.UTC)
	tests := []struct {
		name                     string
		afreshBefore, heldBefore float64
		afresh, held             *float64
		want                     *float64
	}{
		{name: "the summary it replaces read higher", afreshBefore: 0.8, heldBefore: 0.85, afresh: new(0.3), held: new(0.1), want: new(0.1)},
		{name: "summarised afresh higher", afreshBefore: 0.85, heldBefore: 0.8, afresh: new(0.1), held: new(0.3), want: new(0.1)},
		{name: "both alike, the higher reserve", afreshBefore: 0.85, heldBefore: 0.85, afresh: new(0.1), held: new(0.3), want: new(0.3)},
		{name: "both alike, the one that recorded one", afreshBefore: 0.85, heldBefore: 0.85, held: new(0.2), want: new(0.2)},
		{name: "recorded by neither", afreshBefore: 0.85, heldBefore: 0.85},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resetsKnowing([]Reset{{Window: "7d", At: at, Before: tt.afreshBefore, Reserve: tt.afresh}}, []Reset{{Window: "7d", At: at, Before: tt.heldBefore, Reserve: tt.held}})
			if len(got) != 1 || got[0].Before != 0.85 || (got[0].Reserve == nil) != (tt.want == nil) || tt.want != nil && *got[0].Reserve != *tt.want {
				t.Errorf("the resets are %+v, want one from 0.85 under %v", got, tt.want)
			}
		})
	}
}
