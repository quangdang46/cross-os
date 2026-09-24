# Attribution — alt-tab-macos window-switcher kernel (bead ws-8)

**No AltTab source is vendored.** Nothing in this directory is a copy, a
transliteration or a partial extract of lwouis/alt-tab-macos, and no file
anywhere in CrossOS is derived from its GPL-3.0 text. This entry exists only to
record a LEARN-ONLY decision and to give §9.11's merge gate something to check
against: a GPL source can never contribute code, and the blocks below are the
evidence that none did. Each one names the rule that was read, the file and
lines it was read from, and the CrossOS file that implements it in a different
language against different contracts.

The reference was a read-only clone at `tmp/research/alt-tab-macos`, pinned at
the commit below. It is not a dependency, is not built, is not vendored, and is
not required to build, test or ship CrossOS.

Two license facts, both verified at the pinned commit and both worth stating
because a wrong reading here is what the gate exists to catch:

- The application ships under **GPL-3.0**. `LICENCE.md` at the pinned commit is
  the verbatim GNU GPL v3 text, and `Info.plist:20-21` sets
  `NSHumanReadableCopyright` to `GPL-3.0 licence`.
- The repository's root `package.json:5-6` declares `"private": true` and
  `"license": "MIT"`. That file is the private build-tooling manifest (husky,
  commitlint, a swiftformat lint script) and its MIT field does not reach the
  application. **GPL-3.0 is the license of record here**, which is also what
  COMPREHENSIVE_PLAN.md:1132 and :1302 already record.

The project carries no per-file copyright header and no AUTHORS file, so the
copyright line below is the project's own contributor list rather than a
per-file notice.

---

Source repository: lwouis/alt-tab-macos
Source commit: 1cfb7e1df05f2cb87e0cc88f490de97d3e51b753 (tag v11.7.0)
Source file: src/windowserver/README.md:1-20 (the physical plane, the semantic
plane, and the boundary that says which facts come from where)
Original license: GPL-3.0
Original copyright: Copyright (c) lwouis and the alt-tab-macos contributors (no
per-file header in the project; the holder list is docs/contributors.md at this
commit)
CrossOS destination: core/internal/adapter/windowlist_darwin.go (the file names
both planes and why they are separate), core/internal/adapter/windowlist_other.go
(the same seam off darwin, so the split is the contract and not a macOS detail)
Modification: structural port, no code copied. What transfers is the SPLIT: the
WindowServer owns physical window state and stays answerable when an application
is wedged, while only AppKit knows which window inside a process is the key one,
so the two are read from two different places and never conflated
(README.md:3-5, and the boundary at :10-20). CrossOS's two planes are the same
split one layer out: a batched CGWindowList walk answers existence, geometry,
ordered-in and owning pid, and the semantic facts — the real title, whether the
window is the key one — are asked of the owning application only where the
physical plane has no answer. The reason is the reference's own reason: an AX
attribute read is a call INTO the application being listed and costs that
application as much as it is busy, so nothing outside can bound it.
No AltTab source is vendored for this port: the SkyLight/CGS notify-proc tap, the
AppKit key-window ivar reads and the remote-token brute force are all absent, and
so is every line of Swift behind them.
Reason for modification: the reference's split is two private frameworks inside
one macOS process, and CrossOS's is a package boundary the whole repo is built
on. core/pkg/winswitch exists because the same separation keeps a decision kernel
buildable with CGO_ENABLED=0 and testable on any OS, where a Swift app cannot be.
CrossOS license: MIT

---

Source repository: lwouis/alt-tab-macos
Source commit: 1cfb7e1df05f2cb87e0cc88f490de97d3e51b753 (tag v11.7.0)
Source file: src/windowserver/WindowServerQuery.swift:4,9 (one batched query —
"the cost is one IPC for the batch, not one per field"), plus
src/windowserver/PublishedWindows.swift:16-20 (the three window attributes asked
together because the batched call folds them into ONE round trip) and
src/windowserver/README.md:40-41 (no wid-to-element API exists, so elements are
acquired lazily and cached, the inventory grouping every missing wid by process so
one batched attribute read resolves them)
Original license: GPL-3.0
Original copyright: Copyright (c) lwouis and the alt-tab-macos contributors (no
per-file header in the project; the holder list is docs/contributors.md at this
commit)
CrossOS destination: core/internal/adapter/windowlist_darwin.go — ListWindows
(one axListWindows call for the whole physical plane) and fillTitles (one batched
axAppTitles read per pid, for the rows the physical plane left unnamed, pids
visited in sorted order so a failure here is reproducible)
Modification: structural port, no code copied. What transfers is the BUDGET
RULE: every fact that one batched call can carry is carried by it, and the
per-application read happens afterwards, only for what the batch could not
answer, once per application rather than once per window
(WindowServerQuery.swift:9, PublishedWindows.swift:16-20). CrossOS makes the
budget measured rather than asserted — axWindowsReadsForPID is what the test checks
the "a minimized window costs no extra read" property against, and the minimized
flag is decoded from the row already in hand (minimizedFrom) instead of a
kAXMinimized read into the owning app, because that substitution is what keeps
enumeration non-blocking.
No AltTab source is vendored for this port: SLSWindowQueryWindows, the AppKit
windowsWithOptions query, the 250ms brute-force budget and the 30-tile
neighborhood pre-capture are all absent; CrossOS spends at most one AX read per
application and reads titles only.
Reason for modification: the reference needs an AXUIElement per window to raise,
minimize, close and fullscreen it, and it resolves the key/main window through
AppKit ivars because those are the two window attributes AppKit does not put
behind the Space filter. CrossOS needs a name and a decision, so the same
one-batched-then-the-rest shape answers a smaller question, and the routes that
exist only to manufacture an AX element have no destination here.
CrossOS license: MIT

---

Source repository: lwouis/alt-tab-macos
Source commit: 1cfb7e1df05f2cb87e0cc88f490de97d3e51b753 (tag v11.7.0)
Source file: src/switcher/state/SelectionResolver.swift:188-201
(secondVisibleIndex counts VISIBLE windows, not raw indices, and wraps to the only
visible window when there is one), :182, and
src/switcher/state/SelectionResolverSpecs.md:52-58 (the front tile is stepped over
because it is the window you are on, so it is not stepped over when it is not)
Original license: GPL-3.0
Original copyright: Copyright (c) lwouis and the alt-tab-macos contributors (no
per-file header in the project; the holder list is docs/contributors.md at this
commit)
CrossOS destination: core/pkg/winswitch/pick.go — Row.Skippable,
FirstVisibleIndex, SecondVisibleIndex, DefaultPickIndex, PickInputs
Modification: structural port, no code copied. What transfers is the RULE: the
default pick is the second VISIBLE window, not the second index, because hidden
windows sit in the MRU too (a background tab is fronted when discovered, then
hidden once grouped) so index 0 can be hidden and index 1 can be the current
window, and counting indices there selects the window already on top of the
screen (SelectionResolver.swift:192-196). The reference's #5941 correction
travels with it: when a filter keeps the current window out of the list, the
front tile is already "the window you were on before" and the pick is the front
tile, not the second. The reference's `visible` flag and its own filter chain do
not come — the shell hands over the drawn list, so counting its entries already
counts visible tiles, and only the two answers the kernel cannot derive for
itself travel as inputs (CurrentWindowDrawn, VisibleAtSummon).
No AltTab source is vendored for this port: SelectionWindow, the filter chain
behind the `visible` flag, and the NSApp key-window reads that feed
currentWindowIsDrawn are all absent; the kernel is told whether the current
window is drawn.
Reason for modification: Swift objects carrying a computed flag become a Row
value and a pure function over a slice, which is what lets the pick be pinned by
a test on any OS with no window server present.
CrossOS license: MIT

---

Source repository: lwouis/alt-tab-macos
Source commit: 1cfb7e1df05f2cb87e0cc88f490de97d3e51b753 (tag v11.7.0)
Source file: src/switcher/state/SelectionResolverSpecs.md:81-98 (only an ARRIVAL
is stepped over, never a REPLACEMENT; the flag means absent-from-the-list-at-the-
press, not focused-since; the two are told apart by the length of the list, which
is exactly how many arrived rather than replaced; the count is measured on the
summon's first selection pass, the same main-thread turn as the press)
Original license: GPL-3.0
Original copyright: Copyright (c) lwouis and the alt-tab-macos contributors (no
per-file header in the project; the holder list is docs/contributors.md at this
commit)
CrossOS destination: core/pkg/winswitch/pick.go — Row.AppearedAfterSummon and
stepOverNewcomers; core/pkg/winswitch/session.go — Summon, which measures
visibleAtSummon on the first pass
Modification: structural port, no code copied. What transfers is the
DISTINCTION and the arithmetic that reads it: a newcomer that merely took a
departing window's tile moved nothing down, so stepping over it aims a tile too
far, and the list length is what separates the two cases. stepOverNewcomers
therefore steps over at most len(candidates) - visibleAtSummon, and when stepping
leaves nothing to land on the plain rule takes over rather than returning nothing
— the same fallback the reference reaches when every drawn window arrived after
the press (SelectionResolverSpecs.md:86-88, SelectionResolver.swift:178-185).
Nothing is pinned: the answer is recomputed on every refresh, so a window that
closes or stops being drawn drops out of it.
No AltTab source is vendored for this port: the model, the tab-group resolution
and the `appearedAfterSummon` flag's producer are all absent; CrossOS's adapter
hands the flag over as a fact about a row.
Reason for modification: in the reference the press and the measurement are two
statements in one AppKit turn. In CrossOS they are one IPC turn — window.switcher
summon returns a Session already holding visibleAtSummon — and the daemon
refreshes many times while the switcher stays open, which is why the count is
taken once and every later refresh re-derives against it.
CrossOS license: MIT

---

Source repository: lwouis/alt-tab-macos
Source commit: 1cfb7e1df05f2cb87e0cc88f490de97d3e51b753 (tag v11.7.0)
Source file: src/switcher/state/Windows.swift:439-450
(selectedWindowIndexAfterCycling steps modulo the list count and normalizes a
negative step back onto the front) and :397-414 (a selection that ran past the
end while the list shrank is clamped rather than trapped)
Original license: GPL-3.0
Original copyright: Copyright (c) lwouis and the alt-tab-macos contributors (no
per-file header in the project; the holder list is docs/contributors.md at this
commit)
CrossOS destination: core/pkg/winswitch/session.go — wrap (normalizes an index
that stepped off either end into [0, n)), Cycle, and closestBelow
Modification: structural port, no code copied. What transfers is the ARITHMETIC
and where it lands: stepping off either end normalizes back onto the list rather
than trapping or clamping, and a highlight whose window has left backfills onto
whatever now occupies its slot — the same index when a window is there, else the
next candidate down, else the last one when the list shrank below the slot — which
is the reference's adapt step (SelectionResolverSpecs.md:35). The reference
suppresses the wrap on key-repeat and adds a row-scoped allowWrap guard
(Windows.swift:406-412); neither comes, because CrossOS cycles one linear list
rather than a grid and has no held-key path into Cycle.
No AltTab source is vendored for this port: Direction, shouldDisplay, the
throttled selection fix-up and the row/column geometry are all absent; the kernel
sees a candidate list and an index.
Reason for modification: the reference's list is a UI panel whose rows can be
reordered and re-grouped underneath the highlight between two keystrokes.
CrossOS's is a flat candidate list the daemon recomputes on every refresh, so the
wrap is expressed over the candidate count and the vanished-target case is given
its own rule rather than inheriting the panel's geometry.
CrossOS license: MIT

---

Source repository: lwouis/alt-tab-macos
Source commit: 1cfb7e1df05f2cb87e0cc88f490de97d3e51b753 (tag v11.7.0)
Source file: src/switcher/state/SelectionResolverSpecs.md:30-42 (the six-step
priority order, and the split that says selectedTarget means two different things
— while the user has not moved the selection it is merely where the DEFAULT
landed, so the initial pick re-derives on every refresh; once the user cycles or
hovers it is a commitment and is followed by id however the list reorders),
plus src/switcher/ShortcutAction.swift:39-49 (acting on a tile sets
userPickedSelection, so an action that reorders the list does not make the default
pick slide off the window the user aimed at)
Original license: GPL-3.0
Original copyright: Copyright (c) lwouis and the alt-tab-macos contributors (no
per-file header in the project; the holder list is docs/contributors.md at this
commit)
CrossOS destination: core/pkg/winswitch/session.go — Session (userPicked,
targetID, lastIndex, visibleAtSummon), Decide, defaultDecision, land, Pick, Cycle,
Close; core/cmd/crossos/switcher.go — the summon action on key-down and commit on
key-up of the same chord
Modification: structural port, no code copied. What transfers is the MACHINE, in
two halves. First, the release: a default is re-derived from scratch on every
refresh and overwrites the stored target, so it can never trail a window that
slid down the list, while a commitment is followed by id and is what backfills
when its window leaves. Second, the moment of release: acting on a tile is a
commitment, so a commit that reorders the list still lands on the window the user
aimed at. CrossOS adds the half the reference does not need — a release with no
summon behind it commits nothing rather than focusing whatever the default pick
happens to be — because here the key-up arrives over IPC and a dropped summon is
a keystroke the user gets an answer to.
No AltTab source is vendored for this port: SwitcherSession, the hover half of the
rule (a hovered tile also commits), the preview panel and the redraw/scroll
bookkeeping are all absent; a Cycle or an explicit Pick is a commitment.
Reason for modification: the reference keeps its session beside the window server
inside the app process. CrossOS keeps it in the daemon, so a shell reload cannot
strand a highlight on a window the list has since dropped, and so the two states
that must not be confused — a default and a commitment — are two named fields on
one value rather than one field read two ways.
CrossOS license: MIT

---

Source repository: lwouis/alt-tab-macos
Source commit: 1cfb7e1df05f2cb87e0cc88f490de97d3e51b753 (tag v11.7.0)
Source file: src/switcher/state/WindowOrderResolverSpecs.md:16-41 (the ordering
decision order — search, the show-at-the-end buckets, the sort type, the
lastFocusOrder tiebreak — and the one-window-per-app representative, which is a
selection made over this list's rows and so belongs to the caller before it sorts)
Original license: GPL-3.0
Original copyright: Copyright (c) lwouis and the alt-tab-macos contributors (no
per-file header in the project; the holder list is docs/contributors.md at this
commit)
CrossOS destination: core/pkg/winswitch/order.go — Row, OrderOptions, Less, Sort
Modification: structural port, no code copied. What transfers is the ORDER OF
DECISIONS: each step decides only when the two rows differ on it, which makes
Less a strict weak ordering over per-row fields — the property sort.Slice
requires and the one two rows equal on every field exercise, neither ordered
before the other (WindowOrderResolverSpecs.md:33). The sort-mode knobs are
labeled parameters hoisted once per sort rather than read per comparison, and the
per-row facts travel with the Row instead of being re-derived once per
comparison, which is the O(n) snapshotting the reference gets from its
precomputed OrderWindow. The one-window-per-app representative does not come: it
is a selection over the filtered list, so it belongs to the adapter that owns the
filters, not to a kernel handed a finished list.
No AltTab source is vendored for this port: OrderWindow, ApplicationState, the
localizedStandardCompare collation and the sort dispatch in Windows.sort are all
absent; the comparator is byte order over app name, then title, because an order
the tests can pin is worth more at a keystroke than the one a user's locale
would choose.
Reason for modification: a comparator over two live Window objects with a search
rank read through an O(n log n) call becomes a comparator over two Row values,
which is what makes the whole ordering a pure function that runs on any OS.
CrossOS license: MIT

---

Six ported algorithms, seven blocks: the ordering comparator is split out from
the release-to-commit machine because it is a separate kernel (Order asks "in what
sequence", the Session asks "which tile now that the user has had a say") and
merging them would hide that. Every block above is a structural port. No AltTab
source is vendored, for any of them.
