// The first-run gate, tested through the Service rather than through the Host.
//
// TestFirstRunLandingGate in pages_test.go drives Host.OnboardingComplete
// directly, which is exactly the call production was missing — so it asserted
// the intended contract and could not fail on the bug. Everything here goes
// through the one path the shell actually uses: Service.Pages(), which is what
// App.tsx:231 calls and nothing else does.

package shell

import (
	"errors"
	"testing"
)

// gateCore wraps the shared stub so the first-run flag and the daemon's
// reachability are the only things a test in this file varies. Embedding
// rather than extending stubCore leaves every other test's fixture untouched.
type gateCore struct {
	*stubCore
	completed  bool
	failFirstN int
	calls      int
}

func (c *gateCore) OnboardingState() (OnboardingRow, error) {
	c.calls++
	if c.failFirstN > 0 && c.calls <= c.failFirstN {
		return OnboardingRow{}, errors.New("daemon unreachable")
	}
	row, err := c.stubCore.OnboardingState()
	if err != nil {
		return OnboardingRow{}, err
	}
	row.Completed = c.completed
	return row, nil
}

func gateService(t *testing.T, c *gateCore) *Service {
	t.Helper()
	h := NewHost()
	registerAll(t, h)
	return NewService(NewApp(c), h)
}

func landing(t *testing.T, s *Service) string {
	t.Helper()
	pages := s.Pages()
	if len(pages) == 0 {
		t.Fatal("no pages served")
	}
	return pages[0].ID
}

// TestFinishingTheWizardMovesTheLandingPage: the write side. The daemon write
// and the Host write are two different stores, and only the daemon's was wired,
// so the click that finishes setup changed nothing in the session the user was
// looking at.
func TestFinishingTheWizardMovesTheLandingPage(t *testing.T) {
	svc := gateService(t, &gateCore{stubCore: populatedCore()})

	if got := landing(t, svc); got != "core.onboarding" {
		t.Fatalf("a fresh profile opens %s, want the first-run page", got)
	}
	if err := svc.CompleteOnboarding(); err != nil {
		t.Fatalf("CompleteOnboarding: %v", err)
	}
	if got := landing(t, svc); got != "core.home" {
		t.Fatalf("after finishing setup the nav opens %s, want core.home", got)
	}
}

// A refused write must not move the nav. The flow is not finished, and a nav
// that stopped leading would be a second lie on top of the daemon's refusal.
func TestARefusedFinishLeavesTheLandingPageAlone(t *testing.T) {
	c := &gateCore{stubCore: populatedCore()}
	c.stubCore.failSources = errors.New("refused")
	svc := gateService(t, c)

	if err := svc.CompleteOnboarding(); err == nil {
		t.Fatal("a refusing daemon must not report the wizard finished")
	}
	if got := landing(t, svc); got != "core.onboarding" {
		t.Fatalf("after a refused finish the nav opens %s, want the first-run page", got)
	}
}

// TestRelaunchAfterFirstRunLandsOnHome: the read side, and the test that was
// missing. Host.onboarded was a process-local bool that NewHost always built
// false, so a fresh process believed the wizard was unfinished no matter what
// the daemon had persisted. A user who finished setup was sent back to it on
// every launch.
func TestRelaunchAfterFirstRunLandsOnHome(t *testing.T) {
	svc := gateService(t, &gateCore{stubCore: populatedCore(), completed: true})

	if got := landing(t, svc); got != "core.home" {
		t.Fatalf("a relaunch after a finished first run opens %s, want core.home", got)
	}
	// Never call CompleteOnboarding anywhere in this test. The whole claim is
	// that the flag reaches the nav from the daemon's own answer.
}

// TestRelaunchPreservesTheFrozenNavOrder: the landing page is the easy half.
// TestAllPagesDiscovered pins the order, but it runs on a FRESH host, so
// nothing asserted what the nav looks like AFTER the flip. A re-sort that
// moved one page and shuffled the rest would pass a landing-page-only check.
func TestRelaunchPreservesTheFrozenNavOrder(t *testing.T) {
	svc := gateService(t, &gateCore{stubCore: populatedCore(), completed: true})

	// The same list TestAllPagesDiscovered freezes. The first-run page is still
	// a page — finishing the flow does not uninstall it — it just stops
	// LEADING, so it now sits where its declared Order and id put it: same
	// Order 0 as Home, and the id tiebreak sorts "core.home" first.
	want := []string{
		"core.home", "core.onboarding", "core.profiles",
		"core.keyboard", "core.myRules", "core.windows", "core.switcher", "core.shortcuts", "core.finder",
		"core.activity", "core.observe",
		"core.extensions", "core.schemaHelp", "core.safety", "core.about",
	}
	pages := svc.Pages()
	if len(pages) != len(want) {
		t.Fatalf("after onboarding: %d pages, want %d", len(pages), len(want))
	}
	for i, id := range want {
		if pages[i].ID != id {
			t.Fatalf("served[%d]=%s, want %s (the frozen nav order)", i, pages[i].ID, id)
		}
	}
}

// TestFirstRunSeedRetriesAfterAFailedRead is the case the memo decision turns
// on. A guard that burns on the first ATTEMPT — a bare sync.Once — would fail
// this, and the user it strands is a returning one whose daemon happened to be
// slow at that one moment.
func TestFirstRunSeedRetriesAfterAFailedRead(t *testing.T) {
	c := &gateCore{stubCore: populatedCore(), completed: true, failFirstN: 1}
	svc := gateService(t, c)

	// Fail closed: no read has succeeded, so the first-run page keeps leading.
	// A returning user landing on Home here would find every control on it
	// refusing, which reads as a broken app rather than a daemon that is down.
	if got := landing(t, svc); got != "core.onboarding" {
		t.Fatalf("with the daemon unreachable the nav opens %s, want the first-run page to keep leading", got)
	}
	// The second call retries, succeeds, and flips it.
	if got := landing(t, svc); got != "core.home" {
		t.Fatalf("once the daemon answers the nav opens %s, want core.home", got)
	}
	// And a successful seed is a seed: the nav is polled, and re-reading the
	// flag on every poll would be a Core call per tick for no new information.
	svc.Pages()
	if c.calls != 2 {
		t.Fatalf("OnboardingState was called %d times, want 2 — the memo must survive a success", c.calls)
	}
}
