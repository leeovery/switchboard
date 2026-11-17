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
| `accounts` | list of objects | The accounts in the pool, in the order of the config, as `{account, label, primary}`. |

`primary` is set only on the primary of the pool.

If the config names no pools, `pools` lists the pools the router routes for, or else those the
accounts record keeps. It's empty only if none of them names a pool, as before the router has made
the first.
