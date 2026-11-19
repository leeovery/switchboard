# Pins that keep sessions on their accounts

**Status:** to discuss with the owner before anything is built (the owner's call,
18 November 2026).

## The case

An account sometimes has room meant to be used up quickly. A new subscription gets a
week's use in its first day, and Anthropic sometimes grants a free reset. The owner
wants every session on that account until it runs out. Then the sessions should wait,
not move onto the other accounts and spend them, for example overnight while nobody is
watching.

## What switchboard does today

Every pin gives way at a limit (`docs/commands/pin.md`, "Rules every pin follows"). A
pinned session that hits one moves by the usual rules. When none of the pinned accounts
has room, the router chooses among every account in the pool. The pin comes back once
one of its accounts has room again.

Today's workaround is a reserve of 99.99% on every other account, then
`switchboard service restart`. The router stops choosing an account at its cap. If no
account has room below its cap, Claude Code gets a limit refusal and waits. Pools would
also do it, because nothing crosses a pool, but the owner doesn't want a pool for this.

## The two views

- **The owner's.** A pin, almost by definition, isn't "I prefer this account". It
  means "use this account, and only this account or these accounts". So a pin should
  perhaps keep sessions on its accounts at a limit by default. A variant might let the
  pin go when its accounts run out, reverting to `auto`. The owner very much disagrees
  that a stalled unattended run is worse than one that spends another account.
- **Mine, as argued on 18 November.**
  - Keep the default as it is, and add a flag that makes sessions wait, such as
    `switchboard pin <id> --only`.
  - My reasons: pins also come from `run --account` and from per-session pins, and
    those mostly mean "prefer". A stalled unattended run loses hours, and routing
    comes first.

  The owner rejected the second reason. Weigh it again when we discuss.

## To settle

- **The default.** Pins either wait at a limit, or give way as now. Whichever it is,
  name the flag that gives the other.
- **Who the rule covers.** Does it hold for every pin, or only for the pool's pin?
  Pins on one session and `run --account` are the other cases.
- **The name.** "Hold" is taken: a hold is an account held after a refused read (see
  the glossary).
- **What a waiting session sees.** Claude Code's limit message, as when every account
  is out? Does the dashboard, the Log or a notification say that sessions are waiting
  on a pin?
- **Several accounts pinned.** Sessions wait only when every pinned account is out.
- **Reserve.** Does a waiting pin still spend the reserves of the accounts it names,
  as pins do today?
- **When it ends.** Does it end only with `pin auto`, or at the pinned account's
  reset? The owner also floated a pin that lets go on its own.
- **Every request's path.** This changes the router's choice, so it's routing work:
  build it with xhigh and full reviews.
