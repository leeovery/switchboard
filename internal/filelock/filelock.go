// Package filelock owns switchboard's file locks: a lock held on an open
// file, exclusive or shared, waited for or tried without waiting, against
// every other open of the file, in this process or another. A lock goes as
// its file closes, however the process holding it ends, so the file it's
// held on is left in place: a lock on a file removed and made again could be
// held twice at once. Where there's no flock, outside Unix, nothing is
// locked, and every lock is taken: what each guards is macOS's alone.
package filelock

import "os"

// Take opens the file at path, making it, the user's alone, where it isn't
// there, and takes an exclusive lock on it, waiting while another holds one.
// unlock lets the lock go, and may be called more than once.
func Take(path string) (unlock func(), err error) {
	f, err := open(path)
	if err != nil {
		return nil, err
	}
	if err := Lock(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	return closer(f), nil
}

// TryTake takes the lock on the file at path as Take does, but where another
// holds one, reports false at once, holding nothing.
func TryTake(path string) (unlock func(), ok bool, err error) {
	f, err := open(path)
	if err != nil {
		return nil, false, err
	}
	taken, err := TryLock(f)
	if err != nil || !taken {
		_ = f.Close()
		return nil, false, err
	}
	return closer(f), true, nil
}

// TakeDir takes an exclusive lock on the directory dir, waiting while
// another holds one, and returns what lets it go. Where it can't take one,
// it carries on without, its unlock letting nothing go.
func TakeDir(dir string) (unlock func()) {
	d, err := os.Open(dir)
	if err != nil {
		return func() {}
	}
	if err := Lock(d); err != nil {
		_ = d.Close()
		return func() {}
	}
	return closer(d)
}

// open opens the file at path to hold a lock on, making it, the user's
// alone, where it isn't there.
func open(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|os.O_CREATE, 0o600)
}

// closer is what lets go the lock held on f: closing it.
func closer(f *os.File) func() {
	return func() { _ = f.Close() }
}
