# Several Macs and backups: keep your history safe and together

Switchboard can export its history into a folder that Dropbox or iCloud Drive syncs, every hour
by itself. Another Mac can import that folder, so one Mac sees the work of both. You'd want this
to keep a backup of your record, to move to a new Mac, or to run Claude on two Macs.

## Try it

```bash
switchboard export ~/Dropbox/switchboard   # copies the history of this Mac into a folder of its own there
switchboard import ~/Dropbox/switchboard   # brings the history of every Mac in that folder into this one
```

To have the router export by itself, add this to the config:

```toml
[export]
dir = "~/Dropbox/switchboard"   # exported when the router starts, then every hour
```

The router takes up the folder by itself, from its next export.

## What happens

- Each Mac writes into a folder of its own, named `mac-` and the id of the install. Macs that share
  one folder never write over each other.
- Each export writes only the days that changed, and removes nothing from the folder.
- Import matches accounts by their subscription. The same account on two Macs counts as one
  account, under the id it has on this Mac.
- Running an import twice adds nothing new the second time.
- The imported days show in `history`, the dashboard and the other commands within minutes.
- A pool from another Mac keeps its history. Read it with `--pool <id>`, and it shows as
  `not configured here`.

## What it carries

An export holds what switchboard can't build again:

- the request ledger, every request without its content;
- the events of the router;
- the summary of each day;
- the readings history, how each window stood over time;
- the accounts record, which names each account in the files by id and label.

It leaves out your tokens, your config, the logs, and the settings of the dashboard. Keep the
config in a dotfiles repository if you want it on every Mac.

## Restore onto a new Mac

1. Install switchboard and run `switchboard setup`. Add your accounts with their tokens, because
   the export holds none.
2. Run `switchboard import ~/Dropbox/switchboard`. It prints what it took in, such as
   `imported 1 folder: 27 days added, 3 merged`.
3. Set `[export] dir` on the new Mac, so it goes on exporting.

If you moved with Migration Assistant, the new Mac carries the install id of the old one. It then
exports into the folder of the old Mac. To give it a folder of its own, remove `install-id` from
the state directory before the next export.

## Good to know

- `export` works without the router. `import` needs the router, which writes the history.
- macOS asks before the router writes into iCloud Drive, Dropbox or `~/Documents`. Until you
  answer, that export waits, and so does any restart of the router. To allow it later, go to System
  Settings → Privacy & Security.
- If the folder is on a disk that isn't mounted, the export fails. It never writes onto the disk of
  the Mac in its place.
- Import leaves out requests older than the ledger keeps, 400 days unless you set another length.
  It still takes in the summary of each of those days.

## Reference

| Command or key | What it does |
|---|---|
| `switchboard export <dir>` | Exports the history of this Mac into `<dir>` now. |
| `switchboard import <dir>` | Imports every Mac folder in `<dir>`, or the one Mac folder you name. |
| `[export] dir` | The folder the router exports into every hour. Empty means no export. |
| `switchboard status --json \| jq .export` | When the last export succeeded, or why it failed. |

Every flag is in [Commands](commands.md), and the files an export holds are in
[Reference](reference.md). If an export fails, see [Troubleshooting](troubleshooting.md).

## Related

- [Your record and what it's worth](your-record.md): what the history holds, and how to read it.
- [Configuration](configuration.md): the `[export]`, `[ledger]` and `[history]` keys.
- [Pools](pools.md): how a pool keeps the history of its work.
