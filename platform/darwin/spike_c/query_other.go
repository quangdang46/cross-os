//go:build !darwin

// Non-darwin stub: Tier-2 live AX tests are darwin-only; Windows UIA lives in
// platform/windows (bead cross-os-lla covers both OSes, UIA side deferred to
// the windows seam under cross-os-ab4).
package spikec
