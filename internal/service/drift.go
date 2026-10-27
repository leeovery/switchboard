package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// environmentKey is the plist's key for what launchd sets in the router's
// environment.
const environmentKey = "EnvironmentVariables"

// byteOrderMark is what a UTF-8 file may start with before its text.
var byteOrderMark = []byte("\xef\xbb\xbf")

// errNoPlutil is what reading a binary plist fails with without plutil to
// read it back as XML.
var errNoPlutil = errors.New("it's a binary plist, and there's no plutil to read it")

// Options reads back the options the plist in place was installed with: the
// binary it runs, the config file and the log level it gives the router, and
// the variables launchd sets for it, all of them, which Install keeps in
// place of those where it's installed. A path the plist names relatively is
// one under /, as launchd runs the router there. It fails for a plist it
// can't read, that isn't a plist, or that names no program, as then it names
// nothing to keep: one that isn't there fails as fs.ErrNotExist.
func (s *Service) Options(ctx context.Context) (InstallOptions, error) {
	installed, err := s.readPlist(ctx)
	if err != nil {
		return InstallOptions{}, err
	}
	program, _ := lookup(installed, "ProgramArguments")
	opts := optionsOf(program.strings())
	if opts.Executable == "" {
		return InstallOptions{}, fmt.Errorf("the service's plist, %s, names no program", s.Plist())
	}
	opts.Config = asLaunchdRuns(opts.Config)
	opts.environment = map[string]string{}
	variables, _ := lookup(installed, environmentKey)
	for _, v := range variables.entries() {
		value := v.value.Text
		if slices.Contains(madeAbsolute, v.key) || v.key == "HOME" {
			value = asLaunchdRuns(value)
		}
		opts.environment[v.key] = value
	}
	return opts, nil
}

// asLaunchdRuns is path as the router launchd runs finds it, in /: a
// relative one is under /.
func asLaunchdRuns(path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join("/", path)
}

// Drift says how the plist in place differs from the one Install would write
// for opts, a difference a phrase, such as "no ProcessType", naming the keys
// and the arguments that differ but never their values: none when launchd
// would read the two alike, whatever their order of keys or their
// whitespace, or the form a flag's value is given in. The variables launchd
// sets never count: they differ as the shells installing from differ, never
// as switchboards do. It fails as reading the plist does.
func (s *Service) Drift(ctx context.Context, opts InstallOptions) ([]string, error) {
	installed, err := s.readPlist(ctx)
	if err != nil {
		return nil, err
	}
	a, err := s.describe(opts)
	if err != nil {
		return nil, err
	}
	data, err := a.plist()
	if err != nil {
		return nil, fmt.Errorf("describe the service: %w", err)
	}
	want, err := parsePlist(data)
	if err != nil {
		return nil, fmt.Errorf("read back the service's plist as written: %w", err)
	}
	return differences(withoutEnvironment(want), withoutEnvironment(installed), ""), nil
}

// withoutEnvironment is entries without the variables launchd sets.
func withoutEnvironment(entries []entry) []entry {
	return slices.DeleteFunc(entries, func(e entry) bool { return e.key == environmentKey })
}

// readPlist reads the plist in place, its top dictionary's entries: one
// that isn't XML, as in Apple's binary form, as defaults writes one, through
// plutil. It fails as fs.ErrNotExist only where there's no plist.
func (s *Service) readPlist(ctx context.Context) ([]entry, error) {
	data, err := os.ReadFile(s.Plist())
	if err != nil {
		return nil, fmt.Errorf("read the service's plist: %w", err)
	}
	if !isXML(data) {
		// Not wrapped: plutil not there is no plist not there.
		if data, err = s.asXML(ctx); err != nil {
			return nil, fmt.Errorf("read the service's plist, %s, through plutil: %v", s.Plist(), err)
		}
	}
	entries, err := parsePlist(data)
	if err != nil {
		return nil, fmt.Errorf("parse the service's plist, %s: %w", s.Plist(), err)
	}
	return entries, nil
}

// isXML reports whether data reads as XML: its first character, past any
// byte order mark and whitespace, opens a tag.
func isXML(data []byte) bool {
	return bytes.HasPrefix(bytes.TrimLeft(bytes.TrimPrefix(data, byteOrderMark), " \t\r\n"), []byte("<"))
}

// asXML is the plist in place, read back as XML by plutil.
func (s *Service) asXML(ctx context.Context) ([]byte, error) {
	if s.cfg.Plutil == nil {
		return nil, errNoPlutil
	}
	ctx, cancel := context.WithTimeout(ctx, launchctlTimeout)
	defer cancel()
	return s.cfg.Plutil(ctx, "-convert", "xml1", "-o", "-", s.Plist())
}

// optionsOf reads back the options a program, as agent writes it, was
// installed with: its binary, the first argument, and the flags agent gives
// it.
func optionsOf(program []string) InstallOptions {
	plain, flags, _ := arguments(program)
	var opts InstallOptions
	if len(plain) > 0 {
		opts.Executable = plain[0]
	}
	if config, ok := lookup(flags, configFlag); ok {
		opts.Config = config.Text
	}
	if level, ok := lookup(flags, logLevelFlag); ok {
		opts.LogLevel = level.Text
	}
	return opts
}
