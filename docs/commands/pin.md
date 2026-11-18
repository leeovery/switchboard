# `switchboard pin`: choose the account sessions go to

`switchboard pin` tells the router which accounts to send sessions to, in place of its own choice.
It needs the router. For why and when you'd pin, see
[Pinning and moving sessions](../pinning-and-moving.md).

```bash
switchboard pin q4tx
```

```text
new sessions go to q4tx · side
```

## Usage and flags

```bash
switchboard pin <account>... [--move] [--force] \
  [--pool <id>]
switchboard pin auto [--force] [--pool <id>]
switchboard pin <account>|auto --session <id> \
  [--pool <id>]
```

Name each account by its id, as `switchboard accounts` lists it. On this page, `b6s7` is work and
`q4tx` is side.

| Flag | Default | What it does |
|---|---|---|
| `--move` | | Moves running sessions on other accounts there too, each on its next request, at the cost of a cache rebuild each. Not with `auto`. |
| `--force` | | Also clears each session's own pin, including one `run --account` gave. With a second pool, only that pool's sessions. |
| `--session <id>` | | Pins one running session to one account, from its next request, in place of any pin it had. Takes neither `--move` nor `--force`. |
| `--pool <id>` | the pool the verbs act on | With `auto`, clears that pool's pin. With accounts, they must be in that pool. With `--session`, looks only in that pool. |

## What each pin does

| Command | New sessions | Running sessions |
|---|---|---|
| `pin b6s7` | go to work | stay where they are while their caches are warm and their accounts have room |
| `pin b6s7 --move` | go to work | move to work on their next request, except those with a pin of their own |
| `pin b6s7 --move --force` | go to work | all move to work, and their own pins are cleared |
| `pin b6s7 q4tx` | go to the better of work and side | stay where they are while their caches are warm and their accounts have room |
| `pin b6s7 q4tx --move` | go to the better of work and side | those on neither move to the better of them on their next request, except those with a pin of their own |
| `pin auto` | are routed | stay where they are while their caches are warm and their accounts have room |
| `pin auto --force` | are routed | as `pin auto`, and their own pins are cleared |
| `pin b6s7 --session 18bb978f` | aren't affected | that session moves to work on its next request, and stays pinned there |
| `pin auto --session 18bb978f` | aren't affected | that session's own pin is cleared, and it's routed like any other |

A pin goes to new sessions, and to any running session whose account is chosen afresh. With one
account pinned, sessions go to it. With several, they go to the best of them, as the router would
choose if they were the only accounts.

## Rules every pin follows

- **A new pin replaces the one before it.**
- **A pin stays in force while one of its accounts has room.** When none has, the router chooses
  among every account in the pool, as if nothing were pinned.
- **A pin on one session beats the pin on its pool.**
- **Every pin gives way at a limit.** A pinned session that hits one moves by the usual rules,
  rather than fail.
- **A pin spends the reserves of the accounts it names,** and no other. See
  [Room for the Claude app](../room-for-the-claude-app.md).
- **`pin auto` goes back to routing.**

If the router closes the connection as it's asked, such as while it stops, it may have made the
pin first. So `pin` says just that, rather than that the router isn't running. `switchboard status`
shows which.

## Pinning several accounts

Pinning several accounts sets an order to use them up in. With `pin b6s7 q4tx`, work and side take
the new sessions, the better of the two first, until both are out. Only then does anything go to
the other accounts.

For example, say two accounts have a weekly reset banked on claude.ai, and a third hasn't. Pin the
two, let them run out, and reset them by hand. The router sees a reset the next time it reads the
account:

- on its own rounds, within 5 minutes;
- before a choice it makes afresh, once its reading is 15 minutes old;
- at once, with `switchboard usage -r` or `R` on the dashboard.

`usage -r` and `R` probe an account that can take no request, however lately the router read it.
The exception is an account the router probed in the last minute.

## Pins in a pool

Each pool has a pin of its own. The accounts you pin are pinned in the pool they're in, so
`pin <account>...` needs no `--pool`.

| You run | What happens |
|---|---|
| `pin` with accounts of two pools | Refused, naming each one's pool. |
| `pin` with accounts of one pool | Reaches only that pool's sessions. |
| `pin auto` | Clears the pin of the pool the verbs act on. |
| `pin --session` with another pool's account | Refused. |

- **Accounts of two pools** are refused with `pin accounts of one pool: Acme 1 (m2kp) is in pool
  Acme, Work (b6s7) is in pool Default`.
- **The pin, `--move` and `--force`** reach only the sessions in that pool. When no pinned account has
  room, the router chooses among that pool only.
- **`pin auto --pool <id>`** clears the pin of the pool named.
- **A session pinned to an account in another pool** is refused with
  `Acme 1 (m2kp) is in pool Acme; this session is in pool Default`.

Once there's a second pool, what `pin` prints names the pool:

```text
new sessions in pool Acme go to m2kp · Acme 1
routing automatically in pool Acme, the own pin of every session in pool Acme cleared
```

`events` and the Log say so too, as `the own pins of pool Acme's sessions cleared too`. They name
the pool as long as your record holds the work of more than one pool. So a pin from the days of two
pools still names its pool after one is removed.

### A session that runs in two pools

A session resumed in another pool has work in each, and can have a pin of its own in each.

- **`pin --session` with `--pool`** sets or clears the pin in the pool `--pool` names. With
  `--pool`, a session with no work in that pool isn't found.
- **`pin <account> --session` without `--pool`** acts in the pool the account is in, if the session
  runs there.
- **Otherwise,** and for `pin auto --session`, it acts in the pool where the session was routed
  last.

`pin` refuses a pool the config no longer has. It also refuses while the router still runs from
before pools, saying so, and the dashboard's `p` key refuses too. Run
`switchboard service restart` first.

## Naming a session

Give the session id, or as much of it as is unique among the sessions routed in the last
hour. To find the id:

- `switchboard status` lists the sessions.
- `/status` inside Claude Code shows the id.
- Inside a session, `$CLAUDE_CODE_SESSION_ID` holds it.

## Examples

```bash
# new sessions go to side
switchboard pin q4tx
# and running ones move there too
switchboard pin q4tx --move
# new sessions go to the better of work and side
switchboard pin b6s7 q4tx
# back to routing, each session's own pin cleared
switchboard pin auto --force
# one session, by the start of its id
switchboard pin b6s7 --session 18bb
```

## Related

- [Pinning and moving sessions](../pinning-and-moving.md): when to pin, and what a move costs.
- [Pools](../pools.md): the fences each pin keeps inside.
- [status](status.md): the pin in force, and the sessions to pin.
