# Upgrades and restarts: new versions without a refused request

When `brew upgrade` brings a new version, or you change the config, the router restarts by itself.
It waits for a moment with nothing in flight, then replaces itself in place, so no request is
refused. An upgrade also restarts any dashboard you left open, on the tab it showed. A few
upgrades also move your record, once, by themselves.

## Try it

```bash
# Upgrade: the router restarts by itself
brew upgrade switchboard
# Whether a restart is due, and what it
# waits for
switchboard status
# Restart now, rather than wait
switchboard service restart
```

## What happens

- The router restarts itself when the config changes, an upgrade lands, or the Mac's time zone
  changes.
- It waits for a moment with no request in flight and no export running. With many long sessions,
  that can take hours, and `switchboard status` and the dashboard say a restart is due meanwhile.
- It runs the new version in the same process, and hands over the sockets it listens on. A request
  made meanwhile waits a moment, and a `claude` started meanwhile is still routed.
- Some changes need no restart. The router takes up the pools, the pool of each account,
  `[export] dir` and new tokens within about 3 seconds. Anything else waits for the restart.
- A router you started by hand with `serve` only logs that a restart is due.
- Before it restarts, the router asks the new version whether it can start from your config. If
  not, it puts off the restart and goes on running the old version.

## The one-off moves

Two past upgrades moved your record onto something new: account ids, then pools. Each move runs
once, the first time the router starts on that version, while it goes on routing. Until a move is
done, the dashboard says what it waits on. `requests`, `history`, `sessions`, `events`, `export`
and `import` refuse meanwhile.

The move onto account ids:

1. The router asks Anthropic which subscription each token is for, and gives each account the id
   that subscription spells. An account with no label gets its old id as its label.
2. It writes the new ids into the config, and renames each token file. It then needs a restart,
   which the service makes by itself.
3. It rewrites every file that names an account under the new ids, a day at a time.

The move onto pools:

1. If the config has no pools, the router makes the first, labelled `Default`, and writes it to the
   config with every account in it.
2. It takes the pool up at once, and every request from then on names a pool.
3. It gives everything recorded before pools the first pool's id. That part of your record stays
   under the first pool for good, read with `--pool <id>` even once the pool is removed.

If a config put back from a dotfiles repository still names an old id, the router puts it right as
it starts. What ids are is in [`switchboard accounts`](commands/accounts.md).

## Good to know

- Upgrade every Mac that shares a config at the same time. A Mac that moves later joins its tokens
  to the ids the first Mac made, by asking Anthropic which subscription each is for.
- Only the move onto ids asks Anthropic. If it doesn't answer, the router asks again after 30
  seconds, then twice as long each time, up to 5 minutes apart. The move onto pools never waits on
  Anthropic.
- The new version judges your config, so you can add a key it brings before you upgrade. If the
  new binary is too old to answer, the running version judges, and can put off the restart.
- The router reads the two newest days of readings back as it starts. So the recent rates, and the
  week forecast, carry on across a restart.
- The service runs its own copy of switchboard, at a path that never changes. So what you allow it
  in macOS, such as a folder it writes into, holds through upgrades.
- An export waiting on a macOS prompt puts off a restart until you answer it.

## Reference

| Command | What it does |
|---|---|
| `switchboard status` | Says a restart is due, and what it waits for. |
| `switchboard status --json \| jq .restart` | The restart due, and any refusal to start, in its `held` field. |
| `switchboard status --json \| jq .moving` | The stage of a move, and what it waits on. |
| `switchboard service restart` | Restarts the router now, unless an export runs. |

How the router restarts in place is in [`switchboard serve`](commands/serve.md). A restart put
off, and a move that waits, are in [Troubleshooting: upgrades](troubleshooting/upgrades.md) and
[Troubleshooting: account ids](troubleshooting/account-ids.md).

## Related

- [`switchboard service`](commands/service.md): the LaunchAgent that runs the router.
- [Backups and a new Mac](several-macs.md): exporting and importing between Macs.
- [How it works](how-it-works.md): the parts of switchboard, and the request path.
