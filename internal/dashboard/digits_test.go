package dashboard

import (
	"slices"
	"testing"
	"time"
)

func TestBigDigitsAreThreeCellsTallInHalfBlocks(t *testing.T) {
	tests := []struct {
		text    string
		want    []string
		wantEnd int
	}{
		{text: "58%", want: []string{"█▀▀ █▀█ ▀ █", "▀▀█ █▀█ ▄▀", "▀▀▀ ▀▀▀ ▀ ▀"}, wantEnd: 11},
		{text: "1:12", want: []string{"▄█  ▄ ▄█  ▀▀█", " █  ▄  █  █▀▀", "▀▀▀   ▀▀▀ ▀▀▀"}, wantEnd: 13},
		{text: "100%", want: []string{"▄█  █▀█ █▀█ ▀ █", " █  █ █ █ █ ▄▀", "▀▀▀ ▀▀▀ ▀▀▀ ▀ ▀"}, wantEnd: 15},
		{text: "07:42", want: []string{"█▀█ ▀▀█ ▄ █ █ ▀▀█", "█ █   █ ▄ ▀▀█ █▀▀", "▀▀▀   ▀     ▀ ▀▀▀"}, wantEnd: 17},
		{text: "x", want: []string{"", "", ""}, wantEnd: 0},
	}
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			c := newCanvas(20, bigRows)
			end := big(c, 0, 0, tt.text, ink{})
			if got := c.rows(Look{}); !slices.Equal(got, tt.want) {
				t.Errorf("big(%q) drew\n%q\nwant\n%q", tt.text, got, tt.want)
			}
			if end != tt.wantEnd {
				t.Errorf("big(%q) = %d, want the column after its last character, %d", tt.text, end, tt.wantEnd)
			}
		})
	}
}

func TestATimerReadsHoursAndMinutesTillItsLastTen(t *testing.T) {
	tests := []struct {
		left time.Duration
		want string
	}{
		{left: time.Hour + 11*time.Minute + 53*time.Second, want: "1:11"},
		{left: 4*time.Hour + 59*time.Minute, want: "4:59"},
		{left: 10 * time.Minute, want: "0:10"},
		{left: 7*time.Minute + 42*time.Second, want: "07:42"},
		{left: 3 * time.Second, want: "00:03"},
		{left: 0, want: "0:00"},
		{left: -time.Minute, want: "0:00"},
	}
	for _, tt := range tests {
		if got := timer(now, now.Add(tt.left)); got != tt.want {
			t.Errorf("timer() with %v left = %q, want %q", tt.left, got, tt.want)
		}
	}
}
