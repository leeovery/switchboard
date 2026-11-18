# `switchboard import`: one record from several Macs

`import` takes the folders that [`export`](export.md) writes, and adds their record to this Mac's.
Point it at the `<dir>` you export into, and it takes in every Mac folder there. Point it at one
Mac's folder, and it takes in that one. A new Mac uses it to take over the record of the last, and a
Mac that lost its record takes its own back.

```console
$ switchboard import \
    ~/Dropbox/switchboard
imported 1 folder: 27 days added, 3 merged, 12 past keeping left out, 39 days' summaries
```

```bash
switchboard import <dir>
```

`import` takes no flags. A relative `<dir>` is taken from the working directory. It needs the
router, which writes your record, and fails without it. The one exception is a Mac with no config.

Why you'd import, and how to restore onto a new Mac, is in
[Backups and a new Mac](../several-macs.md).

## On a Mac with no config

```console
$ switchboard import ~/Dropbox/switchboard
laid down the config of mac-0123456789abcdef at /Users/you/.config/switchboard/config.toml: 3 accounts in 2 pools
next, run switchboard setup to give each account its token and start the router, then switchboard import /Users/you/Dropbox/switchboard again to bring in the history, once the router has started and finished its first upgrade steps, which takes moments
```

If this Mac has no config and no router runs, `import` lays down the copy of the config that an
export keeps in the folder. With `--config`, it lays the config down at that path. It takes in no
history yet. The accounts and pools keep their ids, so the history lands where it was. The copy
holds the values of the config alone, without the comments. `import` writes `# do not edit`
beside each id, as switchboard does in any config it writes.

| If `<dir>` holds | Then `import` lays down |
|---|---|
| The folder of this Mac, for example a backup of a Mac that lost its config | The copy in that folder, and leaves the accounts record of this Mac as it is. |
| The folder of one other Mac, for example on a new Mac | The copy in that folder, and records each account with its subscription. |
| The folders of several other Macs | Nothing. It names each folder, so you can point `import` at the one you want. |

To lay down the config of one Mac from a shared `<dir>`, give its folder:

```bash
switchboard import \
  ~/Dropbox/switchboard/mac-0123456789abcdef
```

Each account is recorded with its subscription. So `switchboard setup` asks only for each
token, and refuses a token made for another account. If a pool fences a folder that isn't on this
Mac, `import` warns, and names the command that fences the pool anew. If `[export] dir` names a
folder that isn't on this Mac, `import` warns. The router then exports nothing until that folder,
or the folder it goes in, is there.

`import` never writes over anything at the config path. It lays down nothing in these cases:

| If | Then |
|---|---|
| A link or file at the config path isn't a config that loads | It says what's there. |
| This Mac has a history of its own, and `<dir>` holds no folder of this Mac | It says to run `switchboard setup`, then import again. |
| A folder in `<dir>` was passed over, for example one not yet synced | It names each one. Import again once each folder reads. |
| The folder holds no config, for example one exported before the config was copied | It says to export again from the old Mac first, if it still runs. |
| `<dir>` holds no export, or isn't there | It says so. |
| The config in the folder gives a secret as a value | It says why. |

If an import stops after it records the accounts, but before it lays down the config, run it
again. It goes ahead while this Mac holds no history yet.

## What it takes in

A Mac folder is one holding an `export.json` that reads. The folder this Mac exported is taken in
like any other. A folder with no export in it at all is passed over without a word.

| What the folder holds | What `import` does |
|---|---|
| A day this Mac doesn't hold | Copies it in. |
| A day this Mac holds already | Adds the lines this Mac doesn't hold. |
| A line held more times in the folder than on this Mac | Adds it the extra times, for example an event that changed back to how it stood. |
| A day's summary | Copies it in if this Mac holds no line of that day. Otherwise this Mac keeps its own. See below. |
| Lines older than `[ledger] keep`, or readings older than `[history] keep` | Leaves them out, as the router would remove them. Their summaries still come in. |

A summary comes in only if this Mac has none for the day, or an empty one that counts no lines,
while the summary in the folder counts some.

Either way, the summary the day keeps learns the pools of the accounts that sent no requests that
day. If the summary in the folder names the pool of such an account, and this Mac's summary has no
day for that account, the pool comes in. The same holds the other way round, if the summary in the
folder is copied in.

Each line is told apart from the lines of other Macs. So an import repeated, or of the folder this
Mac exported, adds nothing.

Two lines alike but for their pool count as one, and the line this Mac holds stays. Each Mac gives
the days before pools to its own first pool. So a folder taken in before its Mac moved to pools,
and again after, adds nothing the second time.

## Accounts across Macs

The accounts record in each folder joins the accounts record of this Mac, so each day lands on the
right account.

`import` matches each account in the folder to one here, by the first rule that holds:

1. In the folder this Mac exported, an account matches the account here with the same id.
2. An account matches the account here with the same subscription.
3. An account matches the account here with the same id, unless both subscriptions are known and
   differ.

| If the account | Then |
|---|---|
| Matches an account here | Its days go to the account here, under the id this Mac gives it. |
| Matches none, but this config has its id | It's that account, added to the accounts record here. |
| Matches none otherwise | It's added, named by its label on the other Mac, and marked removed if it was removed there. |
| Was relabelled on the other Mac, and this Mac neither configures nor removed it | It takes the new label. |

An added account whose id is taken here already gets a new id, made from the install and the old
id. It gets the same id at every import.

## Pools across Macs

```console
$ switchboard import ~/Dropbox/switchboard
imported 1 folder: 27 days added
switchboard: kept the pool "Client" (c1nt) of mac-fedcba9876543210 apart, as its accounts are in more than one pool here: read it with --pool c1nt
```

Each pool of another Mac counts as the pool here that holds the same accounts. `import` reads
which accounts a pool holds from the copy of the config in each folder. It counts only the
accounts this Mac configures. A pool it can't match stays apart, and `import` names it, as above.
The answer of the router gives the reason as `why`.

| If the pool from the folder | Then | `why` |
|---|---|---|
| Is a pool this Mac configures, or had before | Its days stay in that pool. | |
| Holds accounts that all sit in one pool here | Its days go to that pool here. | |
| Holds accounts that sit in two pools here or more | It stays apart. | `split` |
| Holds no account this Mac configures | It stays apart. | `none_here` |
| Holds no account on the other Mac | It stays apart. | `empty` |
| Comes onto a Mac whose config has no pools | It stays apart. | `no_pools_here` |
| Comes from a folder with no copy of the config | It stays apart, unless another folder in the import places it. | `no_config` |
| Was kept apart by an earlier import | New days go to the pool here. Any days imported before stay apart. | `imported_before` |

A pool id names one pool on every Mac. So `import` places the pools of each Mac the same way from
every folder, whatever their order. It counts every account that the configs in the import put in
the pool. Work that names no pool goes to the first pool of the Mac that recorded it.

`imported_before` means the pool was kept apart, under its id, before this import began. Now its
accounts sit in one pool here, so its new days go to that pool. Any days taken in before stay
under the id, because a line held already stands against the same line in another pool. Read
them with `--pool` and the id. `import` says so at each import, even if no days came in before.

A pool kept apart keeps its record here. Read it with `--pool` and its id. `import` names each
pool it keeps apart once, however many folders hold it. `import` never changes the config or the
pools on this Mac.

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
| An `export.json` that names a config not there yet | Import again once the sync has carried it. |
| A config that doesn't read | Export it again. |
| Another Mac, exported before switchboard made account ids | Export it again from that Mac once it runs this version. |

The folder this Mac exported before account ids is taken in, under the ids its accounts had.

```
switchboard: passed over /Users/you/Dropbox/switchboard/mac-0123456789abcdef: its export.json doesn't read
```

## When it fails

| If | Then |
|---|---|
| `<dir>` holds no Mac folder, or every folder was passed over | `import` fails, saying so. |
| A folder was exported by a newer switchboard | `import` fails. Upgrade switchboard to import it. |
| The router is older than `import` | It fails, and says to restart the router. |
| The router still owes the move of your record onto account ids | It refuses until the move is done, and says what holds it up. |
| Another import is running | It refuses with `Error: an import is running already`, and leaves the running one alone. |
| The router refuses, or a day can't be taken in | It fails, saying why. What came in stays, and importing again takes in the rest. |

To restart the router, run `switchboard service restart`, or stop a router you run by hand and run
`switchboard serve` again. An import keeps running in the router if you interrupt the command, so a second one is refused
until it ends.

## Related

- [`export`](export.md): write the folder that `import` reads.
- [Backups and a new Mac](../several-macs.md): backups, new Macs and two Macs at once.
- [Pools](../pools.md): how a pool keeps the record of its work.
