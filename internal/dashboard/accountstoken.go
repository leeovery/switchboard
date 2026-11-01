package dashboard

import (
	"slices"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// stateWordsAt are the cells from a state's glyph to its words, as
// standingLead and stateWord lay them, where Accounts' token line aligns.
const stateWordsAt = 2

// tokenNoteParts are what's said of a token's year ending, as Accounts draws
// it, of a note warning at the days given: its words in text.muted; what
// warns, as warnedInk colours it, and the command, in text.secondary, each a
// figure, never broken or cut part-way, as "2…" would read as fewer days
// left.
func tokenNoteParts(said []prose.TokenPart, warning int) []part {
	parts := make([]part, len(said))
	for i, p := range said {
		switch p.Kind {
		case prose.TokenWarning:
			parts[i] = figurePart(line{{p.Text, warnedInk(warning)}}, Number{})
		case prose.TokenCommand:
			parts[i] = figurePart(line{{p.Text, secondaryInk}}, Number{})
		default:
			parts[i] = wordsPart(line{{p.Text, mutedInk}})
		}
	}
	return parts
}

// warnedInk is the ink of a token's days left, or that its year's up, of a
// warning at the days given: bold state.destructive at its last warning's,
// as status.TokenYear.Warned has them, else bold accent.attention.
func warnedInk(warning int) ink {
	if warning == status.TokenLastWarning {
		return exhaustedInk
	}
	return alertInk
}

// tokenNoteLines are the lines Detail tells an account's token's year ending
// in, as views.TokenNote has it, wrapped width cells wide, as wrapParts
// wraps them: none where it tells of none.
func tokenNoteLines(note views.TokenNote, noted bool, width int) []line {
	if !noted {
		return nil
	}
	return wrapParts(width, tokenNoteParts(note.Told, note.Warning)...)
}

// tokenShortForms are the forms Compare says an account's token's year
// ending in, as views.TokenNote has it short, each narrower than the one
// before: whole, then what warns alone.
func tokenShortForms(note views.TokenNote) partForms {
	warns := slices.IndexFunc(note.Short, func(p prose.TokenPart) bool { return p.Kind == prose.TokenWarning })
	return partForms{tokenNoteParts(note.Short, note.Warning), tokenNoteParts(note.Short[warns:warns+1], note.Warning)}
}
