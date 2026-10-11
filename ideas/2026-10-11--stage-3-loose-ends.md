# Stage 3's loose ends

Milestone 7's stage 3, the data verbs, went through several rounds of review each. What the reviews
found and left, as not worth holding the stage for, gathered here.

## Two router tests that fail under load

`TestAnAssignmentUsedAgainIsSavedOnceAMinute` and
`TestTheRequestStreamTellsOfTheRequestsARestartFinishes` (`internal/router`) each failed once in a
full run of the tests while several other runs went on at once, and passed every time run alone, five
times each. Each rests on a timing assumption that a loaded machine can break. Find what each waits
on, and wait on it rather than on time.

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

The costs these stages found that grow with time or with what's kept are in Milestones, for the
review before milestone 7's release.
