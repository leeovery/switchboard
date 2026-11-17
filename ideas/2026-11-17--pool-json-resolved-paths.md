# `pool --json` gives each fence folder resolved

`switchboard pool --json` gives each pool's `dirs` as the config writes them:

```json
{"pools": [{"pool": "b2n8", "pool_label": "acme", "dirs": ["~/Code/acme"], "accounts": […]}]}
```

An agent that wants to know which pool a folder is in has to expand `~` itself first. If it
expands it wrongly, as against another home, it matches the wrong pool, or none.

## What it could do

- **Resolved paths beside them:** each pool gives its folders twice, `dirs` as written and a second
  field, such as `paths`, absolute, as the router resolves them. An agent then compares a folder
  to them as they are.
- **One resolver:** the paths come from the config's own resolving, the one the router fences
  sessions by, so the verb and the router never disagree.

## Costs and limits

- It adds a field to a v1 format; `dirs` stays as it is.
- A folder the config can't resolve needs a rule: left out of the resolved list, or given as
  written with a note.

Found by the second v1 fix round's review, 17 November 2026.
