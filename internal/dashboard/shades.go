package dashboard

import (
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/theme"
)

// shadeLevels are how many levels a day is shaded at: level 0, a day with
// nothing on it, then the active days' quarters.
const shadeLevels = 5

// shadeGlyphs tell a day's level by their density, from level 0 to level 4,
// where the look draws no shades of its own, as the terminal theme and
// NO_COLOR don't.
var shadeGlyphs = [shadeLevels]string{"·", "░", "▒", "▓", "█"}

// familyHues are the families' hues, by their names as the version table
// gives them, in its order: Opus, Sonnet, Fable and Haiku.
var familyHues = map[string]theme.Token{
	"opus":   theme.VizFamily1,
	"sonnet": theme.VizFamily2,
	"fable":  theme.VizFamily3,
	"haiku":  theme.VizFamily4,
}

// dayInk is the ink a day is drawn in at its level, 0 to 4: viz.day.0, a day
// with nothing on it, to viz.day.4, the busiest quarter of the active days.
func dayInk(level int) ink {
	return ink{token: theme.VizDay0 + theme.Token(shadeLevel(level))}
}

// stepInk is the ink a day is drawn in at its level, 0 to 4, where it takes
// h in four steps, as a family's strips and the Year filtered to a version
// do: viz.day.0 at level 0, then h's steps, the third h itself.
func stepInk(h hue, level int) ink {
	if shadeLevel(level) == 0 {
		return dayInk(0)
	}
	return ink{token: h.token}
}

// familyHue is the hue of the family with the given name, as the version
// table gives it: viz.family.1 to viz.family.4, Opus's to Haiku's; or
// text.muted, for a family with no hue of its own, as Mythos, or one the
// table doesn't know.
func familyHue(family string) hue {
	if tok, ok := familyHues[family]; ok {
		return hue{token: tok}
	}
	return hue{token: theme.TextMuted}
}

// versionHue is the shade the version is drawn in: the one of viz.version.1
// to viz.version.6 the version table gives it; else its family's hue.
func versionHue(v ledger.Version) hue {
	if v.Shade > 0 {
		return hue{token: theme.VizVersion1 + theme.Token(v.Shade-1)}
	}
	return familyHue(v.Family)
}

// shadeGlyph is the glyph that tells a day's level, 0 to 4, by its density,
// where the look draws no shades of its own: ·, ░, ▒, ▓ and █.
func shadeGlyph(level int) string {
	return shadeGlyphs[shadeLevel(level)]
}

// shadeLevel is level as one a day is shaded at, from 0 to 4.
func shadeLevel(level int) int {
	return min(max(level, 0), shadeLevels-1)
}
