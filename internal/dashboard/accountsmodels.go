package dashboard

import (
	"cmp"
	"slices"
	"strconv"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/views"
)

// Accounts' BY MODEL, Compare's and Detail's alike: a row's use of each
// version the period used, as a stacked bar, a part a version in its shade,
// and the versions it used named with their shares.

// versionsApart sets apart the versions Detail's BY MODEL names.
const versionsApart = "  "

// versionHues are the versions' shades, in their order.
func versionHues(versions []ledger.Version) []hue {
	hues := make([]hue, len(versions))
	for i, v := range versions {
		hues[i] = versionHue(v)
	}
	return hues
}

// shareAmounts are what the row's use of each version given was worth, in
// their order, for its stacked bar: nothing of one it didn't use, or whose
// worth isn't priced.
func shareAmounts(row views.AccountsRow, versions []ledger.Version) []ledger.Picodollars {
	amounts := make([]ledger.Picodollars, len(versions))
	for i, v := range versions {
		if s, ok := shareOf(row, v); ok {
			amounts[i] = amountOf(s.Worth)
		}
	}
	return amounts
}

// versionUsed is a version a row used, and its share of the row's use, nil
// where it isn't known.
type versionUsed struct {
	version ledger.Version
	share   *float64
}

// versionsUsed are the versions of those given that the row used, in their
// order, each with its share.
func versionsUsed(row views.AccountsRow, versions []ledger.Version) []versionUsed {
	var used []versionUsed
	for _, v := range versions {
		if s, ok := shareOf(row, v); ok {
			used = append(used, versionUsed{version: v, share: s.Share})
		}
	}
	return used
}

// versionShares are the versions used given, in their order, as Detail's BY
// MODEL names them in width cells, as versionsNamed has them. Where they
// don't all fit, the smallest are left out, one whose share isn't known
// first, as many as it takes for the rest, and how many more there are, to
// fit.
func versionShares(used []versionUsed, width int) line {
	smallest := make([]int, len(used))
	for i := range smallest {
		smallest[i] = i
	}
	slices.SortStableFunc(smallest, func(a, b int) int { return cmp.Compare(shareSize(used[a].share), shareSize(used[b].share)) })
	for out := range len(used) + 1 {
		var kept []versionUsed
		for i, u := range used {
			if !slices.Contains(smallest[:out], i) {
				kept = append(kept, u)
			}
		}
		if parts := versionsNamed(kept, out); partsWidth(parts) <= width {
			return fitParts(width, parts...)
		}
	}
	return nil
}

// shareSize is a share's size, to leave out the smallest first: one not
// known smaller than any.
func shareSize(share *float64) float64 {
	if share == nil {
		return -1
	}
	return *share
}

// versionsNamed are the versions given, each its ● in its shade, then its
// name, in text.secondary, and its share, as shareParts says it, as "● Opus
// 5.5 55%", versionsApart between them; then, where more is any, how many
// more there are, as "+2 more", in text.subtle.
func versionsNamed(used []versionUsed, more int) []part {
	var parts []part
	for _, u := range used {
		if parts != nil {
			parts = append(parts, wordsIn(versionsApart, ink{}))
		}
		named := line{{"●", versionHue(u.version).ink()}, {" " + prose.Clean(u.version.Name) + " ", secondaryInk}}
		parts = append(append(parts, wordsPart(named)), shareParts(u.share, secondaryInk)...)
	}
	if more > 0 {
		if parts != nil {
			parts = append(parts, wordsIn(versionsApart, ink{}))
		}
		parts = append(parts, wordsIn("+"+strconv.Itoa(more)+" more", dimInk))
	}
	return parts
}
