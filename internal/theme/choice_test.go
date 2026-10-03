package theme_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/theme"
)

func TestAChoiceDrawsATerminalByItsBackground(t *testing.T) {
	tests := []struct {
		name            string
		choice          theme.Choice
		wantLight, want string
	}{
		{name: "nothing chosen: the default pair", choice: theme.Choice{}, wantLight: "tokyo-night-day", want: "nord"},
		{name: "one theme, whatever the background", choice: theme.One("amber"), wantLight: "amber", want: "amber"},
		{name: "a pair", choice: theme.Choice{Light: "exchange", Dark: "amber"}, wantLight: "exchange", want: "amber"},
		{name: "a pair's light half alone, the dark its default", choice: theme.Choice{Light: "exchange"}, wantLight: "exchange", want: "nord"},
		{name: "a pair's dark half alone, the light its default", choice: theme.Choice{Dark: "amber"}, wantLight: "tokyo-night-day", want: "amber"},
		{name: "one theme over a pair it was given too", choice: theme.Choice{Theme: "terminal", Light: "exchange", Dark: "amber"}, wantLight: "terminal", want: "terminal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.choice.For(false); got != tt.wantLight {
				t.Errorf("For(a light background) = %s, want %s", got, tt.wantLight)
			}
			if got := tt.choice.For(true); got != tt.want {
				t.Errorf("For(a dark background) = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestATerminalThatDoesntAnswerIsDrawnAsADarkOne(t *testing.T) {
	if got := (theme.Choice{}).For(theme.Dark(nil)); got != theme.DefaultDark {
		t.Errorf("a terminal that didn't give its background is drawn in %s, want the dark half, %s", got, theme.DefaultDark)
	}
}

func TestSettingAThemeOrAHalfOfThePair(t *testing.T) {
	tests := []struct {
		name string
		got  theme.Choice
		want theme.Choice
	}{
		{name: "one theme clears the pair", got: theme.One("amber"), want: theme.Choice{Theme: "amber"}},
		{name: "the dark half clears the one theme", got: theme.One("amber").WithHalf(true, "exchange"), want: theme.Choice{Dark: "exchange"}},
		{name: "the light half clears the one theme", got: theme.One("amber").WithHalf(false, "exchange"), want: theme.Choice{Light: "exchange"}},
		{name: "a half keeps the other", got: theme.Choice{Light: "terminal"}.WithHalf(true, "amber"), want: theme.Choice{Light: "terminal", Dark: "amber"}},
		{name: "a half replaces itself", got: theme.Choice{Light: "terminal", Dark: "nord"}.WithHalf(false, "exchange"), want: theme.Choice{Light: "exchange", Dark: "nord"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("choice = %+v, want %+v", tt.got, tt.want)
			}
		})
	}
}

func TestBadgesSayWhatEachThemeFills(t *testing.T) {
	tests := []struct {
		name   string
		choice theme.Choice
		want   map[string]string
	}{
		{name: "one theme", choice: theme.One("amber"), want: map[string]string{"amber": "●"}},
		{name: "the default pair", choice: theme.Choice{}, want: map[string]string{"tokyo-night-day": "● light", "nord": "● dark"}},
		{name: "a pair", choice: theme.Choice{Light: "exchange", Dark: "amber"}, want: map[string]string{"exchange": "● light", "amber": "● dark"}},
		{name: "a theme both halves", choice: theme.Choice{Light: "amber", Dark: "amber"}, want: map[string]string{"amber": "● both"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, b := range theme.Builtins() {
				if got, want := tt.choice.Badge(b.Slug), tt.want[b.Slug]; got != want {
					t.Errorf("Badge(%s) = %q, want %q", b.Slug, got, want)
				}
			}
		})
	}
}

func TestNamedIsWhatAChoiceNamesItself(t *testing.T) {
	tests := []struct {
		choice theme.Choice
		want   []string
	}{
		{choice: theme.Choice{}},
		{choice: theme.One("lake"), want: []string{"lake"}},
		{choice: theme.Choice{Light: "lake"}, want: []string{"lake"}},
		{choice: theme.Choice{Light: "lake", Dark: "pond"}, want: []string{"lake", "pond"}},
		{choice: theme.Choice{Light: "lake", Dark: "lake"}, want: []string{"lake"}},
		{choice: theme.Choice{Theme: "lake", Dark: "pond"}, want: []string{"lake"}},
	}
	for _, tt := range tests {
		if got := tt.choice.Named(); !slices.Equal(got, tt.want) {
			t.Errorf("%+v.Named() = %q, want %q", tt.choice, got, tt.want)
		}
	}
}

func TestAThemeThatDoesntLoadGivesWayToItsDefault(t *testing.T) {
	log := logstest.Capture(t)
	dir := t.TempDir()
	write(t, filepath.Join(dir, "lake.theme"), file(nil))
	write(t, filepath.Join(dir, "broken.theme"), file([]string{"canvas"}))
	lib := theme.NewLibrary(dir)
	tests := []struct {
		name            string
		choice          theme.Choice
		wantLight, want string
	}{
		{name: "one theme from its file", choice: theme.One("lake"), wantLight: "lake", want: "lake"},
		{name: "one theme that doesn't load: the default pair", choice: theme.One("broken"), wantLight: "tokyo-night-day", want: "nord"},
		{name: "one theme that isn't there: the default pair", choice: theme.One("gone"), wantLight: "tokyo-night-day", want: "nord"},
		{name: "a half that doesn't load: its default", choice: theme.Choice{Light: "broken", Dark: "lake"}, wantLight: "tokyo-night-day", want: "lake"},
		{name: "a half that can't name a theme: its default", choice: theme.Choice{Light: "lake", Dark: "../lake"}, wantLight: "lake", want: "nord"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for source, pair := range map[string]theme.Pair{"loaded": lib.Pair(tt.choice), "listed": lib.List().Pair(tt.choice)} {
				if got := pair.For(false).Slug; got != tt.wantLight {
					t.Errorf("%s, a light terminal is drawn in %s, want %s", source, got, tt.wantLight)
				}
				if got := pair.For(true).Slug; got != tt.want {
					t.Errorf("%s, a dark terminal is drawn in %s, want %s", source, got, tt.want)
				}
			}
		})
	}
	if !log.Has("level=WARN", `msg="a chosen theme doesn't load, so its default stands in"`, "theme=broken", "missing canvas") {
		t.Errorf("the log reads\n%s\nwant it to say the broken theme doesn't load, and why", log)
	}
}
