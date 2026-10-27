package service

import (
	"fmt"
	"os"
	"slices"
)

// environmentKey is the plist's key for what launchd sets in the router's
// environment.
const environmentKey = "EnvironmentVariables"

// Options reads back the options the plist in place was installed with: the
// binary it runs, the config file and the log level it gives the router, and
// the variables launchd sets for it, all of them, and the log launchd
// writes, which Install keeps in place of those where it's installed. It
// fails for a plist it can't read, that isn't XML, as a binary one isn't, or
// that names no program, as then it names nothing to keep: one that isn't
// there fails as fs.ErrNotExist.
func (s *Service) Options() (InstallOptions, error) {
	installed, err := s.readPlist()
	if err != nil {
		return InstallOptions{}, err
	}
	program, _ := lookup(installed, "ProgramArguments")
	opts := optionsOf(program.strings())
	if opts.Executable == "" {
		return InstallOptions{}, fmt.Errorf("the service's plist, %s, names no program", s.Plist())
	}
	opts.environment = map[string]string{}
	variables, _ := lookup(installed, environmentKey)
	for _, v := range variables.entries() {
		opts.environment[v.key] = v.value.Text
	}
	log, _ := lookup(installed, "StandardOutPath")
	opts.log = log.Text
	return opts, nil
}

// Drift says how the plist in place differs from the one Install would write
// for opts, a difference a phrase, such as "no ProcessType", naming the keys
// and the arguments that differ but never their values: none when launchd
// would read the two alike, whatever their order of keys or their
// whitespace, or the form a flag's value is given in. The variables launchd
// sets never count: they differ as the shells installing from differ, never
// as switchboards do, and OtherEnvironment says how. It fails as reading the
// plist does.
func (s *Service) Drift(opts InstallOptions) ([]string, error) {
	installed, err := s.readPlist()
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

// readPlist reads the plist in place, its top dictionary's entries.
func (s *Service) readPlist() ([]entry, error) {
	data, err := os.ReadFile(s.Plist())
	if err != nil {
		return nil, fmt.Errorf("read the service's plist: %w", err)
	}
	entries, err := parsePlist(data)
	if err != nil {
		return nil, fmt.Errorf("parse the service's plist, %s: %w", s.Plist(), err)
	}
	return entries, nil
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
