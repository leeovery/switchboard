# A faster gate before a release's tag

`mint release` runs four gates before it tags, as `.mint.toml`'s `pre_tag` says: `go build`, `go
vet`, `scripts/test-isolated` and `golangci-lint`, in order, as no CI runs the tests before a tag.
Releasing v0.1.2 spent 73.9 seconds there, nearly all of it the isolated tests, run whole with the
race detector. Yet every pull request merged into main has already passed all four on its own tree,
so a release from main checks again what was just checked.

Ways to take the wait out, keeping the guarantee:

- **Pass once a tree.** When every gate passes, record the tree's hash where the gates can read it;
  `pre_tag` runs them only where the tagged tree has no such record. A release from a gated main
  then takes seconds, and one from anything else is still checked in full.
- **The gates side by side.** `build`, `vet` and `lint` don't need the tests to finish first: one
  script running them alongside the tests, failing if any fails.
- **A faster suite.** Profile it for the slowest packages and tests: testguard's own, which read the
  module's source, take some 13 seconds; long waits in tests that could run on a fake clock.

To settle with the owner: which of these, and where the record of a gated tree lives (beside the
checkout's state, never in the repository).

Later, once the redesign has shipped.
