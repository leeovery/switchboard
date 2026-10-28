package service

import (
	"cmp"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/skill"
	"github.com/leeovery/switchboard/internal/tokens"
)

// errNoRouterAt is what seeing the service fails with when the environment
// it's seen in puts the router's state elsewhere, and Config gives no way to
// ask a router there.
var errNoRouterAt = errors.New("the service keeps its state in another directory, and there's no asking the router there")

// For returns the service as the router it runs sees it, installed with
// opts, as Options read them back: in the environment they keep, as seenIn
// says. For opts that keep no environment, it's the service itself.
func (s *Service) For(opts InstallOptions) (*Service, error) {
	if opts.environment == nil {
		return s, nil
	}
	return s.seenIn(opts.environment)
}

// Planned returns the service as Install, given opts, installs it, with
// opts as it writes them: in the environment opts keep, else the variables
// carried, as they're set where it's installed, with the others the plist in
// place sets, as one added by hand. A service seen already, as For sees it,
// is seen as it is.
func (s *Service) Planned(ctx context.Context, opts InstallOptions) (*Service, InstallOptions, error) {
	switch {
	case opts.environment == nil:
		environment, unread, err := s.installing(ctx)
		if err != nil {
			return nil, InstallOptions{}, err
		}
		opts.environment, opts.unread = environment, unread
	case s.base != nil:
		return s, opts, nil
	}
	seen, err := s.seenIn(opts.environment)
	return seen, opts, err
}

// seenIn returns the service as a router launchd runs in the environment
// env sees it: env over the one the service is asked in, a variable carried
// that env doesn't set unset, in the home env names, if it names one. Its
// state directory, and with it its router, its log and the token files it
// serves with, are those env decides; the router at the state directory the
// service is asked from is asked too, so Status can say when it alone
// answers, and which router the service runs can't be told.
func (s *Service) seenIn(env map[string]string) (*Service, error) {
	root := s.root()
	cfg := root.cfg
	cfg.Getenv = func(name string) string {
		if value, ok := env[name]; ok {
			return value
		}
		if slices.Contains(carried, name) {
			return ""
		}
		return root.cfg.Getenv(name)
	}
	home, set := env["HOME"]
	seen := &Service{cfg: cfg, home: home, emptyHome: set && home == "", base: root}
	dir, err := config.StateDir(cfg.Getenv, seen.homeDir)
	if err != nil {
		return nil, err
	}
	if dir != root.cfg.StateDir {
		if cfg.RouterAt == nil {
			return nil, errNoRouterAt
		}
		seen.cfg.StateDir, seen.cfg.Router, seen.also = dir, cfg.RouterAt(dir), []Router{root.cfg.Router}
	}
	return seen, nil
}

// seenAt is the service as the router of index i among those it may run
// sees it: itself for the first, or where none answered; for the one at the
// state directory it's asked from, itself with that state directory, the
// token files and launchd's log in it, the rest of its environment kept.
func (s *Service) seenAt(i int) *Service {
	if i <= 0 {
		return s
	}
	root := s.root()
	seen := *s
	seen.cfg.StateDir, seen.cfg.Router, seen.also = root.cfg.StateDir, root.cfg.Router, nil
	return &seen
}

// Unplaced is the service as it's seen without its plist, which couldn't be
// read back or placed: the router that answers isn't taken for the one
// launchd runs.
func (s *Service) Unplaced() *Service {
	unplaced := *s
	unplaced.unplaced = true
	return &unplaced
}

// Found is the service as the router answering now sees it, as seenAt says,
// and whether that router answers at the state directory the service is
// asked from, rather than the one its plist keeps: itself where none
// answers.
func (s *Service) Found(ctx context.Context) (*Service, bool) {
	_, i, _ := s.answering(ctx)
	return s.seenAt(i), i > 0
}

// root is the service as it's asked about, before it's seen in another
// environment.
func (s *Service) root() *Service {
	if s.base != nil {
		return s.base
	}
	return s
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
	// Tokens is the directory of the token files, in the state directory;
	// ConfigFile the config file it serves or edits; and ClaudeDir Claude
	// Code's config directory, where the skill goes, "" for none found.
	Tokens, ConfigFile, ClaudeDir string
}

// Places are the router's, installed with opts, as the service is seen. A
// Claude Code config directory under a HOME the plist sets empty is none:
// the router serves all the same, but keeps no skill up to date.
func (s *Service) Places(opts InstallOptions) (Places, error) {
	file, err := s.ConfigFile(opts)
	if err != nil {
		return Places{}, err
	}
	claude, err := skill.ConfigDir(s.cfg.Getenv, s.homeDir)
	switch {
	case errors.Is(err, errEmptyHome):
		return Places{Tokens: tokens.NewStore(s.cfg.StateDir, s.cfg.UID).Dir(), ConfigFile: file}, nil
	case err != nil:
		return Places{}, err
	}
	if claude, err = absolute(claude); err != nil {
		return Places{}, err
	}
	return Places{Tokens: tokens.NewStore(s.cfg.StateDir, s.cfg.UID).Dir(), ConfigFile: file, ClaudeDir: claude}, nil
}

// Apart names what p finds elsewhere than q, as SameFile judges: "state
// directory", "config file" and "Claude Code config directory", in that
// order. One p finds none of isn't named.
func (p Places) Apart(q Places) []string {
	var apart []string
	for _, place := range []struct{ name, a, b string }{
		{"state directory", p.Tokens, q.Tokens},
		{"config file", p.ConfigFile, q.ConfigFile},
		{"Claude Code config directory", p.ClaudeDir, q.ClaudeDir},
	} {
		if place.a != "" && !SameFile(place.a, place.b) {
			apart = append(apart, place.name)
		}
	}
	return apart
}

// SharesConfig reports whether p and q find the same config file.
func (p Places) SharesConfig(q Places) bool {
	return SameFile(p.ConfigFile, q.ConfigFile)
}

// Shared reports whether p and q find the same token files and the same
// config file.
func (p Places) Shared(q Places) bool {
	return SameFile(p.Tokens, q.Tokens) && p.SharesConfig(q)
}

// SameFile reports whether the paths a and b find the same file or
// directory: the same path, or the same file through links. A path not there
// yet finds the same as another where the nearest of their directories that
// is there is the same, and the rest of the two paths below it is too.
func SameFile(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if a == b {
		return true
	}
	aThere, aRest := nearest(a)
	bThere, bRest := nearest(b)
	if aRest != bRest {
		return false
	}
	ai, err := os.Stat(aThere)
	if err != nil {
		return false
	}
	bi, err := os.Stat(bThere)
	return err == nil && os.SameFile(ai, bi)
}

// nearest is path, where it's there, else the nearest of its directories
// that is, with the rest of path below it.
func nearest(path string) (there, rest string) {
	for {
		if _, err := os.Stat(path); err == nil {
			return path, rest
		}
		parent := filepath.Dir(path)
		if parent == path {
			return path, rest
		}
		rest = filepath.Join(filepath.Base(path), rest)
		path = parent
	}
}

// installing is the environment Install writes where it keeps none: the
// variables carried, as they're set where it's installed, and those besides
// that a plist in place sets, as one added by hand, such as HTTPS_PROXY,
// with why the plist couldn't be read back, where it's there but couldn't
// be. A HOME the plist sets isn't kept: launchd gives the router the user's,
// as it gives this shell, so the router finds what the shell does.
func (s *Service) installing(ctx context.Context) (environment map[string]string, unread, err error) {
	environment = map[string]string{}
	installed, err := s.readPlist(ctx)
	switch {
	case err == nil:
		variables, _ := lookup(installed, environmentKey)
		for _, v := range variables.entries() {
			if !slices.Contains(carried, v.key) && v.key != "HOME" {
				environment[v.key] = v.value.Text
			}
		}
	case !errors.Is(err, fs.ErrNotExist):
		unread = err
	}
	here, err := carry(s.cfg.Getenv)
	if err != nil {
		return nil, nil, err
	}
	for _, v := range here {
		environment[v.Name] = v.Value
	}
	return environment, unread, nil
}

// Start is what a router starts from, as serve's check takes it: its state
// directory, its config file, and the level it logs at, "" for the one its
// environment names.
type Start struct {
	StateDir, ConfigFile, LogLevel string
}

// StartsFrom is what the router, installed with opts, starts from, as the
// service is seen.
func (s *Service) StartsFrom(opts InstallOptions) (Start, error) {
	file, err := s.ConfigFile(opts)
	if err != nil {
		return Start{}, err
	}
	return Start{StateDir: s.cfg.StateDir, ConfigFile: file, LogLevel: opts.LogLevel}, nil
}

// errEmptyHome is why a place the router finds under its home can't be
// worked out where launchd sets HOME empty, as it passes one the plist sets
// so: the router serves all the same where none of its places needs it.
var errEmptyHome = errors.New("the service's plist sets HOME empty, where the router finds no home directory")

// homeDir is the home directory the router runs in, as config's paths ask
// for it.
func (s *Service) homeDir() (string, error) {
	if s.emptyHome {
		return "", errEmptyHome
	}
	return cmp.Or(s.home, s.cfg.Home), nil
}
