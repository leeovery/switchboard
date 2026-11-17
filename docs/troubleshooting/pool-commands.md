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
# Open on one pool; press P to turn to the
# next
switchboard dashboard --pool q7mt
# Or leave your first pool unfenced, to open
# on it by default
switchboard pool edit d3wn --unfence
```

## A command can't tell which pool to read

The message is `can't tell which pool to read: neither the config, the router nor the accounts
record names any pools`. The record has moved to pools, but the config names none or can't be
read. The router isn't running or routes by none, and the accounts record keeps none. Put the pools
back in the config, or start the router:

```bash
switchboard service install
```

## A command says there's no pool you removed

The message is `there's no pool "q7mt": name …`. `status`, `usage` and `pin` read the router,
which routes nothing in a pool the config no longer has. The pool's record stays.

```bash
# Shows it as pool Acme (removed 6 Oct)
switchboard requests --pool q7mt
```

`requests`, `history`, `sessions` and `events` read it with `--pool`. They read a pool brought in
from another Mac the same way, shown as `pool Client (not configured here)`.

## `status` says it probed directly

`status` says `probed directly: the router doesn't route by pools yet`, and the dashboard says
`○ probed · router started before pools`. The router is from before pools, still running after an
upgrade.

| Command | What it does meanwhile |
|---|---|
| `status`, `usage`, the dashboard | They probe the pool's accounts themselves. |
| `sessions` | It lists only the sessions in the ledger. |
| `pin` | It refuses. |
| `status --session` | It refuses `--json` with `--pool`. |

`status --session` otherwise reads the session as before. To route by pools:

```bash
switchboard service restart
```

## `pin` says to pin accounts of one pool

A pin belongs to one pool, so the accounts you name must all be in it. Pin each pool's accounts
separately. A session can be pinned only to an account of its own pool.

## `pool add` says the config names no pools

The router makes the first pool, `Default`, the first time it starts on a version with pools. On a
new Mac, that's once setup has added your accounts and started it. If the router isn't running,
start it, then add the pool again:

```bash
switchboard service install
```

If the router is running, its log says what holds it up:

```bash
switchboard logs router
```

## `pool remove` is refused

The pool still has accounts, which it names, or it's the last pool. Move or remove each account
first:

```bash
# Move an account to another pool
switchboard accounts edit m2kp --pool d3wn
switchboard pool remove q7mt
```

## `pool edit --dirs` says to set them by hand

If the pool's `dirs` span several lines in the config, `pool edit` can't change them in place. Edit
them by hand, or put them on one line.

## The config is refused for its pools

The error names what's wrong. Put the config right as it says.

| If the error names | Then |
|---|---|
| Two pools without `dirs` | Fence one of them. Only one pool can be unfenced. |
| Folders that are the same, or one inside another | Give each pool folders of its own. |
| An account with no `pool`, or an unknown one | Give the account a pool that exists. |
| Two primaries in one pool | Keep one. |

Every `[[pool]]` key is in [Configuration](../configuration.md).

## Related

- [`switchboard pool`](../commands/pool.md): making and changing pools.
- [Sessions in a pool](pools.md): what `claude` and the router refuse.
- [Pools](../pools.md): what a pool does, and the fences.
