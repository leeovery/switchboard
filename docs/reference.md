# Reference

The files switchboard writes and the JSON its commands print, field by field, for anyone who reads
them directly, agents included. Each page describes one format. For what a feature does and why,
start from the [docs index](README.md).

Start with [Reading the JSON](reference/reading-the-json.md): the rules there hold for every format.

## Files and setup

| Page | What it holds |
|---|---|
| [Where the files are](reference/files.md) | Every file and folder switchboard keeps, where it lives, and how long it stays. |
| [The session hook](reference/session-hook.md) | The entry `setup` adds to the settings of Claude Code, and what it prints. |
| [What `claude` sets](reference/environment.md) | The variables and headers a routed session starts with. |
| [Tokens](reference/tokens.md) | Where the token of each account is kept, and the record of when it was taken. |

## The record

| Page | What it holds |
|---|---|
| [The request ledger](reference/ledger.md) | The line recorded for each request. |
| [Routing reasons](reference/reasons.md) | The fixed words for why a request went to its account. |
| [The day summary](reference/day-summary.md) | The summary of each day, kept for good. |
| [The readings history](reference/readings.md) | Each reading of a window. |
| [The router events](reference/events.md) | Each event: moves, limits, pins, restarts and the rest. |
| [The accounts record](reference/accounts-record.md) | `accounts.json`: every account and pool this Mac has had. |
| [Export folders](reference/export-folder.md) | `export.json` in a folder an export writes. |

## The JSON the verbs print

| Page | What it holds |
|---|---|
| [The status document](reference/status-json.md) | What `status` and `usage` print. |
| [Accounts in the status document](reference/status-accounts.md) | Each account: its windows, its token, and what holds it back. |
| [`status --session` JSON](reference/status-session-json.md) | Where one session sends its requests, and why. |
| [`history` JSON](reference/history-json.md) | The days, the year, the months, and how worth is priced. |
| [`history` JSON: the account blocks](reference/history-accounts-json.md) | Weeks, tokens, totals, plans and capacity. |
| [`sessions` JSON](reference/sessions-json.md) | The sessions of today, and the page of one session. |
| [`pool` JSON](reference/pool-json.md) | Every pool, with its fence and its accounts. |

## The router

| Page | What it holds |
|---|---|
| [The control API](reference/control-api.md) | The endpoints on `control.sock`, and how each chooses a pool. |
