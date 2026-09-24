// Package winswitch is the switcher kernel: the order windows appear in, the
// default tile a summon lands on, and the session that keeps the highlight on
// the window the user meant while the list churns underneath it.
//
// The split is the app's rule — core decides, the adapter executes, the shell
// renders. Everything here is data in, decision out: no cgo, no syscalls, no
// window server, so the package builds with CGO_ENABLED=0 on every Tier-1
// target and its tests run green on any OS. The adapter hands over Row
// snapshots (ids, MRU rank, app names, the filters it already applied) and
// acts on the index it gets back.
//
// Provenance: the switcher UX is studied from AltTab (open source,
// tmp/research/alt-tab-macos). The rules below are ports of its spec'd
// decisions, not of its code:
//
//   - SelectionResolverSpecs.md:29-58 — the default-pick priority; the second
//     VISIBLE window rather than the second raw index; and #5941, the case
//     where a filter keeps the window on top out of the list, so the pick is
//     the front tile instead of the second one.
//   - SelectionResolverSpecs.md:81-98 — a window that arrived after the press
//     is stepped over only while the drawn list grew past its summon-time
//     length. An arrival is stepped over; a newcomer that merely took a
//     departing window's tile is not, because nothing moved down to pay for it.
//   - WindowOrderResolverSpecs.md:16-41 — the ordering decision order (search,
//     show-at-end buckets, sort type, lastFocusOrder tiebreak) and the
//     one-window-per-app representative, which is a selection made over this
//     list's rows and therefore belongs to the caller before it sorts here.
//
// The three kernels answer different questions and deliberately do not merge:
// Order asks "in what sequence", Pick asks "which tile first", and the Session
// asks "which tile now that the user has had a say". A switcher that re-picks
// on every refresh is the bug this package exists to prevent.
package winswitch
