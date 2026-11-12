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
		{name: "an array on one line, keeping the comment beside it", line: `dirs  = ["~/a", "/b"]  # c`, want: `dirs  = "W"  # c`},
		{name: "an array holding a bracket and a hash in its strings", line: `dirs = ["/a]#", '/b]'] # c`, want: `dirs = "W" # c`},
		{name: "an array of arrays", line: `dirs = [["/a"], []]  # c`, want: `dirs = "W"  # c`},
		{name: "an array going on past the line, set alone", line: `dirs = [  # c`, want: `dirs = "W"`},
		{name: "an array whose string goes on past the line, set alone but for its CR", line: "dirs = [\"\"\"/a\r", want: "dirs = \"W\"\r"},
		{name: "an inline table, set alone", line: `label = {a = 1}  # c`, want: `label = "W"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := withValue(tt.line, `"W"`); got != tt.want {
				t.Errorf("withValue(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}
