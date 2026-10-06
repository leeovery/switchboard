# A ledger of the requests the router routes

The readings history records what Anthropic says of each account's quota as it changes: each
window's use, reset and status. It doesn't say what spent it. A ledger of the requests themselves
would: one line per routed request, with its time, session, model, account, why that account was
chosen, its status, how long it took and how many times it went upstream, and the token counts its
answer ends with (input, output, cache read, cache write), read off the stream's closing usage
event as it passes through, without changing anything.

What it could answer:

- Which sessions and models spend the most, and when.
- What a move really costs: the cache write of the first request on the new account, against the
  reads it replaces.
- How many tokens a percentage point of each window is worth on each model, which Anthropic doesn't
  document, and which the choice of account could then use: room in tokens rather than percent.
- Whether the router's choices worked out: how often a session moved, and why.

The router log holds most of the per-request fields today, but it rotates and isn't shaped for
analysis. The token counts are new, and reading them means code in the path of every answer's
stream, which is why this wants its own design and review rather than riding along with another
change. Keep it separate from the readings history, with the same promises: a file a day, kept for
a set time, fields added and never renamed, nothing personal, never a token.

Milestone 5's dashboard needs half of this: its request stream, `GET /stream` (see the design's
Control API), reads each answer's closing token counts as it passes, to show them live on the
Sessions view and the cards' backs. A ledger would write the same events down, a line a request, so
the reading lands with milestone 5 and the ledger is what's left.

## Settled, 5 October 2026

The dashboard's redesign gives the ledger its first readers: a History tab, of the past year by
day, week and model, with tokens as ccusage reports them; an Accounts tab, of what each account
did and was worth at API prices; and a session's own page, of its requests. Most of what they
show is this ledger's. The owner agreed how it's kept:

- **A file a day, a line a request,** as JSON, written as the readings history is: queued off the
  request path for a goroutine of its own. Its queue is longer than the history's 1,024, as a
  line dropped is a request the History tab never counts.
- **A rollup a day beside it,** per account and model version: requests, tokens by kind, worth at
  API prices, sessions, moves, limits reached and each window's peak. It's written once the day
  has closed, on the hourly round that compresses and removes the readings history's days;
  today's is kept in memory. Views over
  months read the rollups, and a session's page reads its own days' lines.
- **No database.** The queries are known before they're asked, and SQLite would bring a large
  dependency and migrations for what the rollups answer.
- **A benchmark of the proxy path** against an `httptest` upstream, before the ledger's writing
  joins it. There's none today, and the router's cost on a request is judged only by reading
  the code.
- **Agents read it** through read commands with `--json`, sharing the one data layer the
  dashboard draws from, as `status --json` does today.
