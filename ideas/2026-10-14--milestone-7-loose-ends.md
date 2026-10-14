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

## Costs

- A session's page is built again every tenth of a second and on each progress event.
- The traffic copies every request in the stream on each batch (`took`); a map the views keep from
  batch to batch would take only what changed.
- Events filtered by kind walk every day.
- capture's tests take 7 to 15 minutes under the race detector.

## Cleanups

- The tail's carry rule is chosen in capture's two readers as well as in readings.
