# `switchboard export`: a copy of your record and config, kept off this Mac

`export` copies the record and the config of this Mac into a folder of its own inside `<dir>`.
Keep `<dir>` where Dropbox or iCloud Drive syncs it, and your record outlives the Mac. Switchboard
only writes the folder: keeping it safe, or carrying it to another Mac, is up to you.

```console
$ switchboard export \
    ~/Dropbox/switchboard
exported 31 days to /Users/you/Dropbox/switchboard/mac-0123456789abcdef: 3 written, 28 as they were
```

```bash
switchboard export <dir>
```

`export` takes no flags. A relative `<dir>` is taken from the working directory, and `<dir>` is made
if it isn't there. To have the router export every hour, set `dir` under `[export]` in the
[configuration](../configuration.md).

Why you'd export, and how to restore onto a new Mac, is in
[Backups and a new Mac](../several-macs.md).

## Each Mac's folder

The folder is `mac-<id>`, named by the install id. Macs that name the same `<dir>`, such as
through a config kept in a dotfiles repository, never write over each other.

| File | Holds |
|---|---|
| `export.json` | Whose folder it is, when it was last exported to, and its format. |
| `accounts.json` | The accounts record, which names each account by id and label. |
| `config.toml` | A copy of the config, without any token. |
| `ledger/requests-<date>.jsonl.gz` | The request ledger, one file a day. |
| `ledger/events-<date>.jsonl.gz` | The router's events, one file a day. |
| `ledger/day-<date>.json` | Each day's summary. |
| `history/readings-<date>.jsonl.gz` | The readings history, one file a day. |

`gzip -dc` reads each compressed file. The format of every file is in
[Reference: the export folder](../reference/export-folder.md).

The export leaves out what the router builds again, and what isn't part of your record:

- the digests, parts, heads and rows the router makes from the lines;
- `state.json` and `prefs.json`;
- the tokens, which `setup` makes on each Mac;
- the logs.

## The copy of the config

```console
$ switchboard export ~/Dropbox/switchboard
exported 31 days to /Users/you/Dropbox/switchboard/mac-0123456789abcdef: 0 written, 31 as they were
switchboard: couldn't copy the config, so the folder keeps the copy it has: the config gives a value shaped like a token, which a copy of it never holds
```

`export` copies the config that switchboard reads, so a new Mac can lay it down. The copy never
holds a token. Your tokens live in files of their own, and the config names none of them. A config
that can't be copied fails no export.

| If the config | Then |
|---|---|
| Has a token in a comment | The copy hides the token as `[redacted]`. |
| Gives a token as a value, for example inside the `upstream` address | `export` copies no config, keeps the copy in the folder, and says why. |
| Doesn't load | `export` copies no config, keeps the copy in the folder, and says why. |
| Isn't there | `export` copies none, and says nothing. |

The hourly export copies the config the router started from. If the router can't copy it, it
warns once in `router.log`.

## How it writes

| Rule | What it means |
|---|---|
| Only what changed | A day already in the folder, and unchanged since, is left as it is. |
| Recent days | The last three days aren't compressed yet. They're compressed on the way, and copied again once a line is added. |
| Never half written | Each file is written beside its place and renamed into it, so a sync never carries half a file. |
| A line still being written | It's left for the next export. |
| Nothing removed | Nothing in the folder is removed, a day past keeping included. You remove what you no longer want. |
| Leftovers | An export cut short can leave hidden `.<name>.<digits>` files. Each export removes those unchanged for an hour. |

`export` reads the files where they lie, so it doesn't need the router.

## When it fails

| If | Then |
|---|---|
| `<dir>` is a file, or can't be made or written | `export` says so and stops there. |
| A day in the state directory can't be read | `export` exports the rest, names each day it passed over and why, then exits `1`. |
| The router still owes the move of your record onto account ids | `export` refuses until the move is done, and says what holds it up. |

```
switchboard: couldn't export the request ledger of 2026-10-30: open /Users/you/.local/state/switchboard/ledger/requests-2026-10-30.jsonl.gz: permission denied
```

If the hourly export fails, see [Troubleshooting: exports](../troubleshooting/exports.md).

## Disks that keep coarse times

`export` tells an unchanged day by its size and modification time. Some disks keep coarser times
than macOS does.

| Disk | What happens |
|---|---|
| SMB shares, and disks that keep times to the microsecond, 10 ms (exFAT) or the second | Unchanged days are left as they are. |
| FAT, which keeps times to 2 seconds, or a disk that rounds them | Some unchanged files are written again at each export. |

## The install id

The install id is in `install-id`, in the state directory. The first export makes it, and
nothing replaces it.

If you restore the state directory of one Mac onto another, for example with Migration Assistant,
the install id comes with it. The new Mac then exports into the folder of the old one. Remove
`install-id` to give it an id, and a folder, of its own at its next export.

## Related

- [`import`](import.md): bring the folders back in, on this Mac or another.
- [Backups and a new Mac](../several-macs.md): backups, new Macs and two Macs at once.
- [Your record](../your-record.md): what your record holds.
