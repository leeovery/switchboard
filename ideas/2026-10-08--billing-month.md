# A billing month

Accounts' and History's periods are rolling: this week, from the day `week_starts` names, the last
30 days, the last 12 months. Switchboard doesn't know when an account's subscription renews, so it
can't show the month the account is billed for: Accounts prices a plan against a rolling 30 days,
but never against the month its bill covers.

A `renews` day on each account in the config, the day of the month its subscription renews, would
let Accounts step through billing months beside the rolling periods, each account's own, and price
its plan against the month it pays for.

To settle with the owner:

- Where the day lives: on each account, beside `plan`, or read from Anthropic, should it say.
- How Compare shows accounts whose months start on different days side by side, and what
  `all accounts` sums over.

Later, after milestone 7.
