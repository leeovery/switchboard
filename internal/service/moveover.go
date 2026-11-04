package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/routercopy"
)

// ErrPutOff is what MoveOver fails with, wrapped, saying why, where the
// router isn't to move over this time, but to carry on as it is: service
// install is changing the service, or the binary the router would copy isn't
// known to be the one it runs.
var ErrPutOff = errors.New("moving over put off")

// MovingOver is what a router moves over to its own copy of switchboard
// with, as MoveOver moves it.
type MovingOver struct {
	// Binary is the switchboard the router runs, by the path it was started
	// as, which the copy is made of, its links followed, and Running that
	// binary as it was found as the router started, as router.Watch finds
	// it: the copy is made only of that very file.
	Binary  string
	Running router.Watched
	// Restart restarts the router into the copy, own, in place, and returns
	// only where it couldn't.
	Restart func(own routercopy.Copy) error
}

// MoveOver moves the router launchd runs over to its own copy of
// switchboard, as one started from a plist written before the copy does as
// it starts. It makes the copy of m.Binary, and writes the plist in place
// afresh to run the copy, as Install writes it for the options the plist was
// installed with, every variable it sets kept, as Options reads them back.
// launchd is left alone: it reads the plist only as it loads the service, so
// the router it starts next runs the copy. Then m.Restart restarts the router
// into the copy, and returns only where it couldn't, when the plist is put
// back as it was, so launchd never starts a copy the router couldn't become.
//
// It holds the copy's lock throughout, which Install takes as it changes the
// copy and the plist, so neither writes over what the other is writing. It
// fails with ErrPutOff, touching nothing, where the binary wasn't there as
// the router started, as for a moment while an upgrade moves its link on, so
// which file it runs isn't known; where Install holds the lock; where the
// plist in place keeps the copy elsewhere than the router's environment
// does, as one installed since the router started; or where the file copied
// isn't the one the router runs, as when an upgrade moved its link on as it
// started, which the router takes up as any other, and moves over then.
// Otherwise it fails, the plist as it was, where the plist can't be read
// back or written, the copy can't be made, or m.Restart returns an error.
func (s *Service) MoveOver(ctx context.Context, m MovingOver) error {
	if !m.Running.There() {
		return fmt.Errorf("%w: the binary it runs as, %s, couldn't be found as it started", ErrPutOff, m.Binary)
	}
	own, err := s.routerCopy()
	if err != nil {
		return fmt.Errorf("find the router's copy of switchboard: %w", err)
	}
	unlock, ok, err := own.TryLock()
	switch {
	case err != nil:
		return err
	case !ok:
		return fmt.Errorf("%w: service install is setting the service right", ErrPutOff)
	}
	defer unlock()
	was, err := os.ReadFile(s.Plist())
	if err != nil {
		return fmt.Errorf("read the service's plist: %w", err)
	}
	a, err := s.afresh(ctx, was)
	if err != nil {
		return err
	}
	if a.own.Path() != own.Path() {
		return fmt.Errorf("%w: the service's plist keeps the router's copy at %s, not %s, as one installed since the router started", ErrPutOff, a.own.Path(), own.Path())
	}
	if err := m.copyInto(own); err != nil {
		return err
	}
	if err := s.write(a); err != nil {
		return err
	}
	logger.Info("wrote the service's plist afresh, to run the router's copy", "plist", s.Plist(), "program", strings.Join(a.Program, " "))
	if err := m.Restart(own); err != nil {
		err = fmt.Errorf("restart into the router's copy, %s: %w", own.Path(), err)
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

// copyInto makes own, the router's copy, of m.Binary, having removed what
// copies cut short left beside it, but only where the file copied is the one
// the router runs, as m.Running says: otherwise it removes what it copied,
// and fails with ErrPutOff.
func (m MovingOver) copyInto(own routercopy.Copy) error {
	if err := own.RemoveLeftovers(); err != nil {
		return err
	}
	staged, err := own.Stage(m.Binary)
	if err != nil {
		return err
	}
	if !m.Running.Is(staged.From()) {
		putOff := fmt.Errorf("%w: %s leads to another switchboard than the one running, as after another upgrade", ErrPutOff, m.Binary)
		if err := staged.Discard(); err != nil {
			return errors.Join(putOff, err)
		}
		return putOff
	}
	return staged.Commit()
}
