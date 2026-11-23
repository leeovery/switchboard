# The readings history

The router records how full each window of each account is, a line each time a reading changes.
The lines go in `history/readings-<date>.jsonl` in the state directory, a file a day. For what a
window is, see [Limits and routing](../limits-and-routing.md).

```bash
cd ~/.local/state/switchboard/history
gzip -dc readings-2026-11-12.jsonl.gz \
  | head -1 | jq .
```

```json
{
  "at": "2026-11-12T09:30:02Z",
  "account": "b6s7",
  "window": "7d",
  "utilization": 0.42,
  "resets_at": "2026-11-16T18:00:00Z",
  "status": "allowed",
  "source": "answer",
  "reserve": 0.1
}
```

## The fields

| Field | Type | Meaning |
|---|---|---|
| `at` | time | When the reading was taken. |
| `account` | string | The account, by id only. |
| `window` | string | The key of the window, as the API gives it, such as `5h`, `7d` or `7d_oi`. |
| `utilization` | number | How much of the window is used, as a fraction. It can pass 1. |
| `resets_at` | time | When the window resets. Left out if the API didn't say. |
| `status` | string | `allowed`, `allowed_warning` or `rejected`. Left out if the API didn't say. |
| `source` | string | Where the reading came from: `answer`, `probe`, `prime` or `reserve`. |
| `reserve` | number | The reserve that applied to the account when the router took the reading, as a fraction. Left out of lines from older versions. |

## Where a reading comes from

| `source` | The reading came from |
|---|---|
| `answer` | The answer to a request the router routed |
| `probe` | A probe the router sent to read the account |
| `prime` | A prime that started the 5-hour window of the account |
| `reserve` | A change to the account's reserve, with its use as it was |

## The reserve on each line

Each line records the reserve that applied to the account when the router took the reading. The
day's summary counts the minutes the account spent at its cap by that reserve. So a reserve you
change today leaves yesterday's minutes as they were.

| If | `reserve` reads |
|---|---|
| The account has a reserve | That reserve, such as `0.1` |
| The account has no reserve | `0` |
| The pool's pin names the account | `0`, because a pin spends the reserve |
| The line is from an older version | Nothing: the field is left out |

A summary counts a line with no `reserve` by the reserve in your config.

If the reserve changes, the router adds a line with `source` `reserve` for each window that it
read in the last 15 minutes and that hasn't reset since. A window read longer ago records the new
reserve at its next reading.

`history --windows` prints these readings for each account in a pool. See
[`history` JSON](history-json.md).

How long the files stay is in [Where the files are](files.md).
