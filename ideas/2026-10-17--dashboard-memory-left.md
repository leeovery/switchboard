# The dashboard's memory: what the fix left

The memory fix after v0.2.1 has the dashboard's readers of the ledger hold what they show, from one
store of sessions told on as its lines come (see Live updates in the design). Its soak, a synthetic
world of 25,000 requests a day with 9 days behind it, race detector on, put the live heap at
61 to 95 MB before the Log opens and 359 MB at its peak with the Log paged back over all 9 days,
from 2,452 MB. What it left, each a cost, not a wrong answer, gathered here.

## The Log's line

The Log's lines hold about 1 KB each, against the 250 to 300 B the fix planned: 25 MB for a day of
23,000 lines, 254 MB with 9 days paged back. From the soak's heap profile: the line itself, its row
told and its words, about 610 B; its details beyond the row, about 250 B; its windows, kept small,
68 B; and the keys that know a line given again, and the chunks that table it, the rest. Where to
start: the line holds its row whole, told, beside what it was told from, and its details beside
both; holding the figures alone and telling the words as the row or the card is drawn would bring
it toward the plan.

## A request in flight across midnight

A request sent before midnight that ends after it is filed on the day before, which the tail no
longer reads: as it ends, each reader that awaits its line reads the whole day before again for it,
off the screen, a day's file parsed for one line. Reading a past day on from a mark, as today's is
read, the mark kept as the day turns, would read only what the day gained since.

## Smaller

- A session's page decodes each line a read of the tail gains for its points, its usage and its
  limits, a second time beside the store's own decoding of it.
- A page's `Held` reports a change wherever its points' held answers grew, not only where its points
  or its story changed, so the page is built again more often than what it shows changes.
- The Log's read walks every layer of its keys at each read of the tail, for the sessions it wants
  the store to hold, where it could keep them as it takes lines in.
- A Log paused, or with a card open, while another tab shows holds every day from where it paused
  until it follows again, as it lets go only while it follows.
- The soak's guard never opens the page of a session last run before yesterday, whose points finish
  their last run and let its answers go: the page's tests hold it, the guard doesn't.
