package export

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// lockFile is the name of the file in the state directory an import holds
// its lock on.
const lockFile = "import.lock"

// ErrImportRunning is the error of an import refused as another runs: what
// LockImport fails with where another import holds the lock, and what the
// router refuses one with while it runs another.
var ErrImportRunning = errors.New("an import is running already")

// LockImport takes the import's lock, an exclusive lock on import.lock in the
// state directory stateDir, making both where they aren't there, and returns
// what releases it. It fails at once with ErrImportRunning where another
// holds it, waiting for nothing. The file is never removed: the lock goes
// with the process that held it, however that ends, so a lock left behind
// needs no cleaning up.
func LockImport(stateDir string) (unlock func(), err error) {
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, fmt.Errorf("take the import's lock: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(stateDir, lockFile), os.O_RDONLY|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("take the import's lock: %w", err)
	}
	held, err := tryLock(f)
	if err != nil || !held {
		_ = f.Close()
	}
	switch {
	case err != nil:
		return nil, fmt.Errorf("take the import's lock: %w", err)
	case !held:
		return nil, ErrImportRunning
	}
	return func() { _ = f.Close() }, nil
}
