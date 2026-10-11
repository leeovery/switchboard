# Stage 3's loose ends

Milestone 7's stage 3, the data verbs, went through several rounds of review each. What the reviews
found and left, as not worth holding the stage for, gathered here.

## Weeks' headline for a set short every week

Where now is short on 4 or more of 7 weeks and one more account wouldn't halve that (short on 7 of
7, one more 4 of 7), the design's rules fall to their else, "about right": plainly wrong for a set
short every week. A headline for more than one too few needs its owner's words.

## Cleanups left

- `history`: `monthsOf` works out the running month's ratio, then drops it; three ways to word a
  worth (`worthOf`, `worthText`, `worthSaid`); `NewHistory` builds the year, months and tokens for
  the text form, which prints none of them; each account's worth summed in three passes; a check on
  `totals.Accounts` that can't fire; near-duplicate helpers (`shortCount` and `countedOrNone`,
  `dayFirst` and `dayText`, `entryName` and `accountName`); `totalling.add` shadowing `Worth.add`.
- `sessions`: `SessionPage` repeats `ListedSession`'s fields rather than embedding them; views'
  `dayOf` repeats `dayfile.DateOf`.

## What the end review left

The review of the whole stage, run after its release, found these and left them:

- Milestone 5's LATELY words the router's events its own way (`internal/dashboard/recent.go`),
  where `events` words them through `views.Telling`, so the two can disagree until stage 7's
  Overview draws LATELY from `Telling`.
- The session status a statusline asks for every few seconds builds the whole status document to
  read one account's name (`accountStatus`): every account's state, the recent events and every
  session walked, for one name.
- `status` and the dashboard work the router's document out again (`fromRouter`), which only a
  router from before stage 3 needs.
- The ledger's reader and follower warn that they can't read the request ledger at each read, where
  the rounds' listings now warn once until they can.
- Milestone 5's COMING UP lists only the windows its cards show; the document lists a model's own
  week wherever its reset isn't its week's. Whether the Overview's COMING UP, in stage 7, lists the
  reset of a week no card shows is to check against the design then.

The costs these stages found that grow with time or with what's kept are in Milestones, for the
review before milestone 7's release.
