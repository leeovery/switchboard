# Accounts

**Accounts** shows each account in depth, and every account together. It has three views on
sub-tabs, Compare, Detail and Tokens, and `[` and `]` move between them. With one account, Compare
goes, and `[` and `]` move between Detail and Tokens.

<div align="center">
<img src="../../art/accounts-compare.webp" alt="Accounts' Compare over this week: a column each for work, personal and side and one for all accounts, each with its week used, its use by model version, its last seven weeks each at its peak with the limits hit, and its worth at API prices by day" width="840">
</div>

## Compare

Compare has a column for each account, and one for `all accounts`. Each column shows:

- the account's week, as a bar
- its share of use by model version
- WEEKS: how full each of its weeks got, as columns, at least the last eight, or every week of
  the period where there are more. At 12 months a column is a month, its fullest week. A dotted
  line marks the cap of an account that keeps a reserve, and `…` marks this week, so far, except
  at 12 months, where a column is a month. Under
  them, how many of those weeks it hit its limit in and what they had left at a reset, on average:
  `limit hit 3 of 7 · 9% left at a reset`. `all accounts` says how many weeks some account was
  short: `short on 3 of 7`.
- WORTH: what its use was worth at API prices across the period, by hour, two hours, day, week
  or month, the finest that fits, and its peak, `peak Wed $148`. Today is by the hour its
  requests arrived in. The accounts share one scale, so their columns compare; `all accounts`
  has its own, and says so.
- its requests, and the sessions it served and moved
- its limits hit
- the time it spent at its cap or a limit, and what it had left at its last reset
- its plan and its extra usage
- what the plan costs against what its use was worth at API prices. Where an account has no
  `plan`, `all accounts` says how many of the plans it knows, as its worth is every account's.

An account whose token expires within 30 days, or has expired, says so under its state,
`token: 21 days left`.

Where the rows don't all fit, Compare scrolls them under the accounts' names with `j` and `k`.

`←` and `→` move a focus along the columns, scrolling them where they don't all fit. `enter`
opens the focused account in Detail.

Where two accounts' columns and `all accounts`' don't fit side by side, as on a phone, Compare
turns: a group at a time, an account a row. Turned, it has no focus, and its WEEKS and WORTH are
bands, a band an account: WEEKS its week columns, each peak under its column and its facts at the
right; WORTH its chart across the screen, with its peak under it.

## Detail

Detail shows one account, or all of them, chosen from a list at its left. It fills the screen,
and shows:

- the account's windows now
- the period's requests, limits, plan, worth and extra usage
- its use by model
- its weeks, as many as fit across the screen, never fewer than the period's own where they fit,
  and at least six, as its rule names them: `the last nine weeks`. At 12 months, the latest that
  fit, with how many more there are at the left, `‹ 38 more`.
  - Of an account, a day and a week at a time, and under them the facts on its whole weeks:
    `limit hit 3 of the last 7 weeks · 9% left at a reset, on average`. The verdict on how many
    accounts you need is `switchboard history`'s.
  - Of all accounts, a band of week columns an account,
    `all accounts` first, each week's peak under its column and its facts at the right; or, where
    Detail is stacked, as on a phone, or too narrow for a week of bands, a table, a row an
    account, its facts in two columns.
- WORTH: what its use was worth at API prices over the period, across the screen, and its peak
- its SESSIONS now
- its LATELY, every event the router reports, filling the screen down to the key line; the rest
  scrolls, as `▼ 12 more below` says.

An account whose token expires within 30 days, or has expired, says so on a line under its state,
with the command that makes a new one: `token expires 17 Nov 2026 · 21 days left — switchboard
setup makes a new one`.

Detail's keys are in [Keys on Accounts](#keys-on-accounts). The chosen row is lit only where `↑`
and `↓` act, in the list or in the sessions. The other's chosen row stays bold, marked `▸`, and
`SESSIONS` is bright while you're in them. The list widens to fit the accounts' names where the
screen has room, as USAGE's name column does.

## Tokens

Tokens is ccusage's report inside switchboard, over the period. It shows the tokens by kind, and
their worth at API prices against what the plans cost.

- The whole period's row comes first, named for it, `this week`, its figures its rows' summed and
  its worth against what the plans cost over the period, `4.6× its $138 week`, as Compare has it.
  A rule sets it off from the rows under it.
- Then a row a day over this week and 30 days, a week over 12 weeks, or a month over 12 months,
  newest first, each against what the plans cost over it, `5.6× its $20 day`: a whole day's,
  week's or month's price, but for one that begins part-way through, as the oldest month at 12
  months or the one the ledger began in, priced by its days from then. Today is the period's row
  alone.
- `v` splits each row by account, by model, by both, or not at all.
- The rows fill the screen down to the footnote, and `j` and `k` scroll them a row at a time.
- On a narrower screen, the columns give way one at a time: the model count first, then cache
  write, input, output, the worth against the price and cache read. Total and worth always show,
  and the names stay whole until every other column has gone.
- An account you've removed keeps its rows while the period holds its days, by the label it had,
  in faint text, so it's told apart from an account you've given the same label since.

## The period

`<` and `>` step to a shorter or a longer period, which every view counts over. The periods are
today, this week, 30 days, 12 weeks and 12 months. They're never billing months: today and this
week run from their starts, 30 days and 12 months back from today, and 12 weeks over this week and
the 11 before, and 12 months are the days Trends' YEAR counts as its last 12 months. It's one dial for the dashboard,
remembered from one run to the next, and Projects and Sessions' List count over it too.

## Keys on Accounts

| Key | Does |
|---|---|
| `[` `]` | Moves between Compare, Detail and Tokens. |
| `<` `>` | Steps to a shorter or a longer period. See [The period](#the-period). |
| `←` `→` | On Compare, moves the focus along the columns. |
| `enter` | On Compare, opens the focused account in Detail. |
| `↑` `↓` | In Detail, choose the account in the list, `all accounts` first. |
| `→` `←` | In Detail, `→` moves into the chosen account's sessions, and `←` goes back to the list. |
| `↑` `↓`, then `enter` | In Detail's sessions, `↑` and `↓` choose a session, and `enter` opens its page. The screen scrolls to keep the sessions and the chosen one in view. |
| `v` | On Tokens, splits each row by account, by model, by both, or not at all. |
| `j` `k` | Scroll Compare's rows, or Tokens' rows, a row at a time. |
