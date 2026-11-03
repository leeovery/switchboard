package views

import (
	"fmt"
	"testing"
)

// metOfSeeds runs check of each seed from 0 to seeds in a subtest of its
// own, named by the seed, in parallel with the others, each noting the hard
// moments its world meets in a map of its own, and returns, once every one
// has run, the moments any of them met.
func metOfSeeds(t *testing.T, seeds uint64, check func(t *testing.T, seed uint64, met map[string]bool)) map[string]bool {
	t.Helper()
	mets := make([]map[string]bool, seeds)
	t.Run("seeds", func(t *testing.T) {
		for seed := range seeds {
			mets[seed] = make(map[string]bool)
			t.Run(fmt.Sprintf("seed %d", seed), func(t *testing.T) {
				t.Parallel()
				check(t, seed, mets[seed])
			})
		}
	})
	met := make(map[string]bool)
	for _, m := range mets {
		for moment, ok := range m {
			met[moment] = met[moment] || ok
		}
	}
	return met
}
