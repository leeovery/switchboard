# Two of the design's widths could follow their parts

Found on 23 October 2026, at the end of the fluid layout pass, while checking which widths the
dashboard still compares with a figure of its own.

## What happens

Every view now lays itself out by its parts, and shifts to a narrower arrangement only where its
wider one's parts don't fit. A few figures stay, because the design decides them. Two of those
are arrangements rather than sizes:

- **Beside USAGE, from 150 columns.** By account, GRAPHS' charts and ROUTING sit beside each
  account's block from 150 columns (`besideWidth`), with the bars at their 50 cells
  (`besideBar`). Under 150 they take sections of their own.
- **usage's words in full, from 70 columns.** Where USAGE's bars take rows of their own (Detail,
  the phone's stack, the Switchboard's lines, the Runway pool), the words beside a bar are said
  in full from 70 columns (`fullWords`), and short under it.

## Why it matters

Both switch at a width, whatever the names, the windows or the accounts need. With short names,
the charts would fit beside the blocks well under 150; with long ones, the full words might not
fit at 70. Everywhere else the dashboard now switches where its parts stop fitting, so these
two are the exceptions a reader can't predict from what's on screen.

## Ways it could go

- **Beside as an arrangement flex judges.** The blocks at their least (names, the bar's least,
  what follows it, the words) beside the charts at theirs, the bar's natural 50. GRAPHS and
  ROUTING would take one judgement between them. No frame lies between 108 and 149 by account
  today, so the frames would hold, but live drawing between about 110 and 149 would change.
- **The words as a part.** Their natural the full form, leaving to the short one, decided row by
  row. Detail on a phone, and the Switchboard at 108, would say more where their rows have room.
- Leave both as the design has them.

Not decided; to discuss.
