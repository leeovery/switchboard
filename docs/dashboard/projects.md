# Projects

**Projects** shows where the use went: each project the period's sessions ran in, and the one
chosen in full under the list. A project is the repository a session ran in. A session in a
worktree or a subdirectory counts as its repository's, and one started outside any repository as
its own directory's. Each is named by its folder, its parent too where two share a name. The
sessions that didn't start through switchboard's `claude` name no directory, and share a
`no directory` row.

The list ranks the projects by their worth at API prices, the greatest first. Each row has:

- its name
- `live`, its sessions running now
- its sessions, and its worth
- a bar of its worth against the top project's, split by model family in the families' colours,
  as Trends' BY MONTH splits a month's
- its requests
- `last`, when its last request came: `now` while one of its sessions is busy.

`all projects` totals them under a rule, a session counted once. The list takes up to a third of
the view's rows, and scrolls with the choice past them. As the width runs short, the bar narrows to
8 cells, then requests and `last` leave, and then the name is cut.

Under the list, the chosen project shows:

- its name, its path and its sessions running
- its numbers over the period: its sessions, requests and worth, its share of every project's
  worth, and its tokens
- WORTH BY, its worth in the finest bucket that fits: today by hour where a column an hour fits,
  else by two hours; this week by day; 30 days and 12 weeks by day where a column a day fits,
  else by week; and 12 months by week where a column a week fits, else by month, the peak named
  under it
- BY MODEL, each version's share of its worth, those under 1% left out
- ON ACCOUNTS, each account's share, the greatest first
- SESSIONS, its sessions over the period in the List's columns: those running first, then the rest
  under a rule for the day they ended, `ended today`, `ended yesterday`, a weekday within the week
  or a date before it. Each starts at its first request, with its day where it began on another.

Wide, WORTH BY sits in the left half, with BY MODEL over ON ACCOUNTS in the right. Narrower, WORTH
BY runs across the width with the two side by side under it, and on a phone they stack. Where rows
run short, ON ACCOUNTS leaves first, then BY MODEL, then WORTH BY, so SESSIONS keeps three rows at
least.

The chosen row is lit only where `↑` and `↓` act, in the list or in the sessions. The other's chosen
row stays bold, marked `▸`.

## Keys on Projects

| Key | Does |
|---|---|
| `↑` `↓` | Choose the project. |
| `→` `←` | `→` moves into the chosen project's sessions, and `←` goes back to the list. |
| `↑` `↓`, then `enter` | In the sessions, `↑` and `↓` choose a session, and `enter` opens its page. |
| `<` `>` | Step the period, shared with Accounts and Sessions' List. See the period on [Accounts](accounts.md). |
