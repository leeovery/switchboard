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

## Settled

Every 5 minutes: the owner's choice on 26 October 2026, for the release after v0.2.8, fixed, with
no config setting; and the dashboard's interval refresh goes, `R` staying for a read at once.

## Built, 26 October 2026

In the cleanliness release (see the design's The router's rounds, under Priming, and Live updates):

- **The rounds:** `unreadFor` (`internal/router/rounds.go`) is 5 minutes. The backoff for an
  account that can't be read has a constant of its own, `retryMost`, and still doubles from 2
  minutes to the half hour, so a refused token isn't probed every 5 minutes. The lapsed-window rule
  and the minute after any probe are as they were.
- **The dashboard,** while the router answers, no longer has it `POST /refresh` every interval
  (`internal/dashboard/watch`, `plan`): its full read, once an interval, reads the router's
  document as a look does, with its history. `R`, and the refresh a minute after a window on screen
  resets, still ask. Without the router, or with `--probe`, it probes every interval as before, and
  the interval is said to be that alone, in its help, the design's verb table and the guide.
- **The cost:** a probe is a request per model family, two today, each capped at one output token,
  so an idle account takes 24 such requests an hour, where it took 4, against windows that count
  far more. What the router records grows with what changes, not with the probes: the readings
  history keeps a reading only where a window reads otherwise, so use from elsewhere comes in at
  most six lines a half hour where it came in one, the ledger holds no probe, and a
  probe is an event only where it shows a limit lifted. A probe that reads nothing still backs off
  to the half hour. The log, at its default level, notes a probe that read only at debug, and a
  window a probe can't read, as a Fable week a plan lacks, is warned of as it's first found unread,
  then once a half hour at most (`rewarnAfter`), as often as before, the probes between at debug.
  `state.json` keeps how each probe went as a routine change, written once a minute at most, as
  the readings off answers are, and a reading that reads otherwise as news, a second on, as before.
