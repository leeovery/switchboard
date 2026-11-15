# Notifications: know when an account hits a limit or has room again

Switchboard posts a desktop notification when an account hits a limit, has room again, or passes
a share of a window you set. It also warns you a month before the token of an account runs out.
You'd want them if you run sessions you aren't watching.

## Try it

Notifications for limits, room again and the warning at 90% are on from the start. To tune them,
add this to the config:

```toml
[notifications]
limits = true    # an account hits a limit, and which sessions moved off it
room = true      # an account has room again
warning = 0.8    # warn when a window passes 80% of its limit; 0 turns the warning off
moves = true     # every other session move, such as after an hour idle
```

The router takes up the new settings when it restarts. The service restarts it by itself once no
request is in flight, and `switchboard service restart` does it at once.

## What happens

- When an account hits a limit, the notification waits 5 seconds for the sessions that move off
  it. Then it names them: `b6s7 · Work hit its 5-hour limit, back at Mon 18:10 — 3 sessions moved
  to q4tx · Side`.
- When the account can take requests again, you see `b6s7 · Work has room again`.
- When a window passes the warning, you see `b6s7 · Work: Week at 91%`, once until that window
  resets.
- When the token of an account has 30 days left, you're told once, and again at 7 days. It reads
  `b6s7 · Work: token expires 17 Nov 2026 · 21 days left — switchboard setup makes a new one`.
- With more than one pool, each notification starts with the pool, such as `Acme · k3f9 · Acme 1
  has room again`.
- Clicking a notification dismisses it. It opens nothing.

## The notifications app

Notifications come from an app named `Switchboard`, with the switchboard icon. A release carries
the app inside the switchboard binary and writes it out to `~/.local/share/switchboard`. Each
upgrade writes it out again.

macOS asks once whether `Switchboard` may send notifications. It asks when the router first starts
with the app, after `setup` installs the service or after an upgrade. Notifications posted before
you answer wait up to 5 minutes for it. Routing never waits on the question.

## Good to know

- The token warnings always go out, whatever the config says. Each is told once for each token, so
  a restart doesn't repeat it, and a new token starts afresh.
- Only one notification goes out about an account each minute. A later one in that minute is
  dropped, unless it tells of a limit or a token.
- If the router isn't running, an open dashboard posts room again and the warning itself. Pass
  `--no-notify` to `switchboard dashboard` to stop that. While the router runs, the dashboard posts
  none, so you never get the same one twice.
- A switchboard you built with `go install` has no app. It posts through `osascript`, so the
  notifications show as Script Editor.

## Reference

| Key in `[notifications]` | Default | What it turns on |
|---|---|---|
| `limits` | `true` | An account hits a limit, with the sessions the limit moved. |
| `room` | `true` | An account has room again. |
| `warning` | `0.9` | A window passes this share of its limit. `0` turns it off. |
| `moves` | `false` | Every other session move, such as after an hour idle or by a pin. |

If notifications don't show, or show as Script Editor, see
[Troubleshooting](troubleshooting.md). To turn them back on after you turned them off, go to
System Settings → Notifications → Switchboard. Every key is in
[Configuration](configuration.md).

## Related

- [Limits and routing](limits-and-routing.md): what a limit is, and how sessions move off one.
- [Pinning and moving sessions](pinning-and-moving.md): the moves that `moves` tells you about.
- [Room for the Claude app](room-for-the-claude-app.md): keep a share of each window for yourself.
