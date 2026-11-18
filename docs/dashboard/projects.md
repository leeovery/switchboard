# Projects: where your use went

Projects ranks the projects your sessions ran in by what their use was worth, over the period you
choose. Look at it to see which repository took the most, on which accounts and models, and which
sessions ran there. The list is at the top, and the project you choose shows in full under it.

<div align="center">
<img src="../../art/projects.webp" alt="Projects: each project's live sessions, sessions, worth and requests over this week, the chosen one in full under them" width="840">
</div>

*Projects over this week, with switchboard chosen.*

A project is a repository with its worktrees folded in, or the folder a session started in
outside any repository. Each one is named by its folder, with the parent folder too if two share
a name. Four kinds of session count otherwise, as [the ledger](../reference/ledger.md) explains.
The first that fits decides:

- **A `claude` that another session starts outside any repository** counts in that session's
  project, unless a fence takes it into another pool.
- **A session in a Claude Code scratchpad** counts as the project the scratchpad was made for, or
  under `temporary folders` if that repository is gone.
- **A session in another temporary folder** counts under `temporary folders`, a faint row.
- **A session not started through switchboard's `claude`** counts under `no directory`, also
  faint.

## The list: every project, the greatest first

The list ranks projects by worth at API prices. Each row has:

- the name;
- `live`, the sessions running now;
- the sessions, and their worth;
- a bar of the worth against the top project, split by model family as on [Trends](trends.md);
- the requests;
- `last`, when the last request came, or `now` while a session is busy.

`all projects` totals them under a rule, counting each session once. The list takes up to a third
of the rows, and scrolls with your choice past them.

## The chosen project

The chosen project shows:

- **The heading:** the name, the path, and the sessions running.
- **The numbers** over the period: sessions, requests, worth, the share of all worth, and tokens.
- **WORTH BY:** the worth in the finest bucket that fits, with the peak named under it.
- **BY MODEL:** the share of worth by model version, those under 1% left out.
- **ON ACCOUNTS:** the share on each account, the greatest first.
- **SESSIONS:** the sessions over the period, in the columns of the List on
  [Sessions](sessions.md).

WORTH BY picks its bucket by the period:

| Period | Bucket |
|---|---|
| today | an hour, or two hours if a column an hour doesn't fit |
| this week | a day |
| 30 days and 12 weeks | a day, or a week if a column a day doesn't fit |
| 12 months | a week, or a month if a column a week doesn't fit |

SESSIONS lists the running sessions first. The rest follow under a rule for the day they ended:
`ended today`, `ended yesterday`, a weekday this week, or a date before. Each session starts at its
first request, with the day if it began on another.

Each session shows its [session name](sessions.md) alone, such as `router-fix`, because the project
heads the page. Two sessions of one name each add their id, as the List does.

If the sessions don't all fit, the last row counts those out of view, as the List does:
`▲ 3 more sessions · ▼ 12 more sessions`. On a short screen, that row shows only if two sessions
still fit above it.

Only the part where `↑` and `↓` act, the list or the sessions, lights its chosen row. The other
keeps its chosen row bold, marked `▸`.

## Room by size

**Wide,** WORTH BY sits on the left, with BY MODEL over ON ACCOUNTS on the right. Narrower,
WORTH BY runs across the width, with the other two side by side under it. On a phone, all three
stack.

**If rows run short,** the page gives way in this order, so SESSIONS keeps at least 3 rows:

1. The columns of WORTH BY shrink from 6 rows to 3.
2. The list shrinks from a third of the rows to 5 rows.
3. ON ACCOUNTS leaves.
4. Then BY MODEL.
5. Then WORTH BY.
6. Then the list takes up to a third of the rows again, then shrinks toward 2 rows as they run
   shorter.

**If the width runs short,** the list gives way:

1. The bar narrows to 8 cells.
2. The requests and `last` leave.
3. The name is cut.

## Keys on Projects

| Key | Does |
|---|---|
| `↑` `↓` | Chooses the project. |
| the wheel, over the list | Chooses the next project as your fingers move down, whichever way your Mac scrolls. |
| `→` `←` | Moves into the sessions of the chosen project, and back to the list. |
| `↑` `↓`, then `enter` | In the sessions, chooses a session, and opens [its page](session-page.md). |
| `<` `>` | Steps the period, shared with [Accounts](accounts.md) and the List on Sessions. |

Keys on every page are on [Every page](every-page.md). If the wheel chooses the project above as
your fingers move down, see [The wheel on Projects runs against your fingers](../troubleshooting/wheel.md).
