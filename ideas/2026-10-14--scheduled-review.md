# A scheduled review of switchboard's records

A job that runs every so often, reads switchboard's records for what looks wrong, and raises what it
finds: as an issue to pick up, or as a pull request that fixes it. Perhaps a Claude Code routine, or
a scheduled task in the Claude app. Raised by the owner on 14 October 2026, to discuss.

What it would read lives on the machine switchboard runs on: the request ledger, the router's events,
the readings history and the logs. A job running in the cloud can't see them, so it runs there, or
is handed what it reads.

To settle with the owner:

- where it runs, and how often;
- what it reads, never the tokens or the config's secrets;
- what it may do with what it finds: open an issue, open a pull request, or only report;
- whether it starts from the doctor verb's report (`2026-10-14--doctor.md`).
