# Routing rules

The rules the router follows to pick an account for a session, keep the session there, and move
it. For why it works this way, see [Limits and routing](../limits-and-routing.md).

```console
$ switchboard events
  09:00  18bb2c41  started   work
  10:00            pressure  work  …
  11:40  18bb2c41  moved     work → personal  …
```

| Rule | Value |
|---|---|
| The score of an account | Room left in its shared week, divided by the hours until that week resets |
| Close enough to the best | A score of at least 0.8 of the best |
| How long a session stays | An hour after its last request, while the account has room |
| What a move after an idle hour needs | Another account that scores 20% better |
| What a move after a compaction needs | A week that resets sooner, and would otherwise reset more than 2% unused |
| The rate behind pressure | The rise in the 5-hour window over the last 30 minutes |
| How long an account sits out a limit | Until the reset that Anthropic gives, or 5 minutes without one |
| How long an account sits out a refusal | 10 minutes |
| How often a throttled request is tried again | Twice at most, on the same account |

Every rule applies inside each [pool](../pools.md), with the accounts of that pool as the whole
set.

## Choosing an account

The candidates are the accounts with room in every window the session's model counts against. Each gets a score: the room left in its shared week, divided by the hours until that week
resets. So quota that resets tomorrow is used before quota that resets next week. An account with
little left scores low, unless its week resets soon.

Among the accounts that score at least 0.8 of the best, the one whose 5-hour window resets soonest
wins. Whatever is left in a window at its reset is lost. An account without a 5-hour window
comes after those with a reset to come. Between two accounts without one, the higher score wins,
then the one that has used less of its week.

## Staying on an account

A prompt cache belongs to one account, and the first turn after a move costs around 40 times a
warm turn. So the router keeps each session where it is:

| When | What the router does |
|---|---|
| A request of the session is answered with success | Remembers the account for that session, model and pool. |
| A request fails, under an id never used again | Remembers nothing. The quota check that `claude --resume` sends can be one. |
| Less than an hour has passed since the last request | Keeps the session on its account, if the account has room. |
| An hour or more has passed | Scores the session again. It moves only if another account scores 20% better. |
| Claude Code has just compacted the conversation | Looks at the account a new session would get, and moves the session there if that pays. See below. |
| The session resumes in its pool | Finds its account again. |
| The session resumes in another pool | Counts as a new session there. |

A session resumed in another pool keeps its account in the first pool, for the process still
running there.

A Claude Sonnet 5.5 session never moves for idling or a compaction, because its thinking works only
on the account that produced it. A pin of one session keeps it there through a compaction too.

## Moving after a compaction

A busy session never idles for an hour. Left there, it would stay on its account until that account
ran out, while another week reset with room nobody used. A compaction is the cheap moment to move,
because only the system prompt and tools have to be cached again.

At the first request after a compaction, the router looks at the account it would pick for a new
session. The session moves there if both hold:

- that account's week resets before the week of the session's current account;
- at its recent rate, that week would reset with more than 2% unused, above any reserve.

## Pressure

Several busy sessions on one account run its 5-hour window out together. They then all move at
once, and each rebuilds its cache elsewhere. To avoid that, the router steers new work off an
account heading for its limit.

- **The rate:** how fast use of the 5-hour window rose over the last 30 minutes. If the readings
  have a longer gap, the rise is spread over the whole gap. If there's no reading from 10 minutes
  back, the rate is the use since the window started.
- **Under pressure:** at that rate, the window would reach the reserve before it resets. For an
  account without a reserve, or a pinned account, the mark is the limit.
- **What moves:** each of these goes to the best account not under pressure, pinned accounts
  first:
  - a new session, and a request without a session;
  - a session chosen afresh after an idle hour;
  - a move after a compaction;
  - a session moved off an account that can't serve it, or whose own pin gives way.
- **What stays:** running sessions, and a session with a pin of its own.

An account without a 5-hour window is never under pressure. When every account is under pressure,
pressure changes nothing. Use outside the router shows only when the router next reads the
account, so an account gone quiet reads quiet.

## Limits and refusals

The table on [Limits and routing](../limits-and-routing.md) gives what the router does with each
refusal. These are the finer points:

- **A limit keeps back only what it counts.** For example, a limit on the Fable week of an account
  leaves the sessions there on other models where they are.
- **An account comes back early** if a request sent since shows the limit has lifted. That
  happens after you reset a limit by hand on claude.ai.
- **A 403 never reaches Claude Code,** because Claude Code drops its login on one. The router
  sends the request on another account instead.
- **A request that every account refused** says more about the request than about the accounts. No
  account sits out for it, and its session stays where it was.

## Where a week runs out

Runway and `status` say when each week runs out, such as `out ~Tue`. Or they say how full it will
be at its reset, such as `on pace for 92%`. The pace of one account is a poor guide, because the
router decides where use goes. So switchboard plays the use of the whole pool forward a week,
through the choices the router would make. The forecast is only for showing: the router routes on
its own rates.

- **How fast the pool is used:** the rate over the last day, plus the gap between that and the last
  hour or so. That gap fades: about a third is left after three hours, and a twentieth after nine.
  So a burst lifts the next few hours, and a quiet evening never promises a week.
- **Where the use goes:** use already on an account stays there for a while, as its sessions do.
  On an account whose token is refused or missing, it moves on at once. The router learns how long
  use stays from its own answers. Until it has a day of them, it takes the use to
  stay put, so a run-out leans early, and Runway says so.
- **Where the rest goes:** where a new session would go at that moment, among the pinned accounts
  while a pin is in force.
- **How accounts compare:** by the size of their plan. A Max week counts as five Pro weeks, and an
  Ultra week as ten. An account without a `plan` counts as the plan most accounts have. If none has
  one, every account counts the same.

A week runs out when its use reaches the reserve before it resets. Without a reserve, or while a
pin spends the reserve, it runs out at the limit. Otherwise the forecast gives the use at the
reset, never rounded up: a week heading for 99.6% reads 99%.

Without the router's rates, a week goes at the pace of its use so far. That happens when `status`
probes the accounts itself, or reads an older router that gives no rates. The 5-hour window runs out at its recent rate, as pressure
judges it.

| How far off | How the run-out shows |
|---|---|
| Within a day | The time, such as `out ~16:05` or `out ~Mon 05:22` |
| One day to under six | The day only, such as `out ~Tue` |
| Six to seven days, on the weekday of today | The day and the date, such as `out ~Thu 12 Nov` |

Past a day, the run-out shows the day only, because a forecast that far out can miss by a day.

## Related

- [Limits and routing](../limits-and-routing.md): what you'll notice, and why sessions stay put.
- [Pinning and moving](../pinning-and-moving.md): send sessions to an account you choose.
- [Probes and priming](probes-and-priming.md): how the router reads accounts with no traffic.
