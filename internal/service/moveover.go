package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/leeovery/switchboard/internal/routercopy"
)

// MoveOver moves the router launchd runs over to its own copy of
// switchboard, as one started from a plist written before the copy does as
// it starts. It makes the copy of binary, the switchboard the router runs,
// its links followed, and writes the plist in place afresh to run the copy,
// as Install writes it for the options the plist was installed with, every
// variable it sets kept, as Options reads them back. launchd is left alone:
// it reads the plist only as it loads the service, so the router it starts
// next runs the copy. Then restart restarts the router into the copy, and
// returns only where it couldn't, when the plist is put back as it was, so
// launchd never starts a copy the router couldn't become. It fails, the
// plist as it was, where the plist can't be read back or written, the copy
// can't be made, or restart returns an error.
func (s *Service) MoveOver(ctx context.Context, binary string, restart func(own routercopy.Copy) error) error {
	was, err := os.ReadFile(s.Plist())
	if err != nil {
		return fmt.Errorf("read the service's plist: %w", err)
	}
	a, err := s.afresh(ctx, was)
	if err != nil {
		return err
	}
	if err := a.placeCopy(binary); err != nil {
		return err
	}
	if err := s.write(a); err != nil {
		return err
	}
	logger.Info("wrote the service's plist afresh, to run the router's copy", "plist", s.Plist(), "program", strings.Join(a.Program, " "))
	if err := restart(a.own); err != nil {
		err = fmt.Errorf("restart into the router's copy, %s: %w", a.own.Path(), err)
		if back := s.writePlist(was); back != nil {
			return errors.Join(err, fmt.Errorf("put the service's plist back as it was: %w", back))
		}
		return err
	}
	return nil
}

// afresh is the LaunchAgent the plist in place, was as it was read, is
// written afresh as: as Install writes it for the options the plist was
// installed with, in the environment it keeps, as the router it runs sees
// it.
func (s *Service) afresh(ctx context.Context, was []byte) (agent, error) {
	installed, err := s.entriesOf(ctx, was)
	if err != nil {
		return agent{}, err
	}
	opts, err := s.optionsIn(installed)
	if err != nil {
		return agent{}, err
	}
	seen, err := s.For(opts)
	if err != nil {
		return agent{}, err
	}
	return seen.describe(opts)
}
