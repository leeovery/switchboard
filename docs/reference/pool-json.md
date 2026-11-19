# `pool` JSON

`pool --json` lists every pool in the config, with its fence and its accounts. For what a pool
does, see [Pools](../pools.md).

```bash
switchboard pool --json
```

```json
{
  "schema": 1,
  "pools": [
    {
      "pool": "d3wn",
      "pool_label": "Default",
      "dirs": [],
      "paths": [],
      "accounts": [
        {
          "account": "b6s7",
          "label": "work",
          "primary": true
        },
        {
          "account": "h8rc",
          "label": "personal"
        }
      ]
    },
    {
      "pool": "q7mt",
      "pool_label": "Acme",
      "dirs": ["~/Code/acme"],
      "paths": ["/Users/you/Code/acme"],
      "accounts": [
        {
          "account": "m2kp",
          "label": "Acme 1",
          "primary": true
        }
      ]
    }
  ]
}
```

## The fields

| Field | Type | Meaning |
|---|---|---|
| `schema` | number | `1`. See [Reading the JSON](reading-the-json.md). |
| `pools` | list of objects | Every pool, in the order of the config. See below. |

Each pool holds these:

| Field | Type | Meaning |
|---|---|---|
| `pool` | string | The id of the pool. |
| `pool_label` | string | The label of the pool. |
| `dirs` | list of strings | The folders of the fence, as the config writes them, `~/` included. Empty for the unfenced pool. |
| `paths` | list of strings | The folders of the fence in full: one for each of `dirs`, in the same order, absolute and cleaned, with `~/` replaced by your home folder. Empty for the unfenced pool. |
| `accounts` | list of objects | The accounts in the pool, in the order of the config, as `{account, label, primary}`. |

`primary` is set only on the primary of the pool.

The router matches a session's folder to `paths` more loosely than an exact string compare. It
follows links on both sides, and ignores case and how accents are written. So a folder that
doesn't equal any of `paths` as text can still be inside the fence.

If the config names no pools, `pools` lists the pools the router routes for, or else those the
accounts record keeps. It's empty only if none of them names a pool, as before the router has made
the first.
