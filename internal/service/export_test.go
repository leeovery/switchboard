package service

// PlistOf returns the plist of the LaunchAgent that serves as opts says, its
// binary as it's found, for tests to compare with what Install writes.
func (s *Service) PlistOf(opts InstallOptions) ([]byte, error) {
	a, err := s.agent(opts)
	if err != nil {
		return nil, err
	}
	return a.plist()
}

// ProgramOf returns what launchd runs for that LaunchAgent.
func (s *Service) ProgramOf(opts InstallOptions) ([]string, error) {
	a, err := s.agent(opts)
	return a.Program, err
}
