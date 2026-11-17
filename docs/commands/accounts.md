# `switchboard accounts`: list, add, edit and remove accounts

`switchboard accounts` lists your accounts, in config order. Its subcommands add an account, edit
one, give one a new token, and remove one.

```bash
switchboard accounts
```

```text
ID    LABEL     PRIMARY  TOKEN
b6s7  work               usable
h8rc  personal  yes      usable
q4tx  side               usable
```

For each account, the list shows:

- the id and the label;
- the pool, by its label, once there's a second pool;
- whether the account is the [primary](../glossary.md) in its pool;
- whether its token file holds a token switchboard can use, or if not, what would put it right.

## Usage and flags

```bash
switchboard accounts
switchboard accounts add <label> \
  [--pool <id>] [--primary] \
  [--new | --returning <id>]
switchboard accounts edit <id> \
  [--label <label>] [--plan <plan>] \
  [--pool <id>] [--primary] \
  [--reserve <percent>]
switchboard accounts token <id> [--paste]
switchboard accounts remove <id>
```

| Flag | Used by | What it does |
|---|---|---|
| `--label <label>` | `edit` | Shows the account as `<label>`. |
| `--plan <plan>` | `edit` | Sets the plan: `pro`, `max`, `ultra`, or `none`. |
| `--pool <id>` | `add`, `edit` | Adds the account to that pool, or moves it there. |
| `--primary` | `add`, `edit` | Makes the account its pool's primary. |
| `--new` | `add` | Answers that the token is a new account. |
| `--returning <id>` | `add` | Answers that the token is the account this Mac had with that id. |
| `--reserve <percent>` | `edit` | Sets the share of every window the router leaves unused. |
| `--paste` | `token` | Takes a pasted token in place of making one. |

- **`--plan`** also takes the old names `max5x` and `max20x`, and writes the new name.
- **`--reserve`** takes a percentage such as `5%` or `2.5%`, below `100%`, with two decimal places
  at most. `0` or `0%` takes the reserve away. Any other bare number, such as `5` or `0.05`, is
  refused, because either could mean the other.
- **Without `--pool`,** `add` uses the only pool, or else the unfenced pool. If every pool is
  fenced, it fails and asks for `--pool`.

## Ids and labels

Every account has an id that switchboard makes, such as `b6s7`, and a label you give it, such as
`work`. Every screen shows the label, and you can change it whenever you like. The id never
changes, and commands take it: `accounts edit`, `pin`, `run --account` and the rest.

The id is 4 lowercase letters and digits, a letter first. Switchboard spells it from the
subscription, so a subscription has the same id on every Mac, and after a lost state directory. If two
subscriptions spell the same 4 characters, the second gets one more.

A label is required, and no two accounts may show the same one, ignoring case. `add` tidies it,
for example trimming spaces at the ends. It refuses one that looks like a token, runs over 100
characters, or would show as nothing. Every rule is in
[Account labels and config edits](../reference/account-labels.md).

## `add`: register an account

`add` writes an `[[account]]` table to the config, and a token file. It makes the
config if there's none. It always takes a token, and checks it with the API before it saves
anything.

```text
added "Acme 2" as m4zq in pool Acme
```

The token's subscription decides the id:

| The subscription | What `add` does |
|---|---|
| New to this Mac | Gives the account a new id. |
| One this Mac had and removed | Brings the account back under its old id. |
| One another account has | Refuses it, naming that account. |
| Unknown | Saves nothing. Try again later. |

An account that comes back keeps its record:
`added "work" as b6s7, an account this Mac had, so its history carries on`. The subscription is
unknown if the API refused the token or didn't answer.

If the subscription doesn't settle it, for example because the API names none, `add` asks whether
the account is new or one this Mac had. Off a terminal, it lists the choices and fails: answer with
`--new` or `--returning <id>`.

Once the config has pools, `add` names the pool in what it prints, once there's a second. A pool
the config doesn't have is refused before the token is asked for. The first account added to a
pool is its primary, and `add` says so.

## `edit`: change an account

`edit` changes the label, plan, pool, primary or reserve of an account, in the config.
What you don't give stays as it is.

```text
edited q4tx: label "Day job" (was "side"),
plan ultra (was none), the primary in place of
h8rc, reserve 2.5% (was none)
```

The output is one line, wrapped here. If every value given is what the account has already, `edit`
says so and writes nothing.

### Moving an account to another pool

`--pool` moves the account into the pool with the id given. The router takes the move up within
about 3 seconds.

| In the new pool | What `edit` does |
|---|---|
| `--primary` is given | Makes the account the primary there. |
| There are no accounts | Marks the account the primary, as the first in it. |
| There's a primary already | Leaves that primary in place. |
| No account is marked primary | Marks the first one, so it stays the primary. |

In the pool it left, the sessions on the account move off it on their next request. If the account
was the primary there, `edit` says which account is the primary now. Sessions that started on its
token are refused, and must exit and resume. If it was the last account there, `edit` says the pool
has no accounts now. The move rewrites nothing in your record: work the account did stays under the
pool it ran in.

### Making an account the primary

`--primary` makes the account the primary in its pool, in place of the one that had it. If the config
marks no primary, the first account counts as one, and `--primary` on it marks it so.
`--primary=false` is refused: give `--primary` to another account instead.

Sessions already running keep the token they started on until they restart. That's the token of the
former primary, or for a pinned session, of its pinned account. So artifacts and uploads from those
sessions still go out on that account, and `edit` says so. Sessions started direct, while the
router was down, aren't touched. If the new primary has no usable token,
`edit` warns on stderr, because new sessions then start on another account's token.

Sign your browser into the first pool's primary, because artifacts open in a browser signed into
it.

## `token`: replace a token

At a terminal, `token` makes a new token the way `setup` does. It runs `claude setup-token`, which
opens a sign-in page in your browser. Once you approve, `token` checks the new token with the API
and saves it, good for a year from today.

The page opens as it is for the first pool's primary, and in a private window for any other
account. Close any other private window first, because they all share one sign-in. With Safari,
`token` shows the address to open yourself.

The token must be for the subscription the account has. `token` checks the tokens of the others
first, all at once, which takes up to about 10 seconds.

| If | What `token` does |
|---|---|
| The token is for another subscription | Saves nothing, and names the account that has it. |
| The API doesn't answer | Saves the token unchecked, with a warning. |
| The token was made but not read or saved | Shows it, and fails. Paste it with `--paste`. |
| Nothing is saved for another reason | Says why, and fails. |

`ctrl+c` before the token is made saves nothing. Once the token exists, it lasts a year, so
`ctrl+c` waits: `token` saves and checks it first, then exits as interrupted.

## `remove`: take an account away

`remove` deletes the account from the config, with the comments directly above it. Then it
deletes the token file and the token's `.taken` file.

- **Its record stays.** The accounts record keeps it, marked removed, as `team (removed 4 Sep)`.
  Add the subscription again and it comes back under its old id.
- **The only account can't be removed.**
- **Removing the primary** says which account is the primary now.
- **Removing the last account in a pool** says the pool has no accounts now.
- **Sessions running on the removed token** stay routed as the primary's sessions, for a week.
  `remove` says if the primary has no usable token.

The router stops using the token of a removed account within about 3 seconds. If the token file
can't be deleted, `remove` fails, and says why. Token files that are links are covered in
[Tokens](../reference/tokens.md).

## Giving a token

At a terminal, `add` and `token --paste` ask for the token, which doesn't show as you paste it.
Otherwise they read it from stdin. To make a token yourself, run `claude setup-token` while signed
in to that subscription.

Neither saves a token the API refuses, one another account has, or one you interrupt. If the API
doesn't answer, `add` saves nothing, because it can't know the id, while `token` saves it with a
warning.

A token is good for a year from when it's saved. See [Tokens](../reference/tokens.md) for how
switchboard keeps the date. If an account reads `wrong token`, see
[Troubleshooting: tokens](../troubleshooting/tokens.md).

## How the config changes

`accounts` edits the config as text, keeping its comments and layout. Two changes are taken up
within about 3 seconds: which pool an account is in, and the token of an account you remove.

Anything else, such as a new account or a new label, waits for the router to restart. The router
run by the service restarts itself once no request is in flight, and
`switchboard service restart` does it now. A router run by hand with `serve` only logs that a
restart is due.

Until the router restarts, `claude` doesn't start Claude Code on a new account's token, even one
added with `--primary`. It takes the next account the router routes, so the session is routed.

## Examples

Work is `b6s7`, personal is `h8rc` and side is `q4tx`.

```bash
switchboard accounts add personal --primary
switchboard accounts add side
# k2m9 is an account this Mac had
pbpaste | switchboard accounts add team \
  --returning k2m9
switchboard accounts edit q4tx --plan max \
  --reserve 5%
# side moves to Acme, as its primary
switchboard accounts edit q4tx --pool q7mt \
  --primary
switchboard accounts token q4tx
pbpaste | switchboard accounts token q4tx
switchboard accounts remove q4tx
```

## Related

- [Pools](../pools.md): what a pool is, and why an account belongs to one.
- [Room for the Claude app](../room-for-the-claude-app.md): what `--reserve` keeps back.
- [Setup](setup.md): the same work, full screen.
