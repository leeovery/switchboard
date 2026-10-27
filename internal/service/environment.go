package service

import (
	"cmp"
	"errors"
	"path/filepath"
	"slices"

	"github.com/leeovery/switchboard/internal/config"
)

// errNoRouterAt is what For fails with when the environment kept puts the
// router's state elsewhere, and Config gives no way to ask a router there.
var errNoRouterAt = errors.New("the service keeps its state in another directory, and there's no asking the router there")

// For returns the service as the router it runs sees it, installed with
// opts: in the environment opts keep, as Options read it back, over the one
// it's asked in, so its state directory, and with it its router, its log and
// the token files it serves with, are those the environment kept decides.
// For opts that keep none, it's the service itself.
func (s *Service) For(opts InstallOptions) (*Service, error) {
	if opts.environment == nil {
		return s, nil
	}
	cfg := s.cfg
	cfg.Getenv = func(name string) string {
		if value, ok := opts.environment[name]; ok {
			return value
		}
		if slices.Contains(carried, name) {
			return ""
		}
		return s.cfg.Getenv(name)
	}
	dir, err := config.StateDir(cfg.Getenv, s.home)
	if err != nil {
		return nil, err
	}
	if dir != s.cfg.StateDir {
		if cfg.RouterAt == nil {
			return nil, errNoRouterAt
		}
		cfg.StateDir, cfg.Router = dir, cfg.RouterAt(dir)
	}
	return &Service{cfg: cfg}, nil
}

// Getenv reads the environment the service's router runs in, as far as
// switchboard reads it: the one it's asked in, or, as For returns it, the
// one installed over that.
func (s *Service) Getenv(name string) string {
	return s.cfg.Getenv(name)
}

// ConfigFile is the config file the router serves, installed with opts: the
// one opts give, else the one its environment finds.
func (s *Service) ConfigFile(opts InstallOptions) (string, error) {
	if file, err := absolute(opts.Config); err != nil || file != "" {
		return file, err
	}
	file, err := config.Path(s.cfg.Getenv, s.home)
	if err != nil {
		return "", err
	}
	return absolute(file)
}

// OtherEnvironment names the variables carried that decide otherwise where
// the service is asked about than in the environment opts keep, as Options
// read it back: one set to what its absence decides is the same as unset.
// It's none for opts that keep none.
func (s *Service) OtherEnvironment(opts InstallOptions) ([]string, error) {
	if opts.environment == nil {
		return nil, nil
	}
	here, err := carry(s.cfg.Getenv)
	if err != nil {
		return nil, err
	}
	var other []string
	for _, name := range carried {
		value := ""
		if i := slices.IndexFunc(here, func(v variable) bool { return v.Name == name }); i >= 0 {
			value = here[i].Value
		}
		if s.decided(name, value) != s.decided(name, opts.environment[name]) {
			other = append(other, name)
		}
	}
	return other, nil
}

// decided is what the variable carried, name, set to value, "" for unset,
// decides: the file or the directory it finds, as switchboard and Claude
// Code find them, or, for the log level, the value itself.
func (s *Service) decided(name, value string) string {
	only := func(key string) string {
		if key == name {
			return value
		}
		return ""
	}
	var found string
	switch name {
	case "XDG_CONFIG_HOME", "SWITCHBOARD_CONFIG":
		found, _ = config.Path(only, s.home)
	case "XDG_STATE_HOME":
		found, _ = config.StateDir(only, s.home)
	case "CLAUDE_CONFIG_DIR":
		found = cmp.Or(value, filepath.Join(s.cfg.Home, ".claude"))
	default:
		return value
	}
	return filepath.Clean(found)
}

// Decides says what the variable carried, name, decides for the router, in
// a few words, such as "state directory".
func Decides(name string) string {
	switch name {
	case "XDG_CONFIG_HOME", "SWITCHBOARD_CONFIG":
		return "config file"
	case "XDG_STATE_HOME":
		return "state directory"
	case "CLAUDE_CONFIG_DIR":
		return "Claude Code config directory"
	}
	return "log level"
}

// home is the user's home directory, as config's paths ask for it.
func (s *Service) home() (string, error) {
	return s.cfg.Home, nil
}
