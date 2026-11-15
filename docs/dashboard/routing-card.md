# The routing card: choose where new sessions go

The routing card is the only place on the dashboard that changes routing. Use it to pin new
sessions to an account you choose, and to move running sessions there too. It changes nothing
until you press `enter`.

<div align="center">
<img src="../../art/routing.webp" alt="Switchboard's dashboard routing three accounts: the Overview's usage, routing and LATELY, a new session starting on side, then the routing card pinning new and running sessions to side, personal's two moving there" width="840">
</div>

*A new session starts on side, then the routing card pins new and running sessions to side.*

## Opening the card

Press `p` on any tab. The card opens over the page, showing what's set now. It has these rows:

- `auto`, for the router's choice;
- a row for each account, by its label, in the order the router has them.

Each account row shows:

- whether it's open;
- its week and 5-hour window, or `no 5-hour limit` for an account without one;
- why it's closed, such as `at its cap till 17:10`, or else its sessions running.

## Picking

- `↑` and `↓` move between the rows, and `space` picks the row the cursor is on.
- Picking `auto` clears the accounts, and picking an account clears `auto`.
- `m` ticks moving the sessions running elsewhere to the picks too. The card then says how many
  sessions would move, and roughly what rebuilding their caches costs, such as `≈ $6.20`.
- Below the rows, a sentence says what the picks do.

You can't pick an account that can't take a session. That's an account:

- at a limit;
- whose token is refused;
- with no token;
- holding the token of another subscription, which reads `wrong token`.

## Applying

- `enter` applies the picks, as [`switchboard pin`](../commands/pin.md) does. The router's event
  names the dashboard as what set them.
- `esc` closes the card with nothing changed.

## When `p` does nothing

| If | Then |
|---|---|
| The router doesn't answer | `p pin` is drawn faint, and `p` says why instead of opening the card. |
| You ran the dashboard with `--probe` | `p pin` isn't listed, and `p` opens nothing, even while the router answers. |
| There's one account | `p` does nothing, because there's nothing to pin to. If a pin set with [`switchboard pin`](../commands/pin.md) still stands, `p` opens the card to clear it. |

Outside the card, `m` does nothing, so a stray key moves no session.

## A session's own card

On the session page, while the session runs, `p` opens a card for that session
alone. Use it to pin the session to an account, or back to `auto`. It has no `m`.

## Good to know

- **Accounts show by their labels.** They aren't numbered, here or on any tab.
- **A removed account leaves the card within about 3 seconds.** The router takes it out of its
  pool, and sessions still on its token count as the primary's.
- **A new account joins the card once the router restarts.** Until then, Accounts and Trends show
  it by its label only. [Upgrades and restarts](../upgrades-and-restarts.md) says when the router
  restarts.
- **A new order of accounts** shows on the card once the router restarts.
- **The page behind a card stands still.** A card over a page, the routing card or help, fades the
  page behind it. Only the clock keeps ticking, while the figures on the card follow each read.

## Related

- [Pinning and moving](../pinning-and-moving.md): pins, and moving sessions, from the command line.
- [Limits and routing](../limits-and-routing.md): how the router picks an account on `auto`.
- [The session page](session-page.md): where a session's own card opens.
