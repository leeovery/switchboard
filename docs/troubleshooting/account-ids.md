# Troubleshooting: the move onto account ids

The first time the router starts on a version that gives accounts ids, it moves your accounts and
their record onto them. This page covers a move that waits, an import it refuses, and a damaged
accounts record.

## The dashboard waits on the move onto the ids

The move isn't done yet. Until it is, the dashboard says what it waits on, and opens once the move
is done. `requests`, `history`, `sessions`, `events`, `export` and `import` refuse, saying the
same.

```bash
# The last line says what the move waits on
switchboard status
# The stage, and any account it waits on,
# with why
switchboard status --json | jq .moving
```

| If it says | Then |
|---|---|
| `the API hasn't named the organization of <account>'s token` | Check the network, and any proxy or VPN. |
| `the router makes the rest once it restarts onto the new ids its config holds` | Wait, or run `switchboard service restart`. |
| `no router answers to make it` | Start the router with `switchboard service install`, or `switchboard serve`. |
| `the router running is of a version from before the ids` | The router hasn't restarted into the upgrade yet. It makes the move once it does: see [Upgrades and restarts](upgrades.md). |
| `the router couldn't make it, and tries again in a minute` | Put right the reason that follows. |

If the API hasn't named the organization, it didn't answer, or it asked to be asked again. Or a
proxy between you and `api.anthropic.com` answered in its place: only an answer that carries the
request id of Anthropic counts. `switchboard logs -f` shows each ask.

If the router makes the rest once it restarts, it restarts itself onto the new ids once no request
is in flight. `switchboard service restart` does it now. If you started the router by hand with
`serve`, stop it and start it again.

If the router couldn't make the move, a step failed, such as a file it couldn't write. The log has
the reason at warn. Put that right, and the move carries on. Nothing is lost part way.

The router routes while it waits on the API. It asks again after 30 seconds, then twice as long
each time, up to 5 minutes apart. If the API answers but names no organization, for example
because it refuses the token, the router asks for 10 minutes. Then it gives the account an id at
random. An account with no usable token gets a random id at once, unless another Mac sharing the
config may have moved it already.

## Two Macs that share a config

A Mac still on an earlier version finds the config naming ids it has no token files for. Upgrade
every Mac that shares the config. A Mac that upgrades while the network is down waits to join its
accounts to the ids the first Mac made. It carries on once the API answers.

## A day file set aside as damaged

The move repairs a compressed day file it finds damaged as it rewrites it. It keeps the lines
before the damage, sets the old file aside beside it as `<name>.damaged`, and warns of it in the
log. You don't need to do anything. The files are listed in
[Where the files are](../reference/files.md).

## An import refused across the upgrade

`import` names each Mac folder it passes over, and why, then takes in the rest.

| If it says | Then |
|---|---|
| `another Mac exported it before switchboard made account ids` | Upgrade that Mac, let the move finish there, and export again. |
| `its accounts record hasn't synced yet` | Import again once the sync is done. |
| `it holds an export without its export.json` | Export again on that Mac, or wait for the sync. |
| `its accounts record doesn't read` | The `accounts.json` in that folder is damaged. Export again on that Mac. |
| `there's nothing to import in <dir>: each Mac's folder in it was passed over` | See the reason named above each folder. |
| `the history waits on its move onto the accounts' new ids` | The move on this Mac isn't done yet. Import once it is. |

Import here again once the folder has synced. An export without `export.json` was cut short, or
its sync isn't done. The folder this Mac exported before the upgrade is taken in as it is.

## Adding or removing an account fails on the accounts record

`accounts.json` doesn't read, for example after a wrong edit by hand. Switchboard leaves it as it
is. Adding or removing an account fails and names the file. Meanwhile, History and the commands
name each removed account by id. Put the file right, or move it aside. What it holds is in
[The accounts record](../reference/accounts-record.md).

## Related

- [Upgrades and restarts](../upgrades-and-restarts.md): how the move onto ids runs.
- [Exports](exports.md): an export that fails.
- [Several Macs and backups](../several-macs.md): exporting and importing between Macs.
