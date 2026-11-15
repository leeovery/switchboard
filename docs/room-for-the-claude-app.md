# Room for the Claude app: keep part of an account for yourself

A reserve keeps a share of every window on an account that the router never spends. Set one on the
account your Claude app is signed into. Claude Code then leaves room there for the app, claude.ai
and anything else outside switchboard. Without a reserve, the router runs every
account up to its limit.

<p align="center"><img src="../art/overview.webp" alt="The Overview: work at 95% of its 5-hour window, at its cap, with new sessions going elsewhere, and LATELY saying work reached its cap and 3 sessions moved to side" width="840"></p>

*The Overview with work at its cap: new sessions go elsewhere, and its three sessions moved to side.*

## Try it

```bash
switchboard accounts                          # each account, and which is the primary
switchboard accounts edit b6s7 --primary      # b6s7 is the account the Claude app uses
switchboard accounts edit b6s7 --reserve 10%  # keep 10% of every window on b6s7
```

With a reserve of 10%, the cap of the account is at 90%. Once any window on the account reaches
90%, the router stops choosing it.

The primary is the account your browser and the Claude app are signed into. In every session, Claude
Code sends artifacts and uploads on the token of the primary, so they open in that browser. The
conversation itself still goes wherever the router sends it. A reserve works on any account, but
the primary is where you'll usually want one.

## What happens

- Once a window on the account reaches the cap, new sessions skip the account, and sessions on it
  move as they would at a limit.
- When the router compares accounts, it counts only the room below each cap.
- The dashboard marks each cap on the bars with `╎`, and shows `at its cap` with the time the
  account is back.
- When the window resets, the account takes sessions again.
- If only reserved room is left, the router leaves it alone, and Claude Code gets a limit refusal
  from switchboard.
- The Claude app can still use the reserved room, and take the account past its cap, as intended.

## Good to know

- A pin spends the reserve, because a pin is your choice, not the choice of the router. If every
  other account is out and you need the room, run `switchboard pin b6s7 --move`. To hand the
  sessions back to the router, run `switchboard pin auto`.
- The router reads usage off each answer, so one large turn can go a point or two past the cap
  before the router sees it.
- A session that bypasses the router can spend the reserve. That covers `switchboard run --direct`,
  and every session started while the router is down if you set `when_router_down = "direct"`.
- With pools, each pool has a primary of its own. Your browser and the Claude app usually use the
  primary of the first pool.

## Reference

| Setting or command | What it does |
|---|---|
| `reserve = 0.1` | In the table of the account, keeps 10% of every window. The default is `0`. |
| `accounts edit <id> --reserve 10%` | Sets the reserve as a percentage. `0%` takes it away. |
| `primary = true` | In the table of the account, makes it the primary of its pool. |
| `accounts edit <id> --primary` | Makes the account the primary, in place of the one that was. |

The router takes a new reserve up as it restarts, which the service does by itself after any change
to the config. The `[[account]]` keys are in [Configuration](configuration.md), and every flag is in
[Commands](commands.md).

## Related

- [Limits and routing](limits-and-routing.md): how the router picks an account and moves a session.
- [Pinning and moving sessions](pinning-and-moving.md): send sessions to an account you choose.
- [Glossary](glossary.md): what reserve, cap, primary and room mean.
