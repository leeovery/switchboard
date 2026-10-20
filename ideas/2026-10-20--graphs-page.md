# The graphs, redone: a burn-down for every account, readable at a glance

The owner, on 20 October 2026, while the fluid layout pass was being planned:

> I'd also like to revisit the graphs at some point, because I'm still not happy with how they
> work. In the previous version (as in the 0.1 release), we had burn-down graphs where I could see
> all three accounts ... I could see the burn-down of each one. I feel like what we need is almost
> the ability to see a page of graphs, I want to see burn-down for each one. At the moment, our
> graphs are too tall, and they're ... not right. They look good, but they're hard to read. The
> scale's not right; I can't see enough of it to get a good immediate visual idea.

He was happy to park it and come back to it after the fluid pass, as it may need restructuring.

## What it asks for

- **A burn-down for each account, all at once,** as milestone 5's dashboard (v0.1.0) showed them:
  each account's window used against its time, side by side or stacked, so one look compares them.
- **A page of graphs,** rather than charts squeezed into the Overview's GRAPHS section.
- **A scale that reads at a glance.** Today's charts are tall and hard to read: too little of the
  window shows at a time to get a quick picture.

## Where it touches

- The Overview's GRAPHS section (`internal/dashboard/graphs.go`, `chart.go`), and whether GRAPHS
  stays there, moves to a page or tab of its own, or both.
- The fluid pass held GRAPHS out (its slice S13, charts fluid across widths) so it isn't reworked
  twice: this redesign makes it fluid as it goes, by the pass's layout rule (`internal/dashboard/flex`).

## To settle with its owner

The page's place (a tab, a sub-tab, or the Overview's section grown), what each chart shows (the
window's use over its span, the even pace, the cap), how many sit side by side at each width, and
the scale. Drawn in Paper first, as a new arrangement.
