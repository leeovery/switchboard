# Compare's all-accounts column adds up to its columns

Accounts' Compare gives each account a column of its own, beside an all-accounts column. The
all-accounts column counts the whole pool's work over the period. That includes an account moved to
another pool or removed since, which has no column of its own. So the all-accounts column can
differ from the sum of the columns, with nothing on the page to say why.

`switchboard history` and Accounts' Tokens give such an account a row of its own, so their totals
add up.

## What it could do

- **Show the account:** a column for an account moved or removed, named as `history` names it,
  so the columns add up to the all-accounts column.
- **Or say what's left out:** a line under the table, such as `includes 1 account no longer here`,
  so the gap is explained.

## Costs and limits

- A column more takes width Compare may not have on a phone, so the line may suit the narrow form.
- Either wants a design pass and Paper before it's built.

Found by the second v1 fix round's review, 17 November 2026.
