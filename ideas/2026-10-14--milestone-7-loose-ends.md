# Milestone 7's loose ends

Milestone 7, the dashboard's redesign, went through a review of each pull request, of each stage,
and of the whole release, several rounds each. What those reviews found and left, as not worth
holding the release for, gathered here; the patch after v0.2.0 fixed the rest and added what its
own reviews left. Each is an edge case or a cost with no wrong answer.

## On screen

- The Overview's USAGE at 20 columns or fewer, scrolled.
- A session's page under 39 columns, below the phone's floor.
- Click targets at 1 to 30 columns.
- The Log's panel names a session by the lines the Log holds, so across midnight it can name it
  otherwise than the List does.
- The Overview at 14 rows with every section open: USAGE is left its heading alone, or nothing.
- At 11 to 13 rows: Compare says `▼ 25 more below` with nothing shown, Detail's `↑↓ choose` is
  clipped, and a phone's Days pages nothing.
- A view whose scroll changes what it counts, rows or accounts, goes back to its top; keeping the
  reader's place would take a mapping between the two both ways. Where a change of unit was still
  pending as a key moved the focus, the reset undoes that move for one key. On Sessions at step 5,
  the unit can flip as calls come and go.
- The routing card's cursor can sit below its cut, and act on an account not shown, on a terminal
  8 rows tall or less.
- Detail's name column is sized from the config's labels, where an older router's moment may name
  an account otherwise.

## The stream and the Log

- A lost request whose line falls on a past day isn't awaited; only ended rows are.
- With the Log open and no other reader, a request whose line is filed before the Log's first day
  is kept the hour once the Log reads that day back, the tail read at each look meanwhile; its row
  is right throughout.
- A session's page holds the answer and line keys of days the session never ran on (memory only).
- A request that never gets a line on the day tailed, cut off, its router gone mid-request, or its
  line filed on an earlier day no reader awaits, keeps the ledger's tail re-read at every look for
  the hour it's kept.
- Reading a session's lines, a malformed line of another session whose `session` field decodes
  counts as read, so the warning of lines unread undercounts, and counts differently by the session
  asked for.

## The pool

- `status`'s pool counts a tokenless account's room, where `Then` and Runway count none, so an
  account whose token goes missing steps the pool's chart at now.
- A limit can outlast the window it names: a 429's overall reset later than the window's own, or a
  limit naming the 5-hour and the week whose week reads room in a new week while the 5-hour is
  still spent. The moment holds the account, so `Σ pool` counts that window none, while `Then`,
  Runway and the pool's chart before now count its room. Ending it means the holds the router
  told of date a hold where the readings don't show one, which `Then`'s rule leaves out: the
  router's events would carry each window's own end, or the document's own limit would join what's
  told.
- A past column of the pool's chart is known once any account's room is, and counts an account not
  read by then as spent, as `Σ pool` counts one never read: an account added to the readings
  history yesterday shows the pool more used before its first reading than the readings say.
- An account idle longer than the readings tail's carry-in, about 14 days, counts unknown on the
  pool's chart, and days before the readings history's keep aren't summarised for it as they are
  for Runway.

## Costs

- The dashboard's canvas is its biggest allocator (`canvas.grow`, 38% in capture's sweeps): each
  frame allocates its rows of cells afresh.
- Building the Log's stories decodes each request's usage (`CacheLife`, `Tokens`), about 50 ms of
  its 0.3 s first open on a busy ledger.
- `readings.Day.Read` copies and sorts the whole tail on every read, now a week long, which is
  most of what a look on Runway costs as a reading lands (8% over its cost before the week-long
  tail, race detector on).
- While the dashboard reads the documents itself, probing or with no router, Runway and the pool's
  charts each concatenate and sort the whole tail (`views.RunwayReadings`) at every look.

## Cleanups

- Three test fakes, in `views/windows_test.go`, `views/runwayspan_test.go` and
  `dashboard/watch/history_test.go`, carry the tail's runs through `readings.CarriedRuns` where
  `readings.TailOf` now owns the rule.
