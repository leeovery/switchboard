# The ledger's prebuilt records, per pool

A dashboard shows one pool (see Pools, The dashboard in a pool), the one `--pool` names, `P`
turning to the next. It filters what the router's document gives, and its reads of the router's
events, read afresh at each `P`, so Runway's LATELY and the Log's Events tell of the pool shown
alone. Its reads of the request ledger aren't filtered, as much of what they read is built ahead of
time with no pool in it:

- **The Log's day rows,** a day's rows in chunks that `DayRows` points at and `RowChunk` reads. A
  row holds no pool, and a chunk's meta names none, so the Log can't keep one pool's rows of a day
  without reading its raw lines.
- **A day's side,** which `DaySide` gives for the Log's find and its days' sessions.
- **The day digests,** which a session's state rests on, for the Sessions tab and a session's page,
  and which History's and Graphs' points are read from.

So in a config with two pools, the Log's requests, the Sessions tab, a session's page, History and
Graphs tell of every pool's work together, labelled by account, whichever pool the title row names.

## What it could do

- **Each row holds its line's pool,** as each line records it, and each chunk's meta names the
  pools its rows hold, so a pool's read restores only the chunks that may hold its rows, as a
  filter's does, and keeps its rows alone. `views.RowsVersion` moves on, so every day's rows are
  built again, as when a row last gained a field.
- **A day's side and its digests** hold their sessions, worth and points per pool, or a read of one
  pool falls back to the day's lines where they don't.
- **The dashboard's ledger** is read through the pool's read, as `views.PoolRead` reads the verbs'
  files and the dashboard's events, so every tab tells of the pool shown alone, and `P`, starting
  the view afresh, reads the next pool's.
- **A pool the config no longer has.** The verbs that read the files read a removed pool's work,
  or another Mac's from an import, by its id (`--pool`, as `filedPool`'s kept pool reads it),
  never folded into another pool. The dashboard's `--pool` refuses one, naming the pools
  configured: it could take such a pool for History, the Log's requests and Sessions alone, its
  windows and Runway having no accounts to show, once the prebuilt records name each line's pool,
  as above.

## Why it's the second phase's

Reading raw lines instead, wherever the read is one pool's of several, would be right now, but it
gives up the fast opens for exactly the user with two pools. The second phase gave the dashboard its
`--pool` and `P` first; this is where its views each come to be one pool's.

Found by the review of the pools' verbs on 13 November 2026, for Pools' second phase.
