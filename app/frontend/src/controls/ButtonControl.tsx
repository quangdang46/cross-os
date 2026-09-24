// The action button (bead cross-os-itq).
//
// A page declares `action`, this resolves it through the action registry, and
// the one bound method that action names runs. The result is shown rather than
// summarised as "done": safety.panicStop returns what it stopped
// (bridge.go: "Result names what stopped — never a silent kill") and collapsing
// that to a success word throws away the only useful part of the answer.
//
// A `confirm` string expands into an inline Confirm/Cancel pair instead of a
// native dialog. window.confirm blocks the whole webview, cannot be styled with
// the shell's own vocabulary, and is unreachable by keyboard on some webview
// hosts — and "Reset Everything" removes the user's login item, so the one
// affordance standing between them and that must be a real, focusable control.
//
// Confirming that button currently buys a PLAN, not a cleanup: core.reset
// returns safety.PlanReset's step strings and removes no file, no login item
// and no process (main.go handleReset). Reported() below is the only thing
// standing between those verbs and a sentence that reads as a finished reset.

import { useState } from 'react'
import type { ReactElement } from 'react'
import { failedTo, describeResult } from '../lib/wire'
import { commandFor } from './actions'
import { ControlFrame } from './common'
import type { ControlProps } from './common'

// reported renders what a command returned. A list of strings is a PLAN, not a
// receipt: every step in one is phrased as a thing that would happen
// ("remove login item"), and joining them under a heading that says Reset
// Everything tells the user a cleanup ran. The verbs are kept verbatim — they
// are the audited §8.2 scope — under a sentence that says plainly that nothing
// has changed yet.
function reported(label: string, result: unknown): string {
  if (!Array.isArray(result)) return describeResult(result)
  const steps = result.filter((step): step is string => typeof step === 'string' && step !== '')
  if (steps.length === 0) return 'The daemon reported no steps. Nothing has been changed.'
  return `Nothing has been changed yet. This is what "${label}" would do: ${steps.join(' → ')}`
}

export function ButtonControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const [error, setError] = useState('')
  const [outcome, setOutcome] = useState('')
  const [busy, setBusy] = useState(false)
  const [asking, setAsking] = useState(false)

  const label = control.label ?? control.id
  const action = typeof control.action === 'string' ? control.action : ''
  const command = commandFor(action)

  async function run(): Promise<void> {
    if (!command) {
      // Declared by the page, understood by nobody. Say so instead of
      // pretending the click did something.
      const message = `"${action || 'no action'}" is declared on this button, and this build has no command for it.`
      setError(message)
      ctx.note(message)
      return
    }
    setBusy(true)
    setError('')
    setOutcome('')
    try {
      const result = await command(ctx.service)
      const said = reported(label, result)
      setOutcome(said)
      ctx.note(`${label}: ${said}`)
      ctx.refresh()
    } catch (reason) {
      const message = failedTo(label, reason)
      setError(message)
      ctx.note(message)
    } finally {
      setBusy(false)
      setAsking(false)
    }
  }

  return (
    <ControlFrame label={label} note={control.note} error={error}>
      {asking ? (
        <>
          <p className="ctl-value">{control.confirm}</p>
          <div className="ctl-actions">
            <button className="ctl-input" type="button" disabled={busy} onClick={() => void run()}>
              {busy ? 'Working…' : `Yes, ${label}`}
            </button>
            <button className="ctl-input" type="button" disabled={busy} onClick={() => setAsking(false)}>
              Keep things as they are
            </button>
          </div>
        </>
      ) : (
        <div className="ctl-actions">
          <button
            className="ctl-input"
            type="button"
            disabled={busy}
            onClick={() => {
              if (control.confirm) {
                setAsking(true)
                return
              }
              void run()
            }}
          >
            {busy ? 'Working…' : label}
          </button>
        </div>
      )}
      {outcome ? <p className="ctl-value">{outcome}</p> : null}
    </ControlFrame>
  )
}
