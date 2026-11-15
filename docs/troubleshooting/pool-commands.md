# Troubleshooting: commands in a pool

The commands read and change one pool at a time. This page covers what they refuse, and what the
config refuses, for a pool. For what `claude` and the router refuse, see
[Sessions in a pool](pools.md).

## A command says every pool is fenced

The message is `every pool is fenced: name a pool, q7mt (Acme) or r2d4 (Client), with --pool`.
With every pool fenced, there's no pool to read without being told. A command never takes the pool
from the folder you run it in.

```bash
switchboard usage --pool q7mt
```

## The dashboard says it couldn't read usage

The message is `couldn't read usage: every pool is fenced: …`. The dashboard has no pool to show
without being told.

```bash
switchboard dashboard --pool q7mt      # open on one pool; press P to turn to the next
switchboard pool edit q7mt --unfence   # or leave one pool unfenced, to open on it by default
```

## A command can't tell which pool to read

The message is `can't tell which pool to read: neither the config, the router nor the accounts
record names any pools`. The history has moved to pools, but the config names none or can't be
read. The router isn't running or routes by none, and the accounts record keeps none. Put the pools
back in the config, or start the router:

```bash
switchboard service install
```

## A command says there's no pool you removed

The message is `there's no pool "q7mt": name …`. `status`, `usage` and `pin` read the router, which
routes nothing in a pool the config no longer has. The history of the pool stays.

```bash
switchboard requests --pool q7mt   # shows it as pool Acme (removed 6 Oct)
```

`requests`, `history`, `sessions` and `events` read it with `--pool`. They read a pool brought in
from another Mac the same way, shown as `pool Client (not configured here)`.

## `status` says it probed directly

`status` says `probed directly: the router doesn't route by pools yet`, and the dashboard says
`○ probed · router started before pools`. The router is from before pools, still running after an
upgrade.

| Command | What it does meanwhile |
|---|---|
| `status`, `usage`, the dashboard | They probe the accounts of the pool themselves. |
| `sessions` | It lists only the sessions in the ledger. |
| `pin` | It refuses. |
| `status --session` | It reads the session as before, but refuses `--json` together with `--pool`. |

```bash
switchboard service restart
```

## `pin` says to pin accounts of one pool

A pin belongs to one pool, so the accounts you name must all be in it. Pin the accounts of each pool
apart. A session can be pinned only to an account of its own pool.

## `pool add` says the config names no pools

The router makes the first pool when it moves the history to pools, the first time it starts on a
version with them. Once the config names that pool, add the rest.

## `pool remove` is refused

The pool still has accounts, which it names, or it's the last pool. Move or remove each account
first:

```bash
switchboard accounts edit <id> --pool <id>   # move an account to another pool
switchboard pool remove <id>
```

## `pool edit --dirs` says to set them by hand

The config writes the `dirs` of the pool across several lines, which `pool edit` can't change in
place. Edit them in the config, or write them on one line.

## The config is refused for its pools

The error names what's wrong. Put the config right as it says.

| If the error names | Then |
|---|---|
| Two pools without `dirs` | Fence one of them. Only one pool can be unfenced. |
| Folders that are the same, or one inside another | Give each pool folders of its own. |
| An account without a `pool`, or with a pool that isn't configured | Give the account a pool that exists. |
| Two primaries in one pool | Keep one. |

Every `[[pool]]` key is in [Configuration](../configuration.md).

## Related

- [`switchboard pool`](../commands/pool.md): making and changing pools.
- [Sessions in a pool](pools.md): what `claude` and the router refuse.
- [Pools](../pools.md): what a pool does, and the fences.
