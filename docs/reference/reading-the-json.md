# Reading the JSON

These rules hold for every file switchboard writes and every JSON object its commands print. Read
them once before you write a script or an agent that parses any of it.

```bash
switchboard status --json | head -4
```

```json
{
  "schema": 1,
  "generated_at": "2026-11-15T09:30:00Z",
  "source": "router",
```

## Fields are added, never renamed

- A release can add a field to any format. It never renames one.
- While `schema` stays at 1, the meaning and units of a field never change.
- Pass over any field you don't know.

## The schema number

| What prints it | Where `schema` goes |
|---|---|
| `status`, `usage`, `history`, `sessions` and `pool`, with any flag | The first field of the object |
| `requests` and `events` | Put first on each stored line, before the fields of the line itself |
| The files in the state directory | Nowhere: the files hold no schema |

The number goes up only if a field changes its meaning or its units, or is removed. A new field
leaves it as it is.

## Accounts by id, with labels

Every format names an account by its id, such as `b6s7`, and never by its token. Most verb JSON
adds the label beside each id:

| Id field | Label beside it |
|---|---|
| `account` | `label` |
| `from` | `from_label` |
| `to` | `to_label` |
| A list of ids, such as `accounts` or `on` | A list of labels in the same order, such as `labels` or `on_labels` |

Some places give ids only:

- the files, and the stored lines that `requests` and `events` print;
- the status document, except in its `accounts`;
- the assignments of `status --session`;
- the days of `history`. Its top-level `removed` block labels the removed accounts they name.

A removed account is labelled by the label it showed last.

## Pools by id and label

Once the config has pools, the verb JSON names a pool by its id and its label.

| Where | How the pool shows |
|---|---|
| Most verb JSON | `pool` for the id, `pool_label` for the label |
| Lines that `requests` and `events` print | `pool_label` put after `"schema": 1`, beside the stored `pool` |
| The days of `history` | `pool_label` beside the `pool` of each account, and in each of its `pools` |
| The status document | `pool_id` and `pool_label`, because its `pool` field holds the room new sessions have |
| The `events` of the status document | The pool id only |
| The files | The pool id only |

A removed pool is labelled by the label it showed last, as the
[accounts record](accounts-record.md) keeps it.

## Text for people can change

The text a command prints for a person can change in any release. That covers a terminal,
`--pretty`, and any command run without `--json`. Read `--json` instead.

One line is kept stable. `status --session` prints the label of the account the session is on, on
a line of its own, for a statusline to read:

| If the account | The line reads |
|---|---|
| Is configured | Its label, such as `work` |
| Was removed since | The label it had and when it went, such as `spare (removed 1 Aug)` |
| Has no label anywhere | Its id |

`prefs.json` can change in any release too, and so can the files that
[Where the files are](files.md) marks as switchboard's own.

## Reasons are fixed words

The reasons a request went where it did, and why an account couldn't serve it, are fixed words.
[Routing reasons](reasons.md) lists them.
