package setup

import (
	"context"

	"github.com/leeovery/switchboard/internal/config"
)

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
	_, set, err := s.setDay(day)
	return set, err
}

// setDay sets the day as SetDay does, and returns the config as it leaves
// it too: nil where no account is configured.
func (s *Setup) setDay(day string) (*config.Config, config.Day, error) {
	parsed, err := config.ParseDay(day)
	if err != nil {
		return nil, config.Day{}, err
	}
	draft, err := config.Edit(s.Accounts.ConfigPath)
	if err != nil {
		return nil, config.Day{}, err
	}
	cfg := draft.Config()
	switch {
	case cfg == nil && parsed == config.Day{}:
		return nil, parsed, nil
	case cfg == nil:
		return nil, config.Day{}, errNoAccount
	case cfg.Prime.Day == parsed:
		return cfg, parsed, nil
	}
	if err := draft.SetPrimeDay(day); err != nil {
		return nil, config.Day{}, err
	}
	if err := draft.Save(); err != nil {
		return nil, config.Day{}, err
	}
	s.wrote()
	logger.Info("set the priming day", "day", day)
	return draft.Config(), parsed, nil
}

// priming says the day priming spreads the 5-hour windows' resets over, or,
// with none set, asks for one, which the user may leave out.
func (r *run) priming(context.Context) error {
	if r.cfg.Prime.On() {
		r.Terminal.sayf("Priming is on, over %s.", r.cfg.Prime.Day)
		return nil
	}
	r.Terminal.sayf("Priming starts each account's 5-hour window on a schedule, so their resets are spread through your day rather than coming together. It's optional.")
	for {
		day, err := r.Terminal.ask("Your day, HH:MM-HH:MM, such as 08:00-23:00 (Enter to leave priming off): ")
		if err != nil {
			return err
		}
		if day == "" {
			r.Terminal.sayf("Priming stays off.")
			return nil
		}
		if _, err := config.ParseDay(day); err != nil {
			r.Terminal.sayf("%v.", err)
			continue
		}
		cfg, set, err := r.setDay(day)
		if err != nil {
			return err
		}
		r.cfg = cfg
		r.Terminal.sayf("Priming is on, over %s.", set)
		return nil
	}
}
