# Pausing an account

A subscription that's cancelled goes on until the end of its billing period, then lapses, and may
be taken up again later. Switchboard has no word for that today: the account is either configured
and routed, failing once its subscription lapses, or removed, its token deleted and its place in the
config gone. Account identity softens removing, as adding the same organization back restores its
id and its history, but the label, plan, pool and reserve are set up again by hand, and a new token
made.

A plan and being live are two facts. A paused Ultra is still an Ultra, and History prices the days
it was live as one, so pausing isn't a plan, and setup's `not set` isn't a way to say it.

## What it could do

- **`accounts pause <id>` and `accounts resume <id>`,** and a toggle on setup's edit card.
- **A paused account** keeps its label, plan, pool, reserve, token and history, and drops out of
  routing, the forecast, its pool's room and the probes. Every screen marks it paused, and History
  goes on showing its days.
- **Resuming** asks the API whether the token still serves, and asks for a new one where it doesn't.

## To settle

- A pool's primary paused: refused until another account is made primary, or the pool's next
  account taking its parts, as when a primary moves pool.
- Whether a lapse noticed by the router, from the answers a lapsed subscription gets, offers to
  pause the account, or pauses it.

Raised by the owner on 11 November 2026, for after Account identity and Pools are built.
