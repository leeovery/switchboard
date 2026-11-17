# Backups and a new Mac: your record and config, safe off this Mac

Switchboard can export your record and your config into a folder that Dropbox or iCloud Drive
syncs, every hour by itself. If you lose this Mac or move to a new one, an import puts back the
config, then the record. Accounts and pools keep their ids. Another Mac can import the folder too.

## Try it

```bash
# copy this Mac's record and config into a
# folder of its own in ~/Dropbox/switchboard
switchboard export ~/Dropbox/switchboard
```

To have the router export by itself, add this to the config:

```toml
[export]
# exported as the router starts, then every hour
dir = "~/Dropbox/switchboard"
```

The router restarts itself to take up the change once no request is in flight, and exports as it
starts. To restart it now, run `switchboard service restart`.

## What happens

- An export holds the record, which switchboard can't build again, and a copy of the config. The
  copy holds the values of the config alone, without comments, so it never holds a token.
- Each Mac writes into a folder of its own, named `mac-` and the install id. Macs that share one
  folder never write over each other.
- Each export writes only the days that changed, and removes nothing from the folder.
- Import matches accounts by their subscription. The same account on two Macs counts as one
  account, under the id it has on this Mac.
- Running an import twice adds nothing new the second time.
- The imported days show in `history`, the dashboard and the other commands within minutes.

## Move to a new Mac

Install switchboard on the new Mac, but don't run `setup` yet. Run `import` first:

```console
$ switchboard import ~/Dropbox/switchboard
laid down the config of mac-0123456789abcdef at /Users/you/.config/switchboard/config.toml: 3 accounts in 2 pools
next, run switchboard setup to give each account its token and start the router, then switchboard import /Users/you/Dropbox/switchboard again to bring in the history
```

The new Mac has no config, so `import` lays down the config from the folder. Then:

1. Run `switchboard setup`. The Accounts step lists every account without a token, because the
   export holds none. Press `t` on each account to make its token.
2. Run `switchboard import ~/Dropbox/switchboard` again. It prints what it took in, such as
   `imported 1 folder: 27 days added, 3 merged`.

The config keeps `[export] dir`, so the new Mac goes on exporting. The same steps restore a
backup onto this Mac if the config and the record are lost. If only the config is lost, `import`
lays down the config from the folder of this Mac, and keeps its record as it is.

If `<dir>` holds the folders of several other Macs, `import` names them and lays down nothing.
Point it at the folder of the Mac whose config you want.

If you moved with Migration Assistant, the new Mac carries the install id of the old one. It then
exports into the old Mac's folder. To give it a folder of its own, remove `install-id` from the
state directory before the next export.

## Two Macs at once

If this Mac has a config of its own, `import` never changes that config. Each pool of the other
Mac counts as the pool here that holds the same accounts. If those accounts sit in two pools here,
or in none, the pool stays apart. `import` names that pool, and `--pool` with its id reads it.

## Good to know

- `export` works without the router. `import` needs the router, which writes the record, unless
  it lays down a config.
- macOS asks before the router writes into iCloud Drive, Dropbox or `~/Documents`. Until you
  answer, that export waits, and so does any router restart. To allow it later, go to System
  Settings → Privacy & Security.
- If the folder's disk isn't mounted, the export fails. It never falls back to this Mac's disk.
- Import leaves out requests older than the ledger keeps, 400 days unless you set another length.
  It still takes in the summary of each of those days.

## Reference

What an export holds, in its `mac-<id>` folder:

| File | Holds |
|---|---|
| `ledger/requests-<date>.jsonl.gz` | The request ledger: every request, without its content. |
| `ledger/events-<date>.jsonl.gz` | The router's events. |
| `ledger/day-<date>.json` | The summary of each day. |
| `history/readings-<date>.jsonl.gz` | The readings: how each window stood over time. |
| `accounts.json` | The accounts record, naming each account by id and label. |
| `config.toml` | A copy of the values of the config, without comments or tokens. |
| `export.json` | Which Mac the folder belongs to, and when it last exported. |

An export leaves the tokens, the logs and the dashboard settings behind. Every flag is in
[`switchboard export`](commands/export.md) and [`switchboard import`](commands/import.md), and
each file is in [Reference](reference.md). If an export fails, see
[Troubleshooting](troubleshooting/exports.md).

## Related

- [Your record](your-record.md): what the record holds, and how to read it.
- [Configuration](configuration.md): the `[export]`, `[ledger]` and `[history]` keys.
- [Pools](pools.md): how a pool keeps the record of its work, even on another Mac.
