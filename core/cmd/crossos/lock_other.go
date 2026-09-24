//go:build !windows && (!unix || solaris || aix)

// Where neither flock(2) (solaris, aix) nor LockFileEx (lock_windows.go)
// exists, the daemon reports honestly rather than pretending to lock.
// Returning nil here would fail OPEN: a second daemon would read the lock
// file as free and unlink a running daemon's socket. Same never-fake-success
// rule as autostart_other.go.
package main

import "os"

func lockExclusive(f *os.File) error {
	return errLockUnsupported
}
