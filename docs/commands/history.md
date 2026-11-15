# `switchboard history`: what your use was worth

`history` sums up each day of your record, prices it at API prices, and sets that against what your
plans cost. It then judges whether you have as many accounts as your use needs. It covers the last
30 days unless `--since` says otherwise, and works without the router.

```console
$ switchboard history --since 2026-09-14
Tue 6 Oct 2026
  personal  claude-opus-5-5  1 request  20k tokens  1 session  $0.02
  personal: 1 session  ·  1 moved on  ·  highest: 5-hour 10%
  work: no sessions  ·  1 moved off  ·  limits: 5-hour at 22:14  ·  highest: 5-hour 100%
…
Weeks: two is one more than you need
  as now: short on 1 of 3 weeks  ·  with one fewer: short on 1  ·  with one more: short on none
  work: ends most weeks with room  ·  limit hit 1 of 3 weeks  ·  30% left at a reset, on average
  personal: barely used  ·  limit hit 0 of 3 weeks  ·  80% left at a reset, on average
  all accounts: room to spare together  ·  55% left at a reset, on average
  work is the one to drop
```

```bash
switchboard history [--since <when>] \
  [--windows] [--pool <id>] \
  [--json | --pretty]
```

| Flag | Default | What it does |
|---|---|---|
| `--since <when>` | The last 30 days | Starts on the day of `<when>`: a day (`2026-10-01`), a time today (`14:00`), or a while ago (`3h`, `2d`). |
| `--windows` | Off | Prints every reading of every window over those days, in place of the days. |
| `--pool <id>` | The only pool, else the unfenced one | Sums up the work of that pool. See below. |
| `--json` | Off a terminal | Prints the blocks as JSON, even on a terminal. |
| `--pretty` | On a terminal | Prints the text, even off a terminal. |

What worth means, and why it matters, is in [Your record](../your-record.md). The same
numbers show on the History tab of the [dashboard](../dashboard.md).

## The days

Each day has a row for each account and model.

| Column | What it shows |
|---|---|
| Requests | The requests the account served with the model. |
| Tokens | Their tokens. |
| Sessions | Their sessions. |
| Worth | What they'd have cost through the API, at the prices switchboard carries or those in `[prices]`. |

Under the rows come notes for each account. They give its sessions, moves and limits, any week
reset (such as `week reset 10:00`), and the highest use of each window. An account that
served no request gets notes only for a limit or a week reset. If no day has requests, `history`
says so, then shows those notes without totals.

## What counts

| Rule | What it means |
|---|---|
| Requests are messages | Both those sent to Anthropic and those the router answered itself. |
| Quota checks and token counts aren't requests | Claude Code sends them, and they cost next to nothing. A model only they named counts as unused. |
| A quota check is worth nothing | Its tokens are left out of the day. A day summed up before this rule keeps the few its checks spent. |
| Today is summed from its lines | It covers the day as far as it has gone. |
| A model with no price | Shows as unpriced, never free. |
| What a day never counted | Is left out, never taken for none. For example, a day recorded before the ledger counted resets. |

## Plan prices

Each account has a monthly price, set by its `plan` in the config. `history` prorates that price the
same way the dashboard does.

| Period | Priced at |
|---|---|
| A day | 12/365 of a month |
| A week | 12/52 of a month |
| 30 days | A month |
| 12 weeks | 12 × 12/52 of a month |
| 12 months | 12 months, whether they hold 365 days or 366 |

| If | Then |
|---|---|
| A period is still running | It's priced whole. |
| A month or week begins part-way into the days asked for | It's priced by its days from then. |
| The days asked for start before the ledger does | The plans are priced from the first day the ledger holds. |
| An account has no `plan` | It has no cost and no ratio. |
| Some accounts have no `plan` | The totals price only those with one, and say how many, such as `4.4× its $92 · 2 of 3 plans known`. |

## Over all the days

Under the days, `history` shows totals over all of them:

- a column for each account, and one for every account together. Each column gives requests,
  sessions, moves, limits, time at a cap, worth against the plan, and use by model;
- the peak of each week, for each account;
- whether you have as many accounts as your use needs.

### Weeks, and when a week is short

To judge your accounts, `history` looks at the weeks of each account. A week runs from one reset to
the next.

| Term | What it means |
|---|---|
| Peak | The use just before the reset that ended the week. A reset you made by hand counts. |
| Placing | A week goes under the calendar week it began in, up to half-way in, or else the next. |
| The running week | Always this week. Its peak is its highest so far, read from before the days asked for if its week began earlier. |
| A short week | A week in which some account hit its cap: 100% less its `reserve`, at the reserve it has now. |
| A whole week | One in which every account's week is known and has ended. Only whole weeks are judged. |

`week_starts` in the config sets the day calendar weeks begin on.

### The replay

Each whole week is replayed twice, to see what another account, or one fewer, would change:

- **With one fewer:** the dropped account's use is shared out among the rest, by the sizes
  of their plans.
- **With one more:** the new account has the plan most of your accounts have, and every peak falls
  in proportion.

An account without a `plan` counts as the plan most accounts have. If none has one, each counts as
one.

### The verdict

The verdict is the first row that holds:

| Verdict | When |
|---|---|
| `one too few` | 4 or more of every 7 weeks are short, and one more account would at least halve them. |
| `too few` | As many weeks are short, and one more account wouldn't halve them. It says how many would stay short. |
| `one more than you need` | Fewer than 4 of every 7 weeks are short, and one fewer account would add no short week. It names the account to drop. |
| `about right` | None of the above. |

With three accounts or more, `one more than you need` also says `with two fewer:`. That's how many
weeks would be short without the account it names and the next it would drop. If no whole week can be judged yet, `history` says
`Weeks: not enough weeks yet`.

Each account also gets a verdict over its own whole weeks, the first row that holds:

| Account verdict | When |
|---|---|
| `could go` | The verdict is `one more than you need`, and the rest cover this account. See below. |
| `barely used` | It had 75% or more left at a reset, on average. |
| `hits its limit most weeks` | It hit its limit in 3 or more of every 7 weeks. |
| `plenty to spare most weeks` | It had 40% or more left at a reset, on average. |
| `ends most weeks with room` | None of the above. |

An account `could go` only if it hit no limit and used 10% or less on average. What a week had
left is measured to its limit, not its cap. Every account together gets one more
verdict:

| Verdict on every account | When |
|---|---|
| `room to spare together` | A quarter or more was left at a reset, on average. |
| `little to spare together` | Otherwise. |

## Windows

`--windows` prints, in place of the days, a line each time a window changed its use, its reset or
its status.

```console
$ switchboard history --windows \
    --since 10:00
work  Week
  Wed 7 Oct 09:00  41%  resets Mon 12 Oct 10:00  allowed  answer

work  5-hour
  Wed 7 Oct 09:00  30%  resets Wed 7 Oct 14:00  allowed  answer
```

## History in a pool

`--pool` reads one pool's work, by the pool each request was routed in at the time. Work from
before pools counts as the first pool. It takes the windows, plans and weeks from the accounts in
the pool now.

- The days, months and tokens start on the first day of work in the pool. The plans are priced
  from then too, as on the dashboard.
- A removed pool, or one an import brought, is read by its id too.
- Once you have a second pool, the text names the pool it read, such as `pool Acme`.

## Good to know

- The JSON holds the day the prices were read, then the days, the year, months, weeks, tokens,
  totals, plans and capacity. Every field is in [Reference](../reference/history-json.md).
- An account removed shows as `team (removed 4 Sep)` in the text, and by its id in the JSON.
- Just after an upgrade that moves your record, `history` refuses until the router has moved it.

## Examples

```bash
switchboard history --since 7d
# today, as far as it has gone
switchboard history --json | jq '.days[-1]'
# whether you have as many accounts as you need
switchboard history --json | jq '.capacity'
# every reading of every window today
switchboard history --windows --since 14:00
```

## Related

- [Your record](../your-record.md): what the record holds, and what worth means.
- [Configuration](../configuration.md): `plan`, `reserve`, `week_starts` and `[prices]`.
- [`sessions`](sessions.md): the sessions of a day, and what each was worth.
