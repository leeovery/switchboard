# `switchboard import`

`import` brings the history in the folders that [`export`](export.md) writes into the record of this
Mac. Point it at the `<dir>` you export into, and it takes in the folder of every Mac there. Point it
at the folder of one Mac, and it takes in that one. A new Mac uses it to take over the history of
the last, and a Mac that lost its history takes its own back.

```console
$ switchboard import ~/Dropbox/switchboard
imported 1 folder: 27 days added, 3 merged, 12 past keeping left out, 39 days' summaries
```

```bash
switchboard import <dir>
```

`import` takes no flags. A relative `<dir>` is taken from the working directory. It needs the
router, which writes the history, and fails without it.

Why you'd import, and how to restore onto a new Mac, is in
[Several Macs and backups](../several-macs.md).

## What it takes in

A folder of a Mac is one holding an `export.json` that reads. The folder of this Mac is taken in
like any other. A folder with no export in it at all is passed over without a word.

| What the folder holds | What `import` does |
|---|---|
| A day this Mac doesn't hold | Copies it in. |
| A day this Mac holds already | Adds the lines this Mac doesn't hold. |
| A line held more times in the folder than on this Mac | Adds it the extra times, for example an event that changed back to how it stood. |
| The summary of a day | Copies it in if this Mac holds no line of the day, and no summary or an empty one. An empty summary gives way only to one that counts lines. Otherwise this Mac keeps its own. |
| Lines older than `[ledger] keep`, or readings older than `[history] keep` | Leaves them out, as the router would remove them. Their summaries still come in. |

Each line is told apart from the lines of every other Mac. So an import repeated, or of the folder
of this Mac, adds nothing.

Two lines alike but for their pool count as one, and the line this Mac holds stays. Each Mac gives
the days before pools to its own first pool. So a folder taken in before its Mac moved to pools,
and again after, adds nothing the second time.

## Accounts across Macs

The accounts record of each folder joins the record of this Mac, so each day lands on the right
account.

`import` matches each account in the folder to one here, by the first rule that holds:

1. In the folder of this Mac, an account matches the account here with the same id.
2. An account matches the account here with the same subscription.
3. An account matches the account here with the same id, unless both subscriptions are known and
   differ.

| If the account | Then |
|---|---|
| Matches an account here | Its days go to the account here, under the id this Mac gives it. |
| Matches none, but this config has its id | It's that account, added to the record here. |
| Matches none otherwise | It's added, named by its label on the other Mac, and marked removed if it was removed there. |
| Was relabelled on the other Mac, and this Mac neither configures nor removed it | It takes the new label. |

An added account whose id is taken here already gets a new id, made from the install and the old
id. It gets the same id at every import.

A pool from another Mac keeps its history here. Read it with `--pool` and its id.

## What it prints

| What happened | What it prints |
|---|---|
| Days added and merged | `imported 1 folder: 27 days added, 3 merged, 12 past keeping left out, 39 days' summaries` |
| Only summaries, of days past keeping | `imported 1 folder: 12 days' summaries, 12 days' lines past keeping left out` |
| Nothing new | `imported 1 folder: nothing new` |
| Nothing new, with days past keeping | `imported 1 folder: 12 days past keeping left out` |

`import` copies the folders into `import/` in the state directory, then has the router take them in.
It waits for the router however long that takes. The router then sums up the days at once, so
`history` and the dashboard show them within minutes.

## Folders it passes over

`import` passes over these folders, says why for each, and carries on with the rest:

| Folder | What to do |
|---|---|
| An `export.json` that doesn't read, for example a damaged one | Export it again. |
| An export with no `export.json`, cut short or not yet synced | Import again once it's exported whole. |
| An `export.json` that names an accounts record not there yet | Import again once the sync has carried it. |
| An accounts record that doesn't read | Export it again. |
| Another Mac, exported before switchboard made account ids | Export it again from that Mac once it runs this version. |

The folder of this Mac from before account ids is taken in, under the ids its accounts had.

```
switchboard: passed over /Users/you/Dropbox/switchboard/mac-0123456789abcdef: its export.json doesn't read
```

## When it fails

| If | Then |
|---|---|
| `<dir>` holds no folder of a Mac, or every folder was passed over | `import` fails, saying so. |
| A folder was exported by a newer switchboard | `import` fails. Upgrade switchboard to import it. |
| The router is older than `import` | It fails, saying to run `switchboard service restart`, or to stop the router and run `switchboard serve` again. |
| The router still owes the move of the history onto account ids | It refuses until the move is done, and says what holds it up. |
| Another import is running | It refuses with `Error: an import is running already`, and leaves the running one alone. |
| The router refuses, or a day can't be taken in | It fails, saying why. What came in stays, and importing again takes in the rest. |

An import keeps running in the router if you interrupt the command, so a second one is refused
until it ends.

## Related

- [`export`](export.md): write the folder that `import` reads.
- [Several Macs and backups](../several-macs.md): backups, new Macs and two Macs at once.
- [Pools](../pools.md): how a pool keeps the history of its work.
