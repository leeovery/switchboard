package service

// PlistOf returns the plist of the LaunchAgent that serves with the
// switchboard at binary, through zsh sourcing envFile unless it's "", and
// serving the config file at config unless it's "", for tests to compare
// with what Install writes.
func (s *Service) PlistOf(binary, envFile, config string) ([]byte, error) {
	return s.agent(binary, envFile, config).plist()
}

// ProgramOf returns what launchd runs for that LaunchAgent.
func (s *Service) ProgramOf(binary, envFile, config string) []string {
	return s.agent(binary, envFile, config).Program
}
