package adapter

import "errors"

// errTapNotWired marks the native bridge not yet linked (spikes proved the
// shape; the C-ABI/DLL wiring lands behind this error, never as silent pass).
var errTapNotWired = errors.New("adapter: native bridge not yet wired (spike-proven shape, link pending)")

// errZeroWindow rejects move/resize against a zero window id pre-bridge.
var errZeroWindow = errors.New("adapter: zero window id")

// errNoFocusedWindow reports that there is no focused window to act on.
//
// **It is not a permission problem.** The accessibility grant can be present
// and correct while the Accessibility plane has no window to name — a locked
// screen answers with the application element, and an application with no
// windows answers the same way. So this keeps its own identity: a caller
// that mapped it onto ErrPermissionDenied would send the user to System
// Settings to grant a permission they already granted.
var errNoFocusedWindow = errors.New("adapter: no focused window")

// errAXBusy reports that the Accessibility plane did not answer in time.
//
// **IT IS NOT A PERMISSION PROBLEM.** kAXErrorCannotComplete is what the
// AX server answers when the application it is talking to is busy, or when
// the client's own messaging timeout expires — and the bridge's bound is
// 0.15 seconds, so a busy application answers that way on a machine whose
// grant is present and correct.
//
// Sending the user to System Settings for it would be the same lie as
// errNoFocusedWindow: a permission they have already granted. It keeps its
// own identity because "try again" and "grant access" are different
// instructions.
var errAXBusy = errors.New("adapter: the application did not answer in time")

// ErrFocusRefused reports that the window was raised but the platform did
// not hand the activation over.
//
// **The raise worked.** The window is in front; the application did not
// become the frontmost one, so the keyboard stayed where it was. macOS
// swallows an activation now and then — measured on this machine, about one
// time in twelve — and it is the platform's call, not the daemon's.
//
// It keeps its own identity because the caller has to tell it apart from a
// failure: a refusal is a window the user can see and a keyboard that is
// somewhere else, and a switcher that says "could not focus" about it is
// more useful than one that says "focused" and lets the next keystroke go
// to another application.
//
// It is exported because the refusal crosses the wire as a message, and a
// caller that reads the message needs to recognise this one.
var ErrFocusRefused = errors.New("adapter: the activation was refused")
