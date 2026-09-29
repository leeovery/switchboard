package setup

import (
	"context"

	"github.com/leeovery/switchboard/internal/config"
)

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
		return r.setDay(day)
	}
}

// setDay has priming spread the resets over day, in the config.
func (r *run) setDay(day string) error {
	draft, err := config.Edit(r.registry.ConfigPath)
	if err != nil {
		return err
	}
	if err := draft.SetPrimeDay(day); err != nil {
		return err
	}
	if err := draft.Save(); err != nil {
		return err
	}
	r.cfg, r.changed = draft.Config(), true
	logger.Info("set the priming day", "day", day)
	r.Terminal.sayf("Priming is on, over %s.", r.cfg.Prime.Day)
	return nil
}
