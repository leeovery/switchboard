---
name: switchboard
description: Switchboard spreads Claude Code sessions across several Claude subscriptions. Use when the user asks which Claude account a session is on, about usage or limits across their accounts, about pinning or moving sessions between accounts, or about switchboard itself.
---

<!-- switchboard writes this file, and overwrites edits to it. -->
<!-- switchboard skill version: 1 -->

`claude` runs through switchboard, a local router that spreads Claude Code sessions across several
Claude subscriptions, which it calls accounts.

- To find the account a session is on, run `switchboard status --session <id>`. Your own session's
  id is in `$CLAUDE_CODE_SESSION_ID`, and Claude Code's `/status` shows a session's id.
- To see every account's usage, run `switchboard usage`.
- To steer sessions, run `switchboard pin <account>`, which sends new sessions to that account. Add
  `--move` to move running sessions there too, or `--session <id>` to pin that one session alone.
  `switchboard pin auto` goes back to routing.
- To read the router's log, which says why each request went to its account, run
  `switchboard logs`.

A move to another account costs one slower turn while the cache rebuilds there, and a
Claude Sonnet 5.5 session that moves carries on without its earlier reasoning.

Artifacts and uploads always live on the primary account, whichever account the conversation is on.

For the rest, run `switchboard --help`, or a command's `--help`.
