# Troubleshooting: upgrades and restarts

After an upgrade, the router restarts into the new version by itself, at the next moment no request
is in flight. This page covers the times it can't, and what the service says about it.

## A restart held after an upgrade

The new version refuses your config, for example because a rule is stricter. Rather than restart
into a version that would stop, the router keeps running the old one.

```bash
switchboard status                        # shows the restart as held and why, then fails on the config
switchboard status --json | jq .restart   # restart.held gives the refusal
```

The `switchboard` you run is the new version too, so it can't read the config either. Until you put
the config right, this is what you'll see:

| Where | What you see |
|---|---|
| `switchboard status` | The document of the router, with the restart held, then the error in the config. |
| `usage`, or a dashboard you open now | They fail on the config. |
| A dashboard already open | `restart held (upgraded)` on the status line. |
| Each `claude` you start | It says it couldn't read the config, and goes through the running router all the same. It starts without switchboard only if the router doesn't answer. |
| The log of the router | The refusal, at warn. |

Put the config right as the error says. The router restarts at its next quiet moment.

## An upgrade not taken up because the disk is full

Before it restarts, the router copies the new version in beside its own copy. If it can't, it
carries on as it is, and `switchboard status` shows `restart due (upgraded)`. Its log says why
once, at warn.

```bash
df -h ~    # check for room on the disk
```

The router tries the copy again at each moment with no request in flight. Once there's room, it
takes the upgrade up by itself. Until then, `switchboard service restart` fails and says why.

## `service status` says the router's copy isn't run

The service was installed before the router had a copy of switchboard of its own. It runs
switchboard itself, so macOS asks again what you allow the router after each upgrade.

The router moves the service over to its copy by itself as it starts. If it can't, for example
because it can't write the LaunchAgent, the log says so at warn:

```text
can't move over to the router's own copy of switchboard
```

To put it right yourself:

```bash
switchboard service install   # writes the LaunchAgent afresh, running the copy
```

## `service status` says launchd runs the copy from the next login

launchd reads the LaunchAgent only when it loads it, at login. Until then, if launchd starts the
router again, for example after a crash, it starts switchboard itself rather than the copy. An
upgrade that landed since may then refuse the config.

The next login puts it right by itself. To have launchd run the copy now:

```bash
switchboard service install   # has launchd load the LaunchAgent afresh; setup runs it too
```

## Related

- [Account ids](account-ids.md): the upgrade that moves your accounts onto ids.
- [`switchboard service`](../commands/service.md): what `install`, `restart` and `status` do.
- [How it works](../how-it-works.md): how the router restarts in place.
