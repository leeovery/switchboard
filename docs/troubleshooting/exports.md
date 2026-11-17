# Troubleshooting: an export that fails

If you set `dir` in `[export]`, the router exports your record into that folder every hour. This
page covers how to see where the exports stand, and what to do when one fails.

```bash
switchboard status --json | jq .export
```

| Field | What it says |
|---|---|
| `at` | When the last export succeeded. |
| `error` | Why the last one failed. |
| `running` | When the export under way began. |

The router's log has a failure at warn, once until an export succeeds. It also logs the first
export that succeeds after it.

## The error says `macOS refused access to <dir>`

macOS asks before the router writes into iCloud Drive, Dropbox or `~/Documents`, and you didn't
allow it. Allow switchboard in System Settings → Privacy & Security. The next export, within the
hour, goes through.

If you run the router by hand with `switchboard serve`, macOS asks about the terminal you run it
in. The error then says to allow that terminal app.

## The error says `<path> is locked`

A file or folder in the export is locked in the Finder. Unlock it in its Get Info window.

## The folder isn't there

The router makes `dir` only if the folder it goes in exists. So a disk that isn't mounted is never
written onto this Mac's disk in its place. Mount the disk, or make the folder that `dir` goes
in.

## An export shows `running` for long

The export waits on the macOS prompt that asks whether to allow switchboard. Until you answer, no
other export into that folder starts. The router doesn't restart in place either, so an upgrade or
a config change waits too.

| Where | What you see |
|---|---|
| `switchboard status` | The restart waits for the export. |
| `switchboard service restart` | `an export into <dir> is under way: try again once it ends` |

Answer the prompt, or allow switchboard in System Settings → Privacy & Security. Stopping the
router doesn't wait for the export.

## The log says a day was passed over

A day file in the state directory can't be read. The router's log warns of it, for example
`couldn't export the request ledger of 2026-10-30; passed over`, and the rest is exported. That
doesn't fail the export.

## The log says it can't remove a file an export cut short

An export cut short, for example by the router stopping, left a hidden `.<name>.<digits>` file in
the export, and the router can't remove it. That doesn't fail the export. The log line names the
file. Check its permissions, and whether the file or its folder is locked in the Finder. Or remove
it by hand.

## Exporting now

To export now, without the router:

```bash
switchboard export ~/Dropbox/switchboard
```

macOS asks about your terminal apart from the router. So an export that works from the terminal
tells you nothing about what the router may do.

## Related

- [Backups and a new Mac](../several-macs.md): what an export holds, and importing it.
- [`switchboard export`](../commands/export.md): every flag of `export`.
- [Account ids](account-ids.md): an import refused across the upgrade to ids.
