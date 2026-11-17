package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"

	"github.com/BurntSushi/toml"

	"github.com/leeovery/switchboard/internal/atomicfile"
	"github.com/leeovery/switchboard/internal/redact"
)

// ErrTokenValue is what Copy fails with where the config gives a value shaped
// like a token, as an upstream's address can carry one: a copy holds none,
// and the config without it isn't the config.
var ErrTokenValue = errors.New("the config gives a value shaped like a token, which a copy of it never holds")

// Copy returns data, the text of the config file at path, which names it in
// errors, as a copy of it is kept off this Mac, as an export carries one:
// each thing in it shaped like a token hidden, as redact hides one, so a
// token in a comment goes; failing where data isn't a config that loads, as
// Parse reads one, so a copy laid down on another Mac loads there; and with
// ErrTokenValue where hiding them changes the config it makes, as of a token
// in a value.
func Copy(path string, data []byte, homeDir func() (string, error)) ([]byte, error) {
	cfg, err := Parse(path, data, homeDir)
	if err != nil {
		return nil, err
	}
	hidden := []byte(redact.Text(string(data)))
	if bytes.Equal(hidden, data) {
		return data, nil
	}
	if without, err := Parse(path, hidden, homeDir); err != nil || !reflect.DeepEqual(without, cfg) {
		return nil, ErrTokenValue
	}
	return hidden, nil
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
