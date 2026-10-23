# The Overview says less at some taller heights

Found on 23 October 2026, in the release review after the fluid layout pass. It was so in v0.2.6
too: the pass didn't bring it, and didn't take it away.

## What happens

Down the height, the Overview's sections take the rows by their own rules, and two of those rules
give back rows another section had:

- **ROUTING's traces** take two rows where the frame has them, and one where it doesn't, and the
  switch is made more than once as the frame grows: two rows at 70 to 74, one at 75 to 79, two
  again from 80, with the sample at 160 wide.
- **USAGE's blank rows between accounts** come back at 32 rows, taking a row COMING UP had.

So COMING UP and LATELY, which take what's left, go up a row at a time and then drop back. With
the sample at 160 wide, they show 3 to 7 rows, then 3 again at 37, 42 and 47.

## Why it matters

The design's rule is that a taller frame never says less within one arrangement, and a form
switches once. Down the Overview's height, neither holds. Resizing a terminal by a row can take a
line of COMING UP away.

## Ways it could go

- **Rank the sections' growth:** COMING UP and LATELY take their rows before ROUTING's traces go
  to two and USAGE's blank rows come back. The others then grow only from what those two leave,
  so their lines never go.
- **One switch for each section's form down the height,** where its least fits, as across the
  width: the traces' second row from one height, never toggling.
- Leave it, as v0.2.6 has it.

Not decided; to discuss.
