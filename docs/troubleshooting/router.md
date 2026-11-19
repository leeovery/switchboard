# Troubleshooting: the router

Every `claude` you start sends its requests through the router, which the service keeps running.
This page covers a router that's down, and what to check when routing surprises you.

## `claude` won't start, saying the router is down

The router isn't running, doesn't answer, or doesn't say where it listens. By default, `claude`
then starts nothing and says so.

```bash
# Is the service installed and loaded, and
# how does the router answer?
switchboard service status
# If the service isn't installed or loaded
switchboard service install
# If it is, but the router doesn't answer
switchboard service restart
```

An unhealthy router doesn't stop `claude`. It goes through the router all the same, and says so.

| If you want | Then |
|---|---|
| Claude Code without the router, this once | Run `SWITCHBOARD_WHEN_ROUTER_DOWN=direct claude`. |
| `claude` always to go direct then | Set `when_router_down = "direct"` in the [config](../configuration.md). |

A session that goes direct is neither routed nor recorded.

## A session went to an account you didn't expect

The router's log has a line for each request, with the account and why it went there.

```bash
# Follow the router as it routes
switchboard logs -f
# Today's requests, with each account
switchboard requests
# What the router decided: moves, limits,
# pins and refusals
switchboard events
```

`requests` and `events` reach back over earlier days with `--since`. To learn how the router
picks, see [Limits and routing](../limits-and-routing.md).

## An account still reads out after you reset a limit by hand

The router sends nothing to an account at its limit. So it learns of a reset you made on claude.ai
only when it next reads the account, which it does every 5 minutes. To have it read the accounts
now:

```bash
# Or switchboard status -r, or press R on
# the dashboard
switchboard usage -r
```

The command waits up to 10 seconds for the reads, then prints what they found.

## A pin that may or may not have been made

The router stopped or restarted while you pinned, and closed the request without an answer. It may
have made the pin first.

```bash
# Shows any pin in force
switchboard status
# Pin again if it isn't there
switchboard pin <id>
```

## A request Claude Code sent that the router never saw

Some of what Claude Code sends never reaches the router, and some sessions can't be routed at all.
See [How it works](../how-it-works.md) for what doesn't go through the router.

## Related

- [`switchboard service`](../commands/service.md): install, restart and check the service.
- [Upgrades and restarts](upgrades.md): a restart put off, or one that waits.
- [Logs](logs.md): where the logs are, and how to read a line.
