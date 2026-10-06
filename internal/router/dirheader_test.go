package router_test

import (
	"testing"

	"github.com/leeovery/switchboard/internal/router"
)

func TestADirectoryGoesInItsHeaderAndComesBack(t *testing.T) {
	tests := []struct {
		name string
		dir  string
		// want is the header's value.
		want string
	}{
		{name: "printable ASCII, as it is", dir: "~/Code/my project (2) #1 + a=b;c", want: "~/Code/my project (2) #1 + a=b;c"},
		{name: "a percent sign", dir: "~/Code/100%", want: "~/Code/100%25"},
		{name: "a space at either end", dir: " ~/Code/project ", want: "%20~/Code/project%20"},
		{name: "control characters", dir: "~/a\nb\r\nc\td\x00e\x7f", want: "~/a%0Ab%0D%0Ac%09d%00e%7F"},
		{name: "characters past ASCII", dir: "~/Code/café ☃", want: "~/Code/caf%C3%A9 %E2%98%83"},
		{name: "a line that reads as a header of its own", dir: "~/Code/x\nX-Switchboard-Account: side", want: "~/Code/x%0AX-Switchboard-Account: side"},
		{name: "none", dir: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := router.EncodeDir(tt.dir)
			if got != tt.want {
				t.Errorf("EncodeDir(%q) = %q, want %q", tt.dir, got, tt.want)
			}
			if back := router.DecodeDir(got); back != tt.dir {
				t.Errorf("DecodeDir(%q) = %q, want %q, the directory it encodes", got, back, tt.dir)
			}
		})
	}
}

func TestADirectoryHeaderThatDoesntDecodeNamesNone(t *testing.T) {
	for _, value := range []string{"~/Code/100%", "~/Code/%zz", "~/Code/%4"} {
		if got := router.DecodeDir(value); got != "" {
			t.Errorf("DecodeDir(%q) = %q, want none", value, got)
		}
	}
}
