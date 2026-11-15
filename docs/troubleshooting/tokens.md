# Troubleshooting: an account with the wrong token

Each account has a token file, and the API says which subscription a token is for. This page covers
an account whose token belongs to another subscription.

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
