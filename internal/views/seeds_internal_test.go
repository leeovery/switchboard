package views

import (
	"fmt"
	"testing"
)

// metOfSeeds is metOfEachSeed of each seed from 0 to seeds.
func metOfSeeds(t *testing.T, seeds uint64, check func(t *testing.T, seed uint64, met map[string]bool)) map[string]bool {
	t.Helper()
	each := make([]uint64, seeds)
	for i := range each {
		each[i] = uint64(i)
	}
	return metOfEachSeed(t, each, check)
}

// metOfEachSeed runs check of each seed given in a subtest of its own,
// named by the seed, in parallel with the others, each noting the hard
// moments its world meets in a map of its own, and returns, once every one
// has run, the moments any of them met.
func metOfEachSeed(t *testing.T, seeds []uint64, check func(t *testing.T, seed uint64, met map[string]bool)) map[string]bool {
	t.Helper()
	mets := make([]map[string]bool, len(seeds))
	t.Run("seeds", func(t *testing.T) {
		for i, seed := range seeds {
			mets[i] = make(map[string]bool)
			t.Run(fmt.Sprintf("seed %d", seed), func(t *testing.T) {
				t.Parallel()
				check(t, seed, mets[i])
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
