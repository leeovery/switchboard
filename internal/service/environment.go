package service

import (
	"cmp"
	"errors"
	"os"
	"path/filepath"
	"slices"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/skill"
)

// errNoRouterAt is what For fails with when the environment kept puts the
// router's state elsewhere, and Config gives no way to ask a router there.
var errNoRouterAt = errors.New("the service keeps its state in another directory, and there's no asking the router there")

// For returns the service as the router it runs sees it, installed with
// opts: in the environment opts keep, as Options read it back, over the one
// it's asked in, and in the home that environment names, if it names one,
// so its state directory, and with it its router, and the token files it
// serves with, are those the environment kept decides, and its log the one
// the plist names. The router at the state directory it's asked from is
// asked too, should that be the one running, as when launchd was given a
// variable the plist doesn't set. For opts that keep none, it's the service
// itself.
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
	seen := &Service{cfg: cfg, home: opts.environment["HOME"], log: opts.log}
	dir, err := config.StateDir(cfg.Getenv, seen.homeDir)
	if err != nil {
		return nil, err
	}
	if dir != s.cfg.StateDir {
		if cfg.RouterAt == nil {
			return nil, errNoRouterAt
		}
		seen.cfg.StateDir, seen.cfg.Router = dir, cfg.RouterAt(dir)
		seen.also = []Router{s.cfg.Router}
	}
	return seen, nil
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
	file, err := config.Path(s.cfg.Getenv, s.homeDir)
	if err != nil {
		return "", err
	}
	return absolute(file)
}

// Places are where a router, or setup, finds what it reads and writes.
type Places struct {
	// StateDir is its state directory, which holds the token files;
	// ConfigFile the config file it serves; and ClaudeDir Claude Code's
	// config directory, where the skill goes.
	StateDir, ConfigFile, ClaudeDir string
}

// Places are the router's, installed with opts, in the environment the
// service is seen in.
func (s *Service) Places(opts InstallOptions) (Places, error) {
	file, err := s.ConfigFile(opts)
	if err != nil {
		return Places{}, err
	}
	claude, err := skill.ConfigDir(s.cfg.Getenv, s.homeDir)
	if err != nil {
		return Places{}, err
	}
	claude, err = absolute(claude)
	if err != nil {
		return Places{}, err
	}
	return Places{StateDir: s.cfg.StateDir, ConfigFile: file, ClaudeDir: claude}, nil
}

// Apart names what p finds elsewhere than q, a link to the same file or
// directory finding the same: "state directory", "config file" and "Claude
// Code config directory", in that order.
func (p Places) Apart(q Places) []string {
	var apart []string
	for _, place := range []struct{ name, a, b string }{
		{"state directory", p.StateDir, q.StateDir},
		{"config file", p.ConfigFile, q.ConfigFile},
		{"Claude Code config directory", p.ClaudeDir, q.ClaudeDir},
	} {
		if !SameFile(place.a, place.b) {
			apart = append(apart, place.name)
		}
	}
	return apart
}

// Shared reports whether p and q find the same state directory, which
// holds the token files, and the same config file.
func (p Places) Shared(q Places) bool {
	return SameFile(p.StateDir, q.StateDir) && SameFile(p.ConfigFile, q.ConfigFile)
}

// SameFile reports whether the paths a and b find the same file or
// directory: the same path, or, both there, the same file through links.
func SameFile(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	return err == nil && os.SameFile(ai, bi)
}

// installing is the environment Install writes where it keeps none: the
// variables carried, as they're set where it's installed, and those besides
// that a plist in place sets, as one added by hand, such as HTTPS_PROXY.
func (s *Service) installing() (map[string]string, error) {
	environment := map[string]string{}
	if installed, err := s.readPlist(); err == nil {
		variables, _ := lookup(installed, environmentKey)
		for _, v := range variables.entries() {
			if !slices.Contains(carried, v.key) {
				environment[v.key] = v.value.Text
			}
		}
	}
	here, err := carry(s.cfg.Getenv)
	if err != nil {
		return nil, err
	}
	for _, v := range here {
		environment[v.Name] = v.Value
	}
	return environment, nil
}

// homeDir is the home directory the router runs in, as config's paths ask
// for it.
func (s *Service) homeDir() (string, error) {
	return cmp.Or(s.home, s.cfg.Home), nil
}
