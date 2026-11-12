# The Graphs build: what it left

The Graphs tab was built in ten stages, #150 to #168 (see Graphs and Milestones in the design). Each
stage's two reviews, and the release review of the whole since v0.2.7, left these: an improvement,
an edge past ordinary use, or a cost, each judged for after the release. Gathered here, the first
to take up first.

## One owner of the accounts' order

The Accounts tab, Compare and Detail, ordered the accounts by the dashboard's config; Runway, the
Overview and Graphs by the router's document. The two agree but where the config and the router
disagree, as after `switchboard accounts remove` while the router waits to restart, which can be
hours. One owner of the order, read by every view, ends it. Graphs follows Runway, as the design
says a card's label is Runway's.

**Built** in the cleanliness release: `status.Numbering` owns the order, Runway's rule, the
router's place, else after its last in the config's order among those it lacks, and
`Moment.Configured` gives the configured accounts in it. Every tab, the routing card and the
`usage` printout read it. Since account identity, nothing numbers the accounts: the order is all
it owns.

## Runway's week past its view

Runway's live week runs from a day before now for a week, so it ends about six days ahead; a week
window that started within the last day resets after that. Where it runs out in that gap, Runway's
lane says `room all week`, true of what it draws, while `usage`, `status` and GLANCE say `out ~…`.
A Graphs card says the run-out (#168, from the lane's `Beyond`); Runway could say it too, as a
phrase past its view's end, or the design could say why it doesn't.

## Edges

- **GLANCE's name column** was laid from the config alone. **Built** with the order's owner: it's
  laid from the names it draws.
- **A hold then a run-out past the view:** a cap or limit held at now, lifting past Runway's view,
  with a run-out under it after that, would have a card say `back …` where Runway drawn on would say
  `lifts …`, as the stretches inside the view and past it are given their back apart. It needs a
  hold of more than about six days that isn't the view's own reset.
- **The beside form at 52 columns** with `▲` on a long label narrows the words before the label, as
  the turns' order has it; fewer words show, nothing wrong.

## Tests

- A card's `✕` from the moment's hold before the lanes are built (`HeldSince`'s fallback) is drawn
  by the moment's own `Since` and `CapSince`, untested.
- The agreement test held a view's pool figure to USAGE's `Σ pool` through the Overview's pool
  charts, which went with GRAPHS (#171); Graphs' POOL at now could take their place.
- `routingColumns.accountRows`, now in `traces_test.go` alone, copies how ROUTING lays out an
  account's rows; the tests could read the rows from what's drawn.

## Tidy-ups

- One helper for compounding fades (a stale reading's and an account out of the pool's), now
  compounded in more than one place.
- `besideCells` counted in more than one place (`compared.named`, `turnedName`), and a constant and
  a method sharing its name.
- The glyph fallback, `terminal`'s and `NO_COLOR`'s, written three times.
- Milestone 5's retired prefs keys, `view` and `chart`, are still kept as a newer build's unknown
  keys; they could join `retiredKeys` with GRAPHS' (#171).
- `poolStacked.rows` counts the chart, axis and key where `draw` draws nothing of the stack; with
  nothing to stack POOL now draws `│`, its axis and its key in those rows.

## Costs

- Graphs drops its lanes on leaving the tab, so coming back builds the week's again, once a visit:
  about 115 ms on the busy benchmark with the race detector on when it built the 5-hour's too.
- Graphs builds the week's lanes on every message that isn't a frame, as Runway does its one.
- A future version of the day's summary, over a long keep, should pace its re-summary: the readings
  writer's queue of 1,024 drops at about seven answers a second while it waits.

## Looks

- The legend and help draw the reserve as `⠠ ⠠`; a card draws it on its level's dot row, `⢀ ⢀` at a
  5-row chart.
- A configured account the router doesn't hold yet has its label alone on its card, as the design
  says; it could say `not read`.
