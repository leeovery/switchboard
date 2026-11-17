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
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/leeovery/switchboard/internal/atomicfile"
	"github.com/leeovery/switchboard/internal/redact"
)

var (
	// ErrSecretValue is what Copy fails with where the config gives a value
	// that may hold a secret, as an upstream's address can carry a token, a
	// key in its query or fragment, or credentials: a copy holds none, and
	// the config without it isn't the config.
	ErrSecretValue = errors.New("the config gives a value that may hold a secret, such as a token, which a copy of it never holds")
	// ErrCopyDiffers is what Copy fails with where the config, its values
	// written anew, wouldn't load as the config it is.
	ErrCopyDiffers = errors.New("the config, its values written anew, wouldn't load as it is, so no copy of it is made")
)

// Copy returns a copy of the config file at path, which names it in errors,
// whose text is data, to keep off this Mac, as an export carries one: the
// values data gives alone, as TOML decodes them, written anew, so nothing
// but a key and its value reaches the copy, no comment nor any of its
// layout. It fails where data isn't a config that loads, as Parse reads one;
// with ErrSecretValue where a value or a key may hold a secret, as
// heldSecret finds one; and with ErrCopyDiffers where the copy wouldn't load
// as the config data makes, so a copy laid down on another Mac is the
// config there.
func Copy(path string, data []byte, homeDir func() (string, error)) ([]byte, error) {
	cfg, err := Parse(path, data, homeDir)
	if err != nil {
		return nil, err
	}
	var values map[string]any
	if _, err := toml.Decode(string(data), &values); err != nil {
		return nil, unparsed(path, err)
	}
	if heldSecret(cfg, values) {
		return nil, ErrSecretValue
	}
	copied, err := encoded(values)
	if err != nil {
		return nil, ErrCopyDiffers
	}
	if again, err := Parse(path, copied, homeDir); err != nil || !reflect.DeepEqual(again, cfg) {
		return nil, ErrCopyDiffers
	}
	return copied, nil
}

// encoded is values, as TOML decodes a file's, written as a TOML file's
// text: each table's keys in order, then its tables, none indented.
func encoded(values map[string]any) ([]byte, error) {
	var text bytes.Buffer
	enc := toml.NewEncoder(&text)
	enc.Indent = ""
	if err := enc.Encode(values); err != nil {
		return nil, err
	}
	return text.Bytes(), nil
}

// heldSecret reports whether cfg, loaded from a file whose values, as TOML
// decodes them, are values, gives a value that may hold a secret: an
// upstream whose address may carry one, as secretInUpstream finds one; or
// any value or key shaped like a token, read as TOML decodes it, so a token
// written with escapes, or split across lines in a multi-line string, is
// found as the router would read it.
func heldSecret(cfg *Config, values map[string]any) bool {
	return secretInUpstream(cfg.Upstream) || holdsToken(values)
}

// secretInUpstream reports whether upstream, an upstream's address, may
// carry a secret: anything redact.HoldsSecret finds, credentials or a query
// among them, as the launcher judges a value it passes on, or a fragment,
// which can carry a key too. A key in its host or its path can't be told
// from any other name.
func secretInUpstream(upstream string) bool {
	return redact.HoldsSecret(upstream) || strings.Contains(upstream, "#")
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

// LayDown writes data, a config, as Copy makes one, as a new config file at
// path, its ids marked as switchboard marks those it writes, so none is
// edited, making its folder where it's missing: only where nothing is at
// path, never in place of a file, a link or anything else there, as one
// written meanwhile, failing with an error matching fs.ErrExist where
// something is.
func LayDown(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	staged, err := atomicfile.Stage(path, newFilePerm, func(w io.Writer) error {
		_, err := io.WriteString(w, idsMarked(data))
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

// idsMarked returns data, a config's text, with the line setting each
// account's and each pool's id marked, as a Draft marks those it writes,
// where the marks read back as data does: else data as it is, as of text
// that doesn't read as a config.
func idsMarked(data []byte) string {
	text := string(data)
	want, _, err := decode(text)
	if err != nil {
		return text
	}
	return newDocument(text).idsMarked(want).text()
}
