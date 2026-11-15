package cli

import "testing"

// An account added says it's the primary: there, as the first in it, of a
// pool named; the primary alone where it's marked so, or no pool's named, as
// of a config's one pool; and nothing where it isn't.
func TestAnAccountAddedSaysItsThePrimaryThereOnlyOfAPoolNamed(t *testing.T) {
	tests := []struct {
		name, in  string
		own, made bool
		want      string
	}{
		{name: "the first in a pool named", in: " in pool Client", made: true, want: ", the primary there, as the first in it"},
		{name: "the first in a pool unnamed", made: true, want: ", the primary"},
		{name: "marked by the caller, in a pool named", in: " in pool Client", own: true, made: true, want: ", the primary"},
		{name: "not the primary", in: " in pool Client"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := primarySaid(tt.in, tt.own, tt.made); got != tt.want {
				t.Errorf("primarySaid(%q, %t, %t) = %q, want %q", tt.in, tt.own, tt.made, got, tt.want)
			}
		})
	}
}
