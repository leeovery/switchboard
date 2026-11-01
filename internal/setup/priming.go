package setup

import "github.com/leeovery/switchboard/internal/config"

// Priming is the day priming spreads the 5-hour windows' resets over: the
// zero Day where the config sets none, and priming is off.
func (s *Setup) Priming() (config.Day, error) {
	draft, err := config.Edit(s.Accounts.ConfigPath)
	if err != nil || draft.Config() == nil {
		return config.Day{}, err
	}
	return draft.Config().Prime.Day, nil
}

// SetDay has priming spread the resets over day, in the config, and returns
// the day as it's set. A day of "" turns priming off, taking the day out of
// the config. It writes nothing where the day doesn't change, and fails,
// changing nothing, as ParseDay does for a day that isn't one, and with
// errNoAccount for a day to set where no account is configured: it never
// makes the config file.
func (s *Setup) SetDay(day string) (config.Day, error) {
	parsed, err := config.ParseDay(day)
	if err != nil {
		return config.Day{}, err
	}
	draft, err := config.Edit(s.Accounts.ConfigPath)
	if err != nil {
		return config.Day{}, err
	}
	cfg := draft.Config()
	switch {
	case cfg == nil && parsed == config.Day{}:
		return parsed, nil
	case cfg == nil:
		return config.Day{}, errNoAccount
	case cfg.Prime.Day == parsed:
		return parsed, nil
	}
	if err := draft.SetPrimeDay(day); err != nil {
		return config.Day{}, err
	}
	if err := draft.Save(); err != nil {
		return config.Day{}, err
	}
	s.wrote()
	logger.Info("set the priming day", "day", day)
	return parsed, nil
}
