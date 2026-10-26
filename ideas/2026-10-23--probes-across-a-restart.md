# How an account's probes went, across a restart

Found fixing the startup probe that went out again at once after a choice's probe had failed, 23
October 2026: the router now waits a minute after any probe of an account ended before its startup
probe goes out. What's left was so before that fix.

## What happens

The router keeps, of each account, when a probe of it last ended (`probed`), why the last read
nothing (`probeErr`), and how many in a row read nothing (`misses`). None outlasts a restart: the
state file keeps the readings alone, and `state.recall` restores neither, and a handover passes the
new router only its listeners. So the new router knows nothing of the probes the old one made:

- **An account never read** whose probe, by a choice or `POST /refresh`, failed just before the
  restart is probed again by the new router's startup probe, at once rather than a minute on: one
  extra probe at most.
- **An account that can't be read,** whose rounds had backed off to the half hour, is probed on the
  new router's first round, and its backoff starts again from 2 minutes.
- **Its error** reads as none until the next probe of it ends.

## Why it matters

Little: an account that can't be read is rare, and restarts come with upgrades and config changes.
But the design states the minute after a probe ended, and the rounds' backoff, without exception,
and across a restart the router keeps neither.

## Ways it could go

- Keep `probed`, `misses` and `probeErr` in the state file, beside each account's reading, and
  restore them with it, a `probed` from the future, as a clock set back, taken as now.
- Leave it, and say so in the design: the cost is a probe or a few, after a restart.

## Not decided

Whether it's worth three fields in the state file, and, if it is, whether a stopped router's probes
should still count when it starts again hours later. Nothing's been seen to need it.

## Built, 26 October 2026

The first way, approved by its owner on 26 October 2026, built in the cleanliness release (see the
design's The router's rounds, under Priming, and Files):

- **Kept:** `state.json` keeps each account's `probed`, `probe_error` and `misses` beside its
  reading, an account never read but probed in vain with an entry of its own; and `state.recall`
  restores them as it restores the reading (`internal/router/state.go`, `statefile.go`).
- **A stopped router's probes still count** when it starts again, however much later, the open
  question settled: `probed` is a time, so the minute after it has long passed, and a backoff
  restored from `misses` is already due. The error restored is the last thing that account's probe
  said. A `probed` later than the router starts, as after a clock set back, is taken as its start.
- **Another token:** an account whose token file holds a token other than the one `state.json`
  kept the hash of, taken up while the router was away, starts afresh, as one replaced while the
  router runs does. One that loses its usable token starts afresh then, as one that gains one
  does, so nothing of its probes before is kept for a restart with the token back to take up; and
  a probe that ends while it has none, sent before the loss or after, leaves nothing either, as one
  on the token before doesn't.
- **A file edited by hand:** a count of misses is taken from none to the fifth, past which the
  wait stays the half hour, so one below none can't stop the router from starting; and a kept
  error has anything shaped like a token redacted.
- **Its error** is kept with anything shaped like a token redacted (`internal/redact`), as the
  Claude prober's errors already are.
- **Older and newer:** the version stays 1. A router from before ignores the fields, and takes an
  entry with nothing else as no reading. A file from before loads as no probe ended.
