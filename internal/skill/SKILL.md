---
name: switchboard
description: Switchboard spreads Claude Code sessions across several Claude subscriptions. Use when the user asks which Claude account a session is on, about usage or limits across their accounts, about their sessions, requests or what they were worth, about pinning or moving sessions between accounts, or about switchboard itself.
---

<!-- switchboard writes this file, and replaces it, edits and all, with each new version. -->
<!-- switchboard skill version: 10 -->

`claude` runs through switchboard, a local router that spreads Claude Code sessions across several
Claude subscriptions, which it calls accounts.

- To find the account a session is on, run `switchboard status --session <id>`. Your own session's
  id is in `$CLAUDE_CODE_SESSION_ID`, and Claude Code's `/status` shows a session's id.

`status`, `usage`, `sessions`, `requests`, `history` and `events` print JSON in your Bash tool, as
anywhere their output isn't a terminal, with no flag. `status --session <id>` alone doesn't: it
prints the bare account id there too, for a statusline, and JSON only with `--json`.

- To see every account's usage, run `switchboard status`. It prints the status document: each
  account's windows, with how much of each is used, as a fraction, and when it resets; each window's
  even pace, where even use would have put it by now, and its allowance, what can be spent of it an
  hour or a day and still last to its reset; what holds the account back, such as a limit it
  reached, a refusal, or its reserve; how fast its 5-hour window has been used lately, and whether
  that has it under pressure, running out before it resets; and its sessions. With the accounts come
  the pool, the room new sessions can go to, summed across its accounts; what's coming up, soonest
  first, as a limit lifting, a window running out or resetting, or a prime; the global pin, the
  account best used next, and the router's health.
- After a limit is reset by hand on claude.ai, run `switchboard status --refresh`, which has the
  router first probe each account that can take no request, however lately it read it, but never
  twice in a minute, so it sees the reset.
- To see the sessions running and today's, run `switchboard sessions`: each one's account, model,
  what it's doing, its requests today and their `worth`, what they'd have cost through the API, in
  US dollars, and how it came to its account. `switchboard sessions <id>` gives one session, by
  as much of its id as is unique among today's, or a session of an earlier day by its whole id:
  each account it ran on, its moves, its turns, newest first, and its totals.
- To look back, with or without the router, run `switchboard requests`, through the request ledger,
  a JSON object a line for each of today's requests: its session, model and account, why it went
  there, its status, and the usage the API gave, `--session <id>` and `--account <id>` narrowing it;
  `switchboard history`, through the ledger and the readings history, the last 30 days, each
  account's requests by model, with their usage, sessions and `worth`, `unpriced` naming what that
  leaves out, and its moves, the limits it reached and its windows' highest use, then over those
  days each account's totals, its plan and its worth against it, each week's peaks, and whether
  there are as many accounts as the use needs, or with `--windows`, each window's readings; or
  `switchboard events`, the router's events, a JSON object a line: sessions started and moved,
  limits, caps, refusals, primes and pins. `--since` reaches back to a day, a time today or a while
  ago: `2026-10-01`, `14:00`, `2d`.
- `switchboard usage` is the user's printout of every account's windows, and `switchboard dashboard`
  their live dashboard. Point the user to them when they want to look, rather than running them;
  read `status` yourself, as it gives what they show.
- To steer sessions, run `switchboard pin <account>`, which sends new sessions to that account, or
  `switchboard pin <account> <account>...`, which sends them to the best of those accounts until
  every one of them is out, and only then to the others. Add `--move` to move running sessions
  there too, or `--session <id>` to pin that one session alone to one account.
  `switchboard pin auto` goes back to routing.
- To read the router's log, which says why each request went to its account, run
  `switchboard logs`.

A move to another account costs one slower turn while the cache rebuilds there, and a
Claude Sonnet 5.5 session that moves carries on without its earlier reasoning.

Artifacts and uploads always live on the primary account, whichever account the conversation is on.

For the rest, run `switchboard --help`, or a command's `--help`.
