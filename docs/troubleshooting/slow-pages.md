# Troubleshooting: the dashboard is slow to look back

The ledger keeps a file of request lines a day. Beside it, the router writes four smaller files a
day that pages read in place of the lines. This page covers a page that's slow because one of
those files is missing or damaged.

## The files the router works out

Each of these is worked out from the lines of the day. They're in `~/.local/state/switchboard/ledger/`,
or under `$XDG_STATE_HOME` if you set it.

| File | What it holds, or what reads it |
|---|---|
| `digest-<date>.json.gz` | Where each session stood at the end of the day. |
| `parts-<date>.gz` | The part of each session in that day, read by the page of a session. |
| `logday-<date>.json.gz` | The head of the day, read by the find on the Log. |
| `rows-<date>.gz` | The rows of the day, read by the Log going back a day, and by a turn as it opens. |

All four are safe to delete. The router writes them again on its next round, or when it next
starts. Until then, the dashboard reads the lines instead, which is slower but shows the same.

## The page of a session is slow to open

A parts file is damaged, but the size didn't change, so nothing notices until a page reads it. The
page then rests on an older digest, or reads the days of the session whole, at every open. To have
the router write that day again:

```bash
cd ~/.local/state/switchboard/ledger                # or $XDG_STATE_HOME/switchboard/ledger, if you set it
rm digest-2026-10-30.json.gz parts-2026-10-30.gz    # the router writes both again on its next round
```

## The find on the Log is slow to go back

The find reads the head of each past day. It reads the lines of a day only if the head isn't
written yet, which takes half a second or more.

| If | Then |
|---|---|
| You've just upgraded to the version that brings heads in | The router writes one for every day kept, yesterday first, at 1 to 1.5 seconds a day. It starts once every digest due is written, so a long ledger takes some minutes. Wait for it. |
| It's yesterday, early in the morning | The head of yesterday is written about an hour after midnight, just after the digest. |
| One day stays slow at every find | The head is damaged, but the size didn't change, so it's never written again on its own. Delete `logday-<date>.json.gz` for that day. |

## The Log is slow to go back a day, or a turn is slow to open

The Log reads each past day from the rows file, and a turn reads its own past days the same way.
Yesterday reads the lines until about an hour after midnight, as today does.

| If | Then |
|---|---|
| You've just upgraded to the version that brings rows in | The router writes them for every day kept, yesterday first, as it writes heads. Wait for it. |
| The rows file or head of a day is deleted, or damaged so the size changed | That day reads the lines, half a second or more, until the router writes both again. Nothing shows otherwise. |
| One day stays slow every time | The rows file is damaged, but the size didn't change, so the router doesn't notice. Delete `rows-<date>.gz` for that day. |

## Related

- [Your record and what it's worth](../your-record.md): what the ledger holds.
- [Reference](../reference.md): the state directory, and the files in the ledger.
- [Dashboard](../dashboard.md): the Log, and the page of a session.
