# Pausing an account

A subscription that's cancelled stays live until its billing period is over, then ends, and may be
taken up again later. Switchboard has half a word for that. Since #351 an account whose subscription
ended is held out of routing and out of its pool's room until a read of it comes back clean, and
since #349 it shows `subscription lapsed`. Beyond that it's either configured, and probed on every
round, or removed, its token deleted and its place in the config gone. Account identity softens
removing, as adding the same organization back restores its id and its history, but the label,
plan, pool and reserve are set up again by hand, and a new token made.

A plan and being live are two facts. A paused Ultra is still an Ultra, and Accounts' weeks and worth price the
days it was live as one, so pausing isn't a plan, and setup's `not set` isn't a way to say it.

## What it could do

- **`accounts pause <id>` and `accounts resume <id>`,** and a toggle on setup's edit card.
- **A paused account** keeps its label, plan, pool, reserve, token and history, and is kept out of
  the probes, which a held account is still given every half hour, and out of the forecast; routing
  and its pool's room it's out of already, held by #351's hold. Every screen marks it `paused`, and
  Accounts' Detail (its WEEKS) and Compare go on showing its days.
- **Resuming** asks the API whether the token still serves, and asks for a new one where it doesn't.

## To settle

- Whether #351's hold answers pausing by itself: it keeps a lapsed account out of routing without a
  verb, and what's left is the explicit pause, for an account that's live but not wanted, and the
  probes and forecast it still gets.
- A pool's primary paused: refused until another account is made primary, or the pool's next
  account taking its parts, as when a primary moves pool.
- Whether an end noticed by the router, from the answers an ended subscription gets, offers to
  pause the account, or pauses it.

Raised by the owner on 11 November 2026, for after Account identity and Pools are built.
