// Package config locates, parses and validates switchboard's config file, and
// reads account tokens from the environment.
package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

const (
	defaultListen   = "127.0.0.1:4747"
	defaultUpstream = "https://api.anthropic.com"
)

// Example is a small valid config, for showing someone who doesn't have one yet.
const Example = `# One [[account]] per Claude subscription. token_env names the environment
# variable holding that account's token, created with "claude setup-token".

[[account]]
id        = "work"
label     = "Work"
token_env = "CLAUDE_TOKEN_WORK"

[[account]]
id        = "personal"
label     = "Personal"
token_env = "CLAUDE_TOKEN_PERSONAL"
`

// Config is a validated config file with its defaults filled in.
type Config struct {
	Listen   string `toml:"listen"`
	Upstream string `toml:"upstream"`
	// Accounts keep their file order, which is their display order everywhere.
	Accounts []Account `toml:"account"`
}

// Account is one Claude subscription.
type Account struct {
	ID       string `toml:"id"`
	Label    string `toml:"label"`
	TokenEnv string `toml:"token_env"`
}

// Load reads the config file at path, validates it and fills in its defaults.
// When the file doesn't exist, the error matches fs.ErrNotExist.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	cfg := &Config{Listen: defaultListen, Upstream: defaultUpstream}
	meta, err := toml.Decode(string(data), cfg)
	if err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := cfg.validate(meta.Undecoded()); err != nil {
		return nil, fmt.Errorf("invalid config %s:\n%w", path, err)
	}
	cfg.defaultLabels()
	return cfg, nil
}

func (c *Config) defaultLabels() {
	for i, a := range c.Accounts {
		if a.Label == "" {
			c.Accounts[i].Label = a.ID
		}
	}
}
