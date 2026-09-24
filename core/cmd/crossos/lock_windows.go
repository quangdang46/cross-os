//go:build windows

// Windows half of the single-writer lock (bead cross-os-jn1). Windows has no
// flock(2); the equivalent is LockFileEx, an exclusive byte-range lock. It
// is resolved through syscall.NewLazyDLL at run time because the stdlib
// syscall package does not export it (only x/sys/windows does), which keeps
// the daemon on the stdlib-only dependency set its other Windows seam
// (adapter/dll_windows.go) already uses.
//
// This is deliberately a real lock and never a no-op stub. A stub that
// returned nil would fail OPEN: a second daemon would read the lock as free,
// unlink the live daemon's socket and serve beside it — precisely the theft
// cross-os-jn1 exists to prevent. A platform that cannot lock refuses.
package main

import (
	"os"
	"syscall"
	"unsafe"
)

var procLockFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("LockFileEx")

// LockFileEx flag bits (winbase.h). Neither is exported by Go's syscall
// package, and neither changes meaning between Windows versions.
const (
	lockfileFailImmediately = 0x00000001 // report contention now, never block
	lockfileExclusiveLock   = 0x00000002 // exclusive, not shared
)

// errorLockViolation is Win32 ERROR_LOCK_VIOLATION (33), returned when
// another handle already holds the byte range. Not exported by syscall.
const errorLockViolation = syscall.Errno(33)

// lockExclusive takes an exclusive, non-blocking lock on the first byte of f
// — the "one writer, name the loser" contract flock gives on unix. The lock
// lives as long as the handle: listenSocket leaks the fd for the process
// lifetime, so no unlock is called, and closing the fd would release the
// lock and let a second daemon in.
func lockExclusive(f *os.File) error {
	// The OVERLAPPED carries the range to lock (offset 0, length 1) and must
	// stay valid for the call. f is not opened FILE_FLAG_OVERLAPPED, so the
	// handle is synchronous and the lock is held once this returns — a
	// heap allocation outliving the call is not required.
	ol := new(syscall.Overlapped)
	locked, _, err := procLockFileEx.Call(
		f.Fd(),
		uintptr(lockfileExclusiveLock|lockfileFailImmediately),
		0, // dwReserved, must be zero
		1, // nNumberOfBytesToLockLow: the pid record's first byte
		0, // nNumberOfBytesToLockHigh
		uintptr(unsafe.Pointer(ol)),
	)
	if locked != 0 {
		return nil
	}
	if errno, ok := err.(syscall.Errno); ok && errno == errorLockViolation {
		return errLockHeld
	}
	return err
}
