# Day digests, phases 1 to 4: what they left

Phase 1 of the day digests has the router digest each finished day and the store of sessions rest
each session on its newest standing state (see The request ledger and Live updates in the design).
Its soak, a synthetic world of 25,000 requests a day with its long session every day, race detector
off, opens the List in about 0.3 s with 9 or 30 days kept, from 1.4 s and 4.0 s. Phase 2 has each
digest carry each session's page, and a parts file a day hold what grows, so a page rests on its
newest state too: the long session's page opens in 0.31 s and 0.38 s, from 1.71 s and 5.63 s. Phase
3a has a turn read only its own days and the horizon's, and the router write a head a day holding
the day's sessions, so the Log's find reads those, not the day's lines. Phase 3b has the router
write the day's rows beside its head, in chunks of 256, so the Log reads a day back from them, and
a turn only the chunks naming its session. Phase 4 has the Log read each past day through its
filter, holding only the lines it keeps, so the find's pick lands 8 or 22 days back in some 0.4 s
and 37 MB, from 4.6 to 16 s and 277 to 701 MB; and a turn's horizon read back from the end of
today's file, so a turn opens at any age in some 0.03 s, from 0.4 to 1.3 s and 70 MB. What they
left, each a cost but for a few edges, gathered here.

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

Phase 3b's head and rows lift it some 145 MB a day built, for 1 to 1.5 s. After the upgrade that
brings them in, every day kept is built again, 400 by default, one at a time, so the
peak comes once a day built, never all at once, but for as long as the backfill runs.

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
day's rows don't stand. It can't go wrong, as only a line's `at` says whether it's kept. The cut is
to read only `at` of the neighbours' lines, but `dayfile.Read` counts a decode that reports false as
a line unread: it needs a skip outcome of its own, or a peek that reads `at` exactly as `lineIn`
does.

Where a day's side doesn't stand, the find's fallback, `DaySessionsOn`, builds a whole record of
each line of the day only to fold its session's facts: about 6 µs more a line, some 0.13 s more,
a fifth, on a raw day of the soak. It bites yesterday until about 01:00, and every day through the
backfill after this release, as `logdayVersion` 3 outdates every head written before. A fold of
the facts the find reads alone would cut it.

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
- The Log's first open makes the record of each of today's lines, about 22,000 of some 656 B in
  the soak, beside the lines, before `Taken` takes them in: some 14 MB at once. Feeding each
  record through as it's made would cut that passing peak.
- `dayfile` and `linescan` read 4 KiB at a time, beside a `Stat`: in the views' guard, those reads
  and `Stat` calls take most of its time. A bigger read buffer would cut them, there and in every
  raw read.
- `heldRequest.summed` has no test that it sums every field, as the line record's sum has; the two
  share `decodedRequest`, so a fact added there is held only by the record's.
- The watch's soak guard still passes over a turn's `turnLines` as it walks for lines held, but a
  turn now holds records: without the exemption, `TestThePageHoldsOnlyItsTurns` and
  `TestTheLogHoldsOnlyWhatItShows` still pass, so the guard could walk a turn too.

## A day's rows

- A chunk that doesn't read, as one damaged in a rows file as long, is dropped without a word:
  `restoredDay` logs nothing, so its day reads raw at every day back, and a turn of it reads the
  day raw, until it's built again for another reason, as the digesting looks at the file's size
  alone. A debug line a day, the first time a day's chunk fails, would make it visible.
- `RowChunk` opens the rows file once a chunk, some 98 times for a busy day read back. One open a
  day read would trim that.
- `Spend`'s and `KeptWindow`'s stored forms could decode through `json.UnmarshalerFrom`, straight
  from a row's decoder, rather than through `UnmarshalJSON` and a copy of their bytes.
- An idless line in a tail is summed twice, once for its key and once for its row.
- `rowsIn`, as the writer counts a day's rows, and `DayRows.counted`, as a reader holds a head's
  chunks to its count, each sum the chunks' counts. One helper could own it.
- A head damaged after its fields, its size unchanged, is read without its side by the writer,
  judged current and settled, and never built again, while each reader of it reads the day raw.
  It's phase 3a's gap, and now covers the rows' table too. The writer reading a head whole once,
  as it first settles it, would close it.
- A line's record prices its usage through `ledger.SpendOf`, some 7 µs and 76 allocations for a
  typical answer, most of it the decode of the usage into a map of any. So the `requests` verb's
  rows, which took 1.8 µs a line before the record, take some 5.8 µs, and each record the Log, the
  find's raw fallback and the builder make pays it. Working the tokens out of that map, as one
  decode, saves only some 1 µs, and can't keep `odd` exact: the map sees no case-folded or repeated
  key as the tokens' own decode does. A fast path would: for a usage of lowercase ASCII keys alone,
  no escapes, decode once into a typed struct of the known counts, unknown fields refused, and fall
  back to today's two reads on any doubt; its catch is the bill, which comes of the counts' map.

## The Log's read

- **The window.** The Log holds every line it reads from the first day it spans to today: today's
  some 25 MB in the soak, and some 30.5 MB more for each day back unfiltered, 249 MB a week back.
  Holding the read as the view and a page or two either side, addressed by date, chunk and index,
  would hold a few MB however far back it's paged. It would also make widening a filter while paused
  far back cheap: that reads again every day held through the narrower filter, some 3.3 s and
  280 MB 8 days back, 8.4 s and 706 MB 22 days back, what the find's pick cost before phase 4.
  Phase 4 cut it, as a day back unfiltered is minutes of PgUp, and the find and filters, the ways
  back, now hold only what they keep. It's some 12 to 15 hours in the page model the frames hold:
  `LineKey`, `Find`, `KeptFrom`, the wants of a day before and the letting go, all by rows, and a
  paging guard.
- **Thinning the held row.** Each line held is some 1.13 KB with its chunks and keys: its
  `RequestRow` 424 B, `lineDetails` 168 B, each kept window 32 B and each key 64 B. Holding the
  details only as a card opens, and the row's fields the table draws alone, would cut some 45% of
  today's held.
- **The open's second read of today.** The Log's first open reads today's file whole, then feeds the
  store every session of today, which reads it again: some 0.4 s of the open's 0.9 s. Feeding the
  store from the open's own read would save it.
- **A peek decode of a row's session.** A chunk read for a turn or through a filter restores every
  row of it whole, then drops those the turn or the filter doesn't keep. Peeking at a row's
  session, account, model and status first, and restoring only those kept, would save most of the
  decode where a chunk holds a few of a session's rows among many.
- **Late reads decode again the lines they drop.** Every late or awaited read of a day held through
  a filter, as of yesterday just after midnight, decodes again every line the filter dropped, as
  `RecordsToTake` passes over only the ids held: there, its "decodes only what it gained" no longer
  holds. Remembering the ids dropped, or the lines read, would make it so again.
- **A refilter prices its day at the day it's taken on.** A day read again through the join prices
  its lines at the prices of the day it's read again on, so across a change of the price table, a
  day refiltered prices otherwise than its first read did.
- **The failed alone, read back.** No summary counts failures, so a filter of the failed alone,
  of no session, rules out no day, and a read back while the Requests want more reads on, a day's
  metas at a time, to the oldest day kept, as one command: some 1.4 ms a day in the soak, 43 ms for
  30 days, but some 0.6 s for 400 at the long keep, and a ledger kept `forever` longer. And `Len`,
  by which the read back judges whether a day added to the Requests, counts the lines held, never a
  row the stream tells of.
- **The find's summaries, held while it's open.** The find holds every day's summary it walked,
  raw usage and all (`findWalk.summaries`), till it closes. Holding only what it reads of them would
  slim it.
- **The not-live time.** While today has no lines and the newest past lines were passed over
  unread through a filter, the state line's time, not live, can be older than the ledger's last
  write, as it's taken from the lines the read was handed. Taking it from the ledger's own last
  write would close it.
- **A told row after its take.** A request the stream first tells of after its day was read through
  a filter hiding its line shows that line only at the next read awaiting it.
- **A move row at midnight.** A move row reads its session's story from the store, and the store
  isn't fed the sessions of the lines a filter dropped. A move at 00:00:00.001 whose request's line
  arrived at 23:59:59.999 on a day held through a filter, of a session with no other line kept,
  would show no cost. The refilter to whole as the Events show, and the Events reading only the
  days held whole, cover it but for that race at midnight.
- **`keptSessions` over-wants.** Of copies of an id that differ, as only a hand edit makes, it may
  want a session the read holds no line of kept: the store holds it for nothing, and nothing shown
  changes.
- **A body longer than `arrivesWithin` across midnight.** The stream tells when a request went
  upstream, not when it arrived, so its line is taken as filed on the day it was sent or the day a
  minute before, `arrivesWithin`. One that arrived before midnight and went upstream more than a
  minute after it, its body that long in coming, has its line filed on a day `ThroughOn` doesn't
  read whole for it, as it escapes `holdsLineOf`: through a filter hiding it, its line isn't held.
- **An event card's figures depend on how far back the Log holds days.** A cap's or limit's card,
  "sessions N" over the hour before and "known from", reads the lines held: an event in today's
  first hour counts yesterday's lines only where yesterday happens to be held. Following with the
  Requests filtered, `]` lets go of the days held through their filter, so the Events show what a
  fresh open shows, where a Log reading every day whole, holding those days for the hidden
  Requests, shows other figures. A card reading back what it needs itself would make it one answer.
- **Near midnight, the stream holds the days back.** Where what the stream tells of holds the days
  back as of today, as near midnight it may, `holdsBack(today)` has a following Log read again
  every day that doesn't serve before it lets any go, where one reading whole would let go of the
  days before its tables' fill: the same end, with extra reads.
- **`dashboard.LogPage.Filtered`** restates part of the views' `whole()`, which choices keep some
  lines alone: two owners of one fact.
- **A guard over `LogFilter`'s fields.** Six places read every field of a filter. A field added
  later and missed in one would pass every test; a guard that each handles every field would catch
  it.

## A turn's read

The horizon's span, from an hour ago to now, takes in a day either side of it, as a change of time
zone can file a line under a date beside its own. Today's and yesterday's plain files are read back
from their ends, stopping once nothing older is wanted. What's left is the margin after midnight: in
the hour after it, the horizon falls on yesterday, so the day before yesterday is spanned too, and
read wherever its summary names the session. Its file is still plain then, so it's read back, and
stops after its first 1,000 lines, each ending before the stop: some 7 ms, for a margin few lines
ever fall in, and forward whole where the day has a compressed file. Reading the margin day only for
the lines a change of zone could file there would save it.

`sessionHeldIn` clones each of its session's lines' JSON as it holds it, though a turn's read takes
only the line: some of the read's peak. A decode that holds the JSON only for the readers that read
it would save it.

## Edges of a line given twice

- An idless line given twice, whose second copy comes in a tail's gain, is counted once by a page
  held open and twice by the same page opened afresh. It needs idless lines, which only old or
  hand-edited ledgers hold.
- A day head whose day after was counted at none as it was built, then given a line of the head's
  day by a writer in another time zone, reads raw until that day's digest is written.

## The guard

- Since phase 4 it meets a standing day of more than one chunk, the crowded day, of which a filter
  reads some chunks and skips the rest; the other spread worlds' days are small.
- It doesn't compare the Log at 23:59, where a day's rows and its tail meet.
- It runs some 67 s with the race detector on, from 35 s before phase 4, as it now reads the Log
  back through filters, each read again through a join, and a turn read back, each held to every
  day read whole. Its biggest costs are the store's feed, the find, the stores' comparisons and the
  reads through filters; reading the extra day only while its moments are unmet would save about
  1 s. A bigger read buffer would save more (see Smaller).
- `pickLandedLater`, in the watch's Log guard, excuses the Log spanning from a later day than read
  whole by comparing the day it spans from alone: comparing the windows shown would hold it
  tighter.
- No test counts the reads a let-go costs: `goingFrom` letting go of every day to today, rather than
  to the day after the newest that doesn't serve, passes every test, as the days that do serve and
  it drops too are read back after.
