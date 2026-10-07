---
name: switchboard
description: Switchboard spreads Claude Code sessions across several Claude subscriptions. Use when the user asks which Claude account a session is on, about usage or limits across their accounts, about pinning or moving sessions between accounts, or about switchboard itself.
---

<!-- switchboard writes this file, and replaces it, edits and all, with each new version. -->
<!-- switchboard skill version: 7 -->

`claude` runs through switchboard, a local router that spreads Claude Code sessions across several
Claude subscriptions, which it calls accounts.

- To find the account a session is on, run `switchboard status --session <id>`. Your own session's
  id is in `$CLAUDE_CODE_SESSION_ID`, and Claude Code's `/status` shows a session's id.
- To see every account's usage, run `switchboard status --json`. It prints the status document:
  each account's windows, with how much of each is used, as a fraction, and when it resets; what
  holds the account back, such as a limit it reached, a refusal, or its reserve; how fast its
  5-hour window is going, and whether that has it under pressure, running out before it resets;
  and its sessions. With the accounts come the global pin, the account best used next, and the
  router's health.
- After a limit is reset by hand on claude.ai, run `switchboard status --json --refresh`, which
  has the router first probe each account that can take no request, however lately it read it,
  but never twice in a minute, so it sees the reset.
- `switchboard usage` is the dashboard, cards and bars for the user to look at, and
  `switchboard usage -w` keeps it on screen. Suggest it when the user wants to watch; read
  `status --json` yourself.
- To steer sessions, run `switchboard pin <account>`, which sends new sessions to that account, or
  `switchboard pin <account> <account>...`, which sends them to the best of those accounts until
  every one of them is out, and only then to the others. Add `--move` to move running sessions
  there too, or `--session <id>` to pin that one session alone to one account.
  `switchboard pin auto` goes back to routing.
- To look back at requests, run `switchboard requests --json`, a JSON object a line for each of
  today's, as the router's request ledger holds it: its session, model and account, why it went
  there, its status, and the usage the API gave. `--session <id>` and `--account <id>` narrow it,
  and `--since` reaches back to a day, a time today or a while ago: `2026-10-01`, `14:00`, `2d`.
- To look back over days, run `switchboard history --json`: each of the last 30, today's last,
  each account's requests by model, with their usage, sessions and `worth`, what they'd have cost
  through the API, in US dollars, and `unpriced` naming what that leaves out, as `no_usage`,
  requests that got no usage back; and each account's moves, the limits it reached and its
  windows' highest use. Both read the ledger's files, so they work without the router.
- To read the router's log, which says why each request went to its account, run
  `switchboard logs`.

A move to another account costs one slower turn while the cache rebuilds there, and a
Claude Sonnet 5.5 session that moves carries on without its earlier reasoning.

Artifacts and uploads always live on the primary account, whichever account the conversation is on.

For the rest, run `switchboard --help`, or a command's `--help`.
