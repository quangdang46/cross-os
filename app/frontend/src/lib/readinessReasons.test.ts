// The readiness reason table, asserted against the strings the daemon writes.
//
// WHY THIS FILE AND NOT A COMMENT. lib/readinessReasons.ts claims to cover every
// `detail` a readiness row can carry. A claim like that is worth nothing on its
// own — the moment a Go string is reworded, the table quietly stops covering it
// and the fall-through path starts shipping log lines again, with nothing to say
// so. So each daemon string below is written out in full at the line that writes
// it, and the assertion is that the table has words for it.
//
// The second thing pinned here is the part that is easy to get quietly wrong: an
// UNMAPPED reason must produce no paraphrase. A table that guessed at an unknown
// string would put a sentence this shell invented over a fact about somebody's
// machine, which is the failure lib/format.ts's header names ("a formatter that
// guesses wrong would quietly misreport the user's own configuration") and the
// one the reference refuses to commit: menumate's `onboarding.diag.extNotEnabled`
// says "Finder extension not yet enabled" and offers no action at all, because
// there was no door to point at.
//
// Fixture ids are the daemon's own and are NOT page ids, so nothing here spells
// a namespaced "core" prefix — test/shell.test.tsx greps every file under src
// for one.

import { describe, expect, it } from 'vitest'
import { CHIP, explainReadiness, knownReasonCount } from './readinessReasons'

/** Every reason a readiness row can carry, at the Go line that writes it. */
const DAEMON_REASONS: { from: string; detail: string }[] = [
  // core/cmd/crossos/pagedata.go:633 — the daemon row's latched-switch case.
  { from: 'pagedata.go:633', detail: 'PANIC STOP is latched — re-enable from the Safety page' },
  // core/cmd/crossos/pagedata.go:635 — "daemon state: " + the lifecycle string.
  { from: 'pagedata.go:635', detail: 'daemon state: stopped' },
  // core/internal/adapter/tap_cgo.go:43 — errTapDenied. The string in the
  // screenshot, and the one the whole table exists for.
  { from: 'tap_cgo.go:43', detail: 'adapter: tap refused (input-monitoring consent missing?)' },
  // core/cmd/crossos/pagedata.go:644 — uninstalled, with no failure to quote.
  { from: 'pagedata.go:644', detail: 'keyboard interception is not installed' },
  // core/cmd/crossos/pagedata.go:817 and :820 — tapDegraded()'s two terms.
  { from: 'pagedata.go:817', detail: 'keyboard tap keeps timing out — remapping degraded' },
  { from: 'pagedata.go:820', detail: '3 shortcut action(s) could not run — remapping degraded' },
  // core/cmd/crossos/pagedata.go:682, :700, :730 — one string, three call sites.
  { from: 'pagedata.go:682', detail: 'plugin actions stopped by PANIC STOP' },
  // core/cmd/crossos/pagedata.go:702 — surfaceRow's empty surface.
  { from: 'pagedata.go:702', detail: 'no rule or profile capability covers this yet' },
  // core/cmd/crossos/pagedata.go:684 — the per-plugin row.
  { from: 'pagedata.go:684', detail: 'plugin disabled' },
  // core/cmd/crossos/pagedata.go:706 and :752 — the plugin id is spliced in.
  { from: 'pagedata.go:706', detail: 'window-keys is not switched on' },
  { from: 'pagedata.go:752', detail: 'finder-actions is not switched on' },
  // core/cmd/crossos/pagedata.go:735 and :740 — the profile row.
  { from: 'pagedata.go:735', detail: 'no profile is chosen yet' },
  { from: 'pagedata.go:740', detail: 'no profile bundle is named "win11-ltsc"' },
  // core/cmd/crossos/pagedata.go:754 — a label and two counts.
  {
    from: 'pagedata.go:754',
    detail: 'Windows snapping has 2 of 3 shortcuts switched off',
  },
  // core/cmd/crossos/pagedata.go:1634 and :1636 — the first-run step details.
  { from: 'pagedata.go:1634', detail: 'no plugin is switched on yet' },
  { from: 'pagedata.go:1636', detail: '7 of 8 checks are not ready' },
  // core/cmd/crossos/main.go:668 and :1115 — the two spellings of the same
  // latched switch, written by the tap path rather than by readinessRows.
  {
    from: 'main.go:668',
    detail: 'interception stopped by PANIC STOP — re-enable from the Safety page',
  },
  {
    from: 'main.go:1115',
    detail: 'interception stopped by PANIC STOP (re-enable from the Safety page)',
  },
  // The adapter errors main.go:1124 stores verbatim, so any of them can be a
  // keyboard row's reason.
  { from: 'driver.go:27', detail: 'adapter: accessibility permission denied' },
  { from: 'driver.go:30', detail: 'adapter: tap/hook not installed' },
  { from: 'tap_cgo.go:215', detail: 'adapter: tap already running' },
  {
    from: 'tap_cgo.go:272',
    detail: 'adapter: tap install still in progress (no response yet)',
  },
  { from: 'tap_cgo.go:305', detail: 'adapter: tap run loop did not stop within 2s' },
  {
    from: 'seam_errors.go:7',
    detail: 'adapter: native bridge not yet wired (spike-proven shape, link pending)',
  },
  { from: 'errors.go:6', detail: 'adapter: not supported on this platform' },
]

describe('the readiness reason table', () => {
  it('has plain words for every reason the daemon writes', () => {
    // The coverage claim, as an assertion rather than a promise. A Go string
    // reworded without this table changing is exactly the drift that would put
    // log lines back on a first-run page, and the failure here names the line to
    // fix instead of leaving it to be noticed in the product.
    //
    // The list is LONGER than the table on purpose and the counts differ: one
    // entry covers the three spellings of the latched safety stop, and the
    // plugin-off entry covers both surfaces' `<plugin> is not switched on`. An
    // entry that had to be spelled once per call site would be three copies of
    // one sentence, which is the thing this file exists to stop.
    const unsaid = DAEMON_REASONS.filter((row) => explainReadiness(row.detail) === undefined)
    expect(unsaid.map((row) => `${row.from}: ${row.detail}`)).toEqual([])
    expect(knownReasonCount()).toBeGreaterThan(0)
  })

  it('turns a refused tap into what is wrong, then what to do', () => {
    // The two sentences, in the reference's order: the state first, the action
    // second, and the action naming the setting rather than a pane the daemon
    // cannot open (main.go:479-495 opens Accessibility and refuses any other).
    const reason = explainReadiness('adapter: tap refused (input-monitoring consent missing?)')
    expect(reason?.what).toMatch(/permission to see the keys you press/)
    expect(reason?.what).not.toMatch(/adapter:|\?$/)
    expect(reason?.next).toMatch(/Input Monitoring/)
    expect(reason?.next).toMatch(/Accessibility/)
    expect(reason?.tone).toBe('refused')
  })

  it('keeps the daemon’s own values in the sentence it composes', () => {
    // The three composed reasons. A table keyed on literals would silently drop
    // all of them, and these are the ones a reader most needs — the plugin's
    // name, the profile's name, the counts.
    expect(explainReadiness('window-keys is not switched on')?.what).toContain('window keys')
    expect(explainReadiness('no profile bundle is named "win11-ltsc"')?.what).toContain('win11-ltsc')
    const counts = explainReadiness('Windows snapping has 2 of 3 shortcuts switched off')
    expect(counts?.what).toContain('Windows snapping')
    expect(counts?.what).toContain('2 of its 3')
    // The count is not decoration: the reference's outcome sentences keep theirs
    // (DeclutterSheet.swift:102), because a count is what makes "some" actable.
    expect(explainReadiness('7 of 8 checks are not ready')?.what).toContain('7 of the 8')
  })

  it('offers no action for a state with no door to open', () => {
    // The reference's own honesty, from `onboarding.diag.extNotEnabled`: a state
    // with nothing to press says the state and stops. These two are in that class
    // — a build with no bridge and a platform with no keyboard are not things a
    // reader can act on from this window, and a row that offered a button would
    // be a button that runs nothing.
    for (const detail of [
      'adapter: not supported on this platform',
      'adapter: native bridge not yet wired (spike-proven shape, link pending)',
    ]) {
      const reason = explainReadiness(detail)
      expect(reason, `${detail} has words`).toBeTruthy()
      expect(reason?.next, `${detail} names a door it does not have`).toBe('')
    }
    // And a build gap is not dressed as a permission failure, which is the rule
    // UnsupportedControl's own header is about.
    expect(explainReadiness('adapter: not supported on this platform')?.tone).toBe('unread')
    // The empty surface DOES have a door, and this is the one that keeps the
    // empty-state rule from being over-applied: the daemon's own answer is that
    // no rule or profile covers it (pagedata.go:702), and the Profiles page is
    // where that is chosen — so this one names an action.
    expect(explainReadiness('no rule or profile capability covers this yet')?.next).toMatch(
      /Profiles page/,
    )
  })

  it('paraphrases nothing it does not know', () => {
    // The fall-through, which is the one that has to be silent rather than
    // clever. A reason from a build that has added one since this table was
    // written is somebody's real machine, and a sentence this shell invented over
    // it would be worse than the log line.
    expect(explainReadiness('adapter: something nobody has written down yet')).toBeUndefined()
    expect(explainReadiness('window-keys is switched off')).toBeUndefined()
    // Empty is not a reason either: a ready row has no detail, and "no detail"
    // and "a detail with no words" have to land on the same path so the control
    // can draw one of them and neither can be mistaken for the other.
    expect(explainReadiness('')).toBeUndefined()
    expect(explainReadiness('   ')).toBeUndefined()
  })

  it('keeps the tone on the mark, out of the words', () => {
    // The reference's rule, from the diagnosis rows: the mark carries the tone
    // and the text does not (OnboardingView.swift:262-286 — orange or green on
    // the icon, MMColor.label on the text). So a tone may not appear as a word
    // in the sentence, or a screen reader would hear the difference a
    // colour-blind reader is the only one who was making.
    for (const { detail } of DAEMON_REASONS) {
      const reason = explainReadiness(detail)
      expect(reason?.what ?? '', detail).not.toMatch(/\b(error|warning|danger|red|green|orange)\b/i)
    }
  })

  it('names a mark for every tone, and the three the reference uses', () => {
    // ready / not-ready / not-checked, which are the three marks the reference
    // draws: checkmark, exclamationmark.triangle and circle.dashed
    // (OnboardingView.swift:210-213, :262-276).
    expect(Object.values(CHIP)).toEqual(['Ready', 'Not ready', 'Not ready', 'Not checked'])
  })
})
