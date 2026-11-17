package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"

	"github.com/BurntSushi/toml"

	"github.com/leeovery/switchboard/internal/atomicfile"
	"github.com/leeovery/switchboard/internal/redact"
)

// ErrSecretValue is what Copy fails with where the config gives a value that
// may hold a secret, as an upstream's address can carry a token, a key in its
// query or credentials: a copy holds none, and the config without it isn't
// the config.
var ErrSecretValue = errors.New("the config gives a value that may hold a secret, such as a token, which a copy of it never holds")

// Copy returns data, the text of the config file at path, which names it in
// errors, as a copy of it is kept off this Mac, as an export carries one:
// failing where data isn't a config that loads, as Parse reads one, so a
// copy laid down on another Mac loads there; with ErrSecretValue where a value
// it gives may hold a secret, as heldSecret finds one; and else with each
// thing in its text shaped like a token hidden, as redact hides one, so a
// token in a comment goes, failing with ErrSecretValue where hiding one
// changes the config it makes.
func Copy(path string, data []byte, homeDir func() (string, error)) ([]byte, error) {
	cfg, err := Parse(path, data, homeDir)
	if err != nil {
		return nil, err
	}
	if heldSecret(cfg, data) {
		return nil, ErrSecretValue
	}
	hidden := []byte(redact.Text(string(data)))
	if bytes.Equal(hidden, data) {
		return data, nil
	}
	if without, err := Parse(path, hidden, homeDir); err != nil || !reflect.DeepEqual(without, cfg) {
		return nil, ErrSecretValue
	}
	return hidden, nil
}

// heldSecret reports whether cfg, loaded from data, gives a value that may
// hold a secret: an upstream whose address may carry one, as
// redact.HoldsSecret finds one, credentials or a query among them, as the
// launcher judges a value it passes on; or any value or key shaped like a
// token, read as TOML decodes it, so a token written with escapes, or split
// across lines in a multi-line string, is found as the router would read it.
func heldSecret(cfg *Config, data []byte) bool {
	if redact.HoldsSecret(cfg.Upstream) {
		return true
	}
	var decoded map[string]any
	if _, err := toml.Decode(string(data), &decoded); err != nil {
		return true
	}
	return holdsToken(decoded)
}

// holdsToken reports whether v, a value as TOML decodes one, holds anything
// shaped like a token, in a string, or a table's key or value, however deep.
func holdsToken(v any) bool {
	switch v := v.(type) {
	case string:
		return redact.HoldsToken(v)
	case map[string]any:
		for key, value := range v {
			if redact.HoldsToken(key) || holdsToken(value) {
				return true
			}
		}
	case []map[string]any:
		return slices.ContainsFunc(v, func(table map[string]any) bool { return holdsToken(table) })
	case []any:
		return slices.ContainsFunc(v, holdsToken)
	}
	return false
}

// Members returns the accounts the text of a config file, data, configures,
// each by its id and the pool it's in, as written, and nothing else of them:
// read whatever else in it is wrong, as of a config a newer switchboard
// wrote, which may give keys this one doesn't know. It fails where data isn't
// TOML, or gives an account's id or pool as anything but text.
func Members(data []byte) (Accounts, error) {
	var only struct {
		Accounts []struct {
			ID   string `toml:"id"`
			Pool string `toml:"pool"`
		} `toml:"account"`
	}
	if _, err := toml.Decode(string(data), &only); err != nil {
		return nil, fmt.Errorf("parse config: %s", redact.Text(err.Error()))
	}
	accounts := make(Accounts, len(only.Accounts))
	for i, a := range only.Accounts {
		accounts[i] = Account{ID: a.ID, Pool: a.Pool}
	}
	return accounts, nil
}

// LayDown writes data as a new config file at path, making its folder where
// it's missing: only where nothing is at path, never in place of a file, a
// link or anything else there, as one written meanwhile, failing with an
// error matching fs.ErrExist where something is.
func LayDown(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	staged, err := atomicfile.Stage(path, newFilePerm, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
	if err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	defer func() { _ = os.Remove(staged) }()
	// A link, unlike a rename, fails where its name is taken.
	if err := os.Link(staged, path); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
