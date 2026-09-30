# Prompt-cache keep-warm

Before a session's hour lapses, replay its last request with `max_tokens` of 1 on its account,
renewing its cache for a fraction of a rebuild. `max_tokens` isn't part of what the thinking check
covers, so the replay passes it.

Open:

- How the subscription limits count cache reads.
- Stopping for sessions that have closed: `run`'s pid is Claude Code's, which could tag its
  requests.
- The conflict with re-scoring an idle session, which moves it once its cache is cold, at no cost.

Parked since milestone 3's design; moved here from the design's backlog.
