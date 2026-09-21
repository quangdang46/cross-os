package keyboard

import "sync/atomic"

// injectTag is the dwExtraInfo magic CrossOS stamps on every synthesized
// event (reserved in Spike B platform/windows/spike_b). The product adapter
// must stamp it; the callback and router must pass anything carrying it.
const injectTag = 0xC20505

// Modifier bitmask (platform-normalized by the native callback before
// reaching this package — macOS flags and Windows VK states collapse here).
const (
	ModCtrl  uint32 = 1 << 0
	ModShift uint32 = 1 << 1
	ModAlt   uint32 = 1 << 2
	ModMeta  uint32 = 1 << 3 // Cmd on macOS, Win on Windows
)

// IsSynthetic reports whether an event was injected by CrossOS itself. The
// single predicate for both the native callback filter and the router
// guard — defined once so the two can never disagree.
func IsSynthetic(extraInfo uintptr) bool {
	return extraInfo == injectTag
}

// KeyEvent is the minimal per-key transition the callback feeds the state.
// Repeat is signaled by the OS (held key re-fires keydown); DeadKey/IME mark
// composition events the matcher must pass through untouched.
type KeyEvent struct {
	KeyCode   uint32
	Down      bool // true=keydown, false=keyup
	Modifiers uint32
	DeviceID  string
	ExtraInfo uintptr // dwExtraInfo; injectTag ⇒ synthetic
	Repeat    bool    // OS-signaled auto-repeat
	DeadKey   bool    // dead-key composition event
	IME       bool    // IME composition event
}

// State is the atomic keyboard state. All fields are lock-free; Update
// performs no allocation and takes no locks — safe inside the native
// callback. Reads (Words) are equally lock-free.
//
// Coverage: 4×64-bit words = keycodes 0–255, which contains every Windows
// VK (≤254) and every macOS kVK (≤127). Keycodes >255 collapse into the
// overflow presence bit (still-held reporting degrades to "something above
// 255 held" — acceptable: no real keyboard emits those).
type State struct {
	mods    atomic.Uint32 // current modifier bitmask
	words   [4]atomic.Uint64
	extra   atomic.Uint64 // pressed keys beyond 255 (overflow bucket presence)
	repeats atomic.Uint64 // per-callback repeat counter (diagnostic)
	imeOpen atomic.Bool   // IME composition session active
}

// Update folds one KeyEvent into the state. It never decides — only records.
// It returns passThrough=true when the event must bypass matching entirely
// (synthetic self-events, dead-key/IME composition). Callers: callback
// records, then asks the router; synthetic/composition events skip the
// router and pass through.
func (s *State) Update(ev KeyEvent) (passThrough bool) {
	// Synthetic self-events: record nothing, pass through. The injection
	// that produced them was already a decision outcome; re-matching would
	// loop (inject→match→inject).
	if IsSynthetic(ev.ExtraInfo) {
		return true
	}
	// Dead-key/IME composition: the OS owns the composition session. Core
	// must not reinterpret partial composition as shortcuts.
	if ev.DeadKey || ev.IME || s.imeOpen.Load() {
		return true
	}
	if ev.Repeat {
		// v1 repeat policy (locked): repeats re-fire through the matcher
		// (return false = route normally) AND bump the diagnostic counter.
		// The matcher sees the same chord again and re-dispatches — held-key
		// behavior matches native auto-repeat. (review: cross-os-c0)
		s.repeats.Add(1)
		return false
	}
	if mod, ok := modBit(ev.KeyCode); ok {
		if ev.Down {
			s.addMod(mod)
		} else {
			s.clearMod(mod)
		}
		return false
	}
	if ev.Down {
		s.setPressed(ev.KeyCode)
	} else {
		s.clearPressed(ev.KeyCode)
	}
	return false
}

// Words returns the current modifier mask plus the raw bitmap words. The
// matcher probes with IsPressed — no closure, no heap escape: the return
// values are scalars. (review: cross-os-c0 — the previous closure-returning
// Snapshot almost certainly escaped despite the allocation-free claim.)
func (s *State) Words() (mods uint32, w0, w1, w2, w3, extra uint64) {
	return s.mods.Load(),
		s.words[0].Load(), s.words[1].Load(),
		s.words[2].Load(), s.words[3].Load(),
		s.extra.Load()
}

// IsPressed reports whether k is held, given words from Words. Pure helper
// for the fast matcher; keycodes >255 read the overflow bucket.
func IsPressed(w0, w1, w2, w3, extra uint64, k uint32) bool {
	if k < 256 {
		w := w0
		switch k >> 6 {
		case 1:
			w = w1
		case 2:
			w = w2
		case 3:
			w = w3
		}
		return w&(1<<(k&63)) != 0
	}
	return extra != 0
}

// Snapshot is the legacy closure probe kept for callers that prefer it.
// Prefer Words + IsPressed on the hot path (no escape).
func (s *State) Snapshot() (mods uint32, pressed func(uint32) bool) {
	m, w0, w1, w2, w3, ex := s.Words()
	return m, func(k uint32) bool { return IsPressed(w0, w1, w2, w3, ex, k) }
}

// Modifiers returns the current modifier bitmask.
func (s *State) Modifiers() uint32 { return s.mods.Load() }

// Repeats returns the diagnostic repeat counter.
func (s *State) Repeats() uint64 { return s.repeats.Load() }

// SetIME marks an IME composition session open/closed (called from the
// composition-event path, off the hot key path).
func (s *State) SetIME(open bool) { s.imeOpen.Store(open) }

// Reset clears all state (daemon restart / kill-switch / test setup).
func (s *State) Reset() {
	s.mods.Store(0)
	for i := range s.words {
		s.words[i].Store(0)
	}
	s.extra.Store(0)
	s.repeats.Store(0)
	s.imeOpen.Store(false)
}

// addMod atomically sets modifier bits (CAS loop — the Load|Store form
// would lose bits under concurrent modifier presses).
func (s *State) addMod(mod uint32) {
	for {
		old := s.mods.Load()
		if s.mods.CompareAndSwap(old, old|mod) {
			return
		}
	}
}

// clearMod atomically clears modifier bits.
func (s *State) clearMod(mod uint32) {
	for {
		old := s.mods.Load()
		if s.mods.CompareAndSwap(old, old&^mod) {
			return
		}
	}
}

func (s *State) setPressed(k uint32) {
	if k < 256 {
		w := &s.words[k>>6]
		bit := uint64(1) << (k & 63)
		for {
			old := w.Load()
			if w.CompareAndSwap(old, old|bit) {
				return
			}
		}
	}
	s.extra.Store(1)
}

func (s *State) clearPressed(k uint32) {
	if k < 256 {
		w := &s.words[k>>6]
		bit := uint64(1) << (k & 63)
		for {
			old := w.Load()
			if w.CompareAndSwap(old, old&^bit) {
				return
			}
		}
	}
	s.extra.Store(0)
}

// modBit maps a modifier keycode to its bit. BOTH generic and left/right
// codes are listed: some callbacks deliver VK_CONTROL/VK_SHIFT/VK_MENU,
// others deliver VK_L*/VK_R* — the state must not depend on which form the
// platform sent. (review P1: cross-os-c0 — right-Ctrl+C previously recorded
// no ModCtrl.)
func modBit(keyCode uint32) (uint32, bool) {
	switch keyCode {
	case 0x11, 0xA2, 0xA3, 0x3B, 0x3E: // VK_CONTROL L/R, kVK_Control, kVK_RightControl
		return ModCtrl, true
	case 0x10, 0xA0, 0xA1, 0x38, 0x3C: // VK_SHIFT L/R, kVK_Shift, kVK_RightShift
		return ModShift, true
	case 0x12, 0xA4, 0xA5, 0x3A, 0x3D: // VK_MENU L/R, kVK_Option, kVK_RightOption
		return ModAlt, true
	case 0x5B, 0x5C, 0x37: // VK_LWIN/VK_RWIN / kVK_Command
		// NOTE: if macOS reports right-Command as 0x36, add it when Spike A
		// lands (verify against Apple's keycode table). (review: cross-os-c0)
		return ModMeta, true
	}
	return 0, false
}
