# Account labels and config edits

How `switchboard accounts` tidies and refuses a label, and how it writes the config. For the
command itself, see [`switchboard accounts`](../commands/accounts.md).

## How a label is tidied

A label is saved as it shows. Before `add` or `edit` saves a label, it tidies it the same way
the dashboard and the commands tidy text before they show it:

- It drops characters that Unicode draws as nothing, such as zero-width spaces, joiners, variation
  selectors and tags.
- It turns each run of whitespace into one space.
- It trims whitespace and direction marks from both ends.

It keeps such a character if it makes an emoji or a flag, England's, Scotland's or Wales's among
them. It also keeps one that shapes a joining script, such as Persian or Devanagari. Every other
character stays.

Direction marks inside the label stay only if the label holds a right-to-left letter, such as
Hebrew or Arabic. In any other label, a direction mark changes nothing you can see, so it's
dropped.

A label you write into the config by hand is read as written. The dashboard and the commands show
it tidied the same way. If nothing of it shows once tidied, such as a label of only a direction
mark, they show the account's id instead.

A variation selector stays only in these places:

- after a symbol, as in ❤️;
- after non-ASCII punctuation;
- after a letter outside Latin, Greek and Cyrillic;
- in a keycap, as in 1️⃣.

The tags after a black flag that name no other flag with it are dropped.

## Labels that are refused

`add` and `edit --label` refuse a label that:

- looks like a token, checked before `add` asks for the token;
- is over 100 characters as shown;
- holds a control character, such as a newline or a tab;
- holds a bidirectional embedding, override or isolate, which could reorder the row it shows in;
- holds a sign that joins onto the next character, such as an Arabic number sign, which can hide
  that character;
- shows as nothing, such as only spaces or zero-width characters;
- isn't UTF-8 text.

`--label ""` is refused too, because every account has a label. Two accounts can't show the same
label, ignoring case. Private-use glyphs, such as Nerd Font icons, show as they are.

## Labels written by hand

`edit` says a label as the config holds it.

- **A character `add` would drop,** a bidirectional control or a direction mark shows as its
  escape, such as `\u200B`.
- **A sign that joins the next character,** such as an Arabic number sign, also shows as its
  escape, such as `\u0600`. Otherwise it could hide the character after it.
- **What `add` keeps** in an emoji, a keycap, a variant, a flag or a joining script shows as it is,
  unless an escape stands before it.
- **A label written untidily** in the config and given tidily is saved tidily, and `edit` says the
  old one was untidy.

## How `edit` writes the config

`accounts` and `pool` edit the config as text, keeping its comments and layout. A config that's a
link is written where the link leads.

- **A value `edit` replaces** keeps the comment beside it.
- **A key it takes out** goes with its comment, for example the primary mark when another account
  becomes the primary.
- **A label written over several lines,** as a multi-line string, can't be edited as text. A
  label edit of that account is refused, and nothing is written. A plan, reserve or primary edit
  still works: a new key goes below the line that closes the label.

## The order of errors

`edit` reports errors in the flags themselves first, such as a flag with no value or an unknown
flag. Then it reports an id that no account has. Then it reports anything wrong with a value.

## Related

- [`switchboard accounts`](../commands/accounts.md): the command that writes the labels.
- [Tokens](tokens.md): the token files and their dates.
- [The accounts record](accounts-record.md): every account this Mac has had.
