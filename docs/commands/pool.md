# `switchboard pool`: list, make, edit and remove pools

`switchboard pool` lists your pools, in config order. Its subcommands make a pool, edit
one and remove one. For what a pool is and when you'd want one, see [Pools](../pools.md).

```bash
switchboard pool
```

```text
ID    LABEL    DIRS         ACCOUNTS
d3wn  Default  -            b6s7 (primary), q4tx
q7mt  Acme     ~/Code/acme  m2kp (primary), m4zq
```

Each row shows the pool id, label and folders, then the accounts by id, with the primary marked. A
`-` in DIRS marks the unfenced pool.

## Usage and flags

```bash
switchboard pool [--json | --pretty]
switchboard pool add <label> [<dir>...]
switchboard pool edit <id> \
  [--label <label>] [--dirs <dir>]... \
  [--unfence]
switchboard pool remove <id>
```

| Flag | Default | What it does |
|---|---|---|
| `--label <label>` | | `edit`: shows the pool as `<label>`. |
| `--dirs <dir>` | | `edit`: fences the pool to the folders given, one for each `--dirs`, in place of the folders it has. |
| `--unfence` | | `edit`: takes the pool's folders away, making it the unfenced pool. Not with `--dirs`. |
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

A folder inside your home is written from `~/`, even if your shell gave it as a full path. So a
config copied to a Mac with another home fences the same folders there.

If a folder isn't there, `add` makes the pool anyway, because you may make the folder later. It
warns you, in case the name is mistyped. The warning is one line, broken here to fit:

```text
switchboard: ~/Code/acmee isn't a directory, so no
session starts in it yet: if it's mistyped, fence
the pool anew with switchboard pool edit q7mt --dirs
```

If your only pool is fenced, a second fenced pool means commands need `--pool` to read one, and
`add` says so. An unfenced pool added beside fenced ones takes every session started outside the
fences, and commands read it without `--pool`.

`add` refuses a pool if:

- another pool has the same label, ignoring case;
- it would be a second unfenced pool, and `add` names the one there is;
- its folders are the same as, inside, or around another pool's;
- the config names no pools yet.

The router writes the first pool, `Default`, into the config when it first starts. That happens
after setup on a new Mac, and after an upgrade from before pools. Until then, `add` asks you to
start the router with `switchboard service install`. If the router is running,
`switchboard logs router` says what holds it up. If you put back a config from before pools, the
router gives the pools back to it.

A new pool has no accounts. Until you add one, a session started in its folders refuses to start.
Add an account with `accounts add --pool`, or move one in with `accounts edit --pool`. See
[Accounts](accounts.md).

## `edit`: rename or fence a pool

`edit` changes a pool's label or folders. Give one or more of `--label`, `--dirs` and
`--unfence`, and what you don't give stays as it is. It says what it changed, with each value as it
is now and as it was. If every value given is what the pool has already, it says so and writes
nothing.

- **The label and folders** are refused for the same reasons as on `add`, and `edit` warns of a
  folder that isn't there.
- **`--unfence`** is refused if another pool is unfenced already. Once it's done, `edit` says that
  sessions started outside every fence go to the pool now. With several pools, the commands read
  it without `--pool` from then on.
- **Fencing the unfenced pool** leaves every pool fenced, and `edit` says so. `claude` then starts
  outside the fences only within a session, or in a worktree of a repository inside a fence.
- **With several pools all fenced,** a command that reads a pool needs `--pool`, and so does
  `accounts add`. With one pool, every command reads it without `--pool`.
- **`--dirs` replaces every folder.** To add a folder, give the old ones again beside it.
- **A folder written as a full path** in your home is written from `~/` if you give it again with
  `--dirs`, and `edit` says it's rewritten. Older versions of `add` wrote full paths, which match
  on no Mac with another home.
- **Folders the config writes across several lines** can't be changed in place. Edit them by hand.
  Given again as they are, they change nothing, and the rest of the edit goes ahead.

## `remove`: take a pool away

`remove` deletes the pool from the config, with the comments directly above it. It says what
happens to its record:

```text
removed pool q7mt (Acme): its history stays, read with --pool q7mt by requests, history, sessions and events
```

- **A pool with accounts is refused,** naming them. Move each account to another pool with
  `accounts edit --pool`, or remove it, first. That way no edit puts one pool's accounts into
  another.
- **The last pool can't be removed,** because a new account needs a pool to go into.
- **Its folders are free once it's gone.** A session started in them goes to the
  unfenced pool, if there is one.
- **The record stays,** under the same id. `requests`, `history`, `sessions` and `events`
  read it with `--pool <id>`.

## When a change takes effect

The router takes up a change to the pools, and to which pool each account is in, within about 3
seconds. It doesn't need a restart. A new account is different: it waits for the router to restart
itself, once no request is in flight. `switchboard service restart` does it now.

A running session keeps the pool it started in, and the accounts there, until you resume it. That
holds even in folders that a new fence takes in. So once you make or change a fence,
resume the sessions running there. `add` and `edit` both say so.

`pool` edits the config as text, keeping its comments and layout, the same way
[`accounts`](accounts.md) does.

## Examples

```bash
# a pool for a client's work
switchboard pool add Acme ~/Code/acme \
  ~/Code/acme-infra
switchboard pool edit q7mt --label "Acme Ltd"
# move Acme's fence to another folder
switchboard pool edit q7mt --dirs ~/Work/acme
switchboard pool remove q7mt
```

## Related

- [Pools](../pools.md): what a pool does, and when you'd want one.
- [Accounts](accounts.md): add accounts to a pool, or move them between pools.
- [Configuration](../configuration.md): the `[[pool]]` keys.
