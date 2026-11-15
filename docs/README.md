# Switchboard docs

Find the page for what you want to do. Each line says what the page answers.

## Get started

- [The README](../README.md): what switchboard does, how to install it, and the first commands.
- [How it works](how-it-works.md): what switchboard is made of, and the path a request takes.
- [Upgrades and restarts](upgrades-and-restarts.md): what happens when a new version arrives, or
  the config changes.

## Features

- [Limits and routing](limits-and-routing.md): how a session picks an account, and carries on
  when a limit hits.
- [Pools](pools.md): how to keep client work on its own accounts.
- [Claude and switchboard](claude-and-switchboard.md): how Claude reads your limits and paces its
  work.
- [Your record](your-record.md): what switchboard keeps, and what your use would cost at API
  prices.
- [Staggered resets](staggered-resets.md): how your accounts come back one at a time.
- [Room for the Claude app](room-for-the-claude-app.md): how to keep a share of an account for
  the Claude apps.
- [Notifications](notifications.md): which alerts you get, and how to choose them.
- [Pinning and moving](pinning-and-moving.md): how to send sessions to the account you choose.
- [Several Macs and backups](several-macs.md): how to export your record, and bring it to
  another Mac.

## Look things up

- [Dashboard](dashboard.md): what each tab shows, the keys, and the themes.
  - [Overview](dashboard/overview.md): every account's windows, and how the router stands with
    each.
  - [Accounts](dashboard/accounts.md): each account in depth, and every account together.
  - [Projects](dashboard/projects.md): where the use went, project by project.
  - [Sessions](dashboard/sessions.md): where each session's requests are going now.
  - [Runway](dashboard/runway.md): when each account has room, over the week.
  - [Trends](dashboard/trends.md): the shapes of use over time, as charts.
  - [The Log](dashboard/log.md): the router's requests and events, as they happen.
  - [A session's page](dashboard/session-page.md): one session, through its whole life.
  - [Every page](dashboard/every-page.md): the status line, help, the key line, clicks, and the
    keys every page shares.
  - [The routing card](dashboard/routing-card.md): where new sessions go, set from the dashboard.
  - [On a phone](dashboard/phone.md): how each tab lays itself out on a narrow screen.
  - [Running it](dashboard/running-it.md): a pool at a time, how it reads the router, and what it
    remembers.
  - [Themes](dashboard/themes.md): the built-in themes, the picker, and your own.
- [Commands](commands.md): every command and flag, and what each prints.
- [Configuration](configuration.md): every key in the config file, and what it changes.
- [Reference](reference.md): where switchboard keeps its files, and the JSON its commands print.
- [Troubleshooting](troubleshooting.md): what to check when something goes wrong, and where the
  logs are.
- [Glossary](glossary.md): what pool, primary, reserve and the other terms mean.

## Design, for contributors

- [The design](design.md): how switchboard is built, the calls behind it, and the backlog.
- [Contributing](../CONTRIBUTING.md): the gates every change passes.
