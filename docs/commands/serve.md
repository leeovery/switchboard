# `switchboard serve`

`serve` runs the router in the foreground until you stop it. The service normally runs it for you,
so you'd only run it by hand to watch it, or if you haven't installed the service.

```console
$ switchboard serve
time=2026-09-28T09:00:00.012+01:00 level=INFO msg=listening component=router pid=4242 address=127.0.0.1:4747 …
```

```bash
switchboard serve [--log-level <level>]
```

| Flag | Default | What it does |
|---|---|---|
| `--log-level <level>` | `SWITCHBOARD_LOG_LEVEL`, else `info` | Logs at `debug`, `info`, `warn` or `error` and above. |

The router serves two things:

- the proxy that Claude Code sends its requests to, on `listen` (`127.0.0.1:4747` by default);
- its control API, on a unix socket in the state directory.

It logs to `router.log`, and to the terminal when it runs in one. A second router fails to start
while one is running. How the router routes is in [Limits and routing](../limits-and-routing.md).

## Starting

The router starts even if no account has a usable token. It warns in its log that nothing will be
routed until one has. As it starts, it:

- makes the `tokens`, `history` and `ledger` folders private;
- takes up the readings it recorded, so the rates behind pressure and the week forecasts carry on;
- brings the skill for Claude up to date.

## Changes it takes up at once

Every 3 seconds, the router looks at its token files and its config, and takes these up in place:

| Change | What the router does |
|---|---|
| A token file holds a new token | The account goes out on the new token. |
| An account without a token gains one | The account starts taking requests. |
| A token file holds no usable token at two looks in a row | The account has nothing to send on until a token is back. |
| A `wrong token` account gets another token | It takes requests again, and the finding goes. |
| A pool is added, edited or removed, or an account moves pool | Sessions are routed by the new pools. |
| `[export] dir` changes | The next export goes to the new folder. |
| An account leaves the config | Sessions still holding its token go on being routed, as if they held the token of the primary. |

A single look that finds no token, for example while a file is being rewritten, keeps the token the
account had. A new account waits for the router to restart.

## Restarting in place

The router restarts itself to take up a new config, an upgrade or a new time zone. It replaces
itself in place and hands over the sockets it listens on, so no request is refused.

| The router watches | And restarts when |
|---|---|
| Its config file, following links | The file makes another valid config. |
| The binary it upgrades from | The link leads to another binary, for example after `brew upgrade`. |
| `/etc/localtime` | The time zone of the Mac changes. |

The service runs the router from a copy of its own. The binary it upgrades from is the Homebrew
link that `service install` ran from. Run by hand, the router upgrades from the binary it started as.

As it starts, the router compares its copy with the binary the link leads to, by size and time. If
the sizes agree but the times don't, for example on a disk that keeps coarse times, it compares the
bytes. If they differ, an upgrade is due from the start. `service install` compares them the same
way.

### The steps

1. Wait until no export is running, and for a moment with no request in flight.
2. For an upgrade, copy in the new binary beside its own copy, hidden.
3. Ask the binary it would become whether it can start from the config.
4. If a request is in flight by then, wait for the next quiet moment, without asking again.
5. For an upgrade, rename the new copy over the old one.
6. Save its state.
7. Replace itself with the copy, handing over its sockets.

Once an export ends, step 3 can run with a request still in flight, and the answer is kept.

### When it goes differently

| If | Then |
|---|---|
| The config isn't valid at two looks in a row | The change is logged at warn and refused. The router carries on as it was. |
| The new binary refuses to start, for example a newer version refusing the config | The restart is held. The router carries on, and says why in its log, its events and `restart` in `status --json`. |
| A held restart then sees the config or binary change | The router asks again. |
| The binary is from before this check | It can't answer, so the router restarts into it. |
| The upgrade can't be copied in, for example on a full disk | It's logged once at warn. The restart stays due, and the router tries again at the next quiet moment. |
| The binary isn't there, for example while `brew upgrade` moves the link | The router tries again for a couple of seconds, then exits. launchd starts it again. |
| The router is told to stop while it waits | It stops then and there. |
| The router runs by hand | It logs once that a restart is due, and `switchboard status` says to run `serve` again. |

When a restart is refused, an upgrade it copied in is removed, and its own copy stays as it was.

## Moving to its own copy

A service installed before the router had its own copy runs the Homebrew binary directly. Such a
router moves over to a copy as it starts. It makes the copy, writes the LaunchAgent afresh to run
it, and replaces itself in place. It keeps the `--config`, `--log-level` and variables the
LaunchAgent set. launchd runs the copy from the next login.

If it can't move over, for example if it can't write the LaunchAgent, it says why once at warn. It
then carries on from the binary it started as, and `service status` names `service install`. It
tries again at the next start, such as the next upgrade or login. A router you run by hand never
moves over.

## Exit status

`serve` exits `0` once it has stopped, when interrupted or terminated. When a router about to
restart asks a new binary whether it can start, a `78` means it can't. That status never changes
across versions.

## Related

- [`service`](service.md): install, restart and check the LaunchAgent that runs `serve`.
- [`logs`](logs.md): read and follow `router.log`.
- [Configuration](../configuration.md): `listen`, and every other key.
