# Accounts in the status document

Each entry in `accounts` of the [status document](status-json.md) is one account in the pool, in
the order of the config. It says how full each window is, and what keeps the account back.

```bash
switchboard status --json | jq \
  '.accounts[0] | {id, label, plan, token_set}'
```

```json
{
  "id": "b6s7",
  "label": "work",
  "plan": "max",
  "token_set": true
}
```

## The account

| Field | Type | Meaning |
|---|---|---|
| `id` | string | The id of the account. |
| `label` | string | Its label. |
| `primary` | boolean | Set on the [primary](../glossary.md). |
| `reserve` | number | Its reserve, as a fraction. |
| `plan` | string | `pro`, `max` or `ultra`, as configured. Left out if unknown. |
| `fetched_at` | time | When its usage was read. |
| `error` | string | Why its usage couldn't be read, or what puts its token right. |
| `refused_read` | string | Why Anthropic refused its last read: `subscription_ended` or `token_refused`. |

`subscription_ended` means Anthropic refused the login of an ended subscription. `token_refused`
means Anthropic refused the token. What that means for routing turns on where the document comes
from:

| If the document comes from | `refused_read` means |
|---|---|
| The router | The router sends the account nothing until a read comes back clean, unless Anthropic refused the reads of every account in the pool. |
| Probing, with no router running | The probe was refused. The router puts the account on a [hold](../glossary.md) once it reads the account the same way. |

See [Troubleshooting: an account's token or subscription](../troubleshooting/tokens.md).

## The token

| Field | Type | Meaning |
|---|---|---|
| `token_set` | boolean | Whether its token file is there and usable. |
| `token_foreign` | boolean | Set if its token is of another subscription. From the router only. |
| `token_taken` | time | When switchboard took its token. Left out if unknown. |
| `token_good_until` | time | When the token year is up, 365 days after `token_taken`. Left out if unknown. |

If `token_foreign` is set, the account takes no requests until it has its own token. `token_set`
is then `false`, and `error` says what puts it right. See [Tokens](tokens.md).

## The windows

| Field | Type | Meaning |
|---|---|---|
| `windows` | list of objects | Each window of the account. See below. |
| `lapsed` | list of strings | The keys of the windows that have lapsed. |
| `at_reserve` | list of strings | The keys of the windows at the reserve, short of their limits. |
| `extra_usage` | object | The extra usage of the account, as `{status, utilization, resets_at}`. |
| `failures` | list of objects | Each window that couldn't be read, as `{label, window, error}`. |

Each key in `lapsed` names a [lapsed](../glossary.md) window.

Each window holds these:

| Field | Type | Meaning |
|---|---|---|
| `key` | string | The key of the window, such as `5h`, `7d` or `7d_oi`. |
| `label` | string | The name of the window for a person. |
| `utilization` | number | How much is used, as a fraction. |
| `resets_at` | time | When it resets. Left out if unknown. |
| `status` | string | `allowed`, `allowed_warning` or `rejected`. |
| `restarted_at` | time | When it started again before its reset, if a reset by hand started it. |
| `pace` | number | Its even pace: where even use would have put it by now. |
| `allowance` | object | What can be spent and still last to the reset, as `{share, per}`. |

- **`per`** is `hour` for a window of a day or less, and `day` for a longer one. If the reset comes
  within one of those, `per` is left out and `share` is the total.
- **`allowance` is left out** if the window has no room left. A limit or the cap can also keep an
  account from every request. Then every window of that account leaves `allowance` out.
- **A verified Ultra account** has no `5h` window.

## What the router adds

These come from the router only. A document built by probing leaves them out.

| Field | Type | Meaning |
|---|---|---|
| `limit` | object | A limit in force on the account, as `{id, windows, until}`. |
| `refused` | object | A refusal in force on the account, as `{until, status, family}`. |
| `pressure` | object | How fast the pressure window is used, and where it's heading. |
| `rates` | list of objects | How fast each window has been used lately. |
| `sessions` | number | How many sessions went to the account in the last hour. |

- **`limit.windows`** names the windows reached. If it names none, the limit keeps back every
  request.
- **`refused.family`** is set if the refusal keeps back only the requests of one model family.
- **`pressure`** holds `window`, `rate`, `recent`, `since`, `runs_out` and `under`. The account is
  under pressure while `under` is set: the window runs out before it resets.

Each entry of `rates` holds these:

| Field | Type | Meaning |
|---|---|---|
| `window` | string | The key of the window. |
| `rate` | number | Its recent rate, as a share of the window an hour. The 5-hour run-out goes by it. |
| `since` | time | When the recent rate is measured from. |
| `fast` | number | Its fast rate, which the week forecast goes by. Left out at 0. |

An entry without `since` gives a fast rate only. How the router judges pressure is in
[Limits and routing](../limits-and-routing.md).
