package config

import "testing"

func TestWithValue(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{name: "a string, keeping the comment beside it", line: `label = "Work"  # c`, want: `label = "W"  # c`},
		{name: "a bare value, keeping its CR", line: "reserve = 0.1\r", want: "reserve = \"W\"\r"},
		{name: "an array, set alone", line: `label = ["Work"]  # c`, want: `label = "W"`},
		{name: "an array, set alone but for its CR", line: "label = [\"Work\"]  # c\r", want: "label = \"W\"\r"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := withValue(tt.line, `"W"`); got != tt.want {
				t.Errorf("withValue(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}
