package config_test

import (
	"errors"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
)

// Every refusal of an account's id to add is a WrongID, why it's refused
// said alone, never the id, and one an account has already matching
// ErrConfigured.
func TestAnIDsRefusalsSayWhyAlone(t *testing.T) {
	tests := []struct {
		name, id, why string
		is            error
	}{
		{name: "none", id: "", why: "is required"},
		{name: "shaped like a token", id: tokenShaped, why: "looks like a token, which an id mustn't, as it shows wherever the account does"},
		{name: "not a plain file name", id: "../side", why: "must start with a letter or digit and contain only letters, digits, '-' and '_'"},
		{name: "the one pin takes", id: "Auto", why: "is reserved for switchboard pin auto"},
		{name: "an account's already", id: "personal", why: "is already configured", is: config.ErrConfigured},
		{name: "an account's in another case", id: "Personal", why: `differs from "personal" only in case, so they'd share a token file, as macOS ignores case in file names`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := edit(t, writeConfig(t, commented)).AddAccount(config.NewAccount{ID: tt.id})
			wrong, ok := errors.AsType[*config.WrongID](err)
			if !ok || wrong.Why != tt.why {
				t.Fatalf("AddAccount error = %v, want a WrongID saying %q", err, tt.why)
			}
			if tt.is != nil && !errors.Is(err, tt.is) {
				t.Errorf("the error %v doesn't match %v", err, tt.is)
			}
		})
	}
}

// CheckID refuses an id no account can have with a WrongID, as adding one
// does.
func TestCheckIDRefusesWithAWrongID(t *testing.T) {
	err := config.CheckID("../side")
	if _, ok := errors.AsType[*config.WrongID](err); !ok {
		t.Errorf("CheckID error = %v, want a WrongID", err)
	}
}
