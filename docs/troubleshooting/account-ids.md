# Troubleshooting: the move onto account ids

The first time the router starts on a version that gives accounts ids, it moves your accounts and
their history onto them. This page covers a move that waits, an import it refuses, and a damaged
accounts record.

## The dashboard waits on the move onto the ids

The move isn't done yet. Until it is, the dashboard says what it waits on, and opens once the move
is done. `requests`, `history`, `sessions`, `events`, `export` and `import` refuse, saying the
same.

```bash
switchboard status                        # the last line says what the move waits on
switchboard status --json | jq .moving    # the stage, and any account it waits on, with why
```

| If it says | Then |
|---|---|
| `the API hasn't named the organization of <account>'s token` | The API didn't answer, asked to be asked again, or a proxy answered in its place. Check the network, and any proxy or VPN between you and `api.anthropic.com`. `switchboard logs -f` shows each ask. |
| `the router makes the rest once it restarts onto the new ids its config holds` | Run `switchboard service restart`. If you started the router by hand with `serve`, stop it and start it again. |
| `no router answers to make it` | Start the router with `switchboard service install`, or `switchboard serve`. |
| `the router running is of a version from before the ids` | The router hasn't restarted into the upgrade yet. It makes the move once it does: see [Upgrades and restarts](upgrades.md). |
| `the router couldn't make it, and tries again in a minute` | A step failed, such as a file it couldn't write. The reason follows, and the log has it at warn. Put that right, and the move carries on. Nothing is lost part way. |

The router routes while it waits on the API. It asks again after 30 seconds, then twice as long
each time, up to 5 minutes apart. If the API answers but names no organization, for example
because it refuses the token, the router asks for 10 minutes. Then it gives the account an id at
random. An account with no usable token gets a random id at once, unless another Mac sharing the
config may have moved it already.

## Two Macs that share a config

A Mac still on an earlier version finds the config naming ids it has no token files for. Upgrade
every Mac that shares the config. A Mac that upgrades while the network is down waits to join its
accounts to the ids the first Mac made. It carries on once the API answers.

The move onto pools works the same way. If another Mac that shares the config moved first, its
first pool becomes the first pool here. A Mac that moves before that pool reaches its config makes
a pool of its own, which shows as removed once the configs agree.

## A day file set aside as damaged

The move repairs a compressed day file it finds damaged as it rewrites it. It keeps the lines
before the damage, sets the old file aside beside it as `<name>.damaged`, and warns of it in the
log. You don't need to do anything. The files are listed in [Reference](../reference/files.md).

## An import refused across the upgrade

`import` names each Mac folder it passes over, and why, then takes in the rest.

| If it says | Then |
|---|---|
| `another Mac exported it before switchboard made account ids` | On that Mac, upgrade switchboard and let the move finish. Export again there, then import here once the folder has synced. |
| `its accounts record hasn't synced yet` | Import again once the sync is done. |
| `it holds an export without its export.json` | An export was cut short, or a sync isn't done. Export again on that Mac, or wait for the sync, then import again. |
| `its accounts record doesn't read` | The `accounts.json` in that folder is damaged. Export again on that Mac. |
| `there's nothing to import in <dir>: each Mac's folder in it was passed over` | Each folder was passed over for the reason named above it. |
| `the history waits on its move onto the accounts' new ids` | The move on this Mac isn't done yet. Import once it is. |

The folder this Mac exported before the upgrade is taken in as it is.

## Adding or removing an account fails on the accounts record

`accounts.json` doesn't read, for example after a wrong edit by hand. Switchboard leaves it as it
is. Adding or removing an account fails and names the file. Meanwhile, History and the commands
name each removed account by id. Put the file right, or move it aside. What it holds is in
[Reference](../reference/accounts-record.md).

## Related

- [Upgrades and restarts](../upgrades-and-restarts.md): how the move onto ids runs.
- [Exports](exports.md): an export that fails.
- [Several Macs and backups](../several-macs.md): exporting and importing between Macs.
