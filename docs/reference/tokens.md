# Tokens

Each account keeps its token in a file of its own, outside the config. This page covers where the
token files live, how switchboard reads and writes them, and the `.taken` file that says when each
token was taken. Read it if you keep tokens in a secrets manager or a dotfiles repository.

## The token files

```text
# only the token of b6s7
~/.local/state/switchboard/tokens/b6s7
# when switchboard took each latest token
~/.local/state/switchboard/tokens/b6s7.taken
```

| File | What it holds |
|---|---|
| `<state dir>/tokens/<id>` | Only the token. Whitespace around it is ignored. |
| `<state dir>/tokens/<id>.taken` | When switchboard took each of the 8 latest tokens of the account, with a hash of each. |

The state directory is `$XDG_STATE_HOME/switchboard`, else `~/.local/state/switchboard`.
Switchboard reads no token from the environment, and none from the config.

## Who can read them

A token file must be yours, and no one else can read or write it, as `chmod 600` leaves it.
Otherwise the account counts as having no token, and `accounts` and `status` say why and how to
fix it.

Switchboard keeps the `tokens` folder at `0700` whenever it writes there. The router also sets the
folder to `0700` as it starts.

## Writing a token file

`setup`, `accounts add` and `accounts token` write token files. Anything else can write them too,
such as a secrets manager or a dotfiles step:

- Write a temporary file beside the token file, then rename it into place, as switchboard does.
- A token file can be a link to a file kept elsewhere. Switchboard writes a new token through the
  link, to the file it leads to.
- If a new account's token file already holds a usable token, `accounts add` adds the account
  without asking for one.

## When switchboard reads them

A changed token file needs no restart. The router reads the token files at these times:

- when it starts;
- every 3 seconds, for every account;
- when the API refuses an account's token, for that account.

A file found without a usable token at one look keeps the token the router has, because a writer
may be partway through. Only two looks in a row, 3 seconds apart, take the token away.

`claude` looks twice too. If the token file of the [primary](../glossary.md), or of a pinned
account, is there but empty, it looks again a moment later.

## When each token was taken

A setup token is good for a year from when it was made. Switchboard writes when it took each token
in `tokens/<id>.taken`, a pair of lines for each token, the latest first:

```text
taken 2026-10-27T09:30:00Z
sha256 <SHA-256 of the token, lowercase hex>
taken 2026-03-02T14:05:00Z
sha256 <the hash of an earlier token>
```

`switchboard status --json` gives each account `token_taken`, and `token_good_until`, 365 days on.

- A token's year counts from when switchboard took it. Nothing says when a pasted token was
  made, so its year counts from the day you gave it.
- Giving the same token again keeps the earlier date. After 8 other tokens, the `.taken` file
  has forgotten it, and its year counts afresh.
- The hash ties each date to its token. The `.taken` file never holds a token.
- A token written by anything else has no known date, unless the `.taken` file already holds that
  token. Restoring both files together keeps the date.
- A missing or damaged `.taken` file only leaves the date unknown. The token works as before.

### Notifications as the year ends

Whatever the `[notifications]` keys say, the router tells you as a token nears the end of its year:

| When | What you see |
|---|---|
| 30 days or fewer are left | A warning, such as the one below |
| 7 days or fewer are left | The same warning, once more |

The message is one line, wrapped here to fit:

```text
b6s7 · work: token expires 17 Nov 2026
· 30 days left — switchboard setup
makes a new one
```

- **If the year is already up** when a warning goes out, it says `token expired 17 Nov 2026`
  instead. No third notice goes out when the year ends.
- **With two pools or more,** the label of the pool leads the message, as
  `Acme · m2kp · Acme 1: …`.
- **A router started with a token already inside both spans** tells only the nearer one.
- **A restart** never tells either again, and a new token starts afresh.

## The `.taken` file and its permissions

The `.taken` file must be private, as the token file is. If it's anything but a private plain
file, one of these cases applies. In every case, the token works as before.

- **A plain file that others can read but not write,** as a dotfiles checkout leaves it. The
  router makes it private as it starts, and the dates count again.
- **A hard link or a symlink to a file that is yours,** that no one else can write. Switchboard
  keeps the dates. It writes them afresh, private, with the next token.
- **A link to anything else:** switchboard leaves the link and what it leads to, and warns in the
  log. That covers a link to nothing, to another account's token file, or to a file that isn't a
  `.taken` file.
- **Another account's token file or `.taken` file,** however it's reached: switchboard leaves it,
  and warns in the log.
- **A path that another account's token file or `.taken` file links to:** switchboard leaves it,
  and warns in the log.
- **A file that holds anything shaped like a token,** can't be read, or is over 1 MiB: switchboard
  never writes over it. A later token for that id leaves it, with a warning. The date stays unknown
  until you move the file.

## Removing an account

`accounts remove` deletes the `.taken` file with the token file. If either is a link, it deletes
only the link. These cases differ:

- **The file at the `.taken` path holds a token,** can't be read, or is over 1 MiB:
  `accounts remove` leaves the file, with a warning.
- **The same, but the file is a hard link** of the token file, or of the file it leads to. Then
  the file goes with the token file.
- **The token file links to the `.taken` path, and nothing else names that file:** it's the only
  copy of the token, so it stays. While the link is in place, a new token is written there,
  without a `.taken` file, and the log warns.
- **The token file links to the `.taken` path, and the file there has another name:** the `.taken`
  path goes with the token. The file stays under its other name, and `accounts remove` says so.

Writing a token replaces the token file. A hard link you made by hand stops being one at the next
write. The `.taken` path then holds the old token, and switchboard leaves it there, as it leaves
any file holding a token.

## Replaced and removed tokens

A running session keeps the token it started with:

| If | Then |
|---|---|
| You replace an account's token | Sessions started before still carry the old token. The router routes them as that account for 7 days. |
| You remove an account | Sessions started on its token still carry it. The router routes them as sessions of the primary for 7 days. |

The router keeps none of these tokens, only their SHA-256 hashes.

## How `accounts token` checks the subscription

`switchboard accounts token` saves a token only if it's for the account's own subscription.

| If | Then |
|---|---|
| The account's subscription isn't known yet | The token's subscription is taken as the account's own. |
| The API names no subscription for the token | The token is saved. |
| The token is for another subscription | Nothing is saved. |

The message for another subscription is `the token is for another subscription, so nothing is
saved: add it as an account of its own`. It names the account that has the token, if this Mac has
one.

If you configure that account here, the token is its own, given to the wrong account. The message
then says so, and names the command that gives it to that account:

```console
$ switchboard accounts token work
Error: the token is for another subscription, that of "Side" (side), an account configured here, so nothing is saved: it's that account's token, given the wrong one: give it side with switchboard accounts token side
```

## Related

- [Configuration](../configuration.md): the config, which holds no tokens.
- [`accounts`](../commands/accounts.md): `accounts add`, `accounts token` and `accounts remove`.
- [Token troubles](../troubleshooting/tokens.md): an account that shows `no token` or
  `wrong token`.
