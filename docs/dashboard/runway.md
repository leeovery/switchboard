# Runway

**Runway** shows when each account has room, with a lane for each account.

<div align="center">
<img src="../../art/runway-week.webp" alt="Runway over the week: a lane an account to next Wednesday, work at its cap till 17:10, personal's and side's weeks running out at their pace before they reset, and the pool's room under them" width="840">
</div>

Runway shows the week, from a day before now. Every window every model shares counts, the 5-hour
among them, so a lane is without room wherever any of them holds its account back.

Each column of a lane is one cell, showing room, at its cap, or at its limit. The past is dimmed.

Until the router has a day of your sessions, as on a new install, its forecast takes the use on
each account to stay there, so a run-out may come earlier than it will. Runway says so under its
heading, `run-outs firm up once switchboard has a day of your sessions`, where the screen has room.

Words under a lane say where a stretch without room starts and when the account is back. For
example, `capped 14:31  ·  back 17:10` at its cap, `out 13:58  ·  back 15:30` at its limit, or,
ahead, `out ~16:05` where it runs out and `caps ~15:45` where it reaches its cap, `~` marking a
time projected at its pace: more than a day ahead, its day alone, `out ~Sat`, or dated on today's
weekday, `out ~Thu 12 Nov`. Where a cap or a run-out ends as the window resets, room returning
then, the words say `resets Sun 01:00` in place of `back`; a limit always says `back`, as it lifts
whenever it does. Where one hold runs straight into another, the first says
when it lifts, and only the last says when the account is back:
`out 14:00  ·  lifts 19:00  capped 14:00  ·  back Mon 21:00`. Words that don't fit from where a
stretch starts shift left until they do, never over the words before, and drop parts from the end
where even that can't fit them.

With more than one account, the pool's lane sits under the others. It shows how many accounts have
room, a column at a time, and draws a low red bar along its foot where none has. `Σ pool` names it,
as an account's name heads its lane, with the pool's room in the week under it, such as
`week 1.2 of 3 left`, out of the accounts that have the window. Where fewer than two have it, and
not all, there's no pool's lane.

COMING UP lists what happens next, soonest first, and LATELY what befell each account over the
week gone, newest first: a cap, a limit, a refused token, coming under pressure, and room again,
the sessions each moved said in its line, and how long ago at its end, such as `11m ago`. It
reads the whole week off the router's files of events once, as Runway first shows, and keeps it
as the router restarts; till then it shows the router's newest events. Where the screen is wide
enough for both, from 159 columns, and has the rows for COMING UP's every line in one column,
LATELY sits at the left and COMING UP at the right, the past and what's coming as the lanes lie,
each down to the legend. Else COMING UP sits under the pool's lane with LATELY under it, wherever
LATELY's heading and two lines fit: COMING UP in two columns where they fit, from 148 columns,
and LATELY across the screen under them; else the two in one column, as wide as their lines need
to read whole, from 72 columns to the screen's width. Shorter still, or narrower, COMING UP shows
alone. LATELY only takes the rows the lanes and COMING UP leave empty, so it never squeezes them.
Where COMING UP's lines don't all fit, its last says how many more, such as `▼ 3 more`. The legend sits at the foot,
over the key line. On a short screen, as with a phone's keyboard up, LATELY under COMING UP goes
first; then COMING UP is cut to the lines that fit; then the legend gives way, then COMING UP, then the
blank row under the axis; only then do the lanes scroll with `j` and `k`, down to one lane over
the pool's lane at the shortest.

The labels beside the lanes take as many columns as the widest needs, up to 24 cells of label,
and the lanes take the rest. `a` collapses the labels, to give the lanes more room: the lanes
start at the left edge, with no names and no marks, as the status line already says where new
sessions go. Under the start of each lane is its use, such as `66%`, faint for an account out of
the pool, and under the pool's lane its room, short, such as `1.2/3`. The words under a lane
still start under their stretch, but where that would run into the use, they start two cells
after it, as in `100%  out Tue 17:54  ·  back Sun 01:00`. COMING UP stays as it is, each line
naming its account. `a` again expands them; the key line says `a collapse`, its word faint while
the labels are collapsed. Runway remembers which you left it on.

To look back, `←` goes back to this week, then a week at a time, and `→` steps forward, on to the
live view. A week stepped back to has no COMING UP and no LATELY. A step back is never kept.

## Keys on Runway

| Key | Does |
|---|---|
| `a` | Collapses the labels beside the lanes, or expands them again. |
| `←` `→` | `←` goes back to this week, then a week at a time, and `→` steps forward, on to the live view. |
| `j` `k` | Scroll the lanes, where they don't fit. |
