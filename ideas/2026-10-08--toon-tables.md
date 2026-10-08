# TOON for the data verbs' long tables

The data verbs print JSON wherever their output isn't a terminal, which is what an agent reads.
`requests` and `events` print a JSON object a line, and can run to thousands of lines, every one
repeating its field names; `history` prints one object. TOON, a notation for tables that names each
field once, above rows of values, says the same in fewer tokens, which an agent's context pays for.

It could come as a form beside JSON, never in its place: `--toon` on the verbs whose output is a
long table, JSON staying what each prints by default off a terminal.

To settle with the owner:

- Whether agents read TOON as reliably as JSON, measured on the verbs' own tables.
- Which verbs take it, and whether the skill tells agents of it.

Later, after milestone 7.
