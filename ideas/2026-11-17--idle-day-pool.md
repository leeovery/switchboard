# An idle day's pool, recorded as the day is summarised

An account's window facts, such as limits and minutes at its cap, show on every day in the pool
it's in now, as they're the account's. When it moves to another pool, the pool it left keeps its
past: its work, and the window facts of the days it worked there. An idle day is different. The
day's summary records no pool for an account that sent no request, so a read can't tell which pool
the account was in that day. The pool it left therefore drops the account's idle days, such as a
day spent capped with nothing sent.

The v1 release review found this (17 November 2026). For v1, the design states the rule as it
stands: in a pool an account left, its idle days drop.

## What it could do

- **Record the pool on an idle day:** as a day is summarised, an account with no work there takes
  its pool from the config at that time, kept in the summary's `pool` as a worked day's is.
- **Read it as the pool at the time:** a pool's read keeps an idle day of an account moved out
  where the day names the pool, as it keeps a worked day's.

## Costs and limits

- Days summarised before it stay as they are, their idle days going by the pool now.
- A day summarised again after a move would take the new pool, so the record would be exact only
  for days summarised before the account moves.
- The summary's `pool` on an idle day is a new meaning in a v1 format, documented in the ledger's
  reference.
