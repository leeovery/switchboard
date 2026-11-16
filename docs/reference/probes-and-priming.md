# Probes and priming

How the router reads an account that has no traffic, and how it works out the priming schedule.
For what priming does for you, see [Staggered resets](../staggered-resets.md).

```console
$ switchboard status
…
priming 08:00-23:00: work at 04:10 and side at 06:40
```

| Probe or prime | Value |
|---|---|
| What a probe sends | One request for each model family, Haiku and Fable, each capped at one output token |
| An account unread for | 5 minutes, then the router probes it on its rounds |
| Before a week resets | One probe in the last 5 minutes, unless something read the account then |
| A probe that reads nothing | Tried again after 2 minutes, then twice as long each time, up to 30 minutes |
| The retries end | Once a probe reads the account, or the account goes out on another token |
| A probe due while the Mac slept | Sent within a minute of waking |
| A prime after a reset | Sent 5 seconds after the reset |
| A prime that fails | Sent again 5 minutes later, or on the backoff for a probe that reads nothing if that's longer and Anthropic refused the read |

## Probes

Every answer the router relays carries the whole usage of its account. So an account in use
through the router is always current. The router reads the other accounts itself, around the
clock, whether a dashboard is open or not.

A probe is a request, so probing an idle account starts its 5-hour window, if it has one. The
router never probes an account whose 5-hour window has lapsed, except in these cases:

- to prime it;
- on its rounds, if the window the probe starts resets before the account's next prime;
- while the account can take no request anyway, for example while a limit holds back its every
  request, its week reads spent, or Anthropic refused its last read.

Starting a window costs nothing in the last case, and a probe is how the router sees a limit you
reset by hand. An account without a 5-hour window has none to lapse, so the router probes it as it
would any account with room.

The router probes every account it has never read as it starts. If that probe fails, the router
tries again on its rounds, or sooner if a decision or a dashboard needs it. The same goes for an
account that gets its first token while the router runs. That can happen at any hour, so it may
start a window off the schedule, once.

## The rounds

On its rounds, the router reads each account that nothing has read in the last 5 minutes, by an
answer, a probe or a prime:

| The account | What the round does |
|---|---|
| Its 5-hour window has lapsed, and it can take a request | Probes it only if its next prime is more than 5 hours off, so the window resets first. Without priming, probes it. |
| It can take no request anyway | Probes it at any hour, so a reset you make by hand shows within 5 minutes. |
| Anthropic refused its last read: its subscription ended, or its token was refused | Probes it at any hour, on the backoff for a probe that reads nothing. A renewal shows within 30 minutes. A new session's choice doesn't probe it any sooner, nor does the dashboard after a window resets. `R` on the dashboard does, and so do `status --refresh` and `usage --refresh`. |
| Anything else | Probes it. |

The router also probes each account once in the 5 minutes before a week resets. That way the last
use of the week is read before it goes. If that probe starts a lapsed window, the account's next
prime is skipped, and the schedule settles back by itself. When these needs pull against each
other, routing comes first, then accurate readings, then priming.

How each account's probes went outlasts a restart, in `state.json`. A restarted router waits
out the backoff as though it had run on. An account given another token while the router was
stopped starts afresh.

## Commands that probe

Some commands probe every account, because you asked them to:

- `usage --probe` and `dashboard --probe`;
- `usage`, `status` or `dashboard` while the router is down.

`accounts add`, `accounts token` and `setup` probe the token they're given, to check it. `setup`
also probes the token of every account as it opens.

## The schedule

The schedule spreads the 5-hour resets of N accounts over your day. It counts only accounts with a
usable token and a 5-hour window, in the order of the config.

1. Resets fall every 5 hours divided by N. The first falls half a step after the day starts.
2. Each account is primed five hours before its first reset. That time is the account's slot.
3. Each slot is taken back to the ten-minute mark it falls in, and its resets move back with it.
4. Each prime goes out 5 seconds after its slot. The Mac's clock runs a little ahead of the API. The 5 seconds land the prime within the ten minutes of the slot.

Anthropic takes the start of every window back to its ten-minute mark, so the schedule shows where
each window really starts. With two accounts, the first is primed at 04:10 and resets at 09:10,
not at 04:15 and 09:15. Slots move back, never on, so every prime falls before your day starts.

For a day that starts at 08:00:

| Accounts | Primed | Resets |
|---|---|---|
| 4 | 03:30, 04:50, 06:00, 07:20 | 08:30, 09:50, 11:00, 12:20, 13:30, 14:50, 16:00, 17:20, 18:30, 19:50, 21:00, 22:20 |
| 3 | 03:50, 05:30, 07:10 | 08:50, 10:30, 12:10, 13:50, 15:30, 17:10, 18:50, 20:30, 22:10 |
| 2 | 04:10, 06:40 | 09:10, 11:40, 14:10, 16:40, 19:10, 21:40 |

## Primes

A prime is a probe sent to an account whose 5-hour window isn't running. If the newest Fable
doesn't answer, the Fable request falls back to the one before.

| If | Then |
|---|---|
| The account's answers carry no 5-hour window | It gets no slot, and the router spreads the schedule over the rest. The log says `no window to prime`. |
| Every account lacks a 5-hour window | `setup` skips its Priming step and says why. |
| The window is already running, for example after a late night | No prime. The slot shifts for the day. |
| A prime reads nothing, or doesn't start the window | It's logged as a warning, and sent again 5 minutes later. |
| A limit or a spent week holds the account back from every request, or the router refused its token | It isn't primed while it stays that way. |
| Anthropic refused its last read: its subscription ended, or its token was refused | It's primed as usual, but a failed prime waits as long as the probes do. A prime that reads it brings the account back. |
| A window resets during your day, in use or not | The router primes it 5 seconds later, so the windows run back to back. |
| Your day has ended | Priming stops, and the windows lapse overnight. A window a round starts still runs, if it resets before the account's slot. |
| A prime was missed while the Mac slept or the router was away | It goes out as soon as it can, unless your day has ended. |

## Seeing the schedule

- `switchboard status` shows the schedule, and the next prime.
- A 5-hour window that hasn't started says, in `status`, when its account is next primed.
- On the dashboard, COMING UP on the Overview lists the next primes with the resets. Runway leaves
  primes out.

## Related

- [Staggered resets](../staggered-resets.md): what priming does for you, and how to turn it on.
- [Routing rules](routing.md): how the router picks an account.
- [Configuration](../configuration.md): the `[prime]` key.
