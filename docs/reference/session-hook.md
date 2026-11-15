# The session hook

`setup` adds a SessionStart hook to the settings of Claude Code. It tells you when a session starts
without going through switchboard. For why that happens, see
[Claude and switchboard](../claude-and-switchboard.md).

```json
{
  "hooks": [
    {
      "type": "command",
      "command": "[ -x '/opt/homebrew/bin/switchboard' ] && '/opt/homebrew/bin/switchboard' hook session-start || true",
      "timeout": 5
    }
  ]
}
```

## The entry

- The entry is a group of its own under `hooks.SessionStart` in `settings.json`, with no matcher.
- The path is the one of the switchboard that ran `setup`.
- `setup` knows the entry by the shape of its command: the same path in both places, naming a
  binary called `switchboard`.

| If the entry | `setup` |
|---|---|
| Matches the switchboard running `setup` | Leaves it as it is |
| Names a different path | Puts it right |
| Sits under a matcher that doesn't run it for every session | Puts it right |

An empty matcher, `null` and `*` each run the hook for every session.

## What it prints

Every `claude` that switchboard starts carries a mark, `SWITCHBOARD_STARTED`. The hook reads it.

| If the session | `switchboard hook session-start` prints |
|---|---|
| Carries the mark | Nothing |
| Has no mark, and is starting or resumed | The notice below |
| Has no mark, and was cleared or compacted | Nothing, because it said so as it started |
| Sends input the hook can't read | The notice below |

```json
{"systemMessage": "This session isn't going through switchboard, as the claude that started it isn't switchboard's: from a terminal, switchboard setup puts that right; an app that runs its own Claude Code isn't routed."}
```

Claude Code shows the notice to you. It adds nothing to what Claude is told.
