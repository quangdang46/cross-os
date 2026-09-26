// The first-run wizard (bead w6-frontend-setup-renderers).
//
// The steps are DECLARED, never written here: a page sends `steps` and this
// renders them in order. A shell that carried its own four steps would be a
// second copy of the setup flow, and a page that wanted five would get four —
// §3.6c's rule is that a screen is a Go change and never a UI change.
//
// EVERY VERDICT IS THE DAEMON'S. Which step is done, what is still missing from
// it, and whether setup is finished all arrive on one row (the onboardingState read,
// bound as OnboardingState) and this control renders them. It computes none of
// them: the earlier version derived "complete" from rows.every(ready) and kept a
// readiness count of its own, which is a second opinion about a question the
// daemon has already answered — and the two disagree the moment a machine is
// finished by some route other than a green checklist, which is exactly the user
// onboard-1 built the flag for. A done mark this shell invented is a small lie
// that tells a person a permission landed when it did not, so there is nowhere
// in this file to write one.
//
// The one thing held here is which step's body is OPEN on screen, and that is a
// view cursor, not a verdict: it moves when the reader clicks a step and nowhere
// else. The daemon's own cursor is a different fact and is drawn beside it as its
// own mark (current_step), so "the step I am reading" and "the step the daemon is
// waiting on" can never be collapsed into one number that silently picks a
// winner. Two cursors on one flow is the disagreement the row exists to prevent,
// and hiding the second one behind the first would rebuild it.
//
// The steps' writes are the page's DECLARED actions, dispatched through the
// action registry (controls/actions.ts), so an action id appears in exactly one
// table and a control never composes a call the daemon never reviewed. The one
// exception is the finish, and it is not an exception of convenience: finishing
// is not a capability a page lends the wizard, it is the wizard's own last verb
// (CompleteOnboarding), and the daemon serves it for exactly one caller. Routing
// it through a page's action list would mean the shell read its last step out of
// whatever id happened to be declared last — which is how "Finish setup" came to
// re-read readiness and report success without ever recording anything.
//
// A step's body is declared too: `body` names a control KIND whose renderer the
// step draws inline, so the Windows profile is picked from inside the flow
// instead of on a separate page, and a page that wants something else there
// declares something else. The map below is keyed by kind, exactly like the
// registry in index.tsx — no page id and no control id appears in a conditional
// anywhere in this file.
//
// Port source: pcfy-my-mac (raxigan/pcfy-my-mac), the installer that asks for a
// machine's shape before it changes it.
//
//   cmd/param/survey.go:8-14     one question per decision, in order, each with
//                                its OWN body — the survey carries a prompt and a
//                                help string PER question, never one shared blurb
//   cmd/param/param.go:120-138   a question the file has already answered is
//                                deleted from the list and never asked again
//   cmd/common/await.go:30-56    Until(condition, WithPollInterval, WithTimeout)
//                                — ask the machine again on a tick rather than
//                                once, and give up at a NAMED deadline instead
//   cmd/task/task.go:63-76       the two applied together: a command runs, then
//                                the artefact it made is polled for on a 1000ms
//                                interval up to a 60s timeout
//   cmd/launcher.go:72-80        the closing "Almost ready!" hand-off — the
//                                install is not done when the commands return,
//                                it is done once a person has granted what the
//                                list names, so the last thing printed is that
//                                list rather than a success word
//
// The second reference is the rule this file exists to obey: a step the daemon
// has already derived as done is not asked again, which is the daemon-derives-done
// rule applied to steps rather than to questions. Both were flattened before —
// every step shared one body and the wizard asked all four regardless — and the
// flattening is what the per-step detail below puts back.
//
// The third and fourth are why the LAST step is unlike the others, and why that
// difference is POSITIONAL rather than keyed on any id. The reference polls
// because the command it just ran has not finished taking effect — the app it
// installed is not on disk yet — and it polls to a named budget because a wait
// with no deadline is not a wait. The wizard has the same gap: a person grants
// Accessibility in System Settings and comes straight back, and the tap has not
// reinstalled yet, so a SINGLE read says "not ready" about a permission that is
// already granted. Reading once is the very bug the reference's Until exists to
// fix, so the last step polls the readiness verb the checklist already reads —
// a wait, never a verdict — and when the budget runs out it names the timeout
// and hands the reader the list of what is still to grant, which is what
// launcher.go prints instead of "Installed successfully".
//
// No code was copied: the reference is a survey-driven zsh/Go installer and this
// is a declared-schema renderer, so what transfers is the shape — an ordered
// list of named steps, one open at a time, each with its own body, each with its
// own verdict, a bounded wait, and a hand-off instead of a finish line. Tracked
// in third_party/pcfy-my-mac/ATTRIBUTION.md.
//
// THE CHROME IS A SECOND PORT, of menumate's review list — the review gate on the
// pack import sheet, which is the closest thing in that repository to a wizard's
// step list: a list of things a person has to get through, each row carrying a
// mark, the open row filled, and one line under the whole thing saying how much
// is left. The three things that transfer are the MARK, the ROLLUP, and the CARD
// the list sits in; the steps, their order and their wording are the Go flow's
// and are not touched by this port.
//
//   App/UI/PackImportSheet.swift:230-277  reviewList
//     :236-245   a row draws checkmark.circle.fill once its id is in the review
//                set and a 14x14 stroked Circle (1.3pt, label4) until then —
//                so progress is something you SEE down the left edge rather than
//                a word you have to read on each row
//     :239       the mark is WHITE on the selected row and green off it, so the
//                selected row keeps its own ink
//     :253-254   the row's own padding, 10 horizontal and 7 vertical
//     :257       the separator is an overlay on the TOP of every row after the
//                first, so the first row has no rule above it
//     :263-269   the container: MMColor.card inside a RoundedRectangle(8,
//                .continuous) with a 0.5pt MMColor.hairline stroke
//     :271-275   the rollup, UNDER the list: 11pt, label3, leading-aligned,
//                8pt above the card
//   App/UI/Localizable.xcstrings:3268     the rollup's English value,
//                "Viewed %lld / %lld · view all to continue". The "n of m"
//                half is the shape and is kept; the "view all to continue" half
//                is that screen's GATE, and the wizard's Next is not gated on
//                being looked at, so the clause has no honest translation here
//                and is not printed.
//   App/UI/PacksScreen.swift:342-345        the same 8pt-continuous card on a
//                second list in the same app, which is why the radius below is a
//                value of its own and not one of the MMRadius scale constants
//   App/UI/DesignSystem.swift:36, :58     MMColor.green (a semantic colour, not a
//                literal) and MMColor.hairline (separatorColor at 0.7 opacity)
//
// TWO THINGS ARE DELIBERATELY NOT THE REFERENCE'S, and both are because the
// daemon owns every verdict on this page (see the header above):
//
//   - The mark here is the DAEMON's `done`, not a set the reader builds by
//     looking. The reference's row inserts its id into `viewed` on tap
//     (:260-263), so its mark is an acknowledgement; a mark this shell wrote on
//     click would be the shell inventing a verdict, which is the one thing this
//     control exists not to do. So the tap still only SELECTS, and the mark
//     changes when the daemon's next read says it changed.
//   - The mark therefore does not replace the step NUMBER. The reference's list
//     is a flat set of actions and its order is the order they appear in; a
//     wizard is navigated by position ("Step 1 of 4" below, and the Back/Next
//     pair), so the ordinal stays and the mark is added beside it. What the port
//     replaces is the STATUS WORD as the primary signal — a reader can now see
//     how far along they are without reading four chips.
//
// The rollup counts the MARKS in the list above it, which is the reference's
// relationship, and which is why the readiness sentence this file used to print
// is gone. "1 check of 8 ready" and the daemon's own "7 of 8 checks are not
// ready" on the Verify step are one fact said twice in two places, and the
// daemon's copy is the one this shell does not own. The names that sentence
// carried ("Still to do: …") are not lost: every failing row is drawn with its
// reason by the checklist control below this one, and the last step's hand-off
// draws them again when the wait runs out.
//
// No code was copied from menumate either: the reference is SwiftUI and this is
// React over a declared schema, so the row, the mark and the card transfer as
// STRUCTURE and as a set of measurements, and the drawing does not. Tracked in
// third_party/menumate/ATTRIBUTION.md.

import { useCallback, useEffect, useRef, useState } from 'react'
import type { ReactElement } from 'react'
import type { OnboardingRow, OnboardingStep, ReadinessRow, WizardStep } from '../types/controls'
import { humanize, plural } from '../lib/format'
import { asBool, asList, asNumber, asRecord, asText, describeError, failedTo } from '../lib/wire'
import { useResource } from '../lib/useResource'
import { commandFor } from './actions'
import { ProfileListControl } from './ProfileListControl'
import { ReadinessLine } from './ChecklistControl'
import { explainReadiness } from '../lib/readinessReasons'
import { ControlFrame, EmptyState } from './common'
import type { ControlProps } from './common'

/**
 * The daemon's spelling for "I have no unfinished step" (core:onboardingState
 * sets current_step to this once every step it derives is done). It is a cursor
 * value, not a step id, and it is compared as one — a page that declared a step
 * whose title happened to read "Done" must not be mistaken for the flow ending.
 */
const NO_STEP_LEFT = 'done'

/**
 * The wait the last step makes, in the reference's own numbers.
 *
 * pcfy-my-mac polls the artefact it just created every 1000ms and gives up at
 * 60s (cmd/common/await.go:30-56, applied at cmd/task/task.go:63-76 with
 * WithPollInterval(1000*time.Millisecond) and WithTimeout(60*time.Second)). The
 * pair is not decoration: a poll cadence under a second would hammer a daemon
 * that is installing an event tap, and a budget past a minute is longer than a
 * person will sit watching a spinner, so the shell says the number rather than
 * spinning forever.
 *
 * The same reasoning is why the wait is here and not in useResource. That hook's
 * rule is "one read per refresh token, and the cadence belongs to App" — which
 * is right for a page that is showing a snapshot. The last step is not showing
 * a snapshot; it is waiting for the machine to catch up with a write the person
 * just made OUTSIDE the app, and the shell's 5s poll is far too slow to be the
 * thing that notices. So this is a second, deliberate, BUBBLE-SCOPED read that
 * exists only while the last step is open, and the reason it is allowed to
 * break the one-read rule is written down rather than assumed.
 */
const SETTLE_POLL_MS = 1000
const SETTLE_TIMEOUT_MS = 60_000

/** The empty row a first paint shows, and the answer to a source that has not
 *  landed yet. Every field is the daemon's own neutral value, so a step drawn
 *  before the first answer reads as "not done" rather than as a verdict. */
const NO_STATE: OnboardingRow = {
  completed: false,
  current_step: '',
  steps: [],
  readiness: [],
  ready: 0,
  total: 0,
}

/**
 * The bodies a step may declare inline, keyed by control KIND.
 *
 * Kinds, never page or control ids, for the same reason the registry in
 * index.tsx is keyed by kind: a shell that drew a named step's body by
 * comparing strings would make the next page a UI change, which is the rule
 * §3.6c exists to prevent. A new body is a renderer and one entry here.
 */
const STEP_BODIES: Record<string, (props: ControlProps) => ReactElement> = {
  profileList: ProfileListControl,
}

/**
 * A step's own reason, in the same words the checklist row beside it uses.
 *
 * The daemon derives both from one function — handleOnboardingState calls
 * readinessRows() and copies the keyboard row's `Detail` into the permission
 * step's own (core/cmd/crossos/pagedata.go:1598-1618, `keyboardDetail` at
 * :1613) — so the same string reaches two screens. Printing it raw on one and
 * paraphrased on the other is the disagreement ReadinessLine's own header says
 * this control exists to prevent, one screen further out. So both go through
 * lib/readinessReasons.ts, and the two-line shape is the reference's row: the
 * state, then the action, then the daemon's own words underneath in the dimmer
 * voice (OnboardingView.swift:198-213 and :259-286; DeclutterSheet.swift:104-109
 * for the sub-line).
 *
 * The step detail is a `<p>` and not a `<li>` because it is not a row: there is
 * no chip and no id to key it by, and the two lines are one reason, not a list
 * entry pretending to be one.
 */
function StepReason(props: { detail: string }): ReactElement {
  const reason = explainReadiness(props.detail)
  if (!reason) {
    // Unmapped, and said to be unmapped. This slot used to be `ctl-empty`
    // unconditionally, which put the daemon's own line in the dimmer voice AND
    // said nothing about it being untranslated — a reader could not tell a
    // deliberate aside from a log line nobody had rewritten.
    return (
      <>
        <p className="ctl-value">{props.detail}</p>
        <p className="ctl-empty">This build has no plainer wording for that yet.</p>
      </>
    )
  }
  return (
    <>
      <p className="ctl-value">{reason.what}</p>
      {reason.next ? <p className="ctl-value">{reason.next}</p> : null}
      <p className="ctl-empty">{props.detail}</p>
    </>
  )
}

/**
 * actionLabel is what a person reads on the button. An action id is a
 * namespaced capability token, and §1 of the plan is explicit that daemon
 * vocabulary is not English — so the label is the last segment, humanized, and
 * the full id rides along in the note the shell prints after the write, so a
 * report can still name the capability that ran.
 */
function actionLabel(action: string): string {
  const tail = action.split('.').pop() ?? action
  return humanize(tail) || action
}

/** A declared step, whichever of the two forms the page wrote it in. */
function declaredStep(raw: string | WizardStep): WizardStep {
  return typeof raw === 'string' ? { label: raw } : raw
}

/**
 * One step of the daemon's answer, or the neutral row when the daemon has not
 * sent one. A step read off a row that does not exist must not become a verdict
 * in either direction, and `done: false` with no detail is the honest neutral:
 * "not yet", which is what an unanswered question is.
 */
function readStep(value: unknown): OnboardingStep {
  const row = asRecord(value)
  if (!row) return { id: '', label: '', done: false }
  return {
    id: asText(row.id),
    label: asText(row.label),
    done: asBool(row.done) ?? false,
    detail: asText(row.detail) || undefined,
  }
}

function readCheck(value: unknown): ReadinessRow {
  const row = asRecord(value)
  if (!row) return { id: '', label: '', ready: false, detail: '' }
  return {
    id: asText(row.id),
    label: asText(row.label),
    ready: asBool(row.ready) ?? false,
    detail: asText(row.detail),
  }
}

/**
 * readState narrows the wizard's one answer. The fields are read through the
 * same helpers every other control uses because a step machine that decoded a
 * missing step into `undefined` would render a done mark nobody asserted — the
 * exact failure these narrowers exist to stop, and the reason a wrong field
 * type degrades to "not done" rather than to a confident tick.
 */
function readState(value: unknown): OnboardingRow {
  const row = asRecord(value)
  if (!row) return NO_STATE
  return {
    completed: asBool(row.completed) ?? false,
    current_step: asText(row.current_step),
    steps: asList(row.steps).map(readStep),
    readiness: asList(row.readiness).map(readCheck),
    ready: asNumber(row.ready) ?? 0,
    total: asNumber(row.total) ?? 0,
  }
}

/**
 * verdict is the daemon's step joined to a declared one. A step that declares
 * an `id` is joined by that id, which is exact. A step that declares only a
 * title is joined by POSITION, because that is the only key it carries — the
 * daemon derives its steps in the flow's order and a page declares the flow in
 * that same order, so the two agree. A flow that wants a join it can rely on
 * declares ids; a flow that does not still gets a correct answer today, and one
 * that reorders its steps without ids is told so by the missing mark rather than
 * by a wrong one.
 */
function verdict(
  step: WizardStep,
  index: number,
  byId: Map<string, OnboardingStep>,
  byPosition: OnboardingStep[],
): OnboardingStep | undefined {
  if (step.id) return byId.get(step.id)
  return byPosition[index]
}

/**
 * What the settle has to say, which is a WAIT and never a verdict.
 *
 * The daemon still owns every mark on this page; all this carries is whether the
 * shell is still looking, what it saw, and whether the budget ran out. That
 * distinction is the whole point: a control that turned "everything is ready" into
 * a Done mark of its own would be inventing a verdict, and a control that turned
 * "the budget ran out" into "not ready" would be inventing a failure. Neither is
 * written, so the two cases stay tellable apart on screen.
 */
interface Settle {
  /** The rows from the most recent poll, which is the daemon's own answer. */
  rows: ReadinessRow[]
  /** How many reads have landed, so "still checking" can show its own progress. */
  polls: number
  /** True while the wait is still going. */
  waiting: boolean
  /** True once every row the daemon sent is ready. */
  settled: boolean
  /** True when the budget elapsed with rows still unready. */
  timedOut: boolean
  /** The last failure, kept in place like every other source's. */
  error: string
}

const NO_SETTLE: Settle = { rows: [], polls: 0, waiting: false, settled: false, timedOut: false, error: '' }

/**
 * useSettle is the reference's Until, in React.
 *
 * The condition is "every row the daemon sent is ready" and the bound is 60s, so
 * a person who grants a permission and watches the row turn green needs no second
 * click — which is the whole failure this fixes. Two properties of the port are
 * load-bearing:
 *
 *   - The interval is CLEARED on unmount and whenever `active` goes false, so a
 *     reader who walks off the last step is not polled for. The reference's
 *     Until has no such case because its process exits; a window does.
 *   - `armed` restarts the wait from zero. Without it, a reader who clicks
 *     Verify a second time would join a wait that had already burned 59 of its
 *     60 seconds and be told "timed out" for a permission they had just granted,
 *     which is the same lie in a new place. This is the "without a second click"
 *     half of the promise: one click is enough, and clicking again is not a way
 *     to buy more time than the budget allows.
 */
function useSettle(
  read: () => Promise<ReadinessRow[]>,
  refresh: () => void,
  active: boolean,
  armed: number,
): Settle {
  const [settle, setSettle] = useState<Settle>(NO_SETTLE)
  // The reader is re-created every render by its caller (it closes over ctx), so
  // it is read through a ref for the same reason useResource reads its loader
  // through one: a callback that changed identity each pass would restart the
  // effect on every render and poll in a loop.
  const reader = useRef(read)
  reader.current = read
  // Same for the refresh, and for the same reason: an inline arrow would be a
  // new function every pass and would restart the wait on every render.
  const ask = useRef(refresh)
  ask.current = refresh

  useEffect(() => {
    if (!active) {
      setSettle(NO_SETTLE)
      return
    }
    let live = true
    let timer: ReturnType<typeof setTimeout> | undefined
    const started = Date.now()
    let polls = 0

    const tick = (): void => {
      reader.current().then(
        (rows) => {
          if (!live) return
          polls += 1
          const list = Array.isArray(rows) ? rows : []
          // "Every row is ready" is the reference's condition(). A daemon that
          // reports NO rows is not ready — the machine has not confirmed
          // anything — so an empty answer keeps waiting rather than settling on
          // a vacuous truth, and the wait is what the timeout is for.
          const settled = list.length > 0 && list.every((row) => row.ready)
          const expired = !settled && Date.now() - started >= SETTLE_TIMEOUT_MS
          if (settled || expired) {
            // A poll that succeeded is a NEW answer, so the daemon's own row is
            // re-read: the wizard's step verdicts come from there and would
            // otherwise still show the state from before the grant. This is a
            // refresh, not a verdict — the daemon re-derives, the shell waits.
            if (settled) ask.current()
            setSettle({ rows: list, polls, waiting: false, settled, timedOut: expired, error: '' })
            return
          }
          setSettle({ rows: list, polls, waiting: true, settled: false, timedOut: false, error: '' })
          timer = setTimeout(tick, SETTLE_POLL_MS)
        },
        (reason: unknown) => {
          if (!live) return
          // A failed read is a failure the daemon already named; it is shown
          // where every other source's failure is shown, and the wait stops,
          // because polling a source that is answering "unavailable" sixty
          // times a minute helps nobody.
          const message = describeError(reason)
          setSettle({ rows: [], polls, waiting: false, settled: false, timedOut: false, error: message })
        },
      )
    }

    tick()
    return () => {
      live = false
      if (timer !== undefined) clearTimeout(timer)
    }
  }, [active, armed])

  return settle
}

export function WizardControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const actions = Array.isArray(control.actions) ? control.actions : []
  const steps = (Array.isArray(control.steps) ? control.steps : []).map(declaredStep)
  const state = useResource<OnboardingRow>(
    () => ctx.service.OnboardingState().then(readState),
    ctx.refreshToken,
    NO_STATE,
    ctx.note,
  )

  // The open step is view state, and it is the ONLY state here. It resets when
  // the page declares a different number of steps, which is the one case where
  // holding the old index would point at a step that no longer exists.
  const [open, setOpen] = useState(0)
  useEffect(() => {
    setOpen(0)
  }, [steps.length])

  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  const [outcome, setOutcome] = useState('')

  // The last step is the one that waits, and it waits because it is LAST — not
  // because its id reads "verify". A page that calls its last step something
  // else gets the same wait, and a page that inserts a step after it gets the
  // wait on the new last one, which is the only way this rule can stay true for
  // a flow nobody has read yet. A step named by id here would be a page id in a
  // conditional, which is the thing §3.6c exists to prevent.
  const lastIndex = steps.length - 1
  // Bumped by the verify click, which restarts the wait from a full budget.
  const [armed, setArmed] = useState(0)
  const readReadiness = useCallback(async () => (await ctx.service.Readiness()) ?? [], [ctx.service])
  const askDaemon = useCallback(() => ctx.refresh(), [ctx.refresh])
  const settle = useSettle(readReadiness, askDaemon, lastIndex > 0 && open === lastIndex, armed)

  const row = state.data
  const byId = new Map(row.steps.map((step) => [step.id, step]))
  const checks = row.readiness
  // The rollup the reference prints under its list, counting the marks above it
  // and nothing else (PackImportSheet.swift:271-275, viewed.count of
  // actions.count). Every count here is a step the DAEMON has derived as done, so
  // a step the daemon has not answered for is counted as not done — which is
  // the same neutral the mark beside it draws, and neither is a verdict the
  // shell took. It is stated once, here, rather than once per row and once again
  // in a sentence further down the page.
  const doneSteps = steps.filter((step, index) => verdict(step, index, byId, row.steps)?.done).length
  // The daemon's own answer to "is anything left to do", not a count the shell
  // took. Its cursor names the first step it has not derived as done and reads
  // "done" once it has none, so a flow finished by the person saying so — the
  // one thing the daemon cannot re-derive from live state — ends here too.
  const nothingLeft = row.current_step === NO_STEP_LEFT

  // What the HAND-OFF lists is the poll's own rows when there are any, because
  // they are the freshest answer the daemon has given and the reader is about to
  // go and act on them; the onboarding row's copy is from before the last poll
  // and naming a permission the person has already granted is the exact
  // mistake the wait exists to stop. The count above still comes from the
  // daemon's row, because that is the row the STEP verdicts came from and the
  // two must not be read as one number from two sources.
  const handoff = settle.rows.length > 0 ? settle.rows : checks
  const outstanding = handoff.filter((check) => !check.ready)

  async function run(action: string): Promise<void> {
    const command = commandFor(action)
    if (!command) {
      // Declared by the page, understood by nobody — and commandFor answering
      // undefined means the id is in neither registry, which the coverage test
      // in renderers.test.tsx fails on.
      const message = `"${action}" is declared on this wizard, and this build has no command for it.`
      setError(message)
      ctx.note(message)
      return
    }
    setBusy(action)
    setError('')
    setOutcome('')
    try {
      await command(ctx.service)
      setOutcome(`${actionLabel(action)}: done.`)
      ctx.note(`${actionLabel(action)} (${action}): done.`)
      // A write the reader just made is the thing the wait is FOR: the tap has
      // to reinstall, and the daemon is not going to say so until it has. So
      // any declared action run from the last step restarts the wait on a full
      // budget. It is deliberately NOT keyed on an action id — naming
      // "permissions.verify" here would put a second copy of the action
      // vocabulary in this file, which is the one thing the header above this
      // code forbids, and a page whose verify verb is spelled differently would
      // silently lose the wait. A page declares what its last step can do; the
      // shell waits after any of it.
      if (lastIndex > 0 && open === lastIndex) setArmed((n) => n + 1)
      ctx.refresh()
    } catch (reason) {
      const message = failedTo(actionLabel(action), reason)
      setError(message)
      ctx.note(message)
    } finally {
      setBusy('')
    }
  }

  /**
   * finish records the one thing the daemon cannot re-derive. It is the
   * wizard's own last verb rather than a page action, so it is called here and
   * not dispatched through the registry: there is no capability token to look
   * up, because there is exactly one caller and the daemon serves exactly this.
   * The button is offered only once the daemon says nothing is left, and after
   * it lands the re-read is the daemon's row again — so a write that succeeded
   * and a wizard still offering to finish cannot both be true.
   */
  async function finish(): Promise<void> {
    setBusy('finish')
    setError('')
    setOutcome('')
    try {
      await ctx.service.CompleteOnboarding()
      setOutcome('Setup recorded as finished.')
      ctx.note('Setup recorded as finished.')
      ctx.refresh()
    } catch (reason) {
      const message = failedTo('Finish setup', reason)
      setError(message)
      ctx.note(message)
    } finally {
      setBusy('')
    }
  }

  if (steps.length === 0) {
    return (
      <ControlFrame label={control.label ?? control.id} note={control.note} error={state.error}>
        <EmptyState>This page declared a setup flow with no steps in it.</EmptyState>
      </ControlFrame>
    )
  }

  const current = Math.min(open, steps.length - 1)
  const shown = steps[current]
  const shownVerdict = verdict(shown, current, byId, row.steps)
  // Which declared step the daemon is waiting on. Joined the same way every
  // other mark is, so a step the daemon named and a step the reader has open can
  // be different and both can be shown.
  const waiting = steps.findIndex((step, index) => {
    if (row.current_step === NO_STEP_LEFT || row.current_step === '') return false
    return verdict(step, index, byId, row.steps)?.id === row.current_step
  })
  const Body = shown.body ? STEP_BODIES[shown.body] : undefined

  return (
    <ControlFrame
      label={control.label ?? control.id}
      note={control.note}
      error={error || settle.error || state.error}
    >
      <ol className="ctl-steps" aria-label="Setup steps">
        {steps.map((step, index) => {
          const mark = verdict(step, index, byId, row.steps)
          return (
            <li
              className={index === current ? 'ctl-step is-current' : 'ctl-step'}
              key={`${index}-${step.id ?? step.label}`}
            >
              <button
                type="button"
                className="ctl-step-button"
                aria-current={index === current ? 'step' : undefined}
                onClick={() => setOpen(index)}
              >
                <span className="ctl-step-mark" data-done={mark?.done ? 'true' : 'false'} aria-hidden="true" />
                <span className="ctl-step-index" aria-hidden="true">
                  {index + 1}
                </span>
                <span className="ctl-step-label">{step.label}</span>
              </button>
              {/* Three facts, three marks, never one. "Current step" is where the
                  reader is; "Next up" is where the daemon is waiting; "Done" is
                  the daemon's verdict on the step itself. A step that is both
                  open and waiting says both, because collapsing them is how a
                  reader concludes the shell and the daemon agree when they do
                  not. All three are chips rather than step classes because
                  ctl-chip is the whole vocabulary the stylesheet offers for a
                  mark like this, and a renderer that invented a class name for
                  the verdict would be styling a state nothing draws.

                  The MARK on the left of the button is a fourth thing and is the
                  only one you can take in without reading: a filled circle for a
                  step the daemon has derived as done and a stroked ring for one
                  it has not (PackImportSheet.swift:236-245). It is decorative
                  beside the "Done" chip rather than a fourth source of truth —
                  a step the daemon says is done is drawn done whichever way the
                  mark failed to render, which is the rule this stylesheet
                  already applies to every other tick it draws. */}
              {index === current ? <span className="ctl-chip">Current step</span> : null}
              {index === waiting ? <span className="ctl-chip">Next up</span> : null}
              {mark?.done ? <span className="ctl-chip">Done</span> : null}
              {/* The step's OWN body: the reason it is not done, which is the
                  daemon's sentence and not a count of anything. A done step
                  carries none — the daemon omits the field — so the slot is left
                  out rather than filled with a blank line. The SENTENCE is the
                  readiness table's (lib/readinessReasons.ts), the same one the
                  checklist row beside it uses, because the daemon derives both
                  from readinessRows() and a step that said the reason one way
                  and the row another would be this file's own two-answers bug
                  with the rows a screen apart. */}
              {/* The reason a step is not done — on EVERY unfinished step, not
                  only the one the reader has open. That is the point of the
                  list: a person on step 1 can see that step 4 is waiting on a
                  permission they have not granted yet, without clicking
                  through to find out. (An earlier revision showed it for the
                  current step only, to make the list shorter; renderers.test.tsx
                  asserts both non-current reasons are on screen, and it is
                  right — the list's job is to say what is blocking ALL of it.)

                  The list was tall because `.ctl-item.is-block` was
                  `display: block`, which left its sentences inline and welded
                  together. They are one per line now, which is both correct and
                  the same height. */}
              {mark?.detail ? <StepReason detail={mark.detail} /> : null}
            </li>
          )
        })}
      </ol>

      {/* THE ROLLUP, in the reference's position: one line, under the list it
          counts, saying how much is left. It is the only place on the page that
          counts the step marks — the readiness number this file used to print
          here is the same fact as the daemon's own sentence on the Verify step,
          and saying it twice is how two of them came to disagree. */}
      <p className="ctl-rollup">
        {state.loading ? 'Checking…' : `${doneSteps} of ${steps.length} steps done.`}
      </p>

      <p className="ctl-value">
        Step {current + 1} of {steps.length}: {shown.label}
      </p>
      {state.loading ? (
        <p className="ctl-value">Checking what is ready…</p>
      ) : checks.length === 0 ? (
        <EmptyState>The daemon reported no readiness checks, so this step has nothing to verify yet.</EmptyState>
      ) : null}
      {shownVerdict?.detail ? <StepReason detail={shownVerdict.detail} /> : null}
      {row.completed ? <p className="ctl-value">Setup is recorded as finished.</p> : null}

      {/* THE WAIT, and the hand-off it ends in. Both are drawn only on the last
          step and both are the reference's shape (await.go's bounded Until, then
          launcher.go's closing "Almost ready!" list), because that is the only
          point in a setup flow where "are we there yet" is a question rather
          than a step to walk.

          Four states, kept apart on purpose:

            waiting  - still looking. The poll count is the PROGRESS; the
                       reference calls i.Progress() on every tick for the same
                       reason, so a reader can tell a live wait from a dead one
                       instead of watching an unchanging line.
            settled  - the daemon's own rows all came back ready. The step marks
                       above have already been re-derived from that same answer,
                       so this says the wait is over and stops.
            timedOut - the budget elapsed. The reference names the timeout
                       (fmt.Errorf("%w after %v", ErrTimeout, cfg.Timeout)) and
                       then hands over the list; so does this, and the list is
                       the point. A reader told "not ready" with no path forward
                       is exactly the reader launcher.go refuses to leave: it
                       prints what is still to grant, and where.
            error    - the read failed. Shown like every other source's failure,
                       never as a verdict about the machine. */}
      {settle.waiting ? (
        <p className="ctl-value">
          Waiting for the machine to catch up — checked {plural(settle.polls, 'time')}, giving it up
          to {SETTLE_TIMEOUT_MS / 1000} seconds.
        </p>
      ) : null}
      {settle.timedOut ? (
        <>
          <p className="ctl-value">
            Still not ready after {SETTLE_TIMEOUT_MS / 1000} seconds. That is the wait running out,
            not an answer about the machine.
          </p>
          {/* The hand-off. The reference's words are "Almost ready!" and its
              reason is that the install's last task is a PERSON granting
              permissions, not the commands returning. Each line is a daemon
              readiness row drawn by the checklist's own renderer, so the
              sentence that tells someone what to do is the same one the
              checklist gives them a screen away — and the daemon's `detail`
              carries the path, so the shell never composes a System Settings
              location of its own. */}
          <p className="ctl-value">Almost ready! Still to grant:</p>
          <ul className="ctl-list">
            {outstanding.map((check) => (
              <ReadinessLine key={check.id} row={check} />
            ))}
          </ul>
        </>
      ) : null}
      {settle.settled ? (
        <p className="ctl-value">Every check is ready — no second click needed.</p>
      ) : null}

      {/* The declared body, drawn by the renderer for the kind the step named.
          A kind this build cannot draw is NAMED rather than skipped, so the gap
          is self-describing instead of being an empty step. */}
      {shown.body && !Body ? (
        <EmptyState>
          This step declares a “{shown.body}” body, and this build has no renderer for that
          kind, so there is nothing to draw here.
        </EmptyState>
      ) : Body ? (
        <Body control={{ kind: shown.body ?? '', id: shown.id ?? 'body', label: shown.label }} ctx={ctx} />
      ) : null}

      <div className="ctl-actions">
        <button
          type="button"
          className="ctl-button"
          disabled={current === 0}
          onClick={() => setOpen(Math.max(0, current - 1))}
        >
          Back
        </button>
        <button
          type="button"
          className="ctl-button"
          disabled={current >= steps.length - 1}
          onClick={() => setOpen(Math.min(steps.length - 1, current + 1))}
        >
          Next
        </button>
        {actions.map((action) => (
          <button
            key={action}
            type="button"
            className="ctl-button"
            disabled={busy !== ''}
            onClick={() => void run(action)}
          >
            {busy === action ? 'Working…' : actionLabel(action)}
          </button>
        ))}
      </div>

      {/* Offered exactly when the daemon says it has no unfinished step, which
          is its own derivation and not this shell counting rows. After it lands
          the daemon's flag is set, so the button withdraws on the next read
          rather than reporting a finished setup the daemon has not recorded. */}
      {nothingLeft && !row.completed ? (
        <div className="ctl-actions">
          <button
            type="button"
            className="ctl-button"
            disabled={busy !== ''}
            onClick={() => void finish()}
          >
            {busy === 'finish' ? 'Working…' : 'Finish setup'}
          </button>
        </div>
      ) : null}
      {outcome ? <p className="ctl-value">{outcome}</p> : null}
    </ControlFrame>
  )
}
