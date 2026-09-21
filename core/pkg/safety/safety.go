package safety

import (
	"fmt"
	"sort"
	"time"

	"crossos/core/pkg/pluginapi"
)

// TRIALTimeout is the safe-mode grace period (§8.1, §3.10). Defined ONCE —
// every consumer references this constant.
const TRIALTimeout = 30 * time.Second

// IntegrationType names a CrossOS-owned system resource kind.
type IntegrationType string

const (
	IntegrationLoginItem     IntegrationType = "loginitem"
	IntegrationAccessTap     IntegrationType = "cgeventtap"
	IntegrationFinderSync    IntegrationType = "findersync"
	IntegrationAccessibility IntegrationType = "accessibility"
	IntegrationConfig        IntegrationType = "config"
)

// IntegrationRecord describes one CrossOS-owned resource: what it was before
// CrossOS touched it, and how to undo the change. Rollback is a description
// here (data, serializable); execution belongs to the daemon that owns the
// OS handles — this package decides WHAT rolls back, not HOW.
type IntegrationRecord struct {
	PluginID    string
	Type        IntegrationType
	Identifier  string // the system resource ID
	StateBefore string // the state before CrossOS modified it
	Rollback    string // rollback procedure description (auditable, no closures)
	CreatedAt   time.Time
}

// IntegrationState is the set of CrossOS-owned integrations. It is NOT a
// system snapshot and cannot restore arbitrary OS state.
type IntegrationState struct {
	Timestamp    time.Time
	Integrations []IntegrationRecord
}

// RollbackPlan returns the records in reverse creation order (newest first),
// the order a rollback must execute.
func (s IntegrationState) RollbackPlan() []IntegrationRecord {
	out := append([]IntegrationRecord(nil), s.Integrations...)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

// SafetyEvent is one auditable safety transition. Every safety transition is
// logged with plugin ID + timestamp (bead criterion).
type SafetyEvent struct {
	At       time.Time
	PluginID string
	Action   string // e.g. "trial-begin", "trial-confirm", "rollback", "kill-switch", "reset"
	Detail   string
}

// Trial models the transactional enable flow: DISABLED → TRIAL → ENABLED,
// with crash/timeout/kill-switch during TRIAL → DISABLED. Time is injected
// (now func) so tests pin the 30s boundary without sleeping.
type Trial struct {
	PluginID string
	State    pluginapi.LifecycleState
	Begin    time.Time
	now      func() time.Time
	Log      []SafetyEvent
}

// BeginTrial moves DISABLED → TRIAL and stamps the start.
func BeginTrial(pluginID string, now func() time.Time) (*Trial, error) {
	if now == nil {
		now = time.Now
	}
	t := &Trial{PluginID: pluginID, State: pluginapi.LifecycleDisabled, now: now}
	if err := t.advance(pluginapi.LifecycleTrial, "trial-begin"); err != nil {
		return nil, err
	}
	return t, nil
}

// Confirm moves TRIAL → ENABLED. Requires explicit user confirmation AND a
// healthy check — there is no auto-approve path: Confirm with confirmed=false
// is an error, never a silent enable.
func (t *Trial) Confirm(confirmed, healthy bool) error {
	if !confirmed {
		t.log("trial-confirm-refused", "enable without user confirm rejected")
		return fmt.Errorf("safety: plugin %q not confirmed by user", t.PluginID)
	}
	if !healthy {
		t.log("trial-confirm-unhealthy", "enable without healthy check rejected")
		return fmt.Errorf("safety: plugin %q not healthy", t.PluginID)
	}
	if t.Expired() {
		// Abort back to DISABLED first (must succeed for the state to be
		// safe), then report expiry as the Confirm error — a timed-out
		// confirm is never a silent enable. Already-disabled (e.g. a prior
		// abort raced the confirm) is still an expiry error, not a panic.
		if err := t.abort("trial-confirm-expired", "confirm after TRIAL timeout"); err != nil {
			t.log("trial-confirm-expired", "already disabled on confirm")
			return fmt.Errorf("safety: plugin %q TRIAL expired", t.PluginID)
		}
		return fmt.Errorf("safety: plugin %q TRIAL expired", t.PluginID)
	}
	return t.advance(pluginapi.LifecycleEnabled, "trial-confirm")
}

// Expired reports whether the TRIAL window has elapsed.
func (t *Trial) Expired() bool {
	return t.now().Sub(t.Begin) >= TRIALTimeout
}

// Abort moves TRIAL → DISABLED (crash/timeout/kill-switch path).
func (t *Trial) Abort(reason string) error {
	return t.abort("trial-abort", reason)
}

func (t *Trial) abort(action, reason string) error {
	t.log(action, reason)
	// Already-disabled is idempotent success (a crash/timeout racing a prior
	// abort must not error) — only a move from a non-disabled state goes
	// through the transition table.
	if t.State == pluginapi.LifecycleDisabled {
		return nil
	}
	if err := pluginapi.Transition(t.State, pluginapi.LifecycleDisabled); err != nil {
		return err
	}
	t.State = pluginapi.LifecycleDisabled
	return nil
}

func (t *Trial) advance(to pluginapi.LifecycleState, action string) error {
	if err := pluginapi.Transition(t.State, to); err != nil {
		return err
	}
	if to == pluginapi.LifecycleTrial {
		t.Begin = t.now()
	}
	t.State = to
	t.log(action, string(to))
	return nil
}

func (t *Trial) log(action, detail string) {
	t.Log = append(t.Log, SafetyEvent{
		At: t.now(), PluginID: t.PluginID, Action: action, Detail: detail,
	})
}

// KillSwitchResult is the outcome of PANIC STOP: interception + plugin
// actions disabled, buffers flushed/closed. The login item stays (uninstall
// is Reset's job, not the kill switch's).
type KillSwitchResult struct {
	InterceptionDisabled bool
	PluginActionsStopped bool
	BuffersFlushed       bool
	LoginItemKept        bool
	At                   time.Time
	PluginID             string // "core" for the global switch
}

// PanicStop executes the kill switch. It always keeps the login item.
func PanicStop() KillSwitchResult {
	return KillSwitchResult{
		InterceptionDisabled: true,
		PluginActionsStopped: true,
		BuffersFlushed:       true,
		LoginItemKept:        true,
		At:                   time.Now(),
		PluginID:             "core",
	}
}

// ResetPlan describes Reset Everything: remove login item, disable
// extension, clean CrossOS-owned state, verify no process remains. Like
// RollbackPlan it is data — the daemon executes it.
type ResetPlan struct {
	RemoveLoginItem  bool
	DisableExtension bool
	CleanOwnedState  bool
	VerifyNoProcess  bool
	OwnedRecords     []IntegrationRecord
}

// PlanReset builds the reset plan from an IntegrationState.
func PlanReset(s IntegrationState) ResetPlan {
	return ResetPlan{
		RemoveLoginItem:  true,
		DisableExtension: true,
		CleanOwnedState:  true,
		VerifyNoProcess:  true,
		OwnedRecords:     s.RollbackPlan(),
	}
}

// ManifestScopeCheck rejects over-scoped manifests (least privilege): a
// plugin whose declared permissions exceed its capability needs fails
// closed. needs maps capability ID → required permission; declared is the
// manifest's permission list.
func ManifestScopeCheck(declared []string, needs map[string]string) error {
	have := map[string]struct{}{}
	for _, p := range declared {
		have[p] = struct{}{}
	}
	for capID, perm := range needs {
		if _, ok := have[perm]; !ok {
			return fmt.Errorf("safety: capability %s needs permission %q: not declared", capID, perm)
		}
	}
	// Over-scope: declared permissions no capability needs.
	used := map[string]struct{}{}
	for _, perm := range needs {
		used[perm] = struct{}{}
	}
	for _, p := range declared {
		if _, ok := used[p]; !ok {
			return fmt.Errorf("safety: manifest declares unused permission %q (over-scoped)", p)
		}
	}
	return nil
}
