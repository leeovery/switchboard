# `history` JSON

`history --json` prints the days of the ledger in a pool, priced, and the blocks built over them.
On the dashboard, Trends shows the same year and months. Accounts shows the weeks and the tokens, over the period you choose.
For what the record is worth to you, see [Your record](../your-record.md).

```bash
switchboard history --json | jq \
  '{prices_as_of, days: (.days | length),
    verdict: .capacity.verdict}'
```

```json
{
  "prices_as_of": "2026-11-01",
  "days": 30,
  "verdict": "about right"
}
```

## The top level

| Field | Type | Meaning |
|---|---|---|
| `schema` | number | `1`. See [Reading the JSON](reading-the-json.md). |
| `prices_as_of` | string | The date the prices were read. |
| `prices_from_config` | boolean | Set if `[prices]` in the config prices a model or a plan. |
| `days` | list of objects | The summary of each day, priced. See below. |
| `year` | object | The grid that Trends shows as YEAR. |
| `months` | list of objects | A row for each month. |
| `weeks` | list of objects | The peak use of each account, week by week. |
| `tokens` | list of objects | The tokens of each week. |
| `totals` | object | The totals of each account. |
| `plans` | list of objects | What each plan cost against what it was worth. |
| `capacity` | object | Whether you have the right number of accounts. |
| `removed` | list of objects | The removed accounts the days name. |
| `moved` | list of objects | The accounts the days name that are in another pool now: each one's `id` and `label`, and that pool's `pool` and `pool_label`. |

Each block covers the days asked for, since the ledger began, oldest first, with today last. The
blocks from `weeks` on are on [a page of their own](history-accounts-json.md). The `[prices]` keys
are in [Configuration](../configuration.md).

## One pool at a time

Once the config has pools, the history is of one pool:

- the work each request recorded in the pool at the time;
- the work from before pools, counted in the first pool;
- the windows, plans and weeks of the accounts in the pool now.

The windows of an account are its own, whatever pool it's in.

| Blocks | Begin at |
|---|---|
| `days`, `year`, `months`, `tokens`, `removed` and `moved` | The first day of work in the pool, if that's after the days asked for begin |
| `weeks`, `totals` and `plans` | The first day asked for |

## `days`

Each day is its [day summary](day-summary.md), with these added:

| Field | Type | Meaning |
|---|---|---|
| `worth` | number | On each model entry, its worth in US dollars. |
| `unpriced` | list of strings | On each model entry, what the worth leaves out. See Worth, below. |
| `by_family` | list of objects | The worth of the day by model family, as `{family, worth}`. |
| `pool_label` | string | Beside the `pool` of each account, and in each of its `pools`. |

- **`by_family`** counts only models used. A model with only quota checks or counts of tokens
  isn't one.
- **The models in each of `pools`** are priced as the models of the whole day are.
- **`lines` and `bytes` are left out** if the files of the day may hold the work of more than one
  pool. That's the case with two pools or more, or if the accounts record keeps a pool the config
  doesn't.

A day with no requests has no `accounts` if the readings history tells of no account. An account
has a day if its quota checks, its counts of tokens or the readings history give it one.

## `year`

| Field | Type | Meaning |
|---|---|---|
| `days` | list of objects | Each day, as `{day, requests, level}`. |
| `cuts` | list of numbers | The requests a day where each level begins. They're the quarters of the active days. |

`level` runs from 0 to 4: the shade the day is drawn at.

## `months`

| Field | Type | Meaning |
|---|---|---|
| `month` | string | The month. |
| `requests` | number | The requests of the month. |
| `worth` and `unpriced` | number, list | What the requests were worth. |
| `limits` | object | The limits reached, by window. |
| `by_family` | list of objects | The worth of the month by model family. |
| `cost` | number | The cost of the plans of the month, in US dollars. |
| `against` | number | The worth of the accounts against that cost, as a ratio. |
| `plans` | object | How many plans are known, as `{known, of}`. |

- **`cost`** is the whole month. If the days asked for begin partway through, it runs from the
  first day asked for. It starts later if the ledger, or the work in the pool, began later.
- **`against`** is left out for the month so far.

## Worth

Every `worth` is a number in US dollars, exact to the picodollar. It's left out if nothing was
priced. Every share is exact too.

`unpriced` names what a worth leaves out:

| Entry | What it leaves out |
|---|---|
| A path in the usage | A count the price table can't price. |
| `no_usage` | Requests that got no usage back. |
| The id of a model | Requests of a model the table doesn't know. |
| `inference_geo:<geo>`, such as `inference_geo:eu` | Requests of a known model, in a region the table doesn't price it in. |
| `no_model` | Requests that named no model. |
| `too_large` | A worth past some $9.2 million, the most it counts. None of it is priced, and it never wraps round. |

`too_large` can be the worth of a request, of a day, or of a sum of them.

## With `--windows`

`history --windows --json` prints the readings of each window of each account in the pool:

```json
{
  "schema": 1,
  "windows": [
    {
      "account": "b6s7",
      "label": "work",
      "window": "7d",
      "readings": [
        {
          "at": "…",
          "utilization": 0.42,
          "resets_at": "…",
          "status": "allowed",
          "source": "answer"
        }
      ]
    }
  ]
}
```

Each reading has the fields of the [readings history](readings.md), less `account` and `window`.
