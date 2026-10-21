# The graphs, reimagined: a burn-down for every account, readable at a glance

The owner, on 20 October 2026, while the fluid layout pass was being planned:

> I'd also like to revisit the graphs at some point, because I'm still not happy with how they
> work. In the previous version (as in the 0.1 release), we had burn-down graphs where I could see
> all three accounts ... I could see the burn-down of each one. I feel like what we need is almost
> the ability to see a page of graphs, I want to see burn-down for each one. At the moment, our
> graphs are too tall, and they're ... not right. They look good, but they're hard to read. The
> scale's not right; I can't see enough of it to get a good immediate visual idea.

And on where it sits: "making the graphs look better (maybe with a redesign on the current
placement, a new page, or a mixture of the both) requires discussion, and it's not part of this
phase ... we'll circle back to the redesign, the reimagining of graphs." The fluid pass makes
GRAPHS fluid as it's designed today; this is the redesign after it.

## What it asks for

- **A burn-down for each account, all at once,** as milestone 5's dashboard (v0.1.x) showed them in
  its cards: each account's room over its window, so one look compares them.
- **A page of graphs,** or the Overview's GRAPHS reworked in place, or both: to discuss.
- **A scale that reads at a glance.** Today's charts are tall and hard to read: too little of the
  window shows at a time to get a quick picture.

## Where it touches

- The Overview's GRAPHS section (`internal/dashboard/graphs.go`, `chart.go`), and whether GRAPHS
  stays there, moves to a page or tab of its own, or both.
- It lays out by the fluid pass's rule (`internal/dashboard/flex`).

## To settle with its owner

The graphs' place, what each chart shows (the window's use over its span, the even pace, the cap),
how many sit side by side at each width, and the scale. Drawn in Paper first, as a new arrangement.
