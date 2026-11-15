# Several Macs and backups: keep your record safe and together

Switchboard can export your record into a folder that Dropbox or iCloud Drive syncs, every hour
by itself. Another Mac can import that folder, so one Mac sees the work of both. You'd want this
to keep a backup of your record, to move to a new Mac, or to run Claude on two Macs.

## Try it

```bash
# copy this Mac's record into a folder of its
# own in ~/Dropbox/switchboard
switchboard export ~/Dropbox/switchboard
# bring in the record of every Mac there
switchboard import ~/Dropbox/switchboard
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

- An export holds what switchboard can't build again: the requests, the readings of each window,
  the router's events and the daily summaries. It leaves out your tokens and your config.
- Each Mac writes into a folder of its own, named `mac-` and the install id. Macs that share one
  folder never write over each other.
- Each export writes only the days that changed, and removes nothing from the folder.
- Import matches accounts by their subscription. The same account on two Macs counts as one
  account, under the id it has on this Mac.
- Running an import twice adds nothing new the second time.
- The imported days show in `history`, the dashboard and the other commands within minutes.

## Restore onto a new Mac

1. Install switchboard and run `switchboard setup`. Add your accounts with their tokens, because
   the export holds none.
2. Run `switchboard import ~/Dropbox/switchboard`. It prints what it took in, such as
   `imported 1 folder: 27 days added, 3 merged`.
3. Set `[export] dir` on the new Mac, so it goes on exporting.

If you moved with Migration Assistant, the new Mac carries the install id of the old one. It then
exports into the old Mac's folder. To give it a folder of its own, remove `install-id` from the
state directory before the next export.

## Good to know

- `export` works without the router. `import` needs the router, which writes the record.
- macOS asks before the router writes into iCloud Drive, Dropbox or `~/Documents`. Until you
  answer, that export waits, and so does any router restart. To allow it later, go to System
  Settings → Privacy & Security.
- If the folder's disk isn't mounted, the export fails. It never falls back to this Mac's disk.
- Import leaves out requests older than the ledger keeps, 400 days unless you set another length.
  It still takes in the summary of each of those days.

## Reference

| Command or key | What it does |
|---|---|
| `switchboard export <dir>` | Exports this Mac's record into `<dir>` now. |
| `switchboard import <dir>` | Imports every Mac folder in `<dir>`, or the one Mac folder you name. |
| `[export] dir` | The folder the router exports into every hour. Empty means no export. |
| `switchboard status --json \| jq .export` | When the last export succeeded, or why it failed. |

What an export holds, in its `mac-<id>` folder:

| File | Holds |
|---|---|
| `ledger/requests-<date>.jsonl.gz` | The request ledger: every request, without its content. |
| `ledger/events-<date>.jsonl.gz` | The router's events. |
| `ledger/day-<date>.json` | The summary of each day. |
| `history/readings-<date>.jsonl.gz` | The readings: how each window stood over time. |
| `accounts.json` | The accounts record, naming each account by id and label. |
| `export.json` | Which Mac the folder belongs to, and when it last exported. |

The logs and the dashboard settings stay behind too. Every flag is in
[`switchboard export`](commands/export.md) and [`switchboard import`](commands/import.md), and
each file is in [Reference](reference.md). If an export fails, see
[Troubleshooting](troubleshooting/exports.md).

## Related

- [Your record](your-record.md): what the record holds, and how to read it.
- [Configuration](configuration.md): the `[export]`, `[ledger]` and `[history]` keys.
- [Pools](pools.md): how a pool keeps the record of its work, even on another Mac.
