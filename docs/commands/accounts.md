# `switchboard accounts`: list, add, edit and remove accounts

`switchboard accounts` lists your accounts, in the order of the config. Its subcommands add an
account, edit one, replace the token of one, and remove one.

```bash
switchboard accounts
```

```text
ID    LABEL     PRIMARY  TOKEN
b6s7  Work      yes      usable
q4tx  Side               usable
```

For each account, the list shows:

- the id and the label;
- the pool, by its label, once there's a second pool;
- whether the account is the primary of its pool;
- whether the token file holds a token switchboard can use, or if not, why not and what would put
  it right.

## Usage and flags

```bash
switchboard accounts
switchboard accounts add <label> [--pool <id>] [--primary] [--new | --returning <id>]
switchboard accounts edit <id> [--label <label>] [--plan <plan>] [--pool <id>] [--primary] [--reserve <percent>]
switchboard accounts token <id> [--paste]
switchboard accounts remove <id>
```

| Flag | Default | What it does |
|---|---|---|
| `--label <label>` | | `edit`: shows the account as `<label>`. |
| `--plan <plan>` | | `edit`: the plan of the subscription: `pro`, `max`, `ultra`, or `none` to leave it unknown. The old names `max5x` and `max20x` work too, and are written with the new name. |
| `--pool <id>` | the pool the verbs act on | `add`: adds the account to that pool. `edit`: moves the account into that pool. |
| `--primary` | | `add` and `edit`: makes the account the primary of its pool, in place of the one that was. |
| `--new` | | `add`: if it asks which account the token is, answers that it's a new account. |
| `--returning <id>` | | `add`: if it asks which account the token is, answers that it's the account this Mac had with that id. |
| `--reserve <percent>` | none | `edit`: the share of every window the router leaves unused, such as `5%` or `2.5%`. `0%` takes the reserve away. |
| `--paste` | | `token`: takes a pasted token, made with `claude setup-token`, in place of making one. |

Without `--pool`, `add` uses the only pool, or else the unfenced pool. See
[Commands](../commands.md) for how a verb chooses its pool.

## Ids and labels

Every account has an id that switchboard makes, such as `b6s7`, and a label you give it, such as
`Work`. The label is what every screen shows, and you can change it whenever you like. The id never
changes. Every record keys the account by its id, and these commands take it:

- `accounts edit`, `accounts token` and `accounts remove`;
- `pin`;
- `run --account`;
- `requests --account`.

`switchboard accounts` lists each id beside its label, and Claude finds it in `switchboard status`.

The id is 4 lowercase letters and digits, a letter first. Switchboard spells it from the
subscription, which every answer from Anthropic names. So a subscription has the same id on every
Mac, and after a lost state directory. If two subscriptions spell the same 4 characters, the
second gets one character more.

Nothing numbers the accounts. They keep the order of the config on every tab, and the routing card
picks them with `↑`, `↓` and `space`.

## `add`: register an account

`add` writes an `[[account]]` table to the config, and the token file of the account. It makes the
config if there's none. It always takes a token, and checks it with the API before it saves
anything.

```text
added "Acme 2" as m4zq in pool Acme
```

The subscription of the token decides the id of the account:

| The subscription | What `add` does |
|---|---|
| New to this Mac | Gives the account a new id. |
| One this Mac had and removed | Brings the account back under its old id, with the label given now, so its history carries on: `added "Work" as b6s7, an account this Mac had, so its history carries on`. |
| One that an account has already | Refuses it, naming that account. To give that account a new token, use `accounts token`. |
| Unknown, because the API refused the token or didn't answer | Saves nothing, because the id can't be known. Try again later. |

Sometimes the subscription doesn't settle it. That happens if the API names no subscription, or if
the token may be an account removed before switchboard made ids. Then `add` asks whether the
account is new or one this Mac had, naming each choice by its label and when it was last seen. At a
terminal, you pick one. Otherwise `add` lists the choices and fails, and you answer with `--new` or
`--returning <id>`. An answer given when the subscription settled it goes unused, and `add` says
so.

### Adding to a pool

Once the config has pools, `add` puts the account in the pool `--pool` names, or else in the pool
the verbs act on. Once there's a second pool, it names the pool in what it prints.

- A pool the config doesn't have is refused before the token is asked for: `pool "zz" is not
  configured`.
- The first account added to a pool is its primary, and `add` says so: `added "Acme 1" as k3f9 in
  pool Acme, the primary there, as the first in it`.

### Labels

A label is required, and no two accounts may show the same label, ignoring case. `add` tidies the
label before it saves it:

- It drops characters that Unicode draws as nothing, such as zero-width spaces, joiners, variation
  selectors and tags.
- It turns each run of whitespace into one space.
- It trims whitespace and direction marks from both ends.

It keeps such a character if it makes an emoji or a flag, or shapes a joining script such as
Persian or Devanagari. A variation selector stays only in these places:

- after a symbol, as in ❤️;
- after non-ASCII punctuation;
- after a letter outside Latin, Greek and Cyrillic;
- in a keycap, as in 1️⃣.

Direction marks inside the label stay, and so does every other character.

`add` refuses a label that:

- looks like a token, checked before it asks for the token;
- is over 100 characters as shown;
- holds a control character, such as a newline or a tab;
- holds a bidirectional embedding, override or isolate, which could reorder the row it shows in;
- holds a sign that joins onto the next character, such as an Arabic number sign, which can hide
  that character;
- shows as nothing, such as only spaces or zero-width characters;
- isn't UTF-8 text.

Private-use glyphs, such as Nerd Font icons, show as they are.

## `edit`: change an account

`edit` changes the label, plan, pool, primary or reserve of an account, in its table in the config.
Give one or more of `--label`, `--plan`, `--pool`, `--primary` and `--reserve`. What you don't give
stays as it is.

```text
edited side: label "Day job" (was "Side"), plan ultra (was none), the primary in place of personal, reserve 2.5% (was none)
```

`edit` says what it changed, with each value as it is now and as it was. If every value given is
what the account has already, `edit` says so and writes nothing.

### Moving an account to another pool

`--pool` moves the account into the pool with the id given. A pool that isn't configured is
refused, and `edit` names the pools there are.

| In the new pool | What `edit` does |
|---|---|
| `--primary` is given | Makes the account the primary there. |
| The pool has no accounts | Marks the account the primary there, as the first in it, and says so. |
| The pool has a primary already | Leaves that primary in place. |
| The pool has accounts but the config marks none primary | Marks the first one primary, so the account moved in doesn't take over by coming first: `marked k3f9 the primary there, which it stays`. |

In the pool it left, the sessions the router sent to the account move off it on their next request,
with the event `moved: <id> left the pool`. If the account was the primary of that pool, `edit` says
which account is the primary now. If its token was usable, sessions may have started on it, so
`edit` also says:

- that the sessions running on its token are refused now, and must exit and resume;
- which token they start on when resumed: the new primary, or the first account there with a usable
  token.

If the account was the last in that pool, `edit` says the pool has no accounts now, and how to add
one. See [Pools](../pools.md).

The move rewrites no history. The work the account did stays under the pool it ran in, and the
windows of the account go with it.

### Making an account the primary

`--primary` makes the account the primary of its pool, in place of the one that had it. If the
config marks no primary, the first account counts as one, and `--primary` on it marks it so.

Sessions already running through the router keep the token they started on until they restart.
That's the token of the former primary, or of the first account with a usable token, or, for a
pinned session, of its pinned account. What they send outside the conversation, such as artifacts
and uploads, still goes out on that account, and `edit` says so.

- `edit` speaks of pinned sessions only if a pin elsewhere is possible.
- Sessions started direct, while the router was down, aren't touched.
- If the new primary has no usable token, `edit` warns on stderr, because new sessions then start on
  another account.

`--primary=false` is refused. To move the primary, give `--primary` to another account.

Your browser and the Claude apps use the primary of the first pool.

### Values edit refuses

- **A reserve written as a bare number,** such as `5` or `0.05`, is refused, because either could
  mean the other. Write it as a percentage, from `0%` up to but not including `100%`, with two
  decimal places at most.
- **A label** is refused for the same reasons as on `add`, and so is `--label ""`.
- **An id no account has** is reported before anything wrong with a flag value. Errors in the flags
  themselves, such as a missing value or an unknown flag, come first of all.

### How edit writes the config

- A value it replaces keeps the comment beside it. A key it takes out goes with its comment.
- A label written untidily in the config and given tidily is saved tidily, and `edit` says the old
  one was untidy.
- A label written by hand is shown as it is. A character that `add` would drop, a bidirectional
  control or a direction mark shows as its escape. What `add` keeps in an emoji, a keycap, a
  variant, a flag or a joining script shows as it is.
- A label written over several lines, as a multi-line string, can't be edited as text. `edit`
  refuses to change it, and you edit it by hand. Plan, reserve and primary edits still work: a new
  key goes below the line that closes the label.

## `token`: replace a token

At a terminal, `token` makes a new token the way `setup` does, and says each step as it goes. It
runs `claude setup-token`, which opens a sign-in page in your browser. Once you approve, `token`
checks the new token with the API and saves it, good for a year from today.

| Account | Where the sign-in page opens |
|---|---|
| The primary of the first pool | Your browser, as it is, because it's signed in to that account |
| Any other, with a browser that opens a private window from the command line, such as the Chrome family, Edge or Firefox | A private window, so you sign in to that account there |
| Any other, with a browser that can't, such as Safari | Nowhere: `token` shows the address to open yourself |

Close any other private window first, because private windows all share one sign-in.

The token must be for the subscription of the account itself. `token` checks the tokens of the other
accounts first, all at once, to know their subscriptions. That takes as long as the API takes to
answer, up to about 10 seconds.

| If | What `token` does |
|---|---|
| The token is for another subscription | Saves nothing: `the token is for another subscription, so nothing is saved: add it as an account of its own`. It names the account that has it, if there is one. |
| The subscription of the account isn't known yet | Takes the subscription of the token as its own. |
| The API names no subscription | Saves the token. |
| The API doesn't answer | Saves the token unchecked, with a warning. |
| The token was made but couldn't be read off the screen | Shows what the screen showed, token and all, and fails. Paste it with `--paste`. |
| The token was read but couldn't be saved | Shows the token, and why, and fails. Paste it with `--paste`. |
| Nothing is saved for any other reason | Says why, and fails. Run it again, or paste a token with `--paste`. |

`ctrl+c` before the token is made stops `claude setup-token` and saves nothing. Once the token
exists, it lasts a year on the account, so `ctrl+c` waits. `token` says it's saving the token, checks
it, says what came of it, and then exits as interrupted.

## `remove`: take an account away

`remove` deletes the table of the account from the config, with the comments directly above it.
Then it deletes the token file, and the record of when the token was taken.

- **Its history stays.** The accounts record keeps the account, marked removed, with its label.
  History, Accounts and the verbs name it by that label and the day it went, such as
  `Team (removed 4 Sep)`. Add the subscription again and it comes back under its old id.
- **The only account can't be removed.**
- **Removing the primary** says which account is the primary now: the one marked `primary = true`,
  or else the first, in the same pool.
- **Removing the last account of a pool** says the pool has no accounts now. Its sessions aren't
  routed until you add one.
- **Sessions running on the removed token** stay routed, as the primary, for a week from when the
  router takes the change up. That holds while the primary has a usable token, and `remove` says
  if it hasn't.

If Macs share the config, each router keeps the accounts record of its Mac in step with it. An
account removed on one Mac is marked removed on the other, with its label, and one added back is
live again. An account that left the config while no router ran is marked removed by its id only.
The next import that names it gives it its label.

If the token file can't be deleted, `remove` fails. It says the account is out of the config, and
gives the path of the token file and why it wasn't deleted.

A token file or record that's a link is deleted as a link, and the file it leads to stays. A token
file can lead to the path of the record, with the file there having another name too. Then the
record goes with the token, the file stays under its other name, and `remove` says so.

## A token of another subscription

The router holds each account to its subscription. Each probe names the subscription of the token
it went out on. If that's another subscription than the account has, the account reads
`wrong token`, for example after a token file was replaced by hand.

- **It takes no requests.** A conversation request goes out on another account instead. If no
  other account can take it, switchboard answers 503, saying why and what puts it right.
- **Other requests pass through,** such as an upload, on the token Claude Code started with.
- **The dashboard shows it** as `wrong token`, and `status` as `its token is another
  subscription's`. The router files a `foreign` event, and the status document marks the account
  `token_foreign`.
- **The finding stays through restarts,** and goes once the account takes up another token.

An account added on another Mac may have no subscription known here yet. Its first probe then
records the subscription of its token, unless another account has that one. To put a
`wrong token` right, see [Troubleshooting: tokens](../troubleshooting/tokens.md).

## Giving a token

At a terminal, `add` and `token --paste` ask for the token, which doesn't show as you paste it.
Otherwise they read it from stdin, and so does `token`. To make a token yourself, run
`claude setup-token` while signed in to that subscription.

| The token | `add` | `token` |
|---|---|---|
| Refused by the API | Saves nothing. | Saves nothing. |
| Another account has its subscription | Adds nothing. | Saves nothing. |
| The API doesn't answer | Saves nothing, because it can't know the id. | Saves it, with a warning. |
| You interrupt while it waits for the token | Saves nothing. | Saves nothing. |

Once the API has named the subscription, both check the tokens of the other accounts, all at once.

A token is good for a year from when it's saved, because nothing says when a pasted token was made.
Saving a token again keeps the earlier of its two dates, unless 8 other tokens were saved since.
Then it counts afresh. A record restored readable by others but not writable keeps its dates too,
written afresh and private.

## How the config changes

`accounts` edits the config as text, keeping its comments and layout. A config that's a link is
written where the link leads. The router takes the change up itself:

- The router run by the service restarts to read it, once no request is in flight.
- A router started by hand with `serve` logs that a restart is due.

## Examples

Here, the id of work is `b6s7` and the id of side is `q4tx`.

```bash
switchboard accounts add Work --primary
switchboard accounts add Side
pbpaste | switchboard accounts add Team --returning k2m9   # k2m9 is an account this Mac had
switchboard accounts edit q4tx --plan max --reserve 5%
switchboard accounts edit q4tx --pool q7mt --primary       # side moves to Acme, as its primary
switchboard accounts token q4tx
pbpaste | switchboard accounts token q4tx
switchboard accounts remove q4tx
```

## Related

- [Pools](../pools.md): what a pool is, and why an account belongs to one.
- [Room for the Claude app](../room-for-the-claude-app.md): what `--reserve` keeps back.
- [Setup](setup.md): the same work, full screen.
