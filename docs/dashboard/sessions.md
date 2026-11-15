# Sessions

**Sessions** shows where each session's requests are going now. It has two views on sub-tabs, the
Switchboard and the List, and `[` and `]` move between them.

<div align="center">
<img src="../../art/repatch.webp" alt="Sessions' Switchboard as personal reaches its limit: the 429 back red along 41e0's cord, its old cord left hanging, and personal's sessions re-patched to side" width="840">
</div>

## The Switchboard

The Switchboard is the default view. It's drawn as a telephone switchboard:

- The sessions run down the left, as calls.
- The accounts run down the right, as lines.
- A cord runs from each call to its account.

Each session's requests travel its cord, out as they go and back as their answers stream. A
refusal bounces back red, `✕ 429`.

When a limit moves a session, the session joins its new account's group. A short cord hangs red
from the old jack, one for each session the limit moved, while the limit holds. A move made for any
other reason hangs faint for 3 seconds.

Where the cords don't fit, as on a phone, each account's line carries its sessions under it. Its
windows are stacked as the board's lines are, a row each, where the screen is tall enough, and go
side by side on one row where it's short. The pool has no lane, as
no session runs on it: the Overview, Runway and `usage` show it.

While the router isn't answering, the Switchboard and the List show the sessions it last listed.
What one was doing then, such as `↓ answering`, stays as it was but faint, here and on every other
tab, as it may have ended since.

## The List

The List is a log of every session in the period the dial shows at its heading's right, the one
Accounts and Projects share: those running first, then those ended, newest first, under a rule for
the day they ended, `ended today`, `ended yesterday`, `ended Tue`, or beyond six days,
`ended Mon 21 Sep`. Over today, it's today's sessions. Each row shows the session's:

- account, model and state, as `ended 18:20` once it's over
- start time, with its day where it started on another
- requests in the period, and their worth at API prices
- how it came to its account.

A row reads as it does in a project's sessions on Projects. The line over the key line sums the
period, as `30 days  150 sessions · 33,010 requests · $3,257.86 at API prices`. Where the sessions
don't all fit, the row over the key line says how many are out of view, as
`▲ 12 more sessions · ▼ 130 more sessions`, and a day's rule stays at the top while its sessions
scroll under it, so the day always shows.

## Keys on Sessions

| Key | Does |
|---|---|
| `[` `]` | Move between the Switchboard and the List. |
| `↑` `↓` | Choose a session: a call on the Switchboard, or a row on the List. Where it's out of view, the view scrolls to it. |
| `j` `k` | Scroll the List a row. |
| `<` `>` | Step the List's period, on the List alone. See the period on [Accounts](accounts.md). |
| `enter` | Opens the chosen session's page, ended or running. |
| `esc` | Deselects it. |
