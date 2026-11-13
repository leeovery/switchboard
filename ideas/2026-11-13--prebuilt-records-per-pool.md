# The ledger's prebuilt records, per pool

A dashboard shows one pool (see Pools, The dashboard in a pool). Pools' first phase filters what the
router's document gives, and the dashboard's reads of the router's events, so Runway's LATELY and
the Log's Events tell of the pool shown alone. Its reads of the request ledger aren't filtered, as
much of what they read is built ahead of time with no pool in it:

- **The Log's day rows,** a day's rows in chunks that `DayRows` points at and `RowChunk` reads. A
  row holds no pool, and a chunk's meta names none, so the Log can't keep one pool's rows of a day
  without reading its raw lines.
- **A day's side,** which `DaySide` gives for the Log's find and its days' sessions.
- **The day digests,** which a session's state rests on, for the Sessions tab and a session's page,
  and which History's and Graphs' points are read from.

So in a config with two pools, the Log's requests, the Sessions tab, a session's page, History and
Graphs tell of every pool's work together, labelled by account.

## What it could do

- **Each row holds its line's pool,** as each line records it, and each chunk's meta names the
  pools its rows hold, so a pool's read restores only the chunks that may hold its rows, as a
  filter's does, and keeps its rows alone. `views.RowsVersion` moves on, so every day's rows are
  built again, as when a row last gained a field.
- **A day's side and its digests** hold their sessions, worth and points per pool, or a read of one
  pool falls back to the day's lines where they don't.
- **The dashboard's ledger** is read through the pool's read, as `views.PoolRead` reads the verbs'
  files, so every tab tells of the pool shown alone.
- **The pool's read made live.** The dashboard's read of the router's files of events is the pool
  it shows as it opened: after a pool or an account is edited with a dashboard open, its events
  read from the files keep the old placements until it's started again. `P`, changing the pool
  shown, starts the view afresh, which is where the read is to follow the config too.
- **The pools as they stand, held for the run.** The dashboard reads the pools as they stand
  (`pooledConfig`: the config's, else the router's or the accounts record's) once as it opens, as
  its read of the events is fixed then: the first pool its ledger gives work in no pool follows the
  accounts record live, but not a pool added or removed while it's open. `P`'s fresh start is
  where they're read again.

## Why it's the second phase's

Reading raw lines instead, wherever the read is one pool's of several, would be right now, but it
gives up the fast opens for exactly the user with two pools, for a view the first phase leaves as
it was: the dashboard shows the pool the verbs act on, as before pools. The second phase gives the
dashboard its `--pool` and `P`, and is where its views each come to be one pool's.

Found by the review of the pools' verbs on 13 November 2026, for Pools' second phase.
