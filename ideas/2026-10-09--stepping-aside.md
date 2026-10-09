# What a routed session's environment carries past switchboard

`run` gives a routed session's environment what routing needs: the router's base URL, the pin and
directory headers in `ANTHROPIC_CUSTOM_HEADERS`, and, since milestone 7's stage 2,
`CLAUDE_CODE_GATEWAY_HINT_HEADERS=1`. Every process the session starts inherits them. A `claude`
started within it through switchboard that switchboard steps aside for, as when it's given an API
key, or can't help, as when it can't read its config, gets them as they are, as `StepAside` and
`Unaided` pass the environment on but for their mark; only the direct paths take the pin and the
directory off. Found by stage 2's review, 9 October 2026.

So a script within a session that points `claude` at another gateway, with a key of its own, sends
that gateway the pin and directory headers and Claude Code's hint headers. A gateway that refuses
headers it doesn't know, as Claude Code's gateway guide says some do, refuses its requests.

A fix: `run` marks what it set, and stepping aside, or starting unaided, takes off what the mark
names, leaving what the user set. Small, and rare in use: nothing has yet been seen to need it.
