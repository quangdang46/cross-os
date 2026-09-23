//go:build darwin

// macOS virtual keycode → internal (Windows VK) keycode, for the tap
// decision path (bead cross-os-uok).
//
// Why this exists: CGEvent reports macOS virtual keycodes (kVK_*,
// HIToolbox/Events.h) — C is 0x08, Space 0x31, Left 0x7B. The rule table is
// authored in Windows virtual-key codes (C 0x43, Space 0x20, Left 0x25) so
// ONE table describes the Windows muscle memory regardless of host. Without
// this translation the router's exact-equality match can never succeed from
// the live tap, and the app silently passes every key through.
//
// The mapping is not bijective over the whole keyboard (macOS W is 0x0D,
// which is Windows VK_RETURN), so it covers the keys the rules claim plus
// the function/arrow/navigation blocks. Anything unmapped passes through
// unchanged and matches nothing — a wrong mapping would be worse than none.
// TestMain-side coverage check in cmd/crossos asserts every rule keycode
// resolves, so a new rule cannot silently become unreachable.
package adapter

// winVKFromMac maps macOS kVK_* codes to the Windows VK codes the rule
// table uses. Function keys are listed individually because the macOS F-row
// is not contiguous (F1=0x7A, F2=0x78, F3=0x63 …).
var winVKFromMac = map[uint16]uint16{
	// Letters and keys the builtin rules claim.
	0x08: 0x43, // kVK_ANSI_C     -> 'C'
	0x23: 0x50, // kVK_ANSI_P     -> 'P'
	0x31: 0x20, // kVK_Space      -> VK_SPACE
	0x24: 0x0D, // kVK_Return     -> VK_RETURN

	// Arrows.
	0x7B: 0x25, // kVK_LeftArrow  -> VK_LEFT
	0x7C: 0x27, // kVK_RightArrow -> VK_RIGHT
	0x7E: 0x26, // kVK_UpArrow    -> VK_UP
	0x7D: 0x28, // kVK_DownArrow  -> VK_DOWN

	// Navigation the §6.2 shortcut table references.
	0x73: 0x24, // kVK_Home       -> VK_HOME
	0x77: 0x23, // kVK_End        -> VK_END
	0x74: 0x21, // kVK_PageUp     -> VK_PRIOR
	0x79: 0x22, // kVK_PageDown   -> VK_NEXT
	0x35: 0x1B, // kVK_Escape     -> VK_ESCAPE
	0x33: 0x08, // kVK_Delete     -> VK_BACK
	0x30: 0x09, // kVK_Tab        -> VK_TAB

	// Function row.
	0x7A: 0x70, // kVK_F1  -> VK_F1
	0x78: 0x71, // kVK_F2  -> VK_F2
	0x63: 0x72, // kVK_F3  -> VK_F3
	0x76: 0x73, // kVK_F4  -> VK_F4
	0x60: 0x74, // kVK_F5  -> VK_F5
	0x61: 0x75, // kVK_F6  -> VK_F6
	0x62: 0x76, // kVK_F7  -> VK_F7
	0x64: 0x77, // kVK_F8  -> VK_F8
	0x65: 0x78, // kVK_F9  -> VK_F9
	0x6D: 0x79, // kVK_F10 -> VK_F10
	0x67: 0x7A, // kVK_F11 -> VK_F11
	0x6F: 0x7B, // kVK_F12 -> VK_F12
}

// ToWinKeycode translates a macOS virtual keycode to the internal Windows VK
// convention. Unmapped codes return (code, false): the caller passes the
// original through, which matches no rule — the safe outcome.
func ToWinKeycode(mac uint16) (uint16, bool) {
	win, ok := winVKFromMac[mac]
	if !ok {
		return mac, false
	}
	return win, true
}
