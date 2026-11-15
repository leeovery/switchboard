# On a phone: the whole dashboard in 52 columns

The dashboard reads in a phone's terminal as it does in any narrow window. Every tab lays itself
out by what fits, at every width. Each part narrows as far as it still reads, and a tab takes a
narrower arrangement only if its wider one doesn't fit.

<div align="center">
<img src="../../art/phone.webp" alt="The Overview in a phone's terminal, 52 columns wide" width="260">
</div>

*The Overview in a phone's terminal, 52 columns wide.*

## The frame under 100 columns

Only the frame around the page switches at one width. Under 100 columns, it takes its phone form:

- The tabs sit on a row of their own, under the clock. If one row can't hold their names, they take
  as many rows as they need, split as evenly as their order allows.
- The status line, the heading row and the key line say less.

## Each tab on a phone

What a phone shows follows from its width. Each of these happens at that width in any window:

- **Overview, USAGE:** the windows sit side by side if every part of a row fits at its least. With
  two windows and names such as `personal`, that's from 84 columns. Narrower, they stack. As the
  room runs short, the parts give way in turn:
  1. the bars, down to 12 cells;
  2. the words, to their short form;
  3. the name column.
- **Overview, ROUTING and SESSIONS:** each account and each session takes two rows. That happens
  wherever one row doesn't fit: under 83 columns, with a name column of 16.
- **Overview, LATELY:** it comes under COMING UP, as it does wherever the two don't fit side by
  side, under 95 columns.
- **Runway:** COMING UP and LATELY stack, and give way in turn as the rows run short, for example
  with the keyboard up. See [Runway](runway.md).
- **Trends:** the cards stack, one to a row. If rows run short, each card puts its words beside
  its label. See [Trends](trends.md).
- **Projects:** the sections stack: WORTH BY, then BY MODEL, then ON ACCOUNTS. The list and
  SESSIONS give way column by column. See [Projects](projects.md).
- **Accounts:** `against it` and `against` give only the ratio for `all accounts`. If an account
  has no `plan`, they add how many of the plans it's over, compact: `4.4× · 2/3`.
- **The session page:** it keeps the number, start, length, requests, worth and accounts of each
  turn at every width. See [the session page](session-page.md).

On the session page, as the width runs short, these leave in turn:

1. the tools of each turn;
2. `↑ asking`;
3. read, written and out;
4. the requests on each account, then its time there, before its windows.

The rows about now take their short form.

## Related

- [Every page](every-page.md): the status line, the key line and the keys every tab shares.
- [Running it](running-it.md): one pool at a time, and what the dashboard remembers.
