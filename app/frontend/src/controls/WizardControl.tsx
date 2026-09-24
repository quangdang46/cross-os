// The first-run wizard (bead w6-frontend-setup-renderers).
//
// The steps are DECLARED, never written here: a page sends `steps` and this
// renders them in order. A shell that carried its own four steps would be a
// second copy of the setup flow, and a page that wanted five would get four —
// §3.6c's rule is that a screen is a Go change and never a UI change.
//
// The step machine belongs to the daemon (core:onboardingState): which step is
// done, which one is next and whether setup is finished are all derived there
// from live state, and the daemon refuses to serve a cursor it would have to
// store. So this control keeps NO copy of that machine. It holds one thing —
// which step's body is open on screen — and that is a view cursor, not a
// verdict: nothing here ever marks a step done, because a done mark this shell
// invented is exactly the kind of small lie that tells a person a permission
// landed when it did not. The verdicts live in the readiness rows the same page
// renders beside the wizard, read from the same bound source (core:readiness —
// the rows core:onboardingState derives its steps from), so a step that passes
// while the row next to it is red cannot happen.
//
// Next and Back move the cursor and nothing else. What makes a step pass is a
// write the page declared, dispatched through the action registry
// (controls/actions.ts), so an action id appears in exactly one table and a
// control never composes a call the daemon never reviewed.
//
// The completion write is the page's LAST declared action, read as the write
// that records the finish. A page that declares none gets a line saying so
// rather than a button that would quietly succeed — the alternative is a
// "you're done" that nobody ever wrote to the daemon.
//
// Port source: pcfy-my-mac (raxigan/pcfy-my-mac), the installer that asks for a
// machine's shape before it changes it.
//
//   cmd/task/task.go:18-22       Task{Name, Execute} — a list of named steps,
//                                each with its own body, run in order
//   cmd/param/survey.go:8-14     one question per decision, in order, each
//                                with its own help text
//
// No code was copied: the reference is a survey-driven zsh/Go installer and this
// is a declared-schema renderer, so what transfers is the shape — an ordered
// list of named steps, one open at a time, each with a body, and a finish —
// not the implementation. Tracked in third_party/pcfy-my-mac/ATTRIBUTION.md.

import { useEffect, useState } from 'react'
import type { ReactElement } from 'react'
import { humanize, plural } from '../lib/format'
import { failedTo } from '../lib/wire'
import { useResource } from '../lib/useResource'
import { commandFor } from './actions'
import { ControlFrame, EmptyState } from './common'
import type { ControlProps } from './common'

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

export function WizardControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const steps = Array.isArray(control.steps) ? control.steps : []
  const actions = Array.isArray(control.actions) ? control.actions : []
  const readiness = useResource(() => ctx.service.Readiness(), ctx.refreshToken, [], ctx.note)

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

  const rows = readiness.data ?? []
  const ready = rows.filter((row) => row.ready).length
  const complete = rows.length > 0 && ready === rows.length
  // The finish write is the page's last declared action, read in the order the
  // page declared them. actions[actions.length - 1] on an empty list is
  // undefined, which is the case that must say so rather than run nothing.
  const finish = actions[actions.length - 1] ?? ''

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
      ctx.refresh()
    } catch (reason) {
      const message = failedTo(actionLabel(action), reason)
      setError(message)
      ctx.note(message)
    } finally {
      setBusy('')
    }
  }

  if (steps.length === 0) {
    return (
      <ControlFrame label={control.label ?? control.id} note={control.note} error={readiness.error}>
        <EmptyState>This page declared a setup flow with no steps in it.</EmptyState>
      </ControlFrame>
    )
  }

  const current = Math.min(open, steps.length - 1)
  const failing = rows.filter((row) => !row.ready)
  // Once the checks are green the last declared action stops being one write
  // among several and becomes the finish — so it leaves the body list rather
  // than appearing twice, once as itself and once as "Finish setup".
  const bodyActions = complete ? actions.slice(0, -1) : actions

  return (
    <ControlFrame
      label={control.label ?? control.id}
      note={control.note}
      error={error || readiness.error}
    >
      <ol className="ctl-steps" aria-label="Setup steps">
        {steps.map((step, index) => (
          <li className={index === current ? 'ctl-step is-current' : 'ctl-step'} key={`${index}-${step}`}>
            <button
              type="button"
              className="ctl-step-button"
              aria-current={index === current ? 'step' : undefined}
              onClick={() => setOpen(index)}
            >
              <span className="ctl-step-index" aria-hidden="true">
                {index + 1}
              </span>
              <span className="ctl-step-label">{step}</span>
            </button>
            {index === current ? <span className="ctl-chip">Current step</span> : null}
          </li>
        ))}
      </ol>

      {/* The body of the open step: what the daemon says about the checks, the
          writes the page declared, and — once every check is green — the finish
          write. The checks themselves are listed by the readiness control the
          same page renders, so no step here repeats a row that already has a
          home. */}
      <p className="ctl-value">
        Step {current + 1} of {steps.length}: {steps[current]}
      </p>
      {readiness.loading ? (
        <p className="ctl-value">Checking what is ready…</p>
      ) : rows.length === 0 ? (
        <EmptyState>The daemon reported no readiness checks, so this step has nothing to verify yet.</EmptyState>
      ) : (
        <p className="ctl-value">
          {complete
            ? 'Every check is ready.'
            : `${plural(ready, 'check')} of ${rows.length} ready.`}
          {failing.length > 0
            ? ` Still to do: ${failing.map((row) => row.label || humanize(row.id)).join(', ')}.`
            : ''}
        </p>
      )}

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
        {bodyActions.map((action) => (
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

      {complete ? (
        finish ? (
          <div className="ctl-actions">
            <button
              type="button"
              className="ctl-button"
              disabled={busy !== ''}
              onClick={() => void run(finish)}
            >
              {busy === finish ? 'Working…' : 'Finish setup'}
            </button>
          </div>
        ) : (
          <p className="ctl-empty">
            Every check is ready, but this page declared no action that records the finish.
          </p>
        )
      ) : null}
      {outcome ? <p className="ctl-value">{outcome}</p> : null}
    </ControlFrame>
  )
}
