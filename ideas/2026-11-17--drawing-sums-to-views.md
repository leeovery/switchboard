# Three sums move from the drawing package to `views`

Each fact is worked out once, in `status` or `views`, and the dashboard only draws it. Three sums
are still worked out in `internal/dashboard`:

- `movedForms` adds an account's moves in and out over a period, for Accounts.
- `summedSessions` adds the pool's sessions, for its row in the routing view.
- `summedRate` adds the pool's request rates at a moment, on the same row.

A verb that wanted any of them would work it out again, and the two could drift apart.

## What it could do

- **Move them:** each sum becomes a field or a function in `views`, beside the data it sums, and
  the dashboard reads it there.
- **Guard it:** if one fits, `views/guard_test.go` holds that the dashboard sums none of these
  itself, as it holds the other facts.

## Costs and limits

- A refactor with no change on screen: the frames hold that nothing drawn moves.
- A guard against summing in general may catch drawing arithmetic, such as widths, that belongs
  where it is. A narrow guard fits better than a broad one.

Found by the second v1 fix round's review, 17 November 2026.
