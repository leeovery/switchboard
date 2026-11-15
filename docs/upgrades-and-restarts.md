# Upgrades and restarts: new versions without a refused request

When `brew upgrade` brings a new version, or you change the config, the router restarts by itself.
It waits for a moment with no request in flight, then replaces itself in place, so no request is
refused. A few upgrades also move your history onto something new, such as account ids or pools.
Each move happens once, by itself, while the router goes on routing.

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

- The router restarts itself when the config changes, an upgrade lands, or the time zone of the
  Mac changes.
- It waits for a moment with no request in flight and no export running. With many long sessions,
  that can take hours, and `switchboard status` and the dashboard say a restart is due meanwhile.
- It runs the new version in the same process, and hands over the sockets it listens on. A request
  made meanwhile waits a moment, and a `claude` started meanwhile is still routed.
- Adding, editing or removing an account takes effect this way, with no command. A change to the
  pools needs no restart: the router takes it up within about 3 seconds.
- Before it restarts, the router asks the new version whether it can start from your config. If
  not, it holds the restart and goes on running the old version.

## A restart held

A newer version can be stricter, for example about the length of a label. If it refuses your
config, `switchboard status` reads `restart due since Mon 14:02 (upgraded), held:` and the reason.
The dashboard says `restart held (upgraded)`. The router asks again once you change the config, or
the binary changes again. What else you'll see until then is in
[Troubleshooting: upgrades](troubleshooting/upgrades.md).

## The move onto account ids

Before switchboard made ids, you chose each id yourself, and every tab numbered the accounts. The
first time the router starts on a version that makes ids, it moves your accounts onto them:

1. It asks Anthropic which subscription each token is for, and gives each account the id that
   subscription spells. An account with no label gets its old id as its label.
2. It writes the new ids into the config, and renames each token file. The service then restarts
   the router. A router you started with `serve` logs that a restart is due, and the move waits
   for you.
3. It rewrites every file that names an account under the new ids, a day at a time. An account the
   history names that the config no longer has goes into the record as removed.

Until the move is done, the dashboard says what it waits on. `requests`, `history`, `sessions`,
`events`, `export` and `import` refuse meanwhile. If a config put back from a dotfiles repository
still names an old id, the router puts it right as it starts. What ids are is in
[`switchboard accounts`](commands/accounts.md).

## The move onto pools

Once the move onto ids is done, the router moves your history into pools:

1. If the config has no pools, the router makes the first, labelled `Default`, and writes it to the
   config with every account in it. If another Mac sharing the config moved first, the first pool
   there becomes the first pool here.
2. It takes the pool up at once, and every request from then on names a pool.
3. It gives every request, every event that names a pool, and every day summary from before pools
   the id of the first pool.

That history stays under the first pool for good, even if you later remove it. Read it then with
`--pool <id>`. Until the move is done, `requests`, `history`, `sessions`, `events` and `import`
refuse. See [Pools](pools.md).

## Several Macs that share a config

Upgrade every Mac that shares a config at the same time. A Mac still on an earlier version finds
the config naming ids it has no token files for.

- **The first Mac to upgrade** makes the ids and the first pool, and writes them into the config.
- **Each Mac after it** asks Anthropic which subscription each of its tokens is for. It joins each
  token to the account of that subscription, so its history lands on the same id.
- **A Mac that can't reach Anthropic** waits to move, rather than give an account another id.
- **A Mac that moves before the first pool reaches it** makes a pool of its own. That pool shows
  as removed once the configs agree, and holds the history of that Mac from before pools.

## Good to know

- The service runs its own copy of switchboard, at a path that never changes. So what you allow it
  in macOS, such as a folder it writes into, holds through upgrades.
- A new version that refuses to start is removed. The copy is always a version that last ran or
  passed the check, so launchd can always start it again.
- An export waiting on a macOS prompt holds a restart until you answer it.
- The moves need Anthropic to name the subscription of each token. If it doesn't answer, the router
  asks again: after 30 seconds, then twice as long each time, up to 5 minutes apart.

## Reference

| Command | What it does |
|---|---|
| `switchboard status` | Says a restart is due, what it waits for, or why it's held. The last lines say how a move stands. |
| `switchboard status --json \| jq .restart` | The restart that's due, and `held` with the refusal. |
| `switchboard status --json \| jq .moving` | The stage of a move, and any account it waits on. |
| `switchboard service restart` | Restarts the router now. It's refused while an export runs. |

How the router restarts in place is in [`switchboard serve`](commands/serve.md). If a restart is
held or a move waits, see [Troubleshooting](troubleshooting.md).

## Related

- [`switchboard service`](commands/service.md): the LaunchAgent that runs the router.
- [Several Macs and backups](several-macs.md): exporting and importing between Macs.
- [How it works](how-it-works.md): the parts of switchboard, and the request path.
