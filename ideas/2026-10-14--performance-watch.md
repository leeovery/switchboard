# Watching switchboard's CPU and memory

Switchboard should be light: the router in every request's path, and a dashboard that may sit open
for days. Nothing has measured it over time. The reviews measured costs one at a time, and a few
benchmarks hold those as bounds, but nobody has watched the router and a dashboard over days.

The owner (14 October 2026): watch it for a week after milestone 7's release, with a performance
monitor that records each process's CPU and memory a minute, and react to what the data shows.

A baseline, v0.1.6 on the owner's Mac over 11 hours of heavy agent traffic: the router used 0.6% to
1.4% of one core an hour, peaking at 3.3%, its memory between 26 and 61 MB, rising and falling with
no growth; each `usage -w` watch 0.7% to 4.7% of one core and 16 to 31 MB.

What to watch, once a day:

- the router's and each dashboard's CPU and memory, and whether any process's memory keeps climbing
  over days;
- a dashboard left on its heaviest tabs, the Log and Sessions' Switchboard, through a busy day.

Costs already known, in `2026-10-14--milestone-7-loose-ends.md`: the traffic's tick copying the
requests map in a busy hour, `took` copying the stream on each batch, and a session's page rebuilt
every tenth of a second. A performance pass, if the data calls for one, starts there.
