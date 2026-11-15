# `switchboard pool`: list, make, edit and remove pools

`switchboard pool` lists your pools, in the order of the config. Its subcommands make a pool, edit
one and remove one. For what a pool is and when you'd want one, see [Pools](../pools.md).

```bash
switchboard pool
```

```text
ID    LABEL    DIRS         ACCOUNTS
b2n8  Default  -            b6s7 (primary), q4tx
q7mt  Acme     ~/Code/acme  k3f9 (primary), m4zq
```

Each row shows the id and label of the pool, its folders, and its accounts by id, with the primary
marked. A `-` in DIRS marks the unfenced pool.

## Usage and flags

```bash
switchboard pool [--json | --pretty]
switchboard pool add <label> [<dir>...]
switchboard pool edit <id> [--label <label>] [--dirs <dir>]... [--unfence]
switchboard pool remove <id>
```

| Flag | Default | What it does |
|---|---|---|
| `--label <label>` | | `edit`: shows the pool as `<label>`. |
| `--dirs <dir>` | | `edit`: fences the pool to the folders given, one for each `--dirs`, in place of the folders it has. |
| `--unfence` | | `edit`: takes the folders of the pool away, making it the unfenced pool. Not with `--dirs`. |
| `--json` | off a terminal | Prints the pools as JSON, even on a terminal. The fields are in [Reference](../reference/pool-json.md). |
| `--pretty` | on a terminal | Prints the text, even off a terminal. |

## `add`: make a pool

`add` writes a `[[pool]]` table to the config, under an id switchboard makes, and says so:

```text
added pool "Acme" as q7mt, fenced to ~/Code/acme; sessions running in its directories keep the pool they started in, and its accounts, till they're resumed
```

Give each folder as an absolute path, or one starting `~/`. A fence covers everything beneath the
folder, and every git worktree of a repository there, wherever the worktree lives. With no
folders, the pool is the unfenced pool, which takes every session started outside all the fences.

`add` refuses a pool if:

- another pool has the same label, ignoring case;
- it would be a second unfenced pool, and `add` names the one there is;
- its folders are the same as, inside, or around the folders of another pool;
- the config names no pools yet.

The router writes the first pool into the config, as it moves your history to pools after an
upgrade. You add pools after that. If you put back a config from before pools, the router gives the
pools back to it.

A new pool has no accounts. Until you add one, a session started in its folders refuses to start.
Add an account with `accounts add --pool`, or move one in with `accounts edit --pool`. See
[Accounts](accounts.md).

## `edit`: rename or fence a pool

`edit` changes the label or the folders of a pool. Give one or more of `--label`, `--dirs` and
`--unfence`, and what you don't give stays as it is. It says what it changed, with each value as it
is now and as it was. If every value given is what the pool has already, it says so and writes
nothing.

- **The label and folders** are refused for the same reasons as on `add`.
- **`--unfence`** is refused if another pool is unfenced already.
- **`--dirs` replaces every folder.** To add a folder, give the old ones again beside it.
- **Folders the config writes across several lines** can't be changed in place. Edit them by hand.

## `remove`: take a pool away

`remove` deletes the table of the pool from the config, with the comments directly above it. It
says what happens to the history:

```text
removed pool q7mt (Acme): its history stays, read with --pool q7mt by requests, history, sessions and events
```

- **A pool with accounts is refused,** naming them. Move each account to another pool with
  `accounts edit --pool`, or remove it, first. That way no edit puts the accounts of one pool into
  another.
- **The last pool can't be removed,** because a new account needs a pool to go into.
- **The folders of the pool are free once it's gone.** A session started in them goes to the
  unfenced pool, if there is one.
- **The history stays,** under the id of the pool. `requests`, `history`, `sessions` and `events`
  read it with `--pool <id>`.

## When a change takes effect

The router takes up a change to the pools within about 3 seconds, at its next check of the config.
It doesn't need a restart. A new account in a pool is different: it waits for the router to
restart.

A running session keeps the pool it started in, and the accounts of that pool, until you resume
it. That holds even in folders that a new fence takes in. So once you make or change a fence,
resume the sessions running there. `add` and `edit` both say so.

`pool` edits the config as text, keeping its comments and layout, the same way
[`accounts`](accounts.md) does.

## Examples

```bash
switchboard pool add Acme ~/Code/acme ~/Code/acme-infra   # a pool for a client's work
switchboard pool edit q7mt --label "Acme Ltd"
switchboard pool edit b2n8 --dirs ~/Code/employer         # fence the first pool
switchboard pool remove q7mt
```

## Related

- [Pools](../pools.md): what a pool does, and when you'd want one.
- [Accounts](accounts.md): add accounts to a pool, or move them between pools.
- [Configuration](../configuration.md): the `[[pool]]` keys.
