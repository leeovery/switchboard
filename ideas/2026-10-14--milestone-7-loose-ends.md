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
- The routing card's cursor can sit below its cut on a terminal shorter than the card.
- Detail's name column is sized from the config's labels, where an older router's moment may name
  an account otherwise.

## The stream and the Log

- A lost request whose line falls on a past day isn't awaited; only ended rows are.
- With the Log open and no other reader, a request whose line is filed before the Log's first day
  is kept the hour once the Log reads that day back, the tail read at each look meanwhile; its row
  is right throughout.
- A session's page holds the answer and line keys of days the session never ran on (memory only).

## Costs

- Runway builds its lanes again at every message, a resize or any key, and judging its past calls
  `Then` once a column, each scanning every reading of the account: columns × readings a message.
  It's most of what capture's tests still spend, and it slows Runway in the real dashboard.
- The dashboard's canvas is its biggest allocator (`canvas.grow`, 38% in capture's sweeps): each
  frame allocates its rows of cells afresh.
- Building the Log's stories decodes each request's usage (`CacheLife`, `Tokens`), about 50 ms of
  its 0.3 s first open on a busy ledger.

## Cleanups

- Three test fakes, in `views/windows_test.go`, `views/runwayspan_test.go` and
  `dashboard/watch/history_test.go`, carry the tail's runs through `readings.CarriedRuns` where
  `readings.TailOf` now owns the rule.
