# The Graphs build: what it left

The Graphs tab was built in ten stages, #150 to #168 (see Graphs and Milestones in the design). Each
stage's two reviews, and the release review of the whole since v0.2.7, left these: an improvement,
an edge past ordinary use, or a cost, each judged for after the release. Gathered here, the first
to take up first.

## One owner of an account's number

The Accounts tab, Compare and Detail, numbers each account by its place in the dashboard's config;
Runway, the Overview and Graphs by its place in the router's document (`numberedAsRunway`). The two
agree but where the config and the router disagree, as after `switchboard accounts remove` while
the router waits to restart, which can be hours. One owner of the number, read by every view, ends
it. Graphs follows Runway, as the design says a card's label is Runway's.

## Runway's week past its view

Runway's live week runs from a day before now for a week, so it ends about six days ahead; a week
window that started within the last day resets after that. Where it runs out in that gap, Runway's
lane says `room all week`, true of what it draws, while `usage`, `status` and GLANCE say `out ~…`.
A Graphs card says the run-out (#168, from the lane's `Beyond`); Runway could say it too, as a
phrase past its view's end, or the design could say why it doesn't.

## Edges

- **GLANCE's name column** is laid from the config alone, so it's a cell short where a number
  reaches 10 while the config has nine or fewer (ten accounts or more, with the config and the
  router apart).
- **A hold then a run-out past the view:** a cap or limit held at now, lifting past Runway's view,
  with a run-out under it after that, would have a card say `back …` where Runway drawn on would say
  `lifts …`, as the stretches inside the view and past it are given their back apart. It needs a
  hold of more than about six days that isn't the view's own reset.
- **The beside form at 52 columns** with `▲` on a long label narrows the words before the label, as
  the turns' order has it; fewer words show, nothing wrong.

## Tests

- A card's `✕` from the moment's hold before the lanes are built (`HeldSince`'s fallback) is drawn
  by the moment's own `Since` and `CapSince`, untested.
- Capture's fluid sweep exempts whole views only, so GLANCE alone at 173 columns or wider, which
  doesn't grow by design, would fail its wasted third; no fixture draws it. A per-section exemption
  in the sweep, never a loosening.

## Tidy-ups

- One helper for compounding fades (a stale reading's and an account out of the pool's), now
  compounded in more than one place.
- `besideCells` counted in more than one place (`compared.named`, `turnedName`), and a constant and
  a method sharing its name.
- The glyph fallback, `terminal`'s and `NO_COLOR`'s, written three times.
- `poolStacked.rows` counts the chart, axis and key where `draw` draws nothing of the stack; with
  nothing to stack POOL now draws `│`, its axis and its key in those rows.

## Costs

- Graphs drops its lanes on leaving the tab, so coming back builds both windows' again, about 115 ms
  on the busy benchmark with the race detector on, once a visit.
- Graphs builds both windows' lanes on every message that isn't a frame, as Runway does its one.
- A future version of the day's summary, over a long keep, should pace its re-summary: the readings
  writer's queue of 1,024 drops at about seven answers a second while it waits.

## Looks

- Beside GLANCE, WHEN YOU WORK's grid at two cells an hour leaves cells spare at its column's right,
  some 21 at 220 columns.
- The legend and help draw the reserve as `⠠ ⠠`; a card draws it on its level's dot row, `⢀ ⢀` at a
  5-row chart.
- A configured account the router doesn't hold yet has its label alone on its card, as the design
  says; it could say `not read`.
