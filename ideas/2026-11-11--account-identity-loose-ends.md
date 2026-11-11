# Account identity: what it left

The account identity work (#283 to #287, November 2026) moved accounts onto ids switchboard makes,
named them by label, and held each token to its subscription. Its reviews accepted these: an edge
past ordinary use that fails safe, or an improvement. Gathered here for later.

## The move

- **Over-waiting.** Wherever a token file may join an account another Mac moved, the move waits on
  every token file the API didn't answer for, even one whose answer couldn't change the join. A
  dead token counts after the 10-minute nameless run; one that only times out holds the move.
- **A second Mac joining by its token files keeps no old-id `state.json`.** Where its router never
  ran on the shared config's new ids before the upgrade, the pin, session assignments, former
  tokens and token years kept under the old ids are dropped. The usual path, an older router
  restarting onto the new ids first, dropped them before the move anyway.
- **A session pinned by an old id on a Mac joining another's ids** keeps its pin ignored until the
  router next restarts, as the old ids are read once, at start.

## Tokens held to their subscription

- **Two accounts configured on one subscription,** a hand edit, show the second as `wrong token`
  with advice that can't work: `accounts token` and `accounts add` both refuse. Removing one is the
  fix; the words could say so.
- **Setup can't make B's token while A holds it.** Where A's file holds B's token, setup's review
  still counts that organization as A's, so B's own token is refused as A's subscription until A
  is fixed. Leaving a foreign row's organization out would let either go first.
- **`switchboard accounts` shows `wrong token` over a token file that's gone or unreadable,** until
  the router's token upkeep notices the change.
- **The check at the send itself.** A finding that lands in the instant between the choice's check
  and the send still goes out once.
- **A `Moved` event in the re-choice's window.** Where a choice moved a session onto an account
  that then lost its token, that event stays though the request went elsewhere.

## Names

- **"Is a label" can point at a removed account's id** for `accounts edit`, `token` and `remove`,
  where only a removed account has that label; the id given is then refused as no account.
- **An account only another Mac configures,** labelled as one configured here, reads the same in
  the verbs' text, as only removed accounts are marked.
- **An account another Mac configures without a label,** a hand edit, is marked removed here at the
  next look.
- **About fifteen exact-match id lookups** (`a.ID == id`) in launch, setup, capture, dashboard,
  idmove and views could move onto `config.Accounts.ByID`, ignoring case, where that's right.

## Tests

- **`TestASummaryOfADayTheMacNeverSawIsCopiedInWithItsStamp` is timing-dependent.** Under load the
  router can summarise the day an import copied in before the import copies that day's summary, so
  it counts no summary copied. The content is the same. Seen once in a whole run on 11 November;
  it dates from the import's first build (#246).
