# Setup's new pool warns of a fence folder that isn't on this Mac

`switchboard pool add` and `pool edit` warn of each fence folder that isn't a folder on this Mac:

```
switchboard: ~/Code/acme isn't a directory, so no session starts in it yet: if it's mistyped, fence the pool anew with switchboard pool edit b2n8 --dirs
```

Setup's card for a new pool says nothing of it. A folder mistyped there makes the pool all the
same, and no session ever starts in it. You find out only when your sessions land in another pool.

## What it could do

- **Warn on the card:** as the new pool is made, setup says of each fence folder that isn't on
  this Mac what `pool add` says, on the card or the screen after it.
- **One owner of the check:** `pool add`, `pool edit` and setup ask the same function, so the three
  never disagree about which folders are missing.

## Costs and limits

- It's a warning, never a refusal: a folder may be made later, so the pool is made either way, as
  `pool add` makes it.
- A folder on a disk that isn't mounted reads as missing, as it does for `pool add`.

Found by the second v1 fix round's review, 17 November 2026.
