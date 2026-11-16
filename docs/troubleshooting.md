# Troubleshooting

When switchboard doesn't do what you expect, these three commands usually say why. Run them in this
order, then find your symptom in the table below.

1. `switchboard status` shows how each account stands and why, whether the router is healthy,
   and any restart or move the router still owes.
2. `switchboard service status` shows whether the service is installed and loaded, and how the
   router answers.
3. `switchboard logs -f` follows what the router logs, with a line for each request and why it
   went to that account.

## Find your symptom

| Symptom | Page |
|---|---|
| `claude` isn't switchboard's, or a session says it isn't routed | [`claude` doesn't go through switchboard](troubleshooting/claude-link.md) |
| The `setup` screen is too small, won't copy, or carries on without the session hook | [The `setup` screen](troubleshooting/setup.md) |
| `claude` won't start: the router is down | [The router](troubleshooting/router.md) |
| A session went to an unexpected account | [The router](troubleshooting/router.md) |
| An account still reads out after a reset by hand | [The router](troubleshooting/router.md) |
| A pin may not have been made | [The router](troubleshooting/router.md) |
| A pin is refused | [Commands in a pool](troubleshooting/pool-commands.md), or [`switchboard pin`](commands/pin.md) |
| A restart is held or due after an upgrade | [Upgrades and restarts](troubleshooting/upgrades.md) |
| `service status` says the router's copy isn't run | [Upgrades and restarts](troubleshooting/upgrades.md) |
| Something waits on the move onto account ids | [The move onto account ids](troubleshooting/account-ids.md) |
| An import is refused | [The move onto account ids](troubleshooting/account-ids.md) |
| `claude` starts nothing in a folder, or a pool refuses a request | [Sessions in a pool](troubleshooting/pools.md) |
| A command, the dashboard or the config is refused for a pool | [Commands in a pool](troubleshooting/pool-commands.md) |
| An account reads `subscription lapsed` or `token refused` | [An account's token or subscription](troubleshooting/tokens.md) |
| An account reads `wrong token` | [An account's token or subscription](troubleshooting/tokens.md) |
| An export fails, or shows `running` for long | [An export that fails](troubleshooting/exports.md) |
| Notifications don't show, or show as Script Editor | [Notifications that don't show](troubleshooting/notifications.md) |
| The Log, or the session page, is slow to open | [The dashboard is slow to look back](troubleshooting/slow-pages.md) |
| The wheel on Projects chooses the project above as your fingers move down | [The wheel on Projects](troubleshooting/wheel.md) |
| You want to read the logs, or change how much they hold | [The logs](troubleshooting/logs.md) |

Every command and flag is in [Commands](commands.md), and every file switchboard keeps is in
[Files and folders](reference/files.md).
