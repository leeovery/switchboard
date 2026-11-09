# Advice from the ledger

The request ledger holds what's needed to tell the owner which settings would have bought more usage,
worked out over a rolling window and offered as suggestions: auto-compaction's level (compacting at
500k tokens against 1M, say, and the usage that would have bought over six weeks), or compacting a
session once it's been idle an hour.

Most of the data is recorded already: each line's compaction and what it compacted, tokens by kind,
cache writes and how long they last, prompt ids, and moves after idle.

The same reading decides whether an optional keep-alive pays: keeping sessions' caches warm in
working hours, never all day, where the ledger shows whether a ping costs less than the cache writes
it saves. Before a session's hour lapses, the router would replay its last request with
`max_tokens` of 1 on its account, renewing its cache for a fraction of a rebuild; `max_tokens`
isn't part of what the thinking check covers, so the replay passes it. Open: how the subscription
limits count cache reads; stopping for sessions that have closed, as `run`'s pid is Claude Code's,
which could tag its requests; and the conflict with re-scoring an idle session, which moves it once
its cache is cold, at no cost.

Agents that budget their work against a limit would read the same data forward: see
`2026-10-14--usage-for-agents.md`.

Raised by the owner on 13 October 2026, to discuss: which suggestions, over what window, where
they're shown, and whether they come up on their own.
