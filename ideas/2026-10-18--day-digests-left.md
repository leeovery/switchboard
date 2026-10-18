# Day digests, phase 1: what it left

Phase 1 of the day digests has the router digest each finished day and the store of sessions rest
each session on its newest standing state (see The request ledger and Live updates in the design).
Its soak, a synthetic world of 25,000 requests a day with its long session every day, race detector
off, opens the List in about 0.3 s with 9 or 30 days kept, from 1.4 s and 4.0 s. What it left, each
a cost, not a wrong answer, gathered here.

## The build's peak

A day's build lifts the router's heap some 95 MB for under a second, once a day, and as long again
for each day of a backfill. About 47 MB of it is the day's lines, held once, which every session's
fold needs; the rest is garbage the build makes before a collection, the heap left to grow to twice
what's live. Where it comes from, by the build's heap profile:

- The head's count of arrivals decodes every line of the day whole to read its `at`, some 50 MB a
  build. Counting them in the build's own read of the day would save the second pass; decoding `at`
  alone would change which lines count as lines, so it must agree with how the summary counts them.
- A line decoded is allocated twice, as a value and then kept by pointer, some 18 MB.
- A session held from its state prices a copy of its requests where they're already its own, some
  8 MB; the open requests' pricing needs the copy, the rest don't.
- Further down: a build that folds each session as its lines arrive and lets them go, holding no
  day's lines whole; or a memory limit set in the router (`GOMEMLIMIT`).

## A session's state grows with its subagents

A session's state keeps, for each of its threads, when its last request ended and how long its writes
to the cache last, so a long session that runs subagents every day keeps every one: some 4.7 MB raw,
480 KB gzipped, after 30 busy days, and each day's build restores it whole. Trimming those long past
their cache's life would bound it, but a thread missing reads as one whose cache is warm, so the rule
needs care.

## Smaller

- At half past one, a session resting on yesterday's state whose request in flight the stream files
  on yesterday has the store read yesterday's files again for it once, then hold it whole, as its
  state holds none of yesterday's requests: a read the store reading raw doesn't make.
- The rule that a digest stands only where the day before's does is held by the ledger's own tests
  alone: the views' guard can't fail on it, as a line filed onto a day whose digest is gone already
  stops every state resting on that day standing. Dropping it would read fewer days raw after a gap.
- The soak's guard that the List reads no day behind a session's state sees a day only where it
  handed on a line of a session given: one read with no such line passes unseen.
- `Holding`'s count of sessions read whole counts one restored from its state too.
