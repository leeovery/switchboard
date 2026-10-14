# Milestone 7's loose ends

Milestone 7, the dashboard's redesign, went through a review of each pull request, of each stage,
and of the whole release, several rounds each. What those reviews found and left, as not worth
holding the release for, gathered here. Each is an edge case or a cost with no wrong answer.

## On screen

- `↓ answering` stays frozen while the router is silent, until it's heard from again.
- A manual reset lifts its hold a little early.
- The routing card's foot is cut on short terminals.
- On Sessions, at its step 5 in a short terminal, an account with 17 or more calls can't show them
  all.
- COMING UP names the limit at a back that's a refusal's.
- The Overview's USAGE at 20 columns or fewer, scrolled.
- A session's page under 39 columns, below the phone's floor.
- A cent between a session's STORY, summed from its parts, and TURNS' total, each rounded.
- Click targets at 1 to 30 columns.
- The Log's panel names a session by the lines the Log holds, so across midnight it can name it
  otherwise than the List does.
- Sessions' Switchboard at 14 rows, where an account's sessions sit above its line: its scroll line
  counts an account more than it hides, and paging down cuts a call. An account's first row is
  taken as its line's or its group's, whichever is higher; the cords' layout needs its rule for an
  account's rows settled first.
- `↑` and `↓` on Sessions' Switchboard don't scroll it to the session chosen.
- The Overview at 14 rows with every section open: USAGE is left its heading alone, or nothing.
- At 11 to 13 rows: Compare says `▼ 25 more below` with nothing shown, Detail's `↑↓ choose` is
  clipped, and a phone's Days pages nothing.
- `switchboard dashboard --help`'s keys paragraph leaves out the keys of Accounts, Sessions, the
  Log and a session's page; the dashboard's own help has them all.

## Costs

- A session's page is built again every tenth of a second and on each progress event.
- The traffic copies every request in the stream on each batch (`took`); a map the views keep from
  batch to batch would take only what changed.
- In a busy hour, the traffic's tick copies the requests map about once a second, and the views
  hear the whole stream again for each copy. Tidying once a minute would change what shows: the
  views keep a request the router told of until its line comes, whatever its age, so they'd first
  need told requests let go by time.
- Events filtered by kind walk every day.
- capture's tests take 7 to 15 minutes under the race detector.

## Cleanups

- The tail's carry rule is chosen in capture's two readers as well as in readings.
- `views.Running.Models` is read by nothing but tests.
- `claude.Policy` and `claude.SharedWindows` are exported variables any package could reassign.
