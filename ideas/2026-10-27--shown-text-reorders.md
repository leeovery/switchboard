# Shown text that reorders or hides itself

Found by the review of `accounts edit`, 27 October 2026.

## What happens

`prose.Clean`, which every view and command uses to put outside text on one line of a terminal,
turns control characters into spaces and nothing more. Bidi embedding, override and isolate
controls, and other characters drawn as nothing, pass through. A label written into the config by
hand, as `‮krow`, reaches `accounts list` and the dashboard raw, and reorders the rest of its
line. So can other text switchboard shows from outside it: a session's working directory, its
title, a model's name.

`accounts edit` already shows a hand-written label through `prose.Quoted`, which escapes these,
and a label given to `add`, `edit` or setup is cleaned as it's taken. What's written by hand, and
what comes from Claude Code, isn't.

## What it might take

`Clean` drops the bidi controls, and the characters `prose.Unseen` names wherever no sequence owns
them, by the rule `takeLabel` follows: one rule for everything shown. Then a guard that no view
draws outside text past `Clean`, and frames redrawn where a fixture holds such text, which none
does today.

Its cost is a pass over each rune of what's shown, on every draw: measure it on the dashboard's
largest views before landing it.
