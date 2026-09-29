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
