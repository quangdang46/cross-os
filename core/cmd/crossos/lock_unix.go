//go:build unix && !solaris && !aix

// Advisory file locking for the daemon's single-writer guarantee (bead
// cross-os-jn1). syscall.Flock exists on every unix GOOS except solaris and
// aix, so those two fall through to lock_other.go, which refuses to start
// rather than serve unlocked.
package main

import (
	"os"
	"syscall"
)

// lockExclusive is the same lock syscall.Flock(LOCK_EX|LOCK_NB) gave the
// daemon before this was split per GOOS: exclusive, non-blocking, held until
// the fd closes. Contention is reported as errLockHeld so the caller can say
// "stop the leftover daemon" rather than a bare errno; any other errno is
// passed through unchanged, because a lock that failed for some other reason
// is not a running daemon.
func lockExclusive(f *os.File) error {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == syscall.EWOULDBLOCK || err == syscall.EAGAIN {
		return errLockHeld
	}
	return err
}
