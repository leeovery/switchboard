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

## Tried, 9 November 2026

Built and set aside. Drawing a column as the worst state in its span is simple, and showed every
short hold. The words were the hard part. Once short holds were drawn, the words had to describe
holds back to back and overlapping, such as a few minutes at a cap and then the limit. Two rounds of
review each found them hiding a worse hold behind a lesser one, or putting one hold's end on another.
Neither build was merged.

If it's picked up again, start from the words:
- Split a lane into stretches of one state that never overlap.
- Join stretches that run back to back into one run, said once, with when room returns.
- Let lesser holds give way before worse ones, but say the hold at now first.
- Judge room from now.
- Test it over generated sequences of holds rather than case by case.
