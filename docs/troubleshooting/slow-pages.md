# Troubleshooting: the dashboard is slow to look back

The ledger keeps a file of request lines a day. Beside it, the router writes four smaller files a
day that pages read in place of the lines. This page covers a page that's slow because one of
those files is missing or damaged.

## The files the router works out

Each of these is worked out from the lines of one day. They're in
`~/.local/state/switchboard/ledger/`, or in `$XDG_STATE_HOME/switchboard/ledger` if you set
`XDG_STATE_HOME`.

| File | What it holds |
|---|---|
| `digest-<date>.json.gz` | Where each session stood at the end of the day. |
| `parts-<date>.gz` | The part of each session in that day, read by the session page. |
| `logday-<date>.json.gz` | The day's head, read by the Log's find. |
| `rows-<date>.gz` | The day's rows, read by the Log and by a turn. |

All four are safe to delete. The router writes them again on its next round, or when it next
starts. Until then, the dashboard reads the lines instead, which is slower but shows the same.

## The session page is slow to open

A parts file is damaged, but the size didn't change, so nothing notices until a page reads it. The
page then rests on an older digest, or reads the session's days whole, at every open. To have
the router write that day again, from the ledger folder above:

```bash
cd ~/.local/state/switchboard/ledger
# The router writes both again on its next
# round
rm digest-2026-10-30.json.gz \
  parts-2026-10-30.gz
```

## The Log's find is slow to go back

For each past day, the find reads the head. It reads the lines only if the head isn't written
yet, which takes half a second or more.

| If | Then |
|---|---|
| You've just upgraded to the version with heads | Wait. A long ledger takes some minutes. |
| It's yesterday, early in the morning | Wait until about an hour after midnight. |
| One day stays slow at every find | Delete `logday-<date>.json.gz` for that day. |

After the upgrade, the router writes a head for every day kept, yesterday first, at 1 to 1.5
seconds a day. It starts once every digest due is written. The head for yesterday is written about
an hour after midnight, just after the digest. A head damaged without its size changing is never
written again on its own, so delete it and the router writes it again.

## The Log is slow to go back a day, or a turn is slow to open

The Log reads each past day from the rows file, and a turn reads its own past days the same way.
Yesterday reads the lines until about an hour after midnight, as today does.

| If | Then |
|---|---|
| You've just upgraded to the version with rows | Wait for the router to write them. |
| A rows file or head is deleted, or its size changed | Wait for the router to write both again. |
| One day stays slow every time | Delete `rows-<date>.gz` for that day. |

After the upgrade, the router writes rows for every day kept, yesterday first, as it writes heads.
A day without its rows file or head reads the lines, half a second or more, and shows nothing
otherwise. A rows file damaged without its size changing goes unnoticed by the router, so delete
it.

## Related

- [Your record and what it's worth](../your-record.md): what the ledger holds.
- [Where the files are](../reference/files.md): the state directory, and the files in the ledger.
- [The Log](../dashboard/log.md) and [the session page](../dashboard/session-page.md): where the dashboard reads the ledger.
