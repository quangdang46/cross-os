// The daemon's readiness reasons, in words a person can act on.
//
// WHY THIS FILE EXISTS. A readiness row's `detail` is the daemon explaining
// itself to another program. "adapter: tap refused (input-monitoring consent
// missing?)" is a Go error value with a package prefix and a hedge on the end
// (core/internal/adapter/tap_cgo.go:43); it is what `err.Error()` hands back and
// it is correct, and it is not a sentence anybody fixes a permission from. The
// checklist is a FIRST-RUN surface, and the project's own principle is
// "Non-tech first, developers second" — so the row has to say what is wrong and
// what to do, and the raw string has to move out of the headline.
//
// Nothing here decides anything. The verdict is still `row.ready`, the reason
// is still the daemon's, and the raw `detail` is still rendered under the plain
// one on every row (ChecklistControl's ReadinessLine) plus carried in the row's
// `title`. That last part is the rule lib/format.ts already sets for any
// formatter ("a caller that hides the raw value must keep it reachable"): a
// mapping that guesses wrong would quietly misreport somebody's machine, and the
// About page plus the status line's "tap still unavailable: …" note
// (core/cmd/crossos/main.go:708) are where the raw words are read on purpose.
//
// NO URL, NO DEEP LINK. The one settings pane this daemon can open is
// Accessibility, and it refuses any other by name (main.go:479-495, the
// accessibilityPane const at :469 and handleOpenSettings' "this daemon opens only
// accessibility"). A sentence that spelled out a pane the daemon cannot open
// would be the same bug wearing different words, so what these say is the
// setting's NAME — which is what a person reads in System Settings and what
// menumate's own button says ("Open Accessibility Settings",
// onboarding.step3.openAccessibility). The door is the page's existing
// `permissions.openSettings` action, not a sentence pretending to be one.
//
// THE SHAPE is menumate's, from four files read whole:
//
//   App/UI/OnboardingView.swift:198-213  permissionRow — an icon, a 13pt
//     semibold title, an 11.5pt `MMColor.label2` description UNDER it, and a
//     verdict mark on the right that is drawn only when the state is known
//     (`granted: Bool?`, so an unread state is `circle.dashed`, never a red
//     cross). The description says what the grant BUYS YOU, not what the grant
//     is called: onboarding.step3.accessibility.desc is "Simulate ⌘↑ (Go Up) in
//     upload/open dialogs", never "requires the AXIsTrusted API".
//   App/UI/GeneralTab.swift:44-55, 62-75  the same two lines in a preferences
//     row (title 13/label, description 11/label3, spacing 1), with the action as
//     a button beside the row rather than as the tail of the sentence.
//   App/UI/OnboardingView.swift:259-286  the diagnosis rows, which are the
//     closest thing in the reference to this file: a mark that carries the TONE
//     (exclamationmark.triangle.fill in orange, checkmark in green) while the
//     TEXT stays in `MMColor.label` and the sub-line in `label3`. The tone is
//     never in the words, so a colour-blind reader and a screen reader get the
//     same sentence.
//   App/UI/PacksScreen.swift:372-405, GeneralTab.swift:197-230  the empty and
//     destructive states: a state in one voice, what it costs in a second, and
//     the next action in a third — never all three run together in one line.
//
// And the wording voice, from the reference's own English strings
// (App/Localizable.xcstrings):
//
//   onboarding.diag.extNotRegistered  "Extension not registered (run the main
//     app once first)" — a state, then the next action, in a PARENTHESIS. That
//     is the whole shape this file ports, and it is the answer to "does it name
//     the next action": yes, but as an aside to the state, not welded onto it.
//   onboarding.diag.extNotEnabled     "Finder extension not yet enabled" — a
//     state with NO action, because there is nothing to press. Not every state
//     has a door, and a row that invented one would be lying.
//   test.description                  "…This is not a sandbox: scripts still
//     run with your user permissions." — a fact about the machine, said flatly,
//     with no hedge in it.
//
// TWO WARNINGS THIS FILE CARRIES, because both are load-bearing and both are
// about the daemon rather than about the shell:
//
//   1. The daemon's keyboard-row error names INPUT MONITORING (tap_cgo.go:43)
//      and the daemon's only settings deep link opens ACCESSIBILITY
//      (main.go:468-470, whose own comment says "every not-ready keyboard row
//      points the user here"). The daemon disagrees with itself about which
//      grant a red keyboard row is asking for. The sentence below names BOTH
//      grants and the one place they live, because that is the only sentence
//      true of this daemon; asserting either one alone would send somebody to a
//      screen that cannot fix their problem, which is the failure
//      handleOpenSettings' "would send someone to a screen that cannot fix their
//      problem" comment (main.go:483-485) is written to prevent.
//   2. These sentences name PAGES — Safety, Extensions, Profiles, Shortcuts,
//      Activity — by the titles the nav actually serves (app/backend/pages.go
//      contrib() calls: Safety :81, About :98, Extensions :123, Activity :140,
//      Keyboard :192, Windows :232, Shortcuts :263; Profiles in
//      app/backend/profilespage.go:38). A page title is data the daemon serves;
//      it is not composed here, it is quoted, and a renamed page shows a name
//      that has to be re-read in this table.

/**
 * What a state's TONE is, in the reference's four marks.
 *
 * Ported from the two tone vocabularies the reference has, and deliberately
 * merged: menumate carries a `BadgeTone` (DesignSystem.swift:496-517 — gray,
 * accent, green, orange, red) for a per-row mark and a `BannerTone` (:633-652 —
 * orange, red, accent, info) for a block, and the diagnosis rows use neither
 * enum but pick icons directly (OnboardingView:262-276). Four values cover every
 * readiness state, and each is a MARK rather than a text colour — the words
 * always read the same regardless of tone, which is the accessibility half of
 * the port.
 *
 *   ready       — the daemon says it is. green, the checkmark.
 *   switchedOff — something is off and it is the person's to flip. orange, the
 *                 triangle: DeclutterSheet.swift:133 `Badge(…, tone: .orange)` on
 *                 the group it recommends hiding.
 *   refused     — a permission or a hook was refused. red, DeclutterSheet.swift:165
 *                 `Badge("declutter.rejected", tone: .red)`.
 *   unread      — nothing is known. gray, `circle.dashed` at OnboardingView:212.
 *                 Never red: a state the app cannot read is not a failure, and
 *                 the Bool? mark at :210-213 is the reference for that.
 */
export type ReasonTone = 'ready' | 'switchedOff' | 'refused' | 'unread'

/**
 * One sentence of a reason, either already written or a function of the daemon's
 * own line. The function form is for the handful of reasons the daemon COMPOSES
 * — a plugin id, a capability label, a count — which is a third of them; a table
 * that could only hold literals would cover less than half the strings the
 * daemon actually sends, and the uncovered half is exactly the half a reader
 * would have to decode.
 */
type Sentence = string | ((detail: string) => string)

/** One reason, in the reference's two sentences: what is wrong, then what to do. */
export interface PlainReason {
  /**
   * What is wrong, or what it costs — one flat sentence about THIS machine.
   * Never a package prefix, never a `?` hedge, never a Go type name. Empty
   * only when there is nothing wrong to say, which is a ready row's case.
   */
  what: string
  /**
   * The next action, as an imperative, or `''` when there is no door to point
   * at. The reference is honest about that second case
   * (`onboarding.diag.extNotEnabled` names a state and offers nothing), and a row
   * that invented a door would be worse than a row that names none.
   */
  next: string
  tone: ReasonTone
}

/** A table row before the composed sentences are resolved against a detail. */
interface Unresolved {
  what: Sentence
  next: Sentence
  tone: ReasonTone
}

/** The chip word for a verdict, in the reference's mark vocabulary. */
export const CHIP: Record<ReasonTone, string> = {
  ready: 'Ready',
  switchedOff: 'Not ready',
  refused: 'Not ready',
  unread: 'Not checked',
}

/**
 * One entry: a matcher for a daemon string and the words that replace it.
 *
 * `test` is a predicate rather than a string key because HALF of what the daemon
 * sends is composed — a plugin's id, a capability's label, a count — and a table
 * keyed on the literal would only cover the half the daemon hard-codes. Every
 * `test` below is a full match, never a substring: two daemon strings that share
 * a tail ("plugin actions stopped by PANIC STOP" and "plugin disabled") must not
 * collapse into one sentence.
 */
interface Entry {
  test: (detail: string) => boolean
  /** The daemon file and line this string is written at, for the reader. */
  from: string
  reason: Unresolved
}

/** A daemon string, written out in full at exactly one place in the Go source. */
function exact(written: string): (detail: string) => boolean {
  return (detail) => detail === written
}

/** Both pages the settings path lives on, named as a person reads them. */
const GRANTS =
  'Turn on CrossOS under Input Monitoring and Accessibility in System Settings › Privacy & Security.'

/** The re-read this page already offers as a button (`permissions.verify`). */
const VERIFY = 'Choose Verify to ask the daemon again.'

/** Every not-ready reason the daemon can put in a readiness row's `detail`. */
const ENTRIES: Entry[] = [
  {
    // core/cmd/crossos/pagedata.go:633, and the two spellings the tap path
    // writes for the same latched switch (main.go:668 and main.go:1115 differ
    // only in the em dash vs the bracket).
    from: 'core/cmd/crossos/pagedata.go:633, main.go:668, main.go:1115',
    test: (d) =>
      d.includes('PANIC STOP is latched') ||
      d.includes('interception stopped by PANIC STOP'),
    reason: {
      what: 'The safety stop is switched on, so CrossOS is not handling any shortcut.',
      next: 'Re-enable it from the Safety page.',
      tone: 'refused',
    },
  },
  {
    // core/cmd/crossos/pagedata.go:635 — "daemon state: " + the lifecycle
    // string, which is only non-empty on a daemon that is not running or in
    // safe mode (the Ready term at :628-629). The state name itself is left
    // out of the sentence on purpose: `initializing`, `paused`, `stopped` and
    // `disabled` are four different situations with one honest plain form
    // between them, and guessing which one is showing would be the formatter
    // deciding what is true.
    from: 'core/cmd/crossos/pagedata.go:635',
    test: (d) => d.startsWith('daemon state: '),
    reason: {
      what: 'The CrossOS daemon is not running, so no shortcut is being handled.',
      next: 'Open the Activity page to see what it said, then start CrossOS again.',
      tone: 'switchedOff',
    },
  },
  {
    // core/internal/adapter/tap_cgo.go:43 — the string in the screenshot. The
    // log line, and the reason the raw value is no longer the headline.
    from: 'core/internal/adapter/tap_cgo.go:43',
    test: exact('adapter: tap refused (input-monitoring consent missing?)'),
    reason: {
      what: 'CrossOS needs permission to see the keys you press before it can record a shortcut.',
      next: GRANTS,
      tone: 'refused',
    },
  },
  {
    // core/internal/adapter/driver.go:27 — the same refusal reached by the
    // accessibility path, so it gets the same sentence rather than a second
    // wording of the same dead end.
    from: 'core/internal/adapter/driver.go:27',
    test: exact('adapter: accessibility permission denied'),
    reason: {
      what: 'CrossOS needs permission to see the keys you press before it can record a shortcut.',
      next: GRANTS,
      tone: 'refused',
    },
  },
  {
    // core/cmd/crossos/pagedata.go:644 — the case the daemon's own comment at
    // :639-641 calls out: uninstalled with no failure to quote.
    from: 'core/cmd/crossos/pagedata.go:644',
    test: exact('keyboard interception is not installed'),
    reason: {
      what: 'The keyboard hook is not installed, so nothing is being recorded yet.',
      next: VERIFY,
      tone: 'switchedOff',
    },
  },
  {
    // core/internal/adapter/driver.go:30.
    from: 'core/internal/adapter/driver.go:30',
    test: exact('adapter: tap/hook not installed'),
    reason: {
      what: 'The keyboard hook is not installed, so nothing is being recorded yet.',
      next: VERIFY,
      tone: 'switchedOff',
    },
  },
  {
    // core/cmd/crossos/pagedata.go:817 — tapDegraded()'s first term.
    from: 'core/cmd/crossos/pagedata.go:817',
    test: exact('keyboard tap keeps timing out — remapping degraded'),
    reason: {
      what: 'The keyboard hook is installed but not answering, so some shortcuts are being missed.',
      next: VERIFY,
      tone: 'switchedOff',
    },
  },
  {
    // core/cmd/crossos/pagedata.go:820 — tapDegraded()'s second term, with a
    // count the sentence keeps: the reference's own outcome line carries its
    // number too (DeclutterSheet.swift:102 `String(format: …, hiddenCount)`),
    // because a count is what makes "some" actionable.
    from: 'core/cmd/crossos/pagedata.go:820',
    test: (d) => /^\d+ shortcut action\(s\) could not run — remapping degraded$/.test(d),
    reason: {
      what: 'Some shortcuts could not run, so a few key presses did nothing.',
      next: 'Open the Activity page to see which ones.',
      tone: 'switchedOff',
    },
  },
  {
    // core/internal/adapter/tap_cgo.go:215.
    from: 'core/internal/adapter/tap_cgo.go:215',
    test: exact('adapter: tap already running'),
    reason: {
      what: 'The keyboard hook was already running, so there was nothing to change.',
      next: '',
      tone: 'unread',
    },
  },
  {
    // core/internal/adapter/tap_cgo.go:272.
    from: 'core/internal/adapter/tap_cgo.go:272',
    test: exact('adapter: tap install still in progress (no response yet)'),
    reason: {
      what: 'The keyboard hook is still being installed.',
      next: VERIFY,
      tone: 'switchedOff',
    },
  },
  {
    // core/internal/adapter/tap_cgo.go:305 — the %s is a duration.
    from: 'core/internal/adapter/tap_cgo.go:305',
    test: (d) => d.startsWith('adapter: tap run loop did not stop within '),
    reason: {
      what: 'The keyboard hook did not shut down when it was asked to.',
      next: 'Quit CrossOS and open it again.',
      tone: 'switchedOff',
    },
  },
  {
    // core/internal/adapter/seam_errors.go:7. A build with no bridge is not a
    // permission problem, and the UnsupportedControl header's rule — that a row
    // which is not a runtime failure must not be dressed as one — is why this
    // one gets the unread tone and no action: there is nothing the person can do.
    from: 'core/internal/adapter/seam_errors.go:7',
    test: exact('adapter: native bridge not yet wired (spike-proven shape, link pending)'),
    reason: {
      what: 'This build has no keyboard hook for this machine, so shortcuts cannot be recorded.',
      next: '',
      tone: 'unread',
    },
  },
  {
    // core/internal/adapter/errors.go:6.
    from: 'core/internal/adapter/errors.go:6',
    test: exact('adapter: not supported on this platform'),
    reason: {
      what: 'Keyboard shortcuts are not supported on this system.',
      next: '',
      tone: 'unread',
    },
  },
  {
    // core/cmd/crossos/pagedata.go:682, :700 and :730 — three call sites, one
    // string, so one sentence.
    from: 'core/cmd/crossos/pagedata.go:682, :700, :730',
    test: exact('plugin actions stopped by PANIC STOP'),
    reason: {
      what: 'The safety stop is switched on, so this is not running.',
      next: 'Re-enable it from the Safety page.',
      tone: 'refused',
    },
  },
  {
    // core/cmd/crossos/pagedata.go:702 — surfaceRow's empty case, which the
    // daemon's comment at :697-698 calls out: an empty surface says so rather
    // than reporting a green tick for nothing.
    from: 'core/cmd/crossos/pagedata.go:702',
    test: exact('no rule or profile capability covers this yet'),
    reason: {
      what: 'Nothing in this build does this yet, so there is no switch to turn on.',
      next: 'Choose a profile on the Profiles page.',
      tone: 'unread',
    },
  },
  {
    // core/cmd/crossos/pagedata.go:684 — the per-plugin row's own case, which is
    // a different sentence from the surface row's: this one IS a switch the
    // person can flip, and the surface row's is not.
    from: 'core/cmd/crossos/pagedata.go:684',
    test: exact('plugin disabled'),
    reason: {
      what: 'This extension is switched off, so it is not running.',
      next: 'Switch it on from the Extensions page.',
      tone: 'switchedOff',
    },
  },
  {
    // core/cmd/crossos/pagedata.go:706 and :752 — the plugin id is spliced in by
    // the daemon (`p + " is not switched on"`), so this is the first table
    // entry that has to read a value out of the sentence it is replacing. The
    // id is the daemon's (a page id is not a thing src/ may name), and it is
    // humanized for the person and left whole in the raw line below.
    from: 'core/cmd/crossos/pagedata.go:706, :752',
    test: (d) => /^\S+ is not switched on$/.test(d),
    reason: {
      what: (d) => {
        const name = human(d.replace(/ is not switched on$/, ''))
        return `“${name}” is switched off, so the shortcuts it carries are not running.`
      },
      next: 'Switch it on from the Extensions page.',
      tone: 'switchedOff',
    },
  },
  {
    // core/cmd/crossos/pagedata.go:735.
    from: 'core/cmd/crossos/pagedata.go:735',
    test: exact('no profile is chosen yet'),
    reason: {
      what: 'You have not chosen a profile yet, so no set of shortcuts is in place.',
      next: 'Choose one on the Profiles page.',
      tone: 'switchedOff',
    },
  },
  {
    // core/cmd/crossos/pagedata.go:740 — the id is strconv.Quote'd, quotes
    // included, so the sentence keeps them rather than printing a Go-escaped
    // string at somebody.
    from: 'core/cmd/crossos/pagedata.go:740',
    test: (d) => d.startsWith('no profile bundle is named "') && d.endsWith('"'),
    reason: {
      what: (d) => {
        const name = d.slice('no profile bundle is named '.length).replace(/^"|"$/g, '')
        return `This machine remembers a profile called “${name}”, which this build does not ship.`
      },
      next: 'Choose a profile again on the Profiles page.',
      tone: 'switchedOff',
    },
  },
  {
    // core/cmd/crossos/pagedata.go:754 — "%s has %d of %d shortcuts switched
    // off": a capability's label and two counts. The counts stay, because the
    // reference's outcome sentences keep theirs (DeclutterSheet.swift:102, and
    // the trial countdown the shell already renders the same way).
    from: 'core/cmd/crossos/pagedata.go:754',
    test: (d) => /^.+ has \d+ of \d+ shortcuts switched off$/.test(d),
    reason: {
      what: (d) => {
        const [, name, off, total] = d.match(/^(.+) has (\d+) of (\d+) shortcuts switched off$/) as string[]
        return `“${name}” has ${off} of its ${total} shortcuts switched off, so they do nothing yet.`
      },
      next: 'Turn the rest on from the Shortcuts page.',
      tone: 'switchedOff',
    },
  },
  {
    // core/cmd/crossos/pagedata.go:1634 — the first-run step's own detail. It
    // is here rather than in the wizard because the wizard renders it through
    // this same table, and a step that said it one way and the row beside it
    // another is the two-answers bug ReadinessLine exists to prevent.
    from: 'core/cmd/crossos/pagedata.go:1634',
    test: exact('no plugin is switched on yet'),
    reason: {
      what: 'No extension is switched on yet, so no shortcut can be handled.',
      next: 'Switch one on from the Extensions page.',
      tone: 'switchedOff',
    },
  },
  {
    // core/cmd/crossos/pagedata.go:1636 — "%d of %d checks are not ready". The
    // one reason that is already plain, so it is mapped to itself: entering the
    // table is what stops it from being the one row on the page still quoted raw.
    from: 'core/cmd/crossos/pagedata.go:1636',
    test: (d) => /^\d+ of \d+ checks are not ready$/.test(d),
    reason: {
      what: (d) => {
        const [off, total] = d.replace(' checks are not ready', '').split(' of ')
        return `${off} of the ${total} checks are not ready yet.`
      },
      next: '',
      tone: 'switchedOff',
    },
  },
]

/**
 * human is a two-line, dependency-free stand-in for lib/format's humanize.
 *
 * It is duplicated on purpose rather than imported: lib/format's humanize
 * capitalises and splits an IDENTIFIER, and these sentences need the opposite —
 * a name that keeps its hyphens and its lower case inside quotes, because
 * "window-keys" is what the Extensions page will show on the row and quoting it
 * re-spaced would name something the page does not have.
 */
function human(id: string): string {
  return id.replace(/[_-]+/g, ' ')
}

/**
 * explainReadiness turns one daemon `detail` into words a person can act on, or
 * answers `undefined` — which is the honest answer and the one the control acts
 * on, because a reason this table does not know must NOT be dressed in a
 * stranger's sentence. Every daemon string in core/cmd/crossos/pagedata.go's
 * readiness rows, the adapter errors main.go:1124 stores verbatim, and the
 * first-run step details are in here; a string from a build that has added a
 * reason since is not, and falls through to the raw line rather than to a
 * paraphrase.
 *
 * The caller already has the row, and a READY row's `detail` is empty — so an
 * empty detail is not a reason and is answered `undefined` too, which is what
 * keeps "no detail" and "a detail nobody has words for" on the same path: both
 * render as the daemon's line, and the first renders as no line at all.
 */
export function explainReadiness(detail: string): PlainReason | undefined {
  const trimmed = detail.trim()
  if (trimmed === '') return undefined
  for (const entry of ENTRIES) {
    if (!entry.test(trimmed)) continue
    const { what, next, tone } = entry.reason
    return {
      what: typeof what === 'function' ? what(trimmed) : what,
      next: typeof next === 'function' ? next(trimmed) : next,
      tone,
    }
  }
  return undefined
}

/**
 * The reasons this table knows, for the About page and for anyone auditing
 * whether a build's daemon is covered. Exported so a test can assert the
 * coverage rather than a comment claiming it.
 */
export function knownReasonCount(): number {
  return ENTRIES.length
}
