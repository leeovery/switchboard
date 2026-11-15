# Troubleshooting: notifications that don't show

Notifications come from switchboard's own app, `Switchboard`, which the router registers with
macOS. A release carries the app inside the switchboard binary. The router writes it out to
`~/.local/share/switchboard`, or `$XDG_DATA_HOME/switchboard` if you set that, and again after
each upgrade. This page covers notifications that don't show, or show under another name.
For what they tell you, see [Notifications](../notifications.md).

## Notifications were turned off

macOS asked once whether `Switchboard` may send notifications, and they were turned off. Turn them
on in System Settings → Notifications → Switchboard.

Until you do, the router logs a warning, once until a notification goes out:

```text
macOS won't let Switchboard post notifications: turn them on in System Settings → Notifications → Switchboard, or, where they're on, switchboard service restart registers its app again
```

## Notifications are on, but still don't show

macOS can lose track of the app. The router registers it again as it starts.

```bash
switchboard service restart
```

## macOS never asked

macOS asks when the router first starts on a release with the app. To have it ask now:

```bash
switchboard service restart
```

If the question can't be put, the log says so, with the reason macOS gives:

```text
can't ask macOS whether Switchboard may post notifications; posting through its app all the same
```

## Notifications show as Script Editor

Switchboard couldn't write out or register its app, so it posts through `osascript` until the
router next starts. The log says
`can't write out the notifications' app, so notifications are posted through osascript`, with why.
A switchboard built from source has no app, and always posts through `osascript`.

## A notification failed

The log has it at warn as `notification failed`, and the notification is dropped. It isn't sent
again.

```bash
log=$(switchboard logs router --path)
grep 'notification failed' "$log"
```

A notification can also be held back on purpose. Only one about an account goes out each minute,
unless it tells of a limit or a token.

## Related

- [Notifications](../notifications.md): what each notification says, and the keys that tune them.
- [Logs](logs.md): where the logs are, and how to read them.
- [`switchboard service`](../commands/service.md): restarting the router.
