# A dashboard left open across an upgrade

The router takes an upgrade up by itself, handing over in place; a dashboard already running goes on
as the version it started as, reading the new router's document with the old binary's rules. Where
a release changes what a field of the document means, the old dashboard reads it wrong until it's
started again, and nothing tells it, or its reader, that the router underneath has moved on.

v1 met it (the v1 release review, 11 November 2026): #275 made a plan's size one owner's, an Ultra
week 10 Pro weeks where the forecast had 20, so the router's `demand.day` halves for an Ultra pool.
A dashboard started on v0.5.6 still sizes each Ultra account at 20, so once its hold fades the
pool's use lands on the accounts at half its rate and the weeks' run-outs on Overview, Runway and
Graphs drift later, until it's started again. A v1 dashboard on a v0.5.6 router, before the
handover, shows them early, briefly.

## What it could do

- **Notice and say:** the dashboard knows its own version and reads the router's; where they
  differ, a line says the router was upgraded and the dashboard wants starting again.
- **Start itself again:** on noticing, the dashboard execs its new binary in place, its tab and
  place kept, as the router hands over.
- **Carry the rules with the data:** the document names the sizes behind `demand`, so a reader with
  a table of its own can tell, and use the router's.

Each wants a design pass (and Paper, for anything drawn) before it's built.
