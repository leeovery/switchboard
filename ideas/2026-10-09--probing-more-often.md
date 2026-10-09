# Probing more often

The router reads every account itself, on its rounds: each one nothing has read in the last 30
minutes, and each one 5 minutes before each of its weeks resets (see the design's The router's
rounds, under Priming). Settled at 30 minutes for milestone 7's stage 3, 9 October 2026, the
dashboard's default interval.

A probe costs a token or so: a request per model family, each capped at one output token. So the
router could probe more often, every 5 minutes say. What it would buy:

- **Use from elsewhere seen sooner.** Use from the Claude apps on a phone, claude.ai or Claude Code
  run directly reaches the router only when it next reads the account: within minutes rather than
  the half hour, and the recent rates, the projections and the readings history with it.
- **A free reset seen sooner.** A reset made by hand on claude.ai, on an account a limit holds back,
  lifts the limit only once a probe reads it, so the account would take requests again within
  minutes.

To settle with the owner:

- **How often:** every 5 minutes, or another interval, and whether the config says.
- **The dashboard's refresh:** every interval, the dashboard has the router probe what it hasn't
  read in that time, its interval 5 minutes at the least. With the router reading every 5 minutes
  itself, whether that refresh still has a part, or goes, leaving `R` to ask for a read at once.
