package setup_test

import (
	"slices"
	"strings"
	"testing"
)

func TestThePrimingStep(t *testing.T) {
	// dayless is doneConfig without priming, as a config from before setup.
	dayless := strings.TrimSuffix(doneConfig, "\n[prime]\nday = \"08:00-23:00\"\n")
	const explained = "Priming starts each account's 5-hour window on a schedule, so their resets are spread through your day rather than coming together. It's optional.\n"
	const question = "Your day, HH:MM-HH:MM, such as 08:00-23:00 (Enter to leave priming off): "
	tests := []struct {
		name    string
		config  string
		answers []string
		want    string
		// wantConfig is the config setup leaves.
		wantConfig string
		wantRan    []string
	}{
		{
			name:       "on",
			config:     doneConfig,
			answers:    []string{""},
			want:       "Priming is on, over 08:00-23:00.\n",
			wantConfig: doneConfig,
			wantRan:    untouched,
		},
		{
			name:       "off, and left off",
			config:     dayless,
			answers:    []string{"", ""},
			want:       explained + question + "\nPriming stays off.\n",
			wantConfig: dayless,
			wantRan:    untouched,
		},
		{
			name:       "off, and turned on",
			config:     dayless,
			answers:    []string{"", "07:30-22:00"},
			want:       explained + question + "07:30-22:00\nPriming is on, over 07:30-22:00.\n",
			wantConfig: dayless + "\n[prime]\nday = \"07:30-22:00\"\n",
			wantRan:    restarted,
		},
		{
			name:       "off, and turned on past midnight",
			config:     dayless,
			answers:    []string{"", "22:00-02:00"},
			want:       explained + question + "22:00-02:00\nPriming is on, over 22:00-02:00.\n",
			wantConfig: dayless + "\n[prime]\nday = \"22:00-02:00\"\n",
			wantRan:    restarted,
		},
		{
			name:    "off, asked again until the day is one",
			config:  dayless,
			answers: []string{"", "7-22", "08:00-08:00", "07:30-22:00"},
			want: explained + question + "7-22\n" +
				"prime.day \"7-22\": must be two times of day, HH:MM, joined by -, such as 08:00-23:00.\n" +
				question + "08:00-08:00\n" +
				"prime.day \"08:00-08:00\": must end at another time than it starts; an end before the start is past midnight.\n" +
				question + "07:30-22:00\nPriming is on, over 07:30-22:00.\n",
			wantConfig: dayless + "\n[prime]\nday = \"07:30-22:00\"\n",
			wantRan:    restarted,
		},
		{
			name:       "off, with a [prime] table giving no day, kept as it's laid out",
			config:     dayless + "\n# The windows' day.\n[prime]\nday = \"\"  # local time\n",
			answers:    []string{"", "07:30-22:00"},
			want:       explained + question + "07:30-22:00\nPriming is on, over 07:30-22:00.\n",
			wantConfig: dayless + "\n# The windows' day.\n[prime]\nday = \"07:30-22:00\"  # local time\n",
			wantRan:    restarted,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			w.writeConfig(t, tt.config)

			shown := w.runs(t, tt.answers...)
			if got := section(t, shown, "2. Priming"); got != tt.want {
				t.Errorf("the priming step showed\n%s\nwant\n%s", got, tt.want)
			}
			if config := w.readConfig(t); config != tt.wantConfig {
				t.Errorf("the config reads\n%s\nwant\n%s", config, tt.wantConfig)
			}
			if ran := w.launchd.ran(); !slices.Equal(ran, tt.wantRan) {
				t.Errorf("ran launchctl %q, want %q", ran, tt.wantRan)
			}
		})
	}
}
