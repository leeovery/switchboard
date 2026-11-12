package setup

import (
	"slices"

	"github.com/leeovery/switchboard/internal/config"
)

// Primed is the Priming step as the config has it: the day priming spreads
// the 5-hour windows' resets over, the zero Day where it sets none, and
// priming is off; and whether the step is skipped, as SkipsPriming has it.
type Primed struct {
	Day     config.Day
	Skipped bool
}

// Priming is the Priming step as the config has it: the zero Primed where
// there's no config.
func (s *Setup) Priming() (Primed, error) {
	draft, err := config.Edit(s.Accounts.ConfigPath, s.Accounts.HomeDir)
	if err != nil || draft.Config() == nil {
		return Primed{}, err
	}
	cfg := draft.Config()
	plans := make([]string, len(cfg.Accounts))
	for i, a := range cfg.Accounts {
		plans[i] = a.Plan
	}
	return Primed{Day: cfg.Prime.Day, Skipped: SkipsPriming(plans)}, nil
}

// SkipsPriming reports whether setup skips its Priming step for accounts on
// the plans given, one an account, each one of config.Plans or "": where
// there's one at least, and every one is on Ultra, as a verified Ultra
// account has no 5-hour window to start. An unverified Ultra account keeps a
// 5-hour window for a while, and isn't offered priming for it.
func SkipsPriming(plans []string) bool {
	return len(plans) > 0 && !slices.ContainsFunc(plans, func(plan string) bool { return plan != config.PlanUltra })
}

// SkipsPriming reports whether setup skips its Priming step for the
// accounts reviewed, as SkipsPriming has it of their plans.
func (r Review) SkipsPriming() bool {
	plans := make([]string, len(r.Accounts))
	for i, a := range r.Accounts {
		plans[i] = a.Plan
	}
	return SkipsPriming(plans)
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
	draft, err := config.Edit(s.Accounts.ConfigPath, s.Accounts.HomeDir)
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
