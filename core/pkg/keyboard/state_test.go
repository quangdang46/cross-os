package keyboard

import (
	"sync"
	"testing"
)

func TestChordCtrlC(t *testing.T) {
	var s State
	// Ctrl down → C down: matcher would see mods=Ctrl + C pressed.
	if pt := s.Update(KeyEvent{KeyCode: 0x11, Down: true}); pt {
		t.Fatal("modifier down must not pass through")
	}
	if pt := s.Update(KeyEvent{KeyCode: 0x43, Down: true, Modifiers: ModCtrl}); pt {
		t.Fatal("chord key must not pass through")
	}
	mods, pressed := s.Snapshot()
	if mods&ModCtrl == 0 {
		t.Fatalf("mods: got %#x, want Ctrl set", mods)
	}
	if !pressed(0x43) {
		t.Fatal("C not recorded as pressed")
	}
	// Release in reverse: state clears.
	s.Update(KeyEvent{KeyCode: 0x43, Down: false})
	s.Update(KeyEvent{KeyCode: 0x11, Down: false})
	mods, pressed = s.Snapshot()
	if mods != 0 {
		t.Fatalf("mods after release: %#x", mods)
	}
	if pressed(0x43) {
		t.Fatal("C still pressed after keyup")
	}
}

func TestModifierOnly(t *testing.T) {
	var s State
	// Bare modifier press/release tracks the mask, nothing else.
	s.Update(KeyEvent{KeyCode: 0x10, Down: true})
	if got := s.Modifiers(); got&ModShift == 0 {
		t.Fatalf("shift not tracked: %#x", got)
	}
	s.Update(KeyEvent{KeyCode: 0x10, Down: false})
	if got := s.Modifiers(); got != 0 {
		t.Fatalf("shift stuck: %#x", got)
	}
}

func TestRepeatCountedNotMatched(t *testing.T) {
	var s State
	s.Update(KeyEvent{KeyCode: 0x43, Down: true})
	s.Update(KeyEvent{KeyCode: 0x43, Down: true, Repeat: true})
	s.Update(KeyEvent{KeyCode: 0x43, Down: true, Repeat: true})
	if got := s.Repeats(); got != 2 {
		t.Fatalf("repeats: got %d, want 2", got)
	}
	// Repeat must not disturb pressed state.
	_, pressed := s.Snapshot()
	if !pressed(0x43) {
		t.Fatal("repeat cleared pressed state")
	}
}

func TestDeadKeyIMEPassthrough(t *testing.T) {
	var s State
	if pt := s.Update(KeyEvent{KeyCode: 0x43, Down: true, DeadKey: true}); !pt {
		t.Fatal("dead-key event must pass through")
	}
	if pt := s.Update(KeyEvent{KeyCode: 0x43, Down: true, IME: true}); !pt {
		t.Fatal("IME event must pass through")
	}
	// Composition session open: everything passes until closed.
	s.SetIME(true)
	if pt := s.Update(KeyEvent{KeyCode: 0x43, Down: true}); !pt {
		t.Fatal("event during IME session must pass through")
	}
	s.SetIME(false)
	if pt := s.Update(KeyEvent{KeyCode: 0x43, Down: true}); pt {
		t.Fatal("event after IME close must not pass through")
	}
}

func TestSyntheticSelfIgnore(t *testing.T) {
	var s State
	// A CrossOS-injected event (injectTag) records nothing and passes.
	if pt := s.Update(KeyEvent{KeyCode: 0x78, Down: true, ExtraInfo: injectTag}); !pt {
		t.Fatal("synthetic event must pass through")
	}
	mods, pressed := s.Snapshot()
	if mods != 0 || pressed(0x78) {
		t.Fatal("synthetic event polluted state")
	}
	// IsSynthetic is the shared predicate: tag ⇒ true, zero ⇒ false.
	if !IsSynthetic(injectTag) || IsSynthetic(0) {
		t.Fatal("IsSynthetic predicate wrong")
	}
}

func TestHighKeycodesExact(t *testing.T) {
	// Regression (review P0: cross-os-c0): letters are 0x41-0x5A (65-90),
	// previously collapsed into one overflow bit — hold C, probe F9 must be
	// false; hold both, release C, F9 must stay held.
	var s State
	s.Update(KeyEvent{KeyCode: 0x43, Down: true})
	_, pressed := s.Snapshot()
	if !pressed(0x43) {
		t.Fatal("C not recorded")
	}
	if pressed(0x78) {
		t.Fatal("false positive: F9 reported while only C held")
	}
	s.Update(KeyEvent{KeyCode: 0x78, Down: true})
	s.Update(KeyEvent{KeyCode: 0x43, Down: false})
	_, pressed = s.Snapshot()
	if pressed(0x43) {
		t.Fatal("C still held after release")
	}
	if !pressed(0x78) {
		t.Fatal("F9 lost after C released")
	}
	// Words + IsPressed hot path agrees with the closure probe.
	m, w0, w1, w2, w3, ex := s.Words()
	if m != 0 {
		t.Fatalf("mods: %#x", m)
	}
	if !IsPressed(w0, w1, w2, w3, ex, 0x78) || IsPressed(w0, w1, w2, w3, ex, 0x43) {
		t.Fatal("Words/IsPressed disagrees with Snapshot")
	}
}

func TestRightModifiers(t *testing.T) {
	// Right-Ctrl+C must set ModCtrl (review P1: cross-os-c0).
	var s State
	s.Update(KeyEvent{KeyCode: 0xA3, Down: true}) // VK_RCONTROL
	if got := s.Modifiers(); got&ModCtrl == 0 {
		t.Fatalf("right-ctrl not tracked: %#x", got)
	}
	s.Update(KeyEvent{KeyCode: 0x43, Down: true, Modifiers: ModCtrl})
	mods, pressed := s.Snapshot()
	if mods&ModCtrl == 0 || !pressed(0x43) {
		t.Fatal("right-Ctrl+C chord not recorded")
	}
}

func TestConcurrentModifiersNoLostBits(t *testing.T) {
	// CAS modifier RMW: concurrent distinct-modifier presses must not lose
	// bits (review P1: cross-os-c0).
	var s State
	var wg sync.WaitGroup
	mods := []uint32{0x11, 0x10, 0x12, 0x5B}
	for _, m := range mods {
		wg.Add(1)
		go func(k uint32) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				s.Update(KeyEvent{KeyCode: k, Down: true})
			}
		}(m)
	}
	wg.Wait()
	if got := s.Modifiers(); got != ModCtrl|ModShift|ModAlt|ModMeta {
		t.Fatalf("lost modifier bits: %#x", got)
	}
}

func TestConcurrentUpdatesRaceFree(t *testing.T) {
	// Callback-path lock-freedom: hammer from goroutines under -race.
	var s State
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				k := uint32(0x30 + (g+i)%16)
				s.Update(KeyEvent{KeyCode: k, Down: true})
				s.Snapshot()
				s.Update(KeyEvent{KeyCode: k, Down: false})
			}
		}(g)
	}
	wg.Wait()
}

func TestReset(t *testing.T) {
	var s State
	s.Update(KeyEvent{KeyCode: 0x11, Down: true})
	s.Update(KeyEvent{KeyCode: 0x43, Down: true})
	s.SetIME(true)
	s.Reset()
	mods, pressed := s.Snapshot()
	if mods != 0 || pressed(0x43) {
		t.Fatal("reset left state behind")
	}
	if pt := s.Update(KeyEvent{KeyCode: 0x43, Down: true}); pt {
		t.Fatal("IME flag survived reset")
	}
}
