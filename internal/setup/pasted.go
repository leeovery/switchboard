package setup

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/tokens"
)

// Why a paste gives no token setup saves, as the paste card says it, never
// quoting what was pasted.
var (
	errNothingPasted = errors.New("nothing's pasted: paste the token claude setup-token created")
	errMoreThanToken = errors.New("what's pasted holds more than the token: paste the token alone, as claude setup-token showed it")
)

// pastedToken is the token pasted holds. It fails, with setup's words,
// where nothing's pasted but whitespace and box-drawing characters, and
// where the token in it isn't told from the rest. Pasted text with no
// box-drawing character in it is a token typed or copied whole, as
// fromText takes it; with one, it was copied off setup's screen, the token
// card's edges and gutter round it, as fromCard takes it. Each holds
// whether its line breaks reach it or not, as the screen's field leaves
// them out.
func pastedToken(pasted string) (tokens.Token, error) {
	if strings.TrimFunc(pasted, func(r rune) bool { return unicode.IsSpace(r) || boxDrawing(r) }) == "" {
		return tokens.Token{}, errNothingPasted
	}
	joined, ok := fromText(pasted)
	if strings.ContainsFunc(pasted, boxDrawing) {
		joined, ok = fromCard(pasted)
	}
	if !ok || strings.Count(joined, redact.TokenPrefix) > 1 {
		return tokens.Token{}, errMoreThanToken
	}
	token, err := tokens.Parse(joined)
	if err != nil {
		return tokens.Token{}, errMoreThanToken
	}
	return token, nil
}

// fromText is the token in text with no box-drawing character in it: its
// one word, whatever its shape, as accounts add takes one typed; or its
// words, broken by whitespace, joined, where they make one setup token
// whole, as claude.IsSetupToken has it, as nothing else tells a token's
// parts from the words round it. It reports false for anything else.
func fromText(text string) (string, bool) {
	words := strings.Fields(text)
	joined := strings.Join(words, "")
	return joined, len(words) == 1 || claude.IsSetupToken(joined)
}

// fromCard is the token in text copied off the token card, where it's
// wrapped onto lines, each part after the card's │ gutter, the card's edges
// either side, and the page beside the card past them. Box-drawing
// characters split the text into segments. A segment with whitespace
// inside it, once trimmed, is the page's or the card's own words, and is
// left out. Where a blank segment stands between others, the text was
// copied a line at a time, across the card's edges and the blank between
// its left edge and the gutter: then a part is the segment the token starts
// in, or one right after a blank, and every other is the page's, as a word
// beside the card. Otherwise, as a block copied from the gutter on, every
// segment left is a part. The token is its parts joined, where they make
// one setup token whole: it reports false for anything else.
func fromCard(text string) (string, bool) {
	segments := splitAt(text, boxDrawing)
	lineWise := blankBetween(segments)
	var parts []string
	for i, segment := range segments {
		word := strings.TrimSpace(segment)
		if word == "" || strings.ContainsFunc(word, unicode.IsSpace) {
			continue
		}
		if !lineWise || strings.HasPrefix(word, redact.TokenPrefix) || i > 0 && blank(segments[i-1]) {
			parts = append(parts, word)
		}
	}
	joined := strings.Join(parts, "")
	return joined, claude.IsSetupToken(joined)
}

// splitAt is text split at each rune at which splits, the empty segments
// between two of them, or at either end, kept.
func splitAt(text string, at func(rune) bool) []string {
	var segments []string
	from := 0
	for i, r := range text {
		if at(r) {
			segments = append(segments, text[from:i])
			from = i + utf8.RuneLen(r)
		}
	}
	return append(segments, text[from:])
}

// blankBetween reports whether a blank segment stands between two that
// aren't.
func blankBetween(segments []string) bool {
	seen, gap := false, false
	for _, segment := range segments {
		switch {
		case !blank(segment) && gap:
			return true
		case !blank(segment):
			seen = true
		case seen:
			gap = true
		}
	}
	return false
}

// blank reports whether segment holds nothing but whitespace.
func blank(segment string) bool {
	return strings.TrimSpace(segment) == ""
}

// boxDrawing reports whether r is a box-drawing character, as setup's cards
// draw their edges, and the gutter before what setup-token showed, with.
func boxDrawing(r rune) bool {
	return r >= '─' && r <= '╿'
}

// pastedUnanswered is why a pasted token the API didn't answer for, as why
// says, isn't saved: a token cut short, as a copy of its first row alone, has
// a setup token's shape, and saved unchecked would take a working token's
// place.
func pastedUnanswered(why string) string {
	return "the API didn't answer (" + why + "), so it isn't saved: paste it again in a moment"
}

// pastedRefused is why a pasted token the API refused, as err says, isn't
// saved: the refusal's own why, where it gives one, and to check it was
// copied whole, as a bad copy is likelier than a token gone bad.
func pastedRefused(err error) string {
	said := accounts.ErrRefused.Error()
	if refused, ok := errors.AsType[*accounts.RefusedError](err); ok {
		said += " (" + refused.Why + ")"
	}
	return said + ", so it isn't saved: check it was copied whole"
}
