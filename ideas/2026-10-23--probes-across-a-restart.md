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
