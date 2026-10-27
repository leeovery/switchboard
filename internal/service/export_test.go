package service

import "context"

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
