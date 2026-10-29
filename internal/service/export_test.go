package service

import "context"

// HeldSignals are the signals Install holds as it swaps.
var HeldSignals = heldSignals

// HoldSignals holds HeldSignals but those ignored says the process ignores,
// as Install does, through notify and stop in place of os/signal's.
var HoldSignals = holdSignals

// PlistOf returns the plist of the LaunchAgent that serves as opts says, its
// binary as it's found, for tests to compare with what Install writes.
func (s *Service) PlistOf(opts InstallOptions) ([]byte, error) {
	a, err := s.agent(opts)
	if err != nil {
		return nil, err
	}
	return a.plist()
}

// PlannedPlistOf returns the plist Install would write for opts, in the
// environment it plans, as it plans it.
func (s *Service) PlannedPlistOf(ctx context.Context, opts InstallOptions) ([]byte, error) {
	planned, opts, err := s.Planned(ctx, opts)
	if err != nil {
		return nil, err
	}
	return planned.PlistOf(opts)
}

// StateDir is the router's state directory, as the service is seen.
func (s *Service) StateDir() string {
	return s.cfg.StateDir
}

// Getenv reads the environment the service's router runs in, as far as
// switchboard reads it, as the service is seen.
func (s *Service) Getenv(name string) string {
	return s.cfg.Getenv(name)
}

// ProgramOf returns what launchd runs for that LaunchAgent.
func (s *Service) ProgramOf(opts InstallOptions) ([]string, error) {
	a, err := s.agent(opts)
	return a.Program, err
}

// Differences says how the plist have differs from the plist want, as Drift
// sets them apart.
func Differences(want, have []byte) ([]string, error) {
	w, err := parsePlist(want)
	if err != nil {
		return nil, err
	}
	h, err := parsePlist(have)
	if err != nil {
		return nil, err
	}
	return differences(w, h, ""), nil
}
