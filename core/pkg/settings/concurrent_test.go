package settings

import (
	"path/filepath"
	"sync"
	"testing"
)

// TestConcurrentSettersDoNotCrash: two goroutines writing different fields
// must not abort the process. persistLocked used to alias the live maps under
// RLock and walk them after releasing it, which Go treats as a fatal
// concurrent map access — unrecoverable, so no test could observe it coming.
func TestConcurrentSettersDoNotCrash(t *testing.T) {
	s, err := New(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	known := func(string) bool { return true }
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _ = s.SetRuleEnabled("some.rule", true, known) }()
		go func() { defer wg.Done(); _ = s.SetPanicStopped(true) }()
	}
	wg.Wait()
	if got := s.PanicStopped(); !got {
		t.Fatal("the last write did not land")
	}
}
