# `history --windows --json` names the pool it read

`switchboard history --json` and `switchboard sessions --json` say which pool they read:

```json
{"schema": 1, "pool": "b2n8", "pool_label": "Default", "prices_as_of": "2026-11-17", "days": […]}
```

`switchboard history --windows --json` doesn't. An agent reading it can't tell which pool's
accounts the windows belong to without asking again.

## What it could do

- **Name the pool:** the windows' JSON carries `pool` and `pool_label`, as `history --json` does,
  from the same place, so the three verbs say it alike.
- **Document it:** the two fields go in the reference page for `history --windows --json`.

## Costs and limits

- It adds two fields to a v1 format. An agent that reads the document as it is carries on, since
  nothing is taken away.

Found by the second v1 fix round's review, 17 November 2026.
