// Package intent locks the CrossOS Intent + Capability contract.
//
// Bead: cross-os-9tb. Plan: COMPREHENSIVE_PLAN.md §3.4, §3.7, §3.12, Glossary.
//
// Vocabulary (locked — bead pass criterion):
//
//	permission = consent granted by the user (checked by Core).
//	capability = operation Core provides (executed by Core/Adapter).
//	intent     = platform-independent user goal (produced by the resolver).
//
// Core authority rule: Plugin requests, Core decides, Adapter executes.
// Invalid parallel names (keyboard.intercept, window.manage,
// finder.contextMenu) are rejected — see ValidateCapabilityID.
//
// v1 namespace decision (locked; review cross-os-c0): Intent.ID lives in the
// capability-ID namespace (e.g. "window.close"), NOT a separate goal
// vocabulary (COPY, CLOSE_WINDOW). This is faithful to §3.4's struct; the
// Glossary's goal-wording is aspirational for a future goal layer, if one is
// ever wanted (that would be a plan edit to §3.4/Glossary first). The
// resolver bead (cross-os-z9v) inherits this: it resolves to capability IDs.
//
// Deliberate-shape notes:
//   - Intent.Version is int while CapabilityDescriptor.Version is string:
//     intent schemas rev numerically per breaking change; capability
//     contracts rev as semver-ish strings. Converge only if a consumer needs it.
//   - OutputSchema is declared but unpopulated/unvalidated in v1 by design:
//     MVP capabilities return platform-native results the contract does not
//     yet constrain; populating it is a v2 contract revision.
//   - Registry is build-once at startup and thereafter read-only (no mutex).
//     A mutable/hot-reload registry would need synchronization — not v1.
//   - Action execution traces (who/when/result) belong to the dispatcher +
//     permission-manager path, not this package: Authorize binds the grant,
//     the executor logs it.
package intent
