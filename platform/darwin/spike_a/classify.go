// Decision core for Spike A — pure, no syscalls, cross-platform testable.
//
// Mirrors spike B's classifyKey separation: the CGEventTap callback adds only
// OS plumbing (tap location checks, timeout event branch, re-enable call);
// everything decidable lives here.
package spikea

// Decision is what the tap callback returns for one key event.
type Decision int

const (
	// DecisionPass returns the original event untouched.
	DecisionPass Decision = iota
	// DecisionSuppress swallows the original, no replacement.
	DecisionSuppress
	// DecisionSuppressReplace swallows the original AND emits a replacement.
	DecisionSuppressReplace
)

// Key codes (CGKeyCode / kVK_*) relevant to the spike.
const (
	KeyC        = 0x08 // kVK_ANSI_C
	KeyF9       = 0x65 // kVK_F9 (replacement target, never Ctrl+C → no loop)
	FlagControl = 0x01
)

// ClassifyCtrlC decides what to do with a keydown: suppress (+replace) only
// the Ctrl+C combination the spike targets; everything else passes through.
// ctrl reports whether the Control modifier flag was set on the event;
// keyDown gates on key-down (key-up always passes — suppression is down-only,
// the up is meaningless without its down).
func ClassifyCtrlC(keyCode uint16, ctrl, keyDown bool, replace bool) Decision {
	if !keyDown || !ctrl || keyCode != KeyC {
		return DecisionPass
	}
	if replace {
		return DecisionSuppressReplace
	}
	return DecisionSuppress
}

// InjectTag is the dwExtraInfo/source-state tag reserved for
// CrossOS-synthesized input. The spike emits F9 (never Ctrl+C) so no loop is
// possible, but dependent bead cross-os-wge (synthetic self-ignore) must PASS
// anything carrying this tag. Reserved here so the convention is locked at
// spike time, same as spike B's injectTag.
const InjectTag = 0xC20505
