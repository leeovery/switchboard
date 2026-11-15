# Claude and switchboard: Claude knows its own limits

Claude can read the usage of every account, how fast each window is being spent, and when each
resets. So you can ask Claude where a session stands, or to plan a long job around what's left.
`switchboard setup` installs a skill that teaches Claude the commands, so no session needs
switchboard explained.

## Try it

In any session started through switchboard, ask Claude things such as these:

```text
Which account is this session on?
How much of this week is left on each account?
Use what's left of the week before it resets at 08:00, without reaching 100%.
Move this session to personal.
```

To answer, Claude runs commands such as these:

```bash
switchboard status --session "$CLAUDE_CODE_SESSION_ID"   # prints the label of the account this session is on, such as work
switchboard status                                       # every account's windows, as JSON when Claude runs it
switchboard pin q4tx --session "$CLAUDE_CODE_SESSION_ID" # moves this session to personal, named by its id, from the next request
```

## What happens

- Claude reads every window from `switchboard status`: how much is used, when it resets, and how
  much it can spend an hour or a day and still last until the reset.
- Claude names accounts to you by their labels, such as `work`, and gives commands the ids.
- When you ask for a long job paced to a reset, Claude works through it by the rate and run-out
  that switchboard reports.
- Claude looks back with `sessions`, `requests`, `history` and `events`. It sees your sessions,
  their requests and what they were worth at API prices.
- When you ask, Claude pins new sessions to an account, moves running sessions there, or moves one
  session only.
- Inside a pool, Claude reads and acts on that pool only. The pool id is in `$SWITCHBOARD_POOL`, and
  the skill has Claude pass it as `--pool`.

## Good to know

- With one account, Claude still paces its work on live usage.
- The skill is a file that switchboard owns, in `~/.claude/skills/switchboard/`. After an upgrade,
  the router replaces an older copy as it starts, with any edits you made. If you remove the skill,
  it stays removed until you run `switchboard setup` again.
- `setup` also adds a hook that runs as every session starts or resumes. If a session didn't start
  through switchboard, the hook shows you a line saying so. For example, the desktop app and some
  editors run a Claude Code of their own. Claude isn't told, because a hook message to Claude would
  go into every prompt of a headless `claude -p` run.
- A move costs one slower turn while the cache rebuilds on the new account.

## Reference

These are the verbs for agents. Each prints JSON with no flag wherever its output isn't a terminal,
such as the Bash tool of Claude. `--pretty` prints the text instead.

| Command | What Claude gets |
|---|---|
| `switchboard status` | Every account's windows: use, pace, resets, and what holds the account back. |
| `switchboard status --session <id>` | The label of the account the session is on. Add `--json` for JSON. |
| `switchboard status --refresh` | A fresh probe of each account that can take no request, after you reset a limit by hand on claude.ai. |
| `switchboard sessions [<id>]` | Today's sessions, or the page of one session with its turns. |
| `switchboard requests` | Today's requests, one JSON object a line. `--since` reaches further back. |
| `switchboard history` | The last 30 days, summed up and priced against the plans. |
| `switchboard events` | What the router decided and noticed: moves, limits, pins and refusals. |
| `switchboard pin <id>` | Sends new sessions to that account. `--move` and `--session` move running ones. |

Each command prints what the matching dashboard view draws, built by the same code, so the two
never disagree. Every flag is in [Commands](commands.md), and every JSON field is in
[Reference](reference.md). If a session says it isn't going through switchboard, see
[Troubleshooting](troubleshooting.md).

## Related

- [Pinning and moving sessions](pinning-and-moving.md): what a pin does, and when it gives way.
- [Your record](your-record.md): what `requests`, `history` and `sessions` read back.
- [Limits and routing](limits-and-routing.md): how the router picks an account for each session.
