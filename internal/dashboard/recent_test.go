package dashboard

import (
	"testing"

	"github.com/leeovery/switchboard/internal/status"
)

func TestRECENTSaysWhyThereAreNone(t *testing.T) {
	stuck := probed()
	stuck.Fallback = status.Fallback{Router: status.RouterUnhealthy, Reason: "no answer within 500ms"}
	asked := probed()
	asked.Fallback = status.Fallback{}
	tests := []struct {
		name     string
		doc      status.Document
		outdated bool
		want     string
	}{
		{name: "probing, the router not running", doc: probed(), want: "the router isn't running"},
		{name: "probing, the router not answering as it should", doc: stuck, want: "the router isn't answering"},
		{name: "probing as asked", doc: asked, want: "none while probing, as asked"},
		{name: "a router from before it told of events", doc: threeRouted(), outdated: true, want: "restart the router for recent events"},
		{name: "a router that has told of none", doc: threeRouted(), want: "nothing lately"},
		{name: "nothing read yet", doc: status.Document{}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (Frame{Outdated: tt.outdated}).quiet(tt.doc.At(now, claudeLike)).plain(); got != tt.want {
				t.Errorf("quiet() = %q, want %q", got, tt.want)
			}
		})
	}
}
