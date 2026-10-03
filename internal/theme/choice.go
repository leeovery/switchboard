package theme

import "cmp"

// Choice is the themes the dashboard is drawn in, as the preferences file
// keeps it: one theme, whatever the terminal's background, or a pair, one
// for a light background and one for a dark. A half of the pair it doesn't
// name is its default, so the zero Choice is the default pair.
type Choice struct {
	// Theme names the one theme: "" for a pair.
	Theme string `json:"theme,omitempty"`
	// Light and Dark name the pair's halves, where the choice names them.
	Light string `json:"theme_light,omitempty"`
	Dark  string `json:"theme_dark,omitempty"`
}

// One is the choice of the theme slug names alone.
func One(slug string) Choice {
	return Choice{Theme: slug}
}

// IsOne reports whether the choice is one theme rather than a pair.
func (c Choice) IsOne() bool {
	return c.Theme != ""
}

// WithHalf is the choice with slug as the pair's dark half, or its light,
// the other half kept and the one theme cleared.
func (c Choice) WithHalf(dark bool, slug string) Choice {
	if dark {
		return Choice{Light: c.Light, Dark: slug}
	}
	return Choice{Light: slug, Dark: c.Dark}
}

// For is the slug of the theme the choice draws a terminal in, by whether
// its background is dark.
func (c Choice) For(dark bool) string {
	switch {
	case c.IsOne():
		return c.Theme
	case dark:
		return cmp.Or(c.Dark, DefaultDark)
	default:
		return cmp.Or(c.Light, DefaultLight)
	}
}

// Badge says what the theme slug names fills in the choice: ● the one theme,
// ● light or ● dark a half of the pair, ● both its halves; "" for none.
func (c Choice) Badge(slug string) string {
	light, dark := c.For(false) == slug, c.For(true) == slug
	switch {
	case c.IsOne() && light:
		return "●"
	case light && dark:
		return "● both"
	case light:
		return "● light"
	case dark:
		return "● dark"
	default:
		return ""
	}
}

// Named are the slugs the choice names itself, rather than defaults: its one
// theme, or the halves of its pair it names, each once.
func (c Choice) Named() []string {
	if c.IsOne() {
		return []string{c.Theme}
	}
	var named []string
	if c.Light != "" {
		named = append(named, c.Light)
	}
	if c.Dark != "" && c.Dark != c.Light {
		named = append(named, c.Dark)
	}
	return named
}

// Pair is the themes a choice draws in: one for a light terminal and one for
// a dark, the same twice for one theme.
type Pair struct {
	Light, Dark Theme
}

// For is the pair's theme for a terminal whose background is dark, or not.
func (p Pair) For(dark bool) Theme {
	if dark {
		return p.Dark
	}
	return p.Light
}

// pair is the pair the choice draws in, each theme as load gives it, which
// reports false for one that doesn't load: its half's default, a built-in,
// stands in, and for one theme, the default pair.
func (c Choice) pair(load func(slug string) (Theme, bool)) Pair {
	p := Pair{Light: builtins[DefaultLight], Dark: builtins[DefaultDark]}
	if c.IsOne() {
		if t, ok := load(c.Theme); ok {
			return Pair{Light: t, Dark: t}
		}
		return p
	}
	if t, ok := load(c.For(false)); ok {
		p.Light = t
	}
	if t, ok := load(c.For(true)); ok {
		p.Dark = t
	}
	return p
}
