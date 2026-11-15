# Troubleshooting

When switchboard doesn't do what you expect, these three commands usually say why. Run them in this
order, then find your symptom in the table below.

1. `switchboard status` shows how each account stands and why, the health of the router, and any
   restart or move the router still owes.
2. `switchboard service status` shows whether the service is installed and loaded, and how the
   router answers.
3. `switchboard logs -f` follows the log of the router, with a line for each request and why it
   went to that account.

## Find your symptom

| Symptom | Page |
|---|---|
| `claude` in a new terminal isn't switchboard's, or a session says it isn't going through switchboard | [`claude` doesn't go through switchboard](troubleshooting/claude-link.md) |
| The `setup` screen is too small, won't copy, or carries on without the session hook | [The `setup` screen](troubleshooting/setup.md) |
| `claude` won't start because the router is down, or a session went somewhere unexpected | [The router](troubleshooting/router.md) |
| A restart is held or due after an upgrade, or `service status` talks of the router copy | [Upgrades and restarts](troubleshooting/upgrades.md) |
| Something waits on the move onto account ids, or an import is refused across it | [The move onto account ids](troubleshooting/account-ids.md) |
| `claude` starts nothing in a folder, or a request or session is refused for a pool | [Sessions in a pool](troubleshooting/pools.md) |
| A command, the dashboard or the config is refused for a pool | [Commands in a pool](troubleshooting/pool-commands.md) |
| An account reads `wrong token` | [An account with the wrong token](troubleshooting/tokens.md) |
| An export fails, or shows `running` for long | [An export that fails](troubleshooting/exports.md) |
| Notifications don't show, or show as Script Editor | [Notifications that don't show](troubleshooting/notifications.md) |
| The Log, its find, or the page of a session is slow to open | [The dashboard is slow to look back](troubleshooting/slow-pages.md) |
| You want to read the logs, or change how much they hold | [The logs](troubleshooting/logs.md) |

Every command and flag is in [Commands](commands.md), and every file switchboard keeps is in
[Reference](reference.md).
