# Day digests, phases 1 to 3: what they left

Phase 1 of the day digests has the router digest each finished day and the store of sessions rest
each session on its newest standing state (see The request ledger and Live updates in the design).
Its soak, a synthetic world of 25,000 requests a day with its long session every day, race detector
off, opens the List in about 0.3 s with 9 or 30 days kept, from 1.4 s and 4.0 s. Phase 2 has each
digest carry each session's page, and a parts file a day hold what grows, so a page rests on its
newest state too: the long session's page opens in 0.31 s and 0.38 s, from 1.71 s and 5.63 s. Phase
3a has a turn read only its own days and the horizon's, and the router write a head a day holding
the day's sessions, so the Log's find reads those, not the day's lines. Phase 3b has the router
write the day's rows beside its head, in chunks of 256, so the Log reads a day back from them, and
a turn only the chunks naming its session. What they left, each a cost but for a few edges, gathered
here.

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

Phase 2 raised it, with 30 days kept, to some 140 MB and about 1 s, as each session's points take
in every account's answers of the day.

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

## A day head reads the days beside it whole

A day's head is built of exactly the lines `Day` gives of the day, which reads the files of the days
either side of it too, as a change of time zone files a line under a date beside its own; and it
decodes every line of those files whole to keep the few inside the day. So a head's build takes about
0.6 s where the day's own file alone takes about 0.2 s, as does a day back in the Log where the
day's rows don't stand. It can't go wrong, as only a line's `at` says whether it's kept. The cut is to read only `at` of the
neighbours' lines, but `dayfile.Read` counts a decode that reports false as a line unread: it needs a
skip outcome of its own, or a peek that reads `at` exactly as `lineIn` does.

## A request in flight for hours on a session's page

Decided by `joinedLate` and `pinnedFor`, and left: a request in flight more than 6 hours, while later
requests of its session are sealed, draws the session's story with folds that overlap; and a row the
stream tells of whose line arrived before yesterday is counted twice, which needs a request in flight
for over a day and its page shown within the hour after it ends.

And since a turn reads only its own days and the horizon's: a subagent's request of a turn, made days
after the turn's last known request and in flight for over a day untold by the stream, as after a
restart, is filed on its arrival's day, which no read takes in, so the turn opened shows one row
fewer than it did when it read every day. Carrying a turn's span to now where its session has a line
never fed would take it in.

## Smaller

- A restored page's times sent upstream are decoded through a list of times, some 230 B each, before
  they're held as nanoseconds: some 919 MB of garbage for a session running every day of 400.
  Decoding them straight into nanoseconds would make none.
- A page whose parts are spoiled, and whose next older state to fall back on lies three days back
  or more, is read whole: the points can't place a session's own day before yesterday, after its
  base.
- The builder's walk of a session's chain of parts decodes each link's digest, and opens a gzip
  reader for each member, again for every day it builds: for one session running every day of 400,
  some 80 ms and 98 MB of garbage a build, 7.6 MB held, and a backfill growing with the square of
  the days.
- A whole part's times, of a session busy every day and kept for good, reach the 64 MiB a member may
  inflate to after some 500 days: past it, the member is refused at every read, so the page never
  rests and the router reads the session whole every day it runs. A whole part comes only after a
  refused restore, so it needs that and a very long keep.
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

## A day's rows

- A chunk that doesn't read, as one damaged in a rows file as long, is dropped without a word:
  `restoredDay` logs nothing, so its day reads raw at every day back, and a turn of it reads the
  day raw, until it's built again for another reason, as the digesting looks at the file's size
  alone. A debug line a day, the first time a day's chunk fails, would make it visible.
- `RowChunk` opens the rows file once a chunk, some 98 times for a busy day read back. One open a
  day read would trim that; the Log's paging by rows, phase 4, may reshape the read anyway.
- A chunk's meta counts a line given twice as often as it's given, where the Log counts one with an
  id once. Phase 4 sums a finished day's `LogCount` from the metas, so it must count as the Log
  does first.
- `Spend`'s and `KeptWindow`'s stored forms could decode through `json.UnmarshalerFrom`, straight
  from a row's decoder, rather than through `UnmarshalJSON` and a copy of their bytes.
- An idless line in a tail is summed twice, once for its key and once for its row.

## A turn's read

A turn a day back reads its own day from its rows, but the horizon's span, from an hour ago to now,
reads a day either side of it, so yesterday's file is read raw for it anyway, its lines of the
turn's day passed over. Narrowing that margin to the hour after midnight, where a change of time
zone can file a line under yesterday, is the next saving. Phase 4's E1, today's file read back from
its end to the horizon, removes the raw horizon read altogether.

## Edges of a line given twice

- An idless line given twice, whose second copy comes in a tail's gain, is counted once by a page
  held open and twice by the same page opened afresh. It needs idless lines, which only old or
  hand-edited ledgers hold.
- A day head whose day after was counted at none as it was built, then given a line of the head's
  day by a writer in another time zone, reads raw until that day's digest is written.

## The guard

- It never meets a standing day of more than one chunk: the spread worlds' days are small, so a
  turn's chunk choice and a day's whole-or-nothing read over several chunks are held only by the
  ledger's and the views' own tests.
- It doesn't compare the Log at 23:59, where a day's rows and its tail meet.
- It runs some 35 s with the race detector on, past the plan's 25 s. Its biggest costs are the
  store's feed, the find and the stores' comparisons; reading the extra day only while its moments
  are unmet would save about 1 s.
