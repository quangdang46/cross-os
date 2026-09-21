package approute

import (
	"strings"

	"crossos/core/pkg/ctx"
)

// Lists holds the operator-owned category lists. Matching is
// case-insensitive substring over "bundleID\x00executable" (same convention
// as the seed classifier, so seed and config agree on what a "match" is).
type Lists struct {
	Terminals []string
	Remotes   []string
	VMs       []string
	Excluded  []string // user opt-out: full passthrough
	Browsers  []string
	// Devices maps device ID → excluded (true = ignore this keyboard).
	Devices map[string]bool
}

// Router classifies apps from config lists, falling back to the seed
// classifier for anything unlisted. Config wins over seed on conflict:
// listing an app anywhere in Lists overrides its seed entry.
//
// Wiring note (review: cross-os-c0): Router does NOT satisfy
// ctx.Classifier (Route returns mode+category+cause; Classify returns
// mode+category). When s4i wires the shortcut matrix, either add a Classify
// adapter method on Router (dropping cause) so it plugs into ctx.Classifier,
// or document that Route supersedes Classify at the call site. Two parallel
// classification paths must not coexist silently.
// TODO(s4i): choose one wiring and record it here.
type Router struct {
	lists Lists
	seed  ctx.SeedClassifier
}

// New returns a Router over the given lists (nil maps/slices allowed).
func New(l Lists) *Router {
	if l.Devices == nil {
		l.Devices = map[string]bool{}
	}
	return &Router{lists: l}
}

// Route returns (mode, category, cause) for an app. Cause names the list
// that decided, for Recorder trace annotation ("AppMode cause").
func (r *Router) Route(bundleID, executable string) (ctx.AppMode, ctx.AppCategory, string) {
	hay := strings.ToLower(bundleID + "\x00" + executable)
	contains := func(list []string) (string, bool) {
		for _, e := range list {
			if e == "" {
				continue
			}
			if strings.Contains(hay, strings.ToLower(e)) {
				return e, true
			}
		}
		return "", false
	}
	if m, ok := contains(r.lists.Excluded); ok {
		return ctx.AppModeExcluded, ctx.AppUser, "config:excluded:" + m
	}
	if m, ok := contains(r.lists.VMs); ok {
		return ctx.AppModeVM, ctx.AppRemote, "config:vm:" + m
	}
	if m, ok := contains(r.lists.Remotes); ok {
		return ctx.AppModeRemote, ctx.AppRemote, "config:remote:" + m
	}
	if m, ok := contains(r.lists.Terminals); ok {
		return ctx.AppModeTerminal, ctx.AppTerminal, "config:terminal:" + m
	}
	if m, ok := contains(r.lists.Browsers); ok {
		return ctx.AppModeNative, ctx.AppBrowser, "config:browser:" + m
	}
	mode, cat := r.seed.Classify(bundleID, executable)
	return mode, cat, "seed"
}

// DeviceExcluded reports whether a keyboard ID is filtered out. An excluded
// device is ignored (its events never reach the matcher) while other
// devices still trigger — asserted by the two-keyboard test.
func (r *Router) DeviceExcluded(deviceID string) bool {
	return r.lists.Devices[deviceID]
}

// Passthrough reports whether the mode means "CrossOS takes no action"
// (remote/vm/excluded windows get native behavior untouched).
func Passthrough(mode ctx.AppMode) bool {
	return mode == ctx.AppModeRemote || mode == ctx.AppModeVM ||
		mode == ctx.AppModeExcluded
}
