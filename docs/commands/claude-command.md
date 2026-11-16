# `claude`: start Claude Code through the router

Once `setup` has made the link, the `claude` you type is switchboard. It starts the real Claude
Code, pointed at the router, and hands it every argument you give. So `claude --help` prints the
Claude Code help.

```bash
# routed
claude
# the session finds its account again, in its pool
claude --resume
# a local subcommand: straight to Claude Code
claude doctor
```

`claude` takes no flags of its own. It runs [`switchboard run`](run.md), so where `--account`
shows below, it's `switchboard run --account`.

## How it finds the real Claude Code

The link is named `claude`, in the bin directory switchboard keeps. The shell file that `setup`
writes puts that directory first on `PATH`, ahead of the real `claude`.

The real `claude` is the first `claude` on `PATH` that doesn't start switchboard again, or else
the one where its installers put it. Switchboard passes over its own link, any other build of
switchboard, and a wrapper named `claude` that `exec`s switchboard.

## How it starts

| If | `claude` |
|---|---|
| The router answers | Starts Claude Code routed through it. |
| The router is down | Starts nothing, and exits with status 1. |
| The router is down, and you chose direct | Starts Claude Code straight on the API. |
| The config can't be read, but the router answers | Starts Claude Code routed. |
| switchboard can't take part at all | Starts Claude Code without switchboard. |
| An API key is set | Starts Claude Code without switchboard. |
| The first argument is a local subcommand | Hands it straight to Claude Code. |

The router answers if it says where it listens, healthy or not. Each case below gives the line
`claude` writes on stderr.

### Routed

Claude Code starts with `ANTHROPIC_BASE_URL` pointing at the router, and the router picks an
account for each conversation request. Claude Code holds the
[primary](../glossary.md)'s token, or if that isn't usable, the first usable one.

Claude Code also gets `CLAUDE_CODE_GATEWAY_HINT_HEADERS=1`, unless your environment sets it
already. With it, Claude Code tells the router what it tells the API about each request, such as
the prompt the request serves. The router keeps that in the request ledger.

An unhealthy router is routed through all the same. Its failures are upstream, so a direct
connection would meet them too:

```text
switchboard: the router is unhealthy (…) — starting claude through it all the same
```

### Stopped

When the router is down, `claude` starts nothing, exits with status 1, and says how to go on:

```text
switchboard: the router isn't running — not starting claude: see to it with switchboard service status, or go direct this once with SWITCHBOARD_WHEN_ROUTER_DOWN=direct claude
```

`claude` waits at most 5 seconds for a slow router. For the first 2 of those seconds, it asks
again if the router isn't running, because the router may be restarting. If the router won't
start, see [Troubleshooting: the router](../troubleshooting/router.md).

### Direct

With `when_router_down = "direct"` in the config, or `SWITCHBOARD_WHEN_ROUTER_DOWN=direct` for one
launch, Claude Code connects straight to the API when the router is down.

```text
switchboard: the router isn't running — connecting directly on b6s7 · Work
```

- **The token** is `--account`'s, or else the primary's, or else the first usable one.
- **A direct session isn't routed or recorded,** and it can spend the reserve you keep on the primary.
- **`SWITCHBOARD_WHEN_ROUTER_DOWN=stop`** stops one launch, whatever the config says. Any other
  value stops it too, and says so.
- **A router that answers** routes, whatever either setting says.

### Without the config

If switchboard can't read its config, for example because a new version refuses it, `claude`
is still routed while the router answers:

```text
switchboard: couldn't read the config (…) — starting claude through the running router
```

Claude Code gets the primary's token, as the router names it. If that isn't usable, it gets
`--account`'s, or else the first usable one. If no account the router names has a usable token,
the line says `— the running router names no account with a usable token —`. That way you don't
take it for the router being down.

### Without switchboard

When switchboard can't take part at all, `claude` starts as if switchboard weren't there:

```text
switchboard: couldn't read the config (…) — starting claude without it
```

That happens in these cases:

- the config can't be read, and the router doesn't answer;
- no account has a usable token;
- `claude` would go direct, but can't find where the router keeps its state.

### With an API key

If `ANTHROPIC_API_KEY` or `ANTHROPIC_AUTH_TOKEN` is set, Claude Code may use that key in place of
the token switchboard gives it. Its requests would go unrouted, billed to the key. So `claude` starts without
switchboard:

```text
switchboard: ANTHROPIC_API_KEY is set, so Claude Code uses it — starting claude without switchboard
```

### Within a session

A `claude` started inside a routed session, for example by a script, inherits what switchboard set
there: the router address, the custom headers, and the hint setting.

| If the inner `claude` | It |
|---|---|
| Has an API key | Leaves out what switchboard set. |
| Starts without switchboard | Keeps the session's routing, because the session's router still runs. |
| Points at a gateway of its own | Leaves out what switchboard set. |
| Found the router down, or had no config or token | Leaves out what switchboard set. |

When it keeps the routing, the line on stderr ends
`— starting claude through this session's router`. What you set yourself stays, your own custom
headers among them.

### Local subcommands

These go straight to Claude Code, untouched, whether the router runs or not:

- `setup-token`, `update`, `upgrade`, `install` and `doctor`;
- `mcp`, `plugin`, `plugins`, `auth`, `import`, `project`, `auto-mode` and `gateway`;
- `--version`, `-v`, `--help` and `-h`.

Only the first argument counts, so `claude -p doctor` is a prompt.

## In a pool

Once the config has pools, a session is in the pool of the folder `claude` starts in. It keeps
that pool for its whole life. A resumed session gets the pool its folder is in at the time. See
[Pools](../pools.md).

- **Which pool:** the fenced pool whose folders hold the folder, as given or as its links lead, or
  else the unfenced pool.
- **In a git worktree,** it's the pool of the same place in the main checkout,
  wherever the worktree lives. If the worktree itself lies in a fence, the two must agree.
- **In a worktree of a bare repository,** it's the pool where the bare repository lies.
- **A submodule or clone inside a worktree** goes by the worktree.
- **Inside another session,** it's the session's pool, unless a fence holds the folder. See
  [`switchboard run`](run.md).

Inside a pool, the rules above narrow to the pool:

| Case | In a pool |
|---|---|
| Routed | Claude Code holds the primary's token, or else the pool's first usable one. |
| `--account` | Must name an account of the pool. |
| Direct | Goes out only on the pool's accounts. |
| Claude Code's own login | Refused inside a fence, and inside a session of a fenced pool. |

- **Routed,** the router is told the pool in a header, and `SWITCHBOARD_POOL` holds its id for the
  skill.
- **Claude Code's own login** is what `--direct` and a start without switchboard use. It could go
  out on any account, so `claude` refuses it inside a fence, and inside a session of a fenced pool.
- **If the config can't be read,** `claude` takes the fences from the config file, or else from
  the running router, or else from the accounts record. Before your record has moved to pools, it
  starts without switchboard instead.

With two pools or more, a router that doesn't route by the pools in the config counts as down:

- a router from before pools;
- one that would pass the switchboard headers on to the API;
- one whose pools, folders, accounts or primaries differ from the config.

`claude` gives it 5 seconds to take the pools up, because the router checks the config every 3
seconds. Then it stops, or goes direct, as `when_router_down` says.

### When claude won't start in a pool

With pools, `claude` may start nothing, exit with status 1, and say why, ending
`— not starting claude`. For example:

```text
no pool covers ~/Code/other — not starting claude
```

Every message, with what to do, is in
[Troubleshooting: sessions in a pool](../troubleshooting/pools.md).

## Going round switchboard

There are two ways to start Claude Code without switchboard:

- `switchboard run --direct` starts Claude Code on its own login, outside every fence.
- The real `claude`, run by its full path.

If the switchboard binary itself breaks, every `claude` breaks with it, until you take the
switchboard bin directory off `PATH`.

## Launchers for one account

Switchboard defines no launchers for each account. To make one, alias
`switchboard run --account <id> --`:

```bash
# a launcher pinned to side
alias cxside='switchboard run --account q4tx --'
```

## Related

- [run](run.md): the command `claude` runs, with `--account` and `--direct`.
- [Limits and routing](../limits-and-routing.md): how the router picks an account.
- [Configuration](../configuration.md): `when_router_down` and the `[[pool]]` keys.
