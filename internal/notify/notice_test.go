package notify

import (
	"testing"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

func TestNotices(t *testing.T) {
	work := status.Account{ID: "work", Label: "Work"}
	tests := []struct {
		name string
		got  Notice
		want Notice
	}{
		{
			name: "room again",
			got:  RoomAgain(work),
			want: Notice{Account: "work", News: "room again", Message: "work · Work has room again"},
		},
		{
			name: "a warning",
			got:  Warning(work, quota.Window{Key: "7d", Label: "Week", Utilization: 0.912}),
			want: Notice{Account: "work", News: "Week at 91%", Message: "work · Work: Week at 91%"},
		},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s: notice = %+v, want %+v", tt.name, tt.got, tt.want)
		}
	}
}

func TestPassed(t *testing.T) {
	tests := []struct {
		name                   string
		before, after, warning float64
		want                   bool
	}{
		{name: "passing it", before: 0.85, after: 0.91, warning: 0.9, want: true},
		{name: "reaching it", before: 0.85, after: 0.9, warning: 0.9, want: true},
		{name: "short of it", before: 0.85, after: 0.89, warning: 0.9},
		{name: "past it already", before: 0.9, after: 0.95, warning: 0.9},
		{name: "falling back below it", before: 0.95, after: 0.5, warning: 0.9},
		{name: "a warning of none", before: 0, after: 1, warning: 0},
	}
	for _, tt := range tests {
		if got := Passed(tt.before, tt.after, tt.warning); got != tt.want {
			t.Errorf("%s: Passed(%v, %v, %v) = %v, want %v", tt.name, tt.before, tt.after, tt.warning, got, tt.want)
		}
	}
}
