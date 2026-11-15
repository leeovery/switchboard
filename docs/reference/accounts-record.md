# The accounts record

`accounts.json` in the state directory records every account and every pool this Mac has had,
removed ones included. It's how your record names an account long after you remove it.

```json
{
  "version": 2,
  "accounts": [
    {
      "id": "b6s7",
      "organization": "<organization id>",
      "added": "2026-11-12T09:30:00Z",
      "was": "work",
      "pool": "d3wn"
    },
    {
      "id": "t3pz",
      "label": "spare",
      "added": "2026-08-01T00:00:00Z",
      "removed": "2026-09-04T23:59:59Z"
    }
  ],
  "pools": [
    {
      "id": "d3wn",
      "added": "2026-11-12T09:30:00Z",
      "first": true,
      "here": true,
      "written": {
        "primary": "b6s7",
        "primary_known": true
      }
    }
  ]
}
```

- `version` is 2 since pools. A release from before pools reads only version 1, so it never
  writes over an accounts record that holds pools.
- The file is written whole, and only its owner can read it.
- It's kept for good, at a few hundred bytes an account.
- Once the move onto account ids is done, the router keeps it in step with the config. An account
  added or removed on another Mac that shares the config is added, or marked removed, here too.
  See [Several Macs](../several-macs.md).

## Account fields

| Field | Type | Meaning |
|---|---|---|
| `id` | string | The id switchboard made for the account. Every file switchboard keeps names the account by it. |
| `organization` | string | The id of the subscription, as Anthropic's API names it in its answers. Left out if unknown. |
| `label` | string | The label it showed, kept once it's removed. Left out while it's configured. |
| `added` | time | When it was added, in UTC, to the second. |
| `removed` | time | When it was removed. Left out while it's configured. |
| `was` | string | The id it had before switchboard made ids, if it had one. |
| `pool` | string | The id of the pool the config last had it in. Kept once it's removed. |
| `configured` | boolean | Only in the copy an export writes. Set on each account the exporting Mac configures. |

- **`id`** is 4 lowercase letters and digits, a letter first. It's a character longer if 4 would
  clash.
- **`organization`** is unknown, for example, for an account removed before switchboard made ids.
- **`label`** is left out while the account is configured, because the label in the config is the
  one that counts then.
- **`was`** lets an import read a folder exported before ids. It also puts back an old id that a
  config or a pin still names.
- **`configured`** means the `label` is the one in the config of the exporting Mac. A Mac that
  imports it names the account the same way.

## Pool fields

| Field | Type | Meaning |
|---|---|---|
| `id` | string | The id switchboard made for the pool. |
| `label` | string | The label it showed, kept once it's removed. |
| `added` | time | When it was added. |
| `removed` | time | When it was removed. Left out while it's configured. |
| `first` | boolean | Set on the first pool, which holds the work from before pools. |
| `configured` | boolean | Set as on an account, in the copy an export writes. |
| `here` | boolean | Set while this Mac's config has the pool. |
| `written` | object | The pool as this Mac's config last wrote it. |

`here` is never set on a pool that only another Mac has. A copy written by an export holds neither
`here` nor `written`.

The `written` object holds these:

| Field | Type | Meaning |
|---|---|---|
| `label` | string | The label in the config. |
| `dirs` | list of strings | The folders of the fence, as written, such as `~/Code/acme`. |
| `primary` | string | The id of the primary: the account the config marks, else the first. |
| `primary_known` | boolean | Set once a config has said which account is primary. |

With `primary_known` set, a pool without `primary` had no account.

## A config from before pools

If you put back a config from before pools, the router gives it its pools back from the accounts record.
It does so as it starts, and whenever it finds the config naming no pools.

1. It writes the account ids back first.
2. It puts back each pool marked `here`, as last written.
3. Each account goes back to the pool it was in. An account in no pool, such as one added since,
   goes to the unfenced pool.
4. Each primary is marked as the config last had it. A pool that had no account keeps the marks
   the config gives.

Writing the config is a convenience. Whether or not the write works, the router routes each
account as the config would be given back.

| If the router | Then |
|---|---|
| Can write the config | The config has its pools again. |
| Can't | The config stays as it is. The router tries again each minute. |

- **The router can't write the config** if, for example, every pool is fenced and an account has no
  pool to go to. The log says so once.
- **An account with no pool to go to** stays in no pool, so no session uses it.

If the accounts record itself can't be read:

| If the router | Then |
|---|---|
| Has routed within pools since it started | The pools stand as it last routed within them. |
| Never routed within pools | It routes nothing, and reports itself unhealthy. |

A router that routes nothing answers every request with a refusal saying why. That happens, for
example, to a router started on such a config.

## An accounts record that won't read

- Each `id`, `organization` and `was` belongs to one account at most. Ids are compared ignoring
  case.
- An accounts record that doesn't read, for example one damaged or edited wrongly by hand, is left as it is.
  Switchboard never writes over it.
- Meanwhile, anything that would change the accounts record fails, and names the file. Adding or removing an
  account are two examples.
- Meanwhile too, your record names each removed account by its id.

To fix it, put the file right, or move it aside to start a new one. A new accounts record names the
accounts removed before by their ids.

## An account removed while no router ran

An account that left the config while no router ran is marked removed by its id only, with no
label. The next import that names it gives it its label.
