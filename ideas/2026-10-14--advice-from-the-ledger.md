# Advice from the ledger

The request ledger holds what's needed to tell the owner which settings would have bought more usage,
worked out over a rolling window and offered as suggestions: auto-compaction's level (compacting at
500k tokens against 1M, say, and the usage that would have bought over six weeks), or compacting a
session once it's been idle an hour.

Most of the data is recorded already: each line's compaction and what it compacted, tokens by kind,
cache writes and how long they last, prompt ids, and moves after idle.

The same reading decides whether an optional keep-alive pays: keeping sessions' caches warm in
working hours, never all day, as `2026-09-30--prompt-cache-keep-warm.md` describes, where the ledger
shows whether a ping costs less than the cache writes it saves.

Agents that budget their work against a limit would read the same data forward: see
`2026-10-14--usage-for-agents.md`.

Raised by the owner on 13 October 2026, to discuss: which suggestions, over what window, where
they're shown, and whether they come up on their own.
