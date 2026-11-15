# Staggered resets: one account back soon when all are spent

Switchboard can start each account's 5-hour window at a different time, so your accounts reset one
after another. Once all are spent, the next one is back within 5 hours divided by the number of
accounts. You'd want this if two or more of your accounts have a 5-hour window, as Pro and Max
plans do.

Ultra plans no longer have a 5-hour window, so there is nothing to stagger on them.

## Try it

```bash
# The Priming step asks for your day,
# for example 08:00-23:00
switchboard setup
# The priming schedule, and the next prime
switchboard status
```

Or set your day in the config yourself, in local time:

```toml
[prime]
day = "08:00-23:00"
```

The router restarts itself to take up the change once no request is in flight. To restart it now,
run `switchboard service restart`.

Priming is off until you set a day. With three accounts and a day from 08:00, the schedule looks
like this:

| Account | Primed | First reset |
|---|---|---|
| First | 03:50 | 08:50 |
| Second | 05:30 | 10:30 |
| Third | 07:10 | 12:10 |

## What happens

- Before your day starts, the router sends each account a prime: a tiny request that starts the
  5-hour window at the time the schedule gives.
- Through the day, the router primes each account again 5 seconds after every reset, so the
  windows run back to back.
- With three accounts, a reset comes every 1 hour 40 minutes, and each account meets four windows
  in your day instead of three.
- New sessions favour the account whose 5-hour window resets soonest, among accounts that score
  close to the best.
- When your day ends, priming stops, and the windows lapse overnight.
- Each pool gets a schedule of its own, spread across the accounts in that pool.

## Good to know

- An account without a 5-hour window gets no slot, and the router spreads the resets across the
  rest. The router learns this from the account's answers, never from the plan. A new Ultra
  account keeps a temporary 5-hour window until Anthropic verifies it, and is primed until then.
- Each prime costs next to nothing. It sends one request for each model family, each capped at one
  output token.
- The real cost is short windows at the edges of your day. With three accounts, the first has 50
  minutes of its window left at 08:00. Windows also run while you're away from the keyboard.
- If the router needs to probe an account to route well, the probe can start a window off the
  schedule once. The schedule then settles back by itself.

## Reference

| Setting or command | What it does |
|---|---|
| `[prime] day = "08:00-23:00"` | Turns priming on, and spreads the resets over your day, in local time. |
| An end before the start | Means the day runs past midnight, for example `"18:00-02:00"`. |
| No `[prime]` section | Priming is off. This is the default. |
| `switchboard setup` | Its Priming step sets the day, or turns priming off if you clear it. If every account is on Ultra, setup skips the step. |
| `switchboard status` | Shows the schedule, the next prime, and when each 5-hour window resets. |

The `[prime]` key is in [Configuration](configuration.md), and `status` is in
[`switchboard status`](commands/status.md). The full rules of the schedule are in
[Probes and priming](reference/probes-and-priming.md).

## Related

- [Limits and routing](limits-and-routing.md): how the router picks an account and moves a session.
- [Glossary](glossary.md): what window, priming and room mean.
- [Dashboard](dashboard.md): COMING UP on the Overview lists the next resets and primes.
