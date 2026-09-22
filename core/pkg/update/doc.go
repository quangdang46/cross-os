// Package update is the auto-update CLIENT half (bead cross-os-qhp.7).
//
// Split from cross-os-qhp.6 (review: cross-os-ed) because manifest check ->
// download -> verify -> install-with-approval is portable Go with no Mac
// signing/notarization dependency — unlike qhp.6's remaining scope
// (codesign/notarytool, WiX MSI packaging, secrets exercised on a real
// version tag), which stays Mac-runner + Developer-account gated.
//
// Plan: COMPREHENSIVE_PLAN.md §10 Phase 5, §8.4 (updates never silently
// enable new integrations), §14 Q4 (Git-based auto-update, user approval —
// same precedent jpr.1's marketplace Approval already follows).
//
// This package makes release.yml's "manifest: version ... — user approval
// required at install" echo line real on the client side: Manifest decodes
// a release entry, CheckForUpdate decides whether it's newer, Download
// fetches + verifies the SHA256 checksum (mismatch fails closed, never
// installs unverified bytes), Install requires an explicit Approval and
// writes atomically (temp file + rename, never a partially-written target).
package update
