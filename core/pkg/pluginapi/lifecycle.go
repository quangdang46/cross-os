package pluginapi

import (
	"fmt"
	"strconv"
	"strings"
)

// LifecycleState is the plugin lifecycle state (plan §3.10).
//
// Plugin enable is transactional (ownership-scoped, NOT an OS snapshot):
// DISABLED → TRIAL → (user confirms + healthy) → ENABLED.
// crash / timeout / kill-switch during TRIAL → DISABLED.
type LifecycleState string

const (
	LifecycleInitializing LifecycleState = "initializing"
	LifecycleRunning      LifecycleState = "running"
	LifecycleSafeMode     LifecycleState = "safemode" // 30s grace
	LifecyclePaused       LifecycleState = "paused"
	LifecycleStopped      LifecycleState = "stopped"

	// Plugin enable flow states (distinct from daemon states above).
	LifecycleDisabled LifecycleState = "disabled"
	LifecycleTrial    LifecycleState = "trial"
	LifecycleEnabled  LifecycleState = "enabled"
)

// allowedTransitions is the normative transition table. Anything not listed
// is rejected by Transition.
var allowedTransitions = map[LifecycleState][]LifecycleState{
	LifecycleDisabled:     {LifecycleTrial},
	LifecycleTrial:        {LifecycleEnabled, LifecycleDisabled},
	LifecycleEnabled:      {LifecycleDisabled, LifecyclePaused},
	LifecyclePaused:       {LifecycleEnabled, LifecycleDisabled},
	LifecycleInitializing: {LifecycleRunning, LifecycleStopped},
	LifecycleRunning:      {LifecycleSafeMode, LifecyclePaused, LifecycleStopped},
	LifecycleSafeMode:     {LifecycleRunning, LifecycleStopped},
	LifecycleStopped:      {LifecycleInitializing},
}

// Transition validates a lifecycle move, returning an error for any move
// outside the table.
func Transition(from, to LifecycleState) error {
	for _, ok := range allowedTransitions[from] {
		if to == ok {
			return nil
		}
	}
	return fmt.Errorf("pluginapi: illegal lifecycle transition %q → %q", from, to)
}

// CompatibleWith reports whether a plugin manifest's MinCoreVersion is
// satisfied by the running core version. Both must be dot-separated numeric
// semver (missing parts read as zero); empty MinCoreVersion is rejected —
// every manifest must declare the core it was built against.
func (v APIVersion) CompatibleWith(coreVersion string) error {
	if strings.TrimSpace(v.MinCoreVersion) == "" {
		return fmt.Errorf("pluginapi: manifest declares no min_core_version")
	}
	cmp, err := compareSemver(v.MinCoreVersion, coreVersion)
	if err != nil {
		return err
	}
	if cmp > 0 {
		return fmt.Errorf("pluginapi: plugin requires core >= %s, running %s",
			v.MinCoreVersion, coreVersion)
	}
	return nil
}

// compareSemver compares dot-separated numeric versions (-suffixes ignored).
// Returns -1/0/+1 for a </==/> b.
func compareSemver(a, b string) (int, error) {
	pa, err := parseSemver(a)
	if err != nil {
		return 0, fmt.Errorf("pluginapi: bad version %q: %w", a, err)
	}
	pb, err := parseSemver(b)
	if err != nil {
		return 0, fmt.Errorf("pluginapi: bad version %q: %w", b, err)
	}
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			if x < y {
				return -1, nil
			}
			return 1, nil
		}
	}
	return 0, nil
}

func parseSemver(s string) ([]int, error) {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "-"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 0 {
			return nil, fmt.Errorf("not numeric: %q", p)
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty version")
	}
	return out, nil
}
