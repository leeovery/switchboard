# Troubleshooting: an account's token or subscription

Each account has a token file, and the API says which subscription a token is for. This page covers
an account whose subscription has ended, whose token is refused, or whose token belongs to another
subscription.

## An account reads `subscription lapsed`

ROUTING reads `■ subscription lapsed · its subscription needs renewing`, and USAGE reads
`subscription lapsed` in red. When the router last read the account, Anthropic refused the read.
Anthropic refuses the login of an ended subscription that way.

The router sends nothing on that account until a read of it comes back clean:

- Sessions on the account move to the other accounts.
- A pin to the account yields.
- `Σ pool` counts no room on the account.

If every account in a pool reads so, no account there can take a request. What you see turns on the
account Claude Code itself signs in with:

| If Claude Code signs in with | You see |
|---|---|
| The pool's primary, as usual | The request goes out on the primary, and Anthropic refuses it. Claude Code shows an API error, `switchboard: the upstream refused account b6s7 (HTTP 403)`, with the reason Anthropic gave, and doesn't retry. |
| An account you removed, or one that just left the pool | Nothing goes out. Switchboard answers as a usage limit: `switchboard: no account in pool Acme can take the request; account b6s7: subscription lapsed`. |

Renew one account, or give one a new token, to carry on.

To bring the account back, renew the subscription. The router reads the account again within half
an hour, and sends it sessions once that read is clean. Press `R` on the dashboard to read it
sooner. If you no longer use the account, remove it. Switchboard keeps the account's history.

```bash
# Removes the account; its history stays
switchboard accounts remove b6s7
```

## An account reads `token refused`

ROUTING reads `■ token refused · its token needs renewing`. When the router last read the account,
Anthropic refused the token. The router sends the account nothing until a read comes back clean,
the same as for `subscription lapsed`.

Give the account a new token. The router takes up a new token within 3 seconds of the file
changing, and routes to the account at once. The same token written back to the file lifts
nothing, because Anthropic would refuse it again.

```bash
# Makes a new token for work; or press t in
# setup
switchboard accounts token b6s7
```

A read that fails for another reason, such as a timeout or an overloaded API, holds nothing back. The
account keeps taking sessions on its last reading. Such a read doesn't lift a hold either: only a
clean read does.

## An account reads `wrong token`

The account reads `wrong token`, or `its token is another subscription's`. The API says the token
in its file is for another subscription than the account's. That happens if the file was replaced
by hand with another account's token. It also happens if a token was made while the browser was
signed in to the wrong account.

The router sends nothing on that account. Its sessions go to the other accounts, and what Claude
Code sends to other paths passes through as before. If no other account can take a request, Claude
Code gets a 503 saying the same.

| If the token was meant | Then |
|---|---|
| For this account | Make the account a new token. |
| For use, on another subscription | Add that subscription as an account, then give this one its own token. |

When you make the new token, approve it in a browser signed in to the account's
subscription.

```bash
# Makes a new token for work; or press t in
# setup
switchboard accounts token b6s7
# Adds the subscription the wrong token
# belongs to
switchboard accounts add <label>
```

`accounts token` refuses a token of another subscription, so it saves only the right one.

## `wrong token` stays after a restart

The router keeps the finding in `state.json` until the account takes up another token. It reads a
new token within 3 seconds of the file changing.

```bash
# The account is marked token_foreign
switchboard status --json
# A foreign event says when it was found
switchboard events
```

## An account added on another Mac

An account added on another Mac that shares the config may have no subscription known here yet.
Its first probe then records its token's subscription as the account's own. If another account has
that subscription already, it reads `wrong token`.

## Related

- [`switchboard accounts`](../commands/accounts.md): adding accounts and giving them tokens.
- [The `setup` screen](setup.md): a token `setup` made but couldn't save.
- [Limits and routing](../limits-and-routing.md): where sessions go when an account takes none.
