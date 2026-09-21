// Package rule locks the CrossOS conflict-resolution contract.
//
// Bead: cross-os-xtk. Plan: COMPREHENSIVE_PLAN.md §3.5, §3.5b, §3.11, D3.
//
// Resolution is a Core function over candidate rules: filter (enabled,
// AppMode, requires satisfiable) → rank by specificity → break ties by
// priority → exactly ONE winner → one Intent. Plugins never REPLACE another
// plugin's resolved intent after the fact.
//
// Scope (defined here — the plan names it but never enumerates it): the
// narrowing tier a rule claims. Specificity is the computed match depth
// (app+window+device+requires). Scope sets the baseline tier; specificity
// ranks within and across tiers. Priority breaks ties deterministically.
//
// Deliberate-shape notes (review: cross-os-c0):
//   - Device-as-narrowest is a v1 tie-break convention, not a semantic claim
//     that device is strictly narrower than window. Specificity ranks first;
//     scope only orders ties. Revisit if a real window-vs-device tie ever
//     picks the wrong winner.
//   - Resolve always reports DecisionReplace for a winner; the
//     Consume-vs-Replace narrowing belongs to the dispatcher, which must
//     define its predicate before bead cross-os-z9v lands.
//     TODO(dispatcher): state what distinguishes Consume from Replace
//     (no-op intent? capability with no visible effect?) and narrow there.
//   - Negative Priority is allowed (priorities are convention, not
//     constraint); only negative Specificity is rejected as an authoring bug.
//   - Fixture intent IDs (clipboard.copyPath, terminal.openAt) are stand-ins:
//     Resolve is intent-agnostic; do not read them as canonical intent IDs.
package rule
