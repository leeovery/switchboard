# The v1 fixes: what they left

The whole-codebase review before v1 (10 November 2026) was fixed in #272 to #281, and the release
review over them in #282, all released as v0.6.0. Their reviews accepted these: an edge past
ordinary use that fails safe, a cost known and taken, or an improvement. Gathered here for after
v1.

## Recording and summaries

- **The prune hook leaves the rest to the next round.** Where the readings history's prune is the
  one to find more than eight days to summarise again, it summarises those its prune would take
  readings from, and the eight; the rest wait for the ledger's next round, up to an hour, as the
  hook drops the `more` it's told and never asks for a round at once. A version bump or an import
  reaches the ledger's own round first, so it's rarely the hook.
- **A large keep shrink at a version bump.** Where `[history] keep` shrinks a long way in the same
  upgrade as a summary version bump, every day whose readings that prune takes is summarised again
  in one round, uncapped, holding the readings writer for all of them. None loses its readings.
- **A day that can't be summarised before a prune** is tried and warned of again at each prune that
  takes its readings, up to about eight daily prunes as the cutoff moves through its week.
- **A repeated line from an old export.** An export made in the window when a stopped compress
  could count a day twice may add a repeated line once on import.

## The request path

- **The wake window counts.** A broken answer, as a 502 already did, counts toward the router's
  health where it falls in the up to 15 seconds before the router notices the Mac woke: five
  parallel streams broken by a sleep in that window can show the router unhealthy for five minutes.
  Routing carries on; only the warning shows.
- **The dashboard's own probes** (`dashboard --probe`, without the router) stay on Go's default
  connections: no pings, and nothing renewed after a sleep, as the router's probes now are.

## What's shown

- **A broken answer** shows in `requests` alone, as `200 broken off`. The dashboard's Log shows
  canceled, cut off and broken off answers no differently from the rest, and its failed-only filter
  goes by status, so a broken 200 isn't among them.
- **History's all-accounts row** marks a week where every account with a known peak reached its
  cap, while the "short on N weeks" beside it counts a week where any did. As signed off (the old
  rule, a mean of 100%, was the same "every"); whether it should mark "any" is the owner's call.
- **Accounts' WEEKS bar** marks no cap for an account with a reserve, though its figures are inked
  at the cap; the account's own gauges above it draw the reserve's mark.

## Setup and the rest

- **A login shell changed since setup** (`chsh`): uninstall asks the shell the account logs in with
  now, so a startup line setup put in the old shell's files, under another `ZDOTDIR`, is missed;
  uninstall says what it couldn't find rather than claiming it's gone.
- **A fifth file lock:** the dashboard's prefs take a `flock` on the state directory
  (`internal/theme`), a fifth copy beside the four [one-file-lock](2026-11-04--one-file-lock.md)
  names.
