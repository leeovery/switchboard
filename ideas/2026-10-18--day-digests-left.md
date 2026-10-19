# Day digests, phases 1 and 2: what they left

Phase 1 of the day digests has the router digest each finished day and the store of sessions rest
each session on its newest standing state (see The request ledger and Live updates in the design).
Its soak, a synthetic world of 25,000 requests a day with its long session every day, race detector
off, opens the List in about 0.3 s with 9 or 30 days kept, from 1.4 s and 4.0 s. Phase 2 has each
digest carry each session's page, and a parts file a day hold what grows, so a page rests on its
newest state too: the long session's page opens in 0.31 s and 0.38 s, from 1.71 s and 5.63 s. What
they left, each a cost, not a wrong answer, gathered here.

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

Phase 2 raised it, with 30 days kept, to some 152 MB and 0.95 s, as each session's points take in
every account's answers of the day.

## A long page read whole in the router

Where a page's restore is refused, its session is read whole: in the router's build, a chain of
parts that doesn't read, or a model first reading, mid-run, a window only some of the session's
models share. For the soak's long session that's 5.9 s and some 1.79 GB more heap with 30 days kept,
and some 0.6 GB with 9. Most of it is the ledger's read holding the session's lines, some 570 MB:
the lines kept while the days are read ahead, and a sorted copy of them, some 223 MB. Then the
whole fold, some 270 MB, and the answers, some 145 MB. Ways out:

- Fold the read day by day, never holding every day's lines at once.
- Hold only the answers of the days of the session's last run: the runs before it fold at once.
- Drop the sorted copy.

## A session's state grows with its subagents

A session's state keeps, for each of its threads, when its last request ended and how long its writes
to the cache last, so a long session that runs subagents every day keeps every one: some 4.7 MB raw,
480 KB gzipped, after 30 busy days, and each day's build restores it whole. Trimming those long past
their cache's life would bound it, but a thread missing reads as one whose cache is warm, so the rule
needs care.

## Smaller

- A restored page's times sent upstream are decoded through a list of times, some 230 B each, before
  they're held as nanoseconds: some 919 MB of garbage for a session running every day of 400.
  Decoding them straight into nanoseconds would make none.
- A page whose parts are spoiled, and whose next older state to fall back on lies three days back
  or more, is read whole: the points can't place a session's own day before yesterday, after its
  base.
- The backfill's walk of each session's chain of parts reads each link's part again for every day
  it builds, so it grows with the square of the days. Measure it at 400 days.
- The stored forms' `nn`, `pn` and `mp`, whether a request in none is in flight and whether a turn's
  requests are pending, are always false in a digest, as only the stream sets them. Drop them at the
  next version.
- At half past one, a session resting on yesterday's state whose request in flight the stream files
  on yesterday has the store read yesterday's files again for it once, then hold it whole, as its
  state holds none of yesterday's requests: a read the store reading raw doesn't make.
- The rule that a digest stands only where the day before's does is held by the ledger's own tests
  alone: the views' guard can't fail on it, as a line filed onto a day whose digest is gone already
  stops every state resting on that day standing. Dropping it would read fewer days raw after a gap.
- The soak's guard that the List reads no day behind a session's state sees a day only where it
  handed on a line of a session given: one read with no such line passes unseen.
- `Holding`'s count of sessions read whole counts one restored from its state too.
- How many lines the ledger keeps of a day, by its look, is worked out twice, by the reader's look
  and the writer's, each from a count of its own; a test holds them alike. One helper could own it.
