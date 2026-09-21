package ctx

import (
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeResolver struct {
	calls int
	err   error
}

func (f *fakeResolver) Resolve(fast FastContext, req Requires) (LazyContext, error) {
	f.calls++
	if f.err != nil {
		return LazyContext{}, f.err
	}
	lz := LazyContext{}
	if req.Selection {
		lz.Selection = SelectionInfo{Items: []string{"/tmp/a"}, Container: "/tmp"}
	}
	if req.Cursor {
		lz.Cursor = CursorInfo{X: 10, Y: 20}
	}
	if req.Element {
		lz.FocusedElement = "AXWindow/AXButton"
	}
	return lz, nil
}

func (f *fakeResolver) Name() string { return "fake" }

func TestReadIsCacheOnly(t *testing.T) {
	// Keydown-path read: no resolver installed, must still return full fast.
	c := NewCache(nil)
	c.UpdateApp(ApplicationInfo{BundleID: "com.apple.finder", AppMode: AppModeNative})
	c.UpdateWindow(WindowInfo{WindowID: "w1", Role: "AXWindow"})
	c.UpdateDevice("kbd-1")
	f := c.Read()
	if f.AppID != "com.apple.finder" || f.AppMode != AppModeNative ||
		f.WindowID != "w1" || f.DeviceID != "kbd-1" {
		t.Fatalf("cache read wrong: %+v", f)
	}
}

func TestReadLatency(t *testing.T) {
	// Bead criterion: keydown-path context read under 1ms (cache hit).
	c := NewCache(nil)
	c.UpdateApp(ApplicationInfo{BundleID: "x", AppMode: AppModeNative})
	start := time.Now()
	const n = 10000
	for i := 0; i < n; i++ {
		_ = c.Read()
	}
	per := time.Since(start) / n
	t.Logf("per-read: %v", per)
	if per >= time.Millisecond {
		t.Fatalf("read too slow: %v >= 1ms", per)
	}
}

func TestLazyOnlyFiresOnRequires(t *testing.T) {
	fr := &fakeResolver{}
	c := NewCache(fr)
	c.UpdateApp(ApplicationInfo{BundleID: "x"})
	// No requires → no lookup, nil lazy, zero resolver calls.
	if lz := c.ResolveLazy(Requires{}); lz != nil {
		t.Fatal("lazy resolved without requires")
	}
	if fr.calls != 0 {
		t.Fatalf("resolver called without requires: %d", fr.calls)
	}
	// Requires selection → exactly one lookup with selection filled.
	lz := c.ResolveLazy(Requires{Selection: true})
	if lz == nil || len(lz.Selection.Items) != 1 {
		t.Fatalf("selection not resolved: %+v", lz)
	}
	if fr.calls != 1 {
		t.Fatalf("calls: got %d, want 1", fr.calls)
	}
}

func TestLazyNilWithoutResolver(t *testing.T) {
	c := NewCache(nil)
	if lz := c.ResolveLazy(Requires{Selection: true}); lz != nil {
		t.Fatal("expected nil without resolver")
	}
}

func TestLazyErrorYieldsNil(t *testing.T) {
	fr := &fakeResolver{err: errors.New("AX busy")}
	c := NewCache(fr)
	if lz := c.ResolveLazy(Requires{Cursor: true}); lz != nil {
		t.Fatal("expected nil on resolver error")
	}
}

func TestAppModeRouting(t *testing.T) {
	// Terminal executables classify terminal; unknown stays native.
	var sc SeedClassifier
	cases := []struct {
		exe  string
		mode AppMode
		cat  AppCategory
	}{
		{"ghostty", AppModeTerminal, AppTerminal},
		{"com.mitchellh.ghostty", AppModeTerminal, AppTerminal},
		{"mstsc.exe", AppModeRemote, AppRemote},
		{"vmware-vmx", AppModeVM, AppRemote},
		{"firefox", AppModeNative, AppBrowser},
		{"my-random-app", AppModeNative, AppUser},
		{"", AppModeNative, AppUser},
	}
	for _, tc := range cases {
		mode, cat := sc.Classify("", tc.exe)
		if mode != tc.mode || cat != tc.cat {
			t.Fatalf("%q: got %q/%q, want %q/%q",
				tc.exe, mode, cat, tc.mode, tc.cat)
		}
	}
}

func TestSessionInFull(t *testing.T) {
	// §3.3 bead criterion: SessionInfo present in full Context.
	c := NewCache(nil)
	c.UpdateSession(SessionInfo{User: "u", Locked: false})
	f := c.Full(Requires{})
	if f.Session.User != "u" {
		t.Fatalf("session missing: %+v", f.Session)
	}
	if f.Lazy != nil {
		t.Fatal("lazy should be nil without requires")
	}
}

func TestFullWithConcurrentWriter(t *testing.T) {
	// Regression (review P0: cross-os-c0): Full nested two RLocks and
	// deadlocked when an Update landed between them. Hammer Full while a
	// writer updates — must always return.
	c := NewCache(&fakeResolver{})
	c.UpdateApp(ApplicationInfo{BundleID: "x"})
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				c.UpdateApp(ApplicationInfo{BundleID: "x"})
			}
		}
	}()
	done := make(chan bool)
	go func() {
		for i := 0; i < 200; i++ {
			f := c.Full(Requires{Selection: true})
			if f.Fast.AppID != "x" {
				t.Errorf("bad fast: %+v", f.Fast)
				break
			}
		}
		done <- true
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Full deadlocked under concurrent writer")
	}
	close(stop)
	wg.Wait()
}

func TestModifiersOverlay(t *testing.T) {
	// Modifiers are caller-filled: Read returns zero, WithModifiers overlays.
	c := NewCache(nil)
	c.UpdateApp(ApplicationInfo{BundleID: "x"})
	if got := c.Read().Modifiers; got != 0 {
		t.Fatalf("Read().Modifiers should be zero, got %#x", got)
	}
	f := c.Read().WithModifiers(0x05)
	if f.Modifiers != 0x05 || f.AppID != "x" {
		t.Fatalf("overlay wrong: %+v", f)
	}
}

func TestConcurrentReadUpdate(t *testing.T) {
	c := NewCache(nil)
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				_ = c.Read()
			}
		}()
		go func(n int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				c.UpdateApp(ApplicationInfo{BundleID: "x", AppMode: AppModeNative})
			}
		}(g)
	}
	wg.Wait()
	u, r := c.Stats()
	if u == 0 || r == 0 {
		t.Fatalf("counters wrong: updates=%d reads=%d", u, r)
	}
}
