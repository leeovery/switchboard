# Runway drops a hold shorter than a column

Found on 21 October 2026, reviewing the fluid pass's Runway labels. It isn't the labels'
doing.

## What happens

Runway draws each account's lane a column at a time. `views.Timeline.Moment` judges each column
at its middle. A stretch shorter than a column can fall between two middles, and then it isn't
drawn at all: no fill, no `─`, and no words for it under the lane.

The case that showed it: `runway-ten-accounts-160`, lane `6 lab`. The lane has a 50-minute hold,
"runs out ~16:05 at its pace · back 16:55". At 160 columns the week's lanes are 144 wide, 70
minutes a column, and the hold vanishes from both the lane and its words. At 146 columns it
happened to be sampled, so it showed.

## Why it matters

Accurate data comes second only to routing. A cap or a limit that lasts under a column is real,
but whether it shows depends on the terminal's width. On the phone a week column is about 38
minutes at 52 columns and more at 73, so short holds are likelier to vanish there.

## Ways it could go

- A column takes the worst state anywhere in its span, not the state at its middle. A short hold
  then always shows, at least a column wide, at the cost of drawing it longer than it is.
- The words under a lane name the next hold from the timeline itself, not from the columns drawn.
  The words then never lose it, even where the lane can't show it.
- Both of these.

Not decided; to discuss.
