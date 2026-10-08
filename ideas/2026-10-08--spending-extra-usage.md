# Spending extra usage

An account with extra usage turned on can be served past its limit, and billed for it, rather
than refused. The router never chooses to spend it: it passes over an account whose window reads
spent, and moves a session off it before its next request, so only the request that crossed the
limit is billed (see What doesn't go through the router). Once every account is at its limit, the
router answers Claude Code with a 429 of its own, though an account with extra usage on could
serve the request, for money. Nothing says "carry on, and bill it": not the config, not `pin`, not
the dashboard. Milestone 7 reads extra usage into the status document, and Accounts' Detail shows
whether it's on, but nothing acts on it.

Two ways to choose to spend it:

- An option on each account in the config: the router goes on sending to it past its limit once
  no account has room, as a last resort, as a reserve might be spent (see
  [reserve-last-resort](2026-10-03--reserve-last-resort.md)).
- A switch flipped by hand once everything's out, such as from the dashboard's routing picker,
  holding until the next reset, so spending is always chosen at the moment.

To settle with the owner:

- Which of the two, or both.
- What the headers read past a limit with extra usage on, never seen yet (see Checks owed):
  whether a window reads `rejected` or past 1 while extra usage serves, which decides how the
  router tells an account spending it from one at its limit.
- How `status`, the dashboard and the notifications say an account is running on extra usage, and
  how much of it is used, as its `utilization` gives it.
- Whether a pin to an account at its limit, with extra usage on, spends it, as a pin spends a
  reserve.

Later: its owner doesn't use extra usage.
