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
    {"pool": "q7mt", "pool_label": "Acme", "dirs": ["~/Code/acme"], "accounts": [{"account": "b6s7", "label": "Acme 1", "primary": true}]},
    {"pool": "h2kd", "pool_label": "Personal", "dirs": [], "accounts": [{"account": "t3pz", "label": "personal"}]}
  ]
}
```

## The fields

| Field | Type | Meaning |
|---|---|---|
| `schema` | number | `1`. See [Reading the JSON](reading-the-json.md). |
| `pools` | list of objects | Every pool, in the order of the config. Empty if the config has no pools yet. |

Each pool holds these:

| Field | Type | Meaning |
|---|---|---|
| `pool` | string | The id of the pool. |
| `pool_label` | string | The label of the pool. |
| `dirs` | list of strings | The folders of the fence, as the config writes them, `~/` included. Empty for the unfenced pool. |
| `accounts` | list of objects | The accounts in the pool, in the order of the config, as `{account, label, primary}`. |

`primary` is set only on the primary of the pool.
