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
