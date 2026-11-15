# `history` JSON: the account blocks

The blocks of [`history` JSON](history-json.md) that look at each account: `weeks`, `tokens`,
`totals`, `plans`, `capacity`, `removed` and `moved`. They answer how hard each account worked,
what it was worth, and whether you have the right number of accounts.

```bash
switchboard history --json | jq -c \
  '.totals.accounts[]
  | {account, all, requests}'
```

```json
{"account":"b6s7","all":null,"requests":5120}
{"account":"q4tx","all":null,"requests":3911}
{"account":null,"all":true,"requests":9031}
```

`jq` gives `null` for a field the entry leaves out. The last entry has no `account`, and has
`all` set.

## Per account, then all together

Most of these blocks hold one entry for each account, in the order of the config, then one more
for every account together. Each account entry has `account` and `label`. The last entry has
`all: true` in their place.

The blocks don't always hold the same accounts. In a pool, `totals` can also hold an account that
is in another pool now, which `weeks`, `plans` and `capacity` leave out. To match entries across
blocks, match them by `account`, not by their place in the list.

Every `worth` here is in US dollars, with `unpriced` beside it, as
[`history` JSON](history-json.md) explains under Worth.

## `weeks`

Each entry is a calendar week, `{week, accounts}`. A week starts on the day `week_starts` names in
the config, Monday by default.

| Field | Type | Meaning |
|---|---|---|
| `peaks` | object | The highest use of each window a week long, by key. |
| `open` | list of strings | The windows whose week is still running. Their peak is the peak so far. |
| `limits` | object | The limits the account reached that week, by window. |

- **`peaks`** covers the week of the account, and the week of each model that has its own, such as
  Fable.
- **The `all` entry** gives the mean of the peaks of the accounts, and no `limits`.

## `tokens`

Each entry is a calendar week:

| Field | Type | Meaning |
|---|---|---|
| `week` | string | The first day of the week. |
| `input`, `output`, `cache_write`, `cache_read`, `total` | number | The tokens of the week, by kind. |
| `worth` | number | What the tokens were worth. |
| `cost`, `against`, `plans` | number, number, object | What the plans cost that week, the worth against it, and how many plans are known. |
| `by_account` | list of objects | The same counts for each account, in the order of the config. |
| `by_model` | list of objects | The same counts for each model version. |

- **Each entry in `by_account`** has `account` and `label`, and its own `by_model`.
- **Each entry in `by_model`** names the version by its id as `model`. At the week level, it adds
  `on`, the accounts it ran on, the one it was worth most on first, and `on_labels`.
- **A model with only quota checks or counts of tokens** wasn't used. It's in neither list, nor is
  an account that sent only those.

## `totals`

`totals` holds `accounts`, the list of entries.

| Field | Type | Meaning |
|---|---|---|
| `requests` | number | The requests. |
| `sessions` | number | The sessions, each counted once however many days and accounts it ran on. |
| `moved_on`, `moved_off` | number | The sessions moved onto the account, and off it. |
| `limits` | object | The limits reached, by window. |
| `minutes_at_cap`, `minutes_at_limit` | number | The minutes spent at the cap, and at a limit. |
| `left_at_reset` | object | The share left in each window at its last reset. |
| `ran_out` | object | The day each window last reached its limit, in the run that its last reset ended. |
| `worth` | number | What the requests were worth. |
| `by_model` | list of objects | Each model version, as `{model, share, worth}`. |
| `busiest_day` | string | The day with the most requests. |
| `longest_run` | number | The most days in a row with any request. |
| `partial` | list of strings | The counts that some of the days never counted. |

- **`share`** is the part of the use of the account, weighed at API prices. Only versions used are
  listed.
- **The `all` entry** gives no `left_at_reset` or `ran_out`.
- **An account in another pool now** that worked in the pool read has an entry of its own, after
  the pool's accounts and before `all`.
- **The `all` entry** also counts work with no entry of its own: that of removed accounts, and the
  requests no account answered.

## `plans`

| Field | Type | Meaning |
|---|---|---|
| `plan` | string | The plan, as the config names it. |
| `price` | number | Its price a month. |
| `cost` | number | What the plan cost over the days. |
| `against` | number | The worth of the account against that cost. |
| `worth` | number | The worth of the account. |

- **An account on a plan that isn't known** gives only its `worth`.
- **If the ledger began after the first day asked for,** the plan is priced from the first day of
  the ledger. There's no worth before it to set against it.
- **The `all` entry** covers the accounts on a known plan, and adds `plans`, as
  `{"known": 2, "of": 3}`. Its cost and its worth against it are of those accounts only.

## `capacity`

`capacity` judges the whole weeks in which the week of every account is known and has ended.

| Field | Type | Meaning |
|---|---|---|
| `weeks` | number | How many whole weeks it judges. |
| `short` | number | How many of them were short. |
| `with_one_fewer` | number | How many would have been short with one account fewer. |
| `with_one_more` | number | How many would have been short with one account more. |
| `verdict` | string | `one too few`, `too few`, `one more than you need` or `about right`. |
| `drop` and `drop_label` | string | The account to drop, if the verdict is `one more than you need`. |
| `with_two_fewer` | number | How many weeks are short without `drop` and one more account. |
| `accounts` | list of objects | The verdict on each account, then all together. |

- **`with_one_fewer`** drops the account whose loss makes the fewest weeks short. It's left out
  with one account.
- **`with_two_fewer`** is given only with three accounts or more, if the verdict is
  `one more than you need`. The second account dropped is the one whose loss then makes the fewest
  weeks short.

Each entry in `capacity.accounts` judges the whole weeks of that account:

| Field | Type | Meaning |
|---|---|---|
| `verdict` | string | The verdict on the account. See below. |
| `weeks` | number | How many whole weeks the account has. |
| `limits` | number | The weeks it hit its limit in, its peak at its cap. |
| `left_at_reset` | number | What it had left at a reset, on average: left of its limit, not of its cap. |
| `without_it` | number | How many weeks the rest would have been short without it. |

On an account, `verdict` is one of these:

- `could go`
- `barely used`
- `hits its limit most weeks`
- `plenty to spare most weeks`
- `ends most weeks with room`

On the `all` entry, it's `room to spare together` or `little to spare together`.

## `removed`

Each removed account that the days name, as the [accounts record](accounts-record.md) keeps it:

| Field | Type | Meaning |
|---|---|---|
| `id` | string | Its id. |
| `label` | string | The label it showed last. |
| `removed` | time | When it was removed. |

An account that only another Mac configures, never removed here, has no `removed`. The list is
empty if the days name no removed account.

## `moved`

Each account the days name that the config has in another pool than the one read, as one moved
there since:

| Field | Type | Meaning |
|---|---|---|
| `id` | string | Its id. |
| `label` | string | Its label, or its id if it has none, as in its `totals` entry. |
| `pool` | string | The id of the pool it's in now. |
| `pool_label` | string | That pool's label. |

The list is empty if the days name no such account, and always is without pools.
