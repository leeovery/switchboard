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

## Built, 26 October 2026

Built in the cleanliness release, as the design's Launching says under "What `run` set". `run`
marks what it set in `SWITCHBOARD_SET`: the value it gave the base URL and the hint setting, each
with the user's value it replaced, and the lines it added to the custom headers. Stepping aside
takes off what the mark names, leaving what the user set and what a script changed since, and a
session within a session marks only what it sets itself. Starting unaided does the same only where
the base URL has changed since, as a script pointing `claude` at another gateway does: within a
session still sent to the router `run` sent it to, it passes the environment on as it is, mark and
all, as that router still routes and records it, routing coming first. The token isn't marked, as
the user's own can't be written into the mark; nor is any value of the user's holding a secret, as
`redact.HoldsSecret` finds one, such as a gateway's URL with credentials or a key in it, which then
goes when stepping aside rather than going back. A session from before the mark goes on as it did.
`internal/launch` holds it: `env.go`'s `markedAgainst`, `unmarked` and `routedByRun`, and
`launch.go`'s `unaidedEnv`.
