# Export folders

Each folder that `switchboard export` writes for a Mac holds `export.json`, which says whose folder
it is and what it holds. This page lists its fields. For what an export copies, and how to run one,
see [`export`](../commands/export.md) and [Backups and a new Mac](../several-macs.md).

```json
{
  "version": 3,
  "install": "0123456789abcdef",
  "exported_at": "2026-11-12T09:00:00Z",
  "ids": true,
  "accounts": true,
  "config": true
}
```

## The fields

| Field | Type | Meaning |
|---|---|---|
| `version` | number | The version of the folder. `3` since pools. |
| `install` | string | The id of the install whose folder it is, as `install-id` holds it. |
| `exported_at` | time | When the folder was last exported to, in UTC, to the second. |
| `ids` | boolean | `true` if the export named accounts by the ids switchboard makes. Left out of a folder exported before ids. |
| `accounts` | boolean | `true` if the export wrote the accounts record, `accounts.json`, beside it. |
| `config` | boolean | `true` if the folder holds a copy of the config, `config.toml`. Left out if it holds none. |

An export writes `accounts.json` if the Mac has an accounts record. It writes `config.toml` if the
config loads and gives no secret as a value. The copy holds the values of the config alone,
without comments. Otherwise the export removes any copy the folder holds, and leaves `config`
out. So `config: true` always means the copy is from the latest export.

A switchboard from before the copy of the config reads a folder with `config` as it always did. The
field leaves the version at `3`.

## Which switchboard reads which version

| Version | Written by | Read by |
|---|---|---|
| 3 | A switchboard with pools | A switchboard with pools |
| 2 | A switchboard with account ids, before pools | Any switchboard with account ids |
| 1 | A switchboard from before account ids | Any switchboard |

- A switchboard refuses a folder of a newer version than it reads, and says to upgrade.
- A switchboard from before pools refuses version 3, because its days name pools.
- A switchboard from before ids refuses version 2, because its days name accounts by ids it has no
  token files for.
- A folder of version 2 or 1 has its days given the first pool of this Mac as they're imported.

## The order an export writes in

1. It writes the copy of the [accounts record](accounts-record.md) first. So the days a sync
   carries are never ahead of the record that names their accounts.
2. It writes the copy of the config, which says which accounts each pool holds.
3. It writes the days.
4. It writes `export.json` last, once every other file is written.

If a folder was first exported by an older switchboard, a new export removes the old
`export.json` before it starts. So a sync cut short never leaves the new days beside a manifest an
older switchboard would import.

## What `import` does with a folder

| If `export.json` | `import` |
|---|---|
| Says `accounts: true`, and the folder holds `accounts.json` | Takes the folder in |
| Says `accounts: true`, but the folder holds no `accounts.json` | Passes the folder over until it has synced whole |
| Says `config: true`, and the folder holds `config.toml` | Lays the config down on a Mac with none, if it's this Mac's folder or the only folder there. Otherwise reads which accounts each pool holds from it. |
| Says `config: true`, but the folder holds no `config.toml` | Passes the folder over until it has synced whole |
| Leaves out `ids` | Takes the folder in as one from before ids. See [`import`](../commands/import.md). |
